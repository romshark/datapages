package assetcompress

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/backoff"
	"github.com/romshark/datapages/internal/contentcoding"
	"github.com/romshark/datapages/internal/contentcoding/levels"
	"github.com/romshark/datapages/internal/exactpath"
)

// The defaults of [Config].
const (
	// DefaultMinSize is about where compression saves more than the
	// Content-Encoding and Vary headers it adds.
	DefaultMinSize       = 256
	DefaultZstdLevel     = 10
	DefaultBrotliQuality = 6
	DefaultGzipLevel     = 9
)

// window covers an entire asset up to 4 MiB.
const window = 4 << 20

// sniffLen is how much of a file [http.DetectContentType] reads.
const sniffLen = 512

var errChanged = errors.New("the file changed while it was compressed")

// writeAttempts, writeRetry and writeFactor pause 1/2 to all of 1, 2, 4 and 8 s
// between writes.
const (
	writeAttempts = 5
	writeRetry    = time.Second
	writeFactor   = 2
)

// Config configures a [Compressor]. Its zero value selects the defaults.
//
// Requests compress in parallel. Replacements run 1 at a time.
// BenchmarkPrecompress in modules/assetcompress measures example CSS and
// JavaScript files:
//
//	              editor.js, 222 KB   basecoat-1.0.2.css, 218 KB   allocated
//	brotli 6      27.9% in 4 ms       8.8% in 1.5 ms               12 MiB
//	brotli 11     25.6% in 215 ms     7.9% in 142 ms               61 MiB
//	zstd 10       28.6% in 6 ms       9.2% in 2.3 ms               39 MiB
//	gzip 9        29.5% in 5 ms       10.0% in 2.6 ms              1.3 MiB
type Config struct {
	// Store keeps copies. Nil keeps them in memory until exit; a store from
	// [github.com/romshark/datapages/modules/assetcompress/disk.New] keeps them
	// in files, which also survive restarts.
	Store Store

	// Codings lists codings in preference order. The default is brotli then gzip.
	// Negotiation matches [datapages.SSECompressionConfig.Codings].
	Codings []datapages.Coding

	// MinSize leaves smaller files uncompressed. 0 selects [DefaultMinSize],
	// and 1 compresses every nonempty eligible file.
	MinSize int

	// Levels applies to stored copies. 0 selects the defaults.
	Levels Levels

	// InitialLevels, at most Levels, applies to the copy that a request creates,
	// also while [Compressor.Precompress] compresses the file, until a compression
	// at Levels replaces it. 0 selects Levels.
	InitialLevels Levels
}

// Levels sets a level per coding.
type Levels struct {
	// ZstdLevel ranges from 1 to 22. Levels other than 1, 3, 6 and 10 use the
	// nearest lower level, including in keys and ETags.
	ZstdLevel int

	// BrotliQuality is from 1 to 11. Quality 11 saves 6% to 11% over 6 but
	// takes 45 to 95 times longer and uses 4 to 5 times the memory. Use
	// [Compressor.Precompress] or [Config.InitialLevels] to avoid request
	// latency.
	BrotliQuality int

	// GzipLevel is from 1 to 9.
	GzipLevel int
}

// defaultLevels replaces the zero levels of [Config.Levels].
var defaultLevels = Levels{
	ZstdLevel:     DefaultZstdLevel,
	BrotliQuality: DefaultBrotliQuality,
	GzipLevel:     DefaultGzipLevel,
}

// or replaces the zero levels of l with those of fallback.
func (l Levels) or(fallback Levels) Levels {
	return Levels{
		ZstdLevel:     cmp.Or(l.ZstdLevel, fallback.ZstdLevel),
		BrotliQuality: cmp.Or(l.BrotliQuality, fallback.BrotliQuality),
		GzipLevel:     cmp.Or(l.GzipLevel, fallback.GzipLevel),
	}
}

// of returns the level that coding uses.
func (l Levels) of(coding datapages.Coding) int {
	switch coding {
	case datapages.CodingZstd:
		return contentcoding.ZstdLevel(l.ZstdLevel)
	case datapages.CodingBrotli:
		return l.BrotliQuality
	default:
		return l.GzipLevel
	}
}

// validate returns the first level out of range, prefixing its message.
func (l Levels) validate(prefix string) error {
	return levels.All(prefix, l.ZstdLevel, l.BrotliQuality, l.GzipLevel)
}

