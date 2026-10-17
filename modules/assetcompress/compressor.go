package assetcompress

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/contentcoding"
	"github.com/romshark/datapages/modules/assetcompress/inmem"
)

// The defaults of [Config].
const (
	DefaultZstdLevel     = 19
	DefaultBrotliQuality = 6
	DefaultGzipLevel     = 9
)

// window covers an entire asset up to 4 MiB.
const window = 4 << 20

// sniffLen is how much of a file [http.DetectContentType] reads.
const sniffLen = 512

var errChanged = errors.New("the file changed while it was compressed")

// Config configures a [Compressor]. Its zero value selects the defaults.
//
// A Compressor compresses 1 file at a time. BenchmarkPrecompress in
// modules/assetcompress measures example CSS and JavaScript files:
//
//	              editor.js, 222 KB   basecoat.css, 218 KB   allocated
//	brotli 6      27.9% in 4 ms       8.8% in 1.5 ms         12 MiB
//	brotli 11     25.6% in 215 ms     7.9% in 142 ms         61 MiB
//	zstd 19       28.6% in 6 ms       9.2% in 2.3 ms         39 MiB
//	gzip 9        29.5% in 5 ms       10.0% in 2.6 ms        1.3 MiB
type Config struct {
	// Store keeps the copies. Nil keeps them in memory until the process exits.
	// A store from [github.com/romshark/datapages/modules/assetcompress/disk.New]
	// keeps them across restarts.
	Store Store

	// Codings lists codings in preference order and negotiates them like
	// [datapages.SSECompressionConfig.Codings]. The default is brotli and gzip.
	Codings []datapages.Coding

	// ZstdLevel is from 1 to 22. 0 selects [DefaultZstdLevel].
	ZstdLevel int

	// BrotliQuality is from 1 to 11. 0 selects [DefaultBrotliQuality].
	// Quality 11 saves 6% to 11% over 6 but takes 45 to 95 times longer and
	// uses 4 to 5 times the memory. Use [Compressor.Precompress] to avoid
	// request latency.
	BrotliQuality int

	// GzipLevel is from 1 to 9. 0 selects [DefaultGzipLevel].
	GzipLevel int
}

// Compressor compresses static assets and serves the compressed copies.
// Pass it to [datapages.WithAssetsCompression].
//
// A file of at least 256 bytes is eligible when its type is text, HTML
// included, JSON, JavaScript, XML including SVG, or WASM. Each eligible file
// is compressed once per coding, on its first request or by
// [Compressor.Precompress]. Its copy is stored under the file's SHA-256, the
// coding and the level. Dev mode disables compression.
//
// A Compressor is safe for concurrent use, also by several servers.
type Compressor struct {
	codings []*contentcoding.Coding
	// suffixes maps a coding to the suffix of its store keys, its name and
	// level, as in br6.
	suffixes map[*contentcoding.Coding]string
	store    Store

	// compressing limits encoder memory to 1 file at a time.
	compressing chan struct{}

	lockPuts sync.Mutex
	puts     map[string]*assetPut // By store key.
}

var _ datapages.AssetsCompressor = (*Compressor)(nil)

type assetPut struct {
	done chan struct{}
	err  error
}