// Compressor serves compressed static assets through
// [datapages.WithAssetsCompression].
//
// Eligible types are text, JSON, JavaScript, XML, YAML, TOML, SVG, WASM,
// TrueType and OpenType fonts, icons and BMP images. Each eligible content is
// compressed once per coding, lazily or by [Compressor.Precompress], and keyed
// by its SHA-256, coding and level. Dev mode disables compression.
//
// A Compressor is safe for concurrent use by several servers.
type Compressor struct {
	codings []*contentcoding.Coding
	// formats holds the copy format of each coding, at the same index.
	formats []copyFormat
	store   Store // Nil without [Config.Store].
	minSize int64

	lockPuts sync.Mutex
	puts     map[string]*assetPut // By store key.

	// memory holds copies until stored, every copy without a store, and an
	// initial copy under the key of its replacement.
	memory sync.Map
	// recompressing permits 1 replacement.
	recompressing chan struct{}
	// background counts running [Compressor.finish] calls.
	background sync.WaitGroup
	// writeRetry is [writeRetry] except in tests.
	writeRetry time.Duration
}

// copyFormat is how 1 coding names its copies and creates the initial ones.
type copyFormat struct {
	suffix        string                // Of store keys and ETags, such as br6.
	initial       *contentcoding.Coding // Of the copies that requests create.
	initialSuffix string
}

var _ datapages.AssetsCompressor = (*Compressor)(nil)

type assetPut struct {
	done   chan struct{}
	suffix string // Of the copy it creates.
	copied compressed
	err    error
}

// compressed is a copy and its suffix.
type compressed struct {
	data   []byte
	suffix string
}

// New returns a compressor or the first invalid field of conf.
func New(conf Config) (*Compressor, error) {
	if err := conf.validate(); err != nil {
		return nil, fmt.Errorf("assetcompress: %w", err)
	}
	codings := conf.Codings
	if len(codings) == 0 {
		codings = []datapages.Coding{datapages.CodingBrotli, datapages.CodingGzip}
	}
	c := &Compressor{
		store:         conf.Store,
		minSize:       int64(cmp.Or(conf.MinSize, DefaultMinSize)),
		puts:          make(map[string]*assetPut),
		recompressing: make(chan struct{}, 1),
		writeRetry:    writeRetry,
	}
	levels := conf.Levels.or(defaultLevels)
	initialLevels := conf.InitialLevels.or(levels)
	for _, name := range codings {
		level, initial := levels.of(name), initialLevels.of(name)
		cd := contentcoding.New(string(name), level, window)
		f := copyFormat{suffix: cd.Name() + strconv.Itoa(level), initial: cd}
		f.initialSuffix = f.suffix
		if initial != level {
			f.initial = contentcoding.New(string(name), initial, window)
			f.initialSuffix = cd.Name() + strconv.Itoa(initial)
		}
		c.codings = append(c.codings, cd)
		c.formats = append(c.formats, f)
	}
	return c, nil
}

func (conf Config) validate() error {
	for i, coding := range conf.Codings {
		switch coding {
		case datapages.CodingZstd, datapages.CodingBrotli, datapages.CodingGzip:
		default:
			return fmt.Errorf("unknown coding %q", coding)
		}
		if slices.Contains(conf.Codings[:i], coding) {
			return fmt.Errorf("coding %q listed 2 times", coding)
		}
	}
	if conf.MinSize < 0 {
		return fmt.Errorf("min size (%d) must not be negative", conf.MinSize)
	}
	if err := conf.Levels.validate(""); err != nil {
		return err
	}
	if err := conf.InitialLevels.validate("initial "); err != nil {
		return err
	}
	levels := conf.Levels.or(defaultLevels)
	switch initial := conf.InitialLevels; {
	case initial.ZstdLevel > levels.ZstdLevel:
		return fmt.Errorf("initial zstd level (%d) must not exceed zstd level (%d)",
			initial.ZstdLevel, levels.ZstdLevel)
	case initial.BrotliQuality > levels.BrotliQuality:
		return fmt.Errorf(
			"initial brotli quality (%d) must not exceed brotli quality (%d)",
			initial.BrotliQuality, levels.BrotliQuality,
		)
	case initial.GzipLevel > levels.GzipLevel:
		return fmt.Errorf("initial gzip level (%d) must not exceed gzip level (%d)",
			initial.GzipLevel, levels.GzipLevel)
	}
	return nil
}

// Handler implements [datapages.AssetsCompressor]. It hashes each asset on its
// first request unless sum, which may be nil, has its SHA-256, serves an accepted copy,
// and delegates other requests to next, including those whose response is marked
// Cache-Control: no-transform or typed with a Content-Type that isn't eligible.
// Assets must not change before server exit. The handler weakens an existing ETag,
// adds the coding and level to it, and logs copy failures before delegating to next.
// New copies go out from memory and reach a [Config.Store] in the background,
// in up to 5 attempts within 15 s, after any replacement at [Config.Levels].
// Refused copies stay in memory. Requests share each compression and write.
func (c *Compressor) Handler(
	fsys http.FileSystem, sum func(name string) ([sha256.Size]byte, bool),
	next http.Handler, logger *slog.Logger,
) http.Handler {
	if datapages.IsDevMode() {
		return next
	}
	return &handler{c: c, fsys: fsys, sum: sum, next: next, logger: logger}
}

// Precompress creates every missing copy of fsys except in dev mode. It stops
// at the first error or when ctx ends. A panic returns as a [datapages.PanicError].
//
// Content-based keys let the embed.FS passed to [datapages.WithAssets] cover
// its served subdirectory. Run Precompress in a goroutine to serve meanwhile.
func (c *Compressor) Precompress(ctx context.Context, fsys fs.FS) (err error) {
	defer recoverPanic(&err)
	if datapages.IsDevMode() {
		return nil
	}
	hfs := http.FS(fsys)
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if err := c.precompressFile(hfs, "/"+name); err != nil {
			return fmt.Errorf("precompressing %s: %w", name, err)
		}
		return nil
	})
}

func (c *Compressor) precompressFile(fsys http.FileSystem, name string) (err error) {
	defer recoverPanic(&err)
	f, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return err
	}
	info, err := c.describe(name, st, f, nil)
	if err != nil || !info.compressible {
		return err
	}
	for i := range c.codings {
		if err := c.precompressCopy(fsys, name, info.sum, info.keys[i], i); err != nil {
			return err
		}
	}
	return nil
}