// New returns a compressor configured by conf.
// It reports the first invalid field of conf.
func New(conf Config) (*Compressor, error) {
	if err := conf.validate(); err != nil {
		return nil, fmt.Errorf("assetcompress: %w", err)
	}
	codings := conf.Codings
	if len(codings) == 0 {
		codings = []datapages.Coding{datapages.CodingBrotli, datapages.CodingGzip}
	}
	c := &Compressor{
		suffixes:    make(map[*contentcoding.Coding]string, len(codings)),
		store:       conf.Store,
		compressing: make(chan struct{}, 1),
		puts:        make(map[string]*assetPut),
	}
	if c.store == nil {
		c.store = inmem.New()
	}
	for _, name := range codings {
		var cd *contentcoding.Coding
		var level int
		switch name {
		case datapages.CodingZstd:
			level = cmp.Or(conf.ZstdLevel, DefaultZstdLevel)
			cd = contentcoding.NewZstd(level, window)
		case datapages.CodingBrotli:
			level = cmp.Or(conf.BrotliQuality, DefaultBrotliQuality)
			cd = contentcoding.NewBrotli(level, window)
		case datapages.CodingGzip:
			level = cmp.Or(conf.GzipLevel, DefaultGzipLevel)
			cd = contentcoding.NewGzip(level)
		}
		c.codings = append(c.codings, cd)
		c.suffixes[cd] = cd.Name() + strconv.Itoa(level)
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
	switch {
	case conf.ZstdLevel != 0 && (conf.ZstdLevel < 1 || conf.ZstdLevel > 22):
		return fmt.Errorf("zstd level (%d) must be from 1 to 22", conf.ZstdLevel)
	case conf.BrotliQuality != 0 && (conf.BrotliQuality < 1 || conf.BrotliQuality > 11):
		return fmt.Errorf("brotli quality (%d) must be from 1 to 11", conf.BrotliQuality)
	case conf.GzipLevel != 0 && (conf.GzipLevel < 1 || conf.GzipLevel > 9):
		return fmt.Errorf("gzip level (%d) must be from 1 to 9", conf.GzipLevel)
	}
	return nil
}

// Handler implements [datapages.AssetsCompressor]. It returns a handler that
// sends the copy of a file of fsys that the request accepts, creating the copy
// when it is missing, and passes other requests to next.
//
// An ETag set on the response before the handler runs gets the coding and level
// appended, as in "tag.br6", which gives each copy its own tag. A copy that
// cannot be created or read is logged to logger, and next sends the file
// uncompressed.
func (c *Compressor) Handler(
	fsys http.FileSystem, next http.Handler, logger *slog.Logger,
) http.Handler {
	if datapages.IsDevMode() {
		return next
	}
	return &handler{c: c, fsys: fsys, next: next, logger: logger}
}

// Precompress creates the missing copies of every eligible file of fsys in
// every coding. It does nothing in dev mode.
// It stops at the first failed read or store and when ctx ends.
//
// A copy is stored under the content of its file, never under its path.
// Precompressing the embed.FS passed to [datapages.WithAssets] therefore
// creates the copies the server looks up, whichever subdirectory of it the
// server serves. A file the server does not serve adds a copy no request reads.
//
// Requests create the copies that are still missing. Per 200 KB, brotli 6
// takes a few milliseconds and brotli 11 takes 140 to 215 ms.
// Run Precompress in a goroutine to serve meanwhile.
func (c *Compressor) Precompress(ctx context.Context, fsys fs.FS) error {
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

func (c *Compressor) precompressFile(fsys http.FileSystem, name string) error {
	f, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return err
	}
	info, err := describe(name, st, f)
	if err != nil || !info.compressible {
		return err
	}
	for _, cd := range c.codings {
		copied, err := c.copyOf(fsys, name, info.hash, info.hash+"."+c.suffixes[cd], cd)
		if err != nil {
			return err
		}
		_ = copied.Close()
	}
	return nil
}

// handler sends the copies of 1 file system. It describes the files itself
// because 2 servers that share a [Compressor] may serve different files under
// 1 path.
type handler struct {
	c      *Compressor
	fsys   http.FileSystem
	next   http.Handler
	logger *slog.Logger
	files  sync.Map // Cleaned path to assetFile, read without a lock per request.
}

// assetFile describes a file as it was when hashed.
type assetFile struct {
	size         int64
	modTime      time.Time
	contentType  string
	compressible bool
	hash         string // Hex SHA-256 of the file, empty when not compressible.
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.serve(w, r) {
		h.next.ServeHTTP(w, r)
	}
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) bool {
	if strings.Contains(r.Header.Get("Range"), ",") {
		// Content-Encoding applies to the multipart document, not its parts.
		return false
	}
	name := path.Clean("/" + r.URL.Path)
	switch {
	case strings.HasSuffix(r.URL.Path, "/"):
		// http.FileServer answers a directory with its index.html.
		name = path.Join(name, "index.html")
	case strings.HasSuffix(name, "/index.html"):
		// http.FileServer redirects it to its directory.
		return false
	}
	// Skip the open when the extension rules out compression, as [describe] would.
	ct := mime.TypeByExtension(path.Ext(name))
	if ct != "" && !contentcoding.Compressible(ct) {
		return false
	}
	info, ok := h.file(name)
	if !ok || !info.compressible {
		return false
	}
	header := w.Header()
	contentcoding.AddVary(header, "Accept-Encoding")
	cd := contentcoding.Negotiate(h.c.codings, r.Header["Accept-Encoding"])
	if cd == nil {
		return false
	}
	key := info.hash + "." + h.c.suffixes[cd]
	copied, err := h.c.copyOf(h.fsys, name, info.hash, key, cd)
	if errors.Is(err, errChanged) {
		// Size and modification time did not expose this edit.
		h.files.Delete(name)
	}
	if err != nil {
		h.logger.Warn("compressing an asset",
			slog.String("path", name),
			slog.String("coding", cd.Name()),
			slog.Any("err", err))
		return false
	}
	defer func() { _ = copied.Close() }()
	// ServeContent returns 500 for a copy that cannot seek.
	_, err = copied.Seek(0, io.SeekEnd)
	if err == nil {
		_, err = copied.Seek(0, io.SeekStart)
	}
	if err != nil {
		h.logger.Warn("reading a compressed asset",
			slog.String("path", name), slog.String("key", key), slog.Any("err", err))
		return false
	}
	header.Set("Content-Type", info.contentType)
	if tag := header.Get("Etag"); strings.HasSuffix(tag, `"`) {
		// Strong tags differ by representation.
		header.Set("Etag", tag[:len(tag)-1]+"."+h.c.suffixes[cd]+`"`)
	}
	http.ServeContent(codedWriter{w, cd.Name()}, r, "", info.modTime, copied)
	return true
}

// codedWriter adds Content-Encoding only when a response sends the copy.
//
// [http.ServeContent] omits Content-Length when Content-Encoding is set early.
// It also retains early headers on bodyless 412 responses.
type codedWriter struct {
	http.ResponseWriter
	coding string
}

func (w codedWriter) WriteHeader(code int) {
	if code == http.StatusOK || code == http.StatusPartialContent {
		w.Header().Set("Content-Encoding", w.coding)
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

// file returns the description of name, which it keeps until the size or the
// modification time changes. It reports false when name is not a file.
func (h *handler) file(name string) (assetFile, bool) {
	f, err := h.fsys.Open(name)
	if err != nil {
		return assetFile{}, false
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return assetFile{}, false
	}
	if v, ok := h.files.Load(name); ok {
		info := v.(assetFile)
		if info.size == st.Size() && info.modTime.Equal(st.ModTime()) {
			return info, true
		}
	}
	info, err := describe(name, st, f)
	if err != nil {
		return assetFile{}, false
	}
	h.files.Store(name, info)
	return info, true
}

// describe returns the type of f, the file name with the metadata st,
// and the hash of f when it is eligible.
func describe(name string, st fs.FileInfo, f io.Reader) (assetFile, error) {
	info := assetFile{size: st.Size(), modTime: st.ModTime()}
	var head [sniffLen]byte
	n, err := io.ReadFull(f, head[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return assetFile{}, err
	}
	// Match http.FileServer by preferring the extension over sniffing.
	info.contentType = mime.TypeByExtension(path.Ext(name))
	if info.contentType == "" {
		info.contentType = http.DetectContentType(head[:n])
	}
	info.compressible = info.size >= datapages.DefaultResponseCompressionMinSize &&
		contentcoding.Compressible(info.contentType)
	if info.compressible {
		sum := sha256.New()
		_, _ = sum.Write(head[:n])
		if _, err := io.Copy(sum, f); err != nil {
			return assetFile{}, err
		}
		info.hash = hex.EncodeToString(sum.Sum(nil))
	}
	return info, nil
}

// copyOf opens the copy under key and creates it when it is missing.
// The file name of fsys must hash to sum.
func (c *Compressor) copyOf(
	fsys http.FileSystem, name, sum, key string, cd *contentcoding.Coding,
) (io.ReadSeekCloser, error) {
	if copied, err := c.store.Open(key); !errors.Is(err, fs.ErrNotExist) {
		return copied, err
	}
	if err := c.compress(fsys, name, sum, key, cd); err != nil {
		return nil, err
	}
	return c.store.Open(key)
}

// compress creates 1 copy per key while concurrent callers wait.
func (c *Compressor) compress(
	fsys http.FileSystem, name, sum, key string, cd *contentcoding.Coding,
) error {
	c.lockPuts.Lock()
	if p, ok := c.puts[key]; ok {
		c.lockPuts.Unlock()
		<-p.done
		return p.err
	}
	p := &assetPut{done: make(chan struct{})}
	c.puts[key] = p
	c.lockPuts.Unlock()

	c.compressing <- struct{}{}
	p.err = c.put(fsys, name, sum, key, cd)
	<-c.compressing

	c.lockPuts.Lock()
	delete(c.puts, key)
	c.lockPuts.Unlock()
	close(p.done)
	return p.err
}

func (c *Compressor) put(
	fsys http.FileSystem, name, sum, key string, cd *contentcoding.Coding,
) error {
	f, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(encodeChecked(pw, f, sum, cd))
	}()
	err = c.store.Put(key, pr)
	// Stop the encoder when the store stops reading early.
	_ = pr.CloseWithError(err)
	return err
}

// encodeChecked encodes src and rejects a source that does not hash to sum.
func encodeChecked(
	w io.Writer, src io.Reader, sum string, cd *contentcoding.Coding,
) error {
	enc := cd.Get(w)
	defer cd.Put(enc)
	digest := sha256.New()
	if _, err := io.Copy(enc, io.TeeReader(src, digest)); err != nil {
		return err
	}
	if !hashIs(digest, sum) {
		return errChanged
	}
	return enc.Close()
}

func hashIs(digest hash.Hash, sum string) bool {
	return hex.EncodeToString(digest.Sum(nil)) == sum
}