// precompressCopy creates and stores the copy under key in the coding at i when missing.
func (c *Compressor) precompressCopy(
	fsys http.FileSystem, name string, sum [sha256.Size]byte, key string, i int,
) error {
	_, file, _, err := c.lookup(key, i)
	if err == nil && file != nil {
		return file.Close()
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	m, created, err := c.compress(fsys, name, sum, key, i, false)
	if err != nil || !created || c.store == nil {
		return err
	}
	// Drop the copy even after a failed put, since no request would store it.
	err = c.put(key, m.data)
	c.memory.Delete(key)
	return err
}

// handler owns file descriptions because servers sharing a [Compressor] may
// serve different content at the same path.
type handler struct {
	c      *Compressor
	fsys   http.FileSystem
	sum    func(name string) ([sha256.Size]byte, bool) // Nil for none.
	next   http.Handler
	logger *slog.Logger
	files  sync.Map // Cleaned path to *assetFile, read without a lock per request.
}

type assetFile struct {
	name         string // The path that opens the file, with a leading slash.
	size         int64
	modTime      time.Time
	contentType  []string // Shared by every response.
	compressible bool
	sum          [sha256.Size]byte // Zero when not compressible.
	// keys and etags follow the order of [Compressor.codings].
	keys  []string
	etags []atomic.Pointer[taggedETag]
}

// taggedETag is the ETag of a copy, made from the ETag of its file.
type taggedETag struct {
	file, suffix string
	value        []string // Shared by every response.
}

// etag returns the weak ETag of the copy in the coding at i, made from suffix and file,
// the ETag of the file. Encoder versions differ in the bytes of a copy,
// which neither file nor suffix tells apart.
func (f *assetFile) etag(i int, file, suffix string) []string {
	if t := f.etags[i].Load(); t != nil && t.file == file && t.suffix == suffix {
		return t.value
	}
	opaque := strings.TrimPrefix(file, "W/")
	t := &taggedETag{
		file:   file,
		suffix: suffix,
		value:  []string{"W/" + opaque[:len(opaque)-1] + "." + suffix + `"`},
	}
	f.etags[i].Store(t)
	return t.value
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.serve(w, r) {
		h.next.ServeHTTP(w, r)
	}
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) bool {
	if contentcoding.NoTransform(w.Header()) {
		// The response compressor of the server passes such a response through.
		return false
	}
	rel := exactpath.Clean(r.URL.Path)
	switch {
	case r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/"):
		// http.FileServer answers a directory with its index.html.
		// An empty path is the root that http.StripPrefix leaves of its prefix.
		rel = path.Join(rel, "index.html")
	case rel == "index.html" || strings.HasSuffix(rel, "/index.html"):
		// http.FileServer redirects it to its directory.
		return false
	}
	info := h.compressible(rel)
	if info == nil {
		return false
	}
	header := w.Header()
	// http.FileServer keeps a type set before it. Such a type can rule out
	// compression but cannot allow it for a file whose own type is not eligible.
	declared, typed := header["Content-Type"]
	if typed && (len(declared) == 0 || !contentcoding.Compressible(declared[0])) {
		return false
	}
	contentcoding.AddVaryAcceptEncoding(header)
	if strings.Contains(r.Header.Get("Range"), ",") {
		// Content-Encoding applies to the multipart document, not its parts.
		return false
	}
	cd := contentcoding.Negotiate(h.c.codings, r.Header["Accept-Encoding"])
	if cd == nil {
		return false
	}
	i := slices.Index(h.c.codings, cd)
	key := info.keys[i]
	data, file, suffix, err := h.c.copyOf(h.fsys, info.name, info.sum, key, i, h.logger)
	if errors.Is(err, errChanged) {
		// sum keeps the SHA-256 of the content before the change.
		h.files.Delete(rel)
		h.describeFile(rel, nil)
	}
	if err != nil {
		logCompressFailure(h.logger, info.name, cd, err)
		return false
	}
	if file != nil {
		defer func() { _ = file.Close() }()
		// ServeContent returns 500 for a copy that cannot seek.
		_, err = file.Seek(0, io.SeekEnd)
		if err == nil {
			_, err = file.Seek(0, io.SeekStart)
		}
		if err != nil {
			h.logger.Warn("reading a compressed asset",
				slog.String("path", info.name),
				slog.String("key", key), slog.Any("err", err))
			return false
		}
	}
	if !typed {
		header["Content-Type"] = info.contentType
	}
	if tag := header.Get("Etag"); strings.HasSuffix(tag, `"`) {
		// Tags differ by representation.
		header["Etag"] = info.etag(i, tag, suffix)
	}
	s := servings.Get().(*serving)
	s.w = codedWriter{w, cd}
	var content io.ReadSeeker = file
	if file == nil {
		s.reader.Reset(data)
		content = &s.reader
	}
	http.ServeContent(&s.w, r, "", info.modTime, content)
	s.w = codedWriter{}
	s.reader.Reset(nil)
	servings.Put(s)
	return true
}

// serving holds what [http.ServeContent] uses for 1 response.
// Nothing keeps it after the call, which lets a pool reuse it.
type serving struct {
	w      codedWriter
	reader bytes.Reader
}

var servings = sync.Pool{New: func() any { return new(serving) }}

// logCompressFailure logs a failed copy of name in cd.
func logCompressFailure(
	logger *slog.Logger, name string, cd *contentcoding.Coding, err error,
) {
	if panicked, ok := errors.AsType[datapages.PanicError](err); ok {
		logger.Error("recovered panic while compressing an asset",
			slog.String("path", name),
			slog.String("coding", cd.Name()),
			slog.Any("panic", panicked.Value),
			slog.String("stack", string(panicked.Stack)))
		return
	}
	logger.Warn("compressing an asset",
		slog.String("path", name),
		slog.String("coding", cd.Name()),
		slog.Any("err", err))
}

// codedWriter adds Content-Encoding only when a response sends the copy.
//
// [http.ServeContent] omits Content-Length when Content-Encoding is set early.
// It also retains early headers on bodyless 412 responses.
type codedWriter struct {
	http.ResponseWriter
	coding *contentcoding.Coding
}

func (w codedWriter) WriteHeader(code int) {
	if code == http.StatusOK || code == http.StatusPartialContent {
		w.coding.SetContentEncoding(w.Header())
	}
	w.ResponseWriter.WriteHeader(code)
}

// ReadFrom preserves sendfile support from the underlying writer.
func (w codedWriter) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(w.ResponseWriter, src)
}

// Unwrap returns the underlying writer for [http.ResponseController].
func (w codedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// compressible returns the description of the file at rel,
// an [exactpath.Clean] path, when the handler compresses the file, or nil.
func (h *handler) compressible(rel string) *assetFile {
	// Skip the open when the extension rules out compression, as [describe] would.
	ct := contentcoding.TypeByExtension(path.Ext(rel))
	if ct != "" && !contentcoding.Compressible(ct) {
		return nil
	}
	if info := h.file(rel); info != nil && info.compressible {
		return info
	}
	return nil
}

// file returns the cached description of the file at rel, an [exactpath.Clean]
// path, or creates it once. It returns nil for a file it cannot describe.
func (h *handler) file(rel string) *assetFile {
	if v, ok := h.files.Load(rel); ok {
		return v.(*assetFile)
	}
	return h.describeFile(rel, h.sum)
}

// describeFile describes the file at rel and caches the description.
// It returns nil for a file it cannot describe.
func (h *handler) describeFile(
	rel string, sum func(string) ([sha256.Size]byte, bool),
) *assetFile {
	name := "/" + rel
	f, err := h.fsys.Open(name)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return nil
	}
	info, err := h.c.describe(name, st, f, sum)
	if err != nil {
		return nil
	}
	h.files.Store(rel, &info)
	return &info
}

// describe hashes f when its size and type permit compression, unless sum,
// which may be nil, has its SHA-256.
func (c *Compressor) describe(
	name string, st fs.FileInfo, f io.Reader,
	sum func(string) ([sha256.Size]byte, bool),
) (assetFile, error) {
	info := assetFile{name: name, size: st.Size(), modTime: st.ModTime()}
	// Match http.FileServer by preferring the extension over sniffing.
	ct := contentcoding.TypeByExtension(path.Ext(name))
	var head []byte
	if ct == "" {
		var buf [sniffLen]byte
		n, err := io.ReadFull(f, buf[:])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return assetFile{}, err
		}
		head = buf[:n]
		ct = http.DetectContentType(head)
	}
	info.contentType = []string{ct}
	info.compressible = info.size >= c.minSize && contentcoding.Compressible(ct)
	if info.compressible {
		var ok bool
		if sum != nil {
			info.sum, ok = sum(strings.TrimPrefix(name, "/"))
		}
		if !ok {
			digest := sha256.New()
			_, _ = digest.Write(head)
			if _, err := io.Copy(digest, f); err != nil {
				return assetFile{}, err
			}
			info.sum = [sha256.Size]byte(digest.Sum(nil))
		}
		hexSum := hex.EncodeToString(info.sum[:])
		info.keys = make([]string, len(c.codings))
		for i, f := range c.formats {
			info.keys[i] = hexSum + "." + f.suffix
		}
		info.etags = make([]atomic.Pointer[taggedETag], len(c.codings))
	}
	return info, nil
}

// lookup returns the copy under key in the coding at i and its suffix,
// as bytes from memory or as an open file from the store.
// The error wraps [fs.ErrNotExist] when neither has the copy.
func (c *Compressor) lookup(
	key string, i int,
) (data []byte, file io.ReadSeekCloser, suffix string, err error) {
	if v, ok := c.memory.Load(key); ok {
		m := v.(compressed)
		return m.data, nil, m.suffix, nil
	}
	if c.store == nil {
		err = fs.ErrNotExist
	} else {
		file, err = c.store.Open(key)
	}
	return data, file, c.formats[i].suffix, err
}

// copyOf returns the copy under key and its suffix like [Compressor.lookup],
// creating it from name when absent. name must hash to sum. The creating call finishes
// the copy, which stays in memory until stored so no other call creates it again.
// Without a store, only an initial copy needs finishing.
func (c *Compressor) copyOf(
	fsys http.FileSystem, name string, sum [sha256.Size]byte, key string, i int,
	logger *slog.Logger,
) (data []byte, file io.ReadSeekCloser, suffix string, err error) {
	data, file, suffix, err = c.lookup(key, i)
	if !errors.Is(err, fs.ErrNotExist) {
		return data, file, suffix, err
	}
	f := c.formats[i]
	m, created, err := c.compress(fsys, name, sum, key, i, true)
	if err != nil {
		return nil, nil, "", err
	}
	if created && (c.store != nil || m.suffix != f.suffix) {
		c.background.Go(func() { c.finish(fsys, name, sum, key, i, m, logger) })
	}
	return m.data, nil, m.suffix, nil
}

// finish replaces m at the level of the coding at i if needed,
// then stores it if the compressor has a store.
func (c *Compressor) finish(
	fsys http.FileSystem, name string, sum [sha256.Size]byte, key string, i int,
	m compressed, logger *slog.Logger,
) {
	if suffix := c.formats[i].suffix; m.suffix != suffix {
		cd := c.codings[i]
		c.recompressing <- struct{}{}
		data, err := encode(fsys, name, sum, cd)
		<-c.recompressing
		if err != nil {
			logCompressFailure(logger, name, cd, err)
			return
		}
		m = compressed{data: data, suffix: suffix}
		c.memory.Store(key, m)
	}
	if c.store != nil {
		c.write(key, m.data, logger)
	}
}

// compress creates 1 copy per key in memory, in the coding at i at its level or,
// for initial, at its initial level, while concurrent callers wait, possibly for
// another level. It reports whether this call created the copy under key.
//
// Callers for an initial copy do not wait for the copy at the level of the
// coding, which [Compressor.Precompress] creates and which takes longer.
// They share an initial copy, which stays in memory until the other replaces it.
func (c *Compressor) compress(
	fsys http.FileSystem, name string, sum [sha256.Size]byte, key string, i int,
	initial bool,
) (copied compressed, created bool, err error) {
	f := c.formats[i]
	cd, suffix := c.codings[i], f.suffix
	if initial {
		cd, suffix = f.initial, f.initialSuffix
	}
	c.lockPuts.Lock()
	putKey := key
	full, running := c.puts[key]
	if running && initial && full.suffix != suffix {
		// Store keys contain no slash.
		putKey = key + "/" + suffix
	}
	if p, ok := c.puts[putKey]; ok {
		c.lockPuts.Unlock()
		<-p.done
		return p.copied, false, p.err
	}
	// A compression can end after the caller's look into memory.
	if v, ok := c.memory.Load(key); ok {
		c.lockPuts.Unlock()
		return v.(compressed), false, nil
	}
	p := &assetPut{done: make(chan struct{}), suffix: suffix}
	c.puts[putKey] = p
	c.lockPuts.Unlock()

	data, err := encode(fsys, name, sum, cd)
	p.copied, p.err = compressed{data: data, suffix: suffix}, err
	c.lockPuts.Lock()
	// A copy beside full enters memory only while full runs, which replaces it.
	if p.err == nil && (putKey == key || c.puts[key] == full) {
		c.memory.Store(key, p.copied)
	}
	delete(c.puts, putKey)
	c.lockPuts.Unlock()
	close(p.done)
	return p.copied, putKey == key, p.err
}

// encode compresses name and rejects a file that does not hash to sum.
// It recovers panics so callers release their key or permit.
func encode(
	fsys http.FileSystem, name string, sum [sha256.Size]byte, cd *contentcoding.Coding,
) (_ []byte, err error) {
	defer recoverPanic(&err)
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var copied bytes.Buffer
	enc := cd.Get(&copied)
	defer cd.Put(enc)
	digest := sha256.New()
	if _, err := io.Copy(enc, io.TeeReader(f, digest)); err != nil {
		return nil, err
	}
	if [sha256.Size]byte(digest.Sum(nil)) != sum {
		return nil, errChanged
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return copied.Bytes(), nil
}

// write stores b under key, then drops it from memory. The last failure is logged.
func (c *Compressor) write(key string, b []byte, logger *slog.Logger) {
	for attempt := range backoff.Attempts(writeAttempts, c.writeRetry, writeFactor) {
		err := c.put(key, b)
		if err == nil {
			c.memory.Delete(key)
			return
		}
		var panicked datapages.PanicError
		switch {
		case errors.As(err, &panicked):
			// A store defect, logged even if a later attempt succeeds.
			logger.Error("recovered panic while storing a compressed asset",
				slog.String("key", key),
				slog.Any("panic", panicked.Value),
				slog.String("stack", string(panicked.Stack)))
		case attempt == writeAttempts:
			logger.Warn("storing a compressed asset", slog.String("key", key),
				slog.Int("attempts", attempt), slog.Any("err", err))
		}
	}
}

// put stores b under key, returning a panic as a [datapages.PanicError].
func (c *Compressor) put(key string, b []byte) (err error) {
	defer recoverPanic(&err)
	return c.store.Put(key, bytes.NewReader(b))
}

// recoverPanic turns a panic into *err as a [datapages.PanicError].
func recoverPanic(err *error) {
	if v := recover(); v != nil {
		*err = datapages.PanicError{Value: v, Stack: debug.Stack()}
	}
}
