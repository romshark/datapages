package httpserve

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"mime"
	"net/http"
	"net/textproto"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/romshark/datapages"
)

var (
	// ErrFileNoBody reports a [datapages.File] without a Body.
	ErrFileNoBody = errors.New("datapages.File has no Body")

	// ErrFileNoType reports a [datapages.File] without a Type.
	ErrFileNoType = errors.New("datapages.File has no Type")

	// ErrFileInvalidETag reports a [datapages.File] whose ETag
	// cannot stand between the quotes of an entity tag.
	ErrFileInvalidETag = errors.New("datapages.File has an invalid ETag")

	// ErrFileInvalidCache reports a [datapages.FileCache]
	// whose fields contradict each other.
	ErrFileInvalidCache = errors.New("datapages.File has an invalid Cache")

	// ErrFileReservedHeader reports a [datapages.File] whose Header sets
	// a header that one of its fields or [http.ServeContent] writes.
	ErrFileReservedHeader = errors.New("datapages.File sets a reserved header in Header")

	// ErrFileDirectory reports a [datapages.File] whose Body is a directory,
	// as [os.Root.Open] opens for the path of one. [ServeFile] wraps it with
	// [datapages.ErrNotFound].
	ErrFileDirectory = errors.New("datapages.File has a directory as Body")

	// ErrFileSeek reports a [datapages.File] whose Body fails to seek.
	ErrFileSeek = errors.New("datapages.File has a Body that fails to seek")
)

// fileHeaders are the headers [ServeFile] and [http.ServeContent] write,
// by canonical name. A field of [datapages.File] is the only way to set each.
var fileHeaders = [...]string{
	"Accept-Ranges",
	"Cache-Control",
	"Content-Disposition",
	"Content-Length",
	"Content-Range",
	"Content-Type",
	"Etag",
	"Last-Modified",
	"X-Content-Type-Options",
}

// reservedHeaders holds [fileHeaders] for lookup.
var reservedHeaders = func() map[string]bool {
	m := make(map[string]bool, len(fileHeaders))
	for _, name := range fileHeaders {
		m[name] = true
	}
	return m
}()

// ServeFile writes f as the response to r and closes its Body. It writes
// nothing and returns [ErrFileNoBody], [ErrFileNoType], [ErrFileInvalidETag],
// [ErrFileInvalidCache], [ErrFileReservedHeader], [ErrFileDirectory] or
// [ErrFileSeek] for an incomplete or invalid f. A Read error after the status
// ends the response early, and nothing reports it.
//
// A write that blocks for the send timeout of the server ends the response,
// see [datapages.WithFileSendTimeout]. A middleware that wraps w without
// an Unwrap method leaves the writes unlimited.
//
// A writer between w and the server that edits response bodies passes the file
// through unchanged if it has a PassThrough method, as the one
// offline.Middleware installs does.
//
// A panic in Body panics ServeFile with a [datapages.PanicError] that carries
// the stack of the panic, once Body is closed. A panic before the body leaves
// the headers of w as they were, which keeps those of the file off the error
// response. For a multi-range request, [http.ServeContent] reads Body on
// a goroutine of its own, where ServeFile recovers the panic.
func ServeFile(w http.ResponseWriter, r *http.Request, f datapages.File) error {
	if f.Body == nil {
		return ErrFileNoBody
	}
	body := f.Body
	var guard *guardedBody
	if strings.Contains(r.Header.Get("Range"), ",") {
		// [http.ServeContent] reads the parts of a multi-range response from
		// a goroutine it doesn't wait for once the copy to the client fails.
		guard = &guardedBody{body: body}
		body = guard
	}
	defer func() { _ = body.Close() }()
	cacheControl, err := cacheControl(f.Cache)
	switch {
	case f.Type == "":
		return ErrFileNoType
	case !validETag(f.ETag):
		return ErrFileInvalidETag
	case err != nil:
		return err
	}
	for name := range f.Header {
		if reservedHeaders[textproto.CanonicalMIMEHeaderKey(name)] {
			return fmt.Errorf("%w: %s", ErrFileReservedHeader, name)
		}
	}
	h := w.Header()
	var saved savedHeaders
	saved.save(h, f.Header)
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		if !ResponseBodyWritten(w) {
			saved.restore(h)
		}
		if _, ok := v.(datapages.PanicError); !ok {
			v = datapages.PanicError{Value: v, Stack: debug.Stack()}
		}
		panic(v)
	}()
	if err := checkBody(f.Body); err != nil {
		return err
	}
	h.Set("Content-Type", f.Type)
	h.Set("X-Content-Type-Options", "nosniff")
	if f.ETag != "" {
		// [http.ServeContent] reads it back for the conditional requests.
		h.Set("ETag", `"`+f.ETag+`"`)
	}
	switch {
	case cacheControl != "":
		h.Set("Cache-Control", cacheControl)
	case h.Get("Cache-Control") == "":
		h.Set("Cache-Control", "no-cache")
	}
	if v := contentDisposition(f.Disposition); v != "" {
		h.Set("Content-Disposition", v)
	}
	addHeader(h, f.Header)
	passFileThrough(w)
	limitFileWrites(w)
	http.ServeContent(w, r, "", f.ModTime, body)
	if guard != nil {
		// Close waits for the goroutine's call of Body under way, and none follows.
		_ = guard.Close()
		if p := guard.recovered(); p != nil {
			panic(*p)
		}
	}
	return nil
}

// checkBody reports a Body that [http.ServeContent] would answer with a broken
// response and report nothing of: a directory, whose first Read fails after
// the status went out, and a Body that fails to seek, which gets a plain 500.
// It seeks to the end and back to the start, as ServeContent does next.
func checkBody(body io.ReadSeeker) error {
	if s, ok := body.(interface{ Stat() (fs.FileInfo, error) }); ok {
		if info, err := s.Stat(); err == nil && info.IsDir() {
			return fmt.Errorf("%w: %w", datapages.ErrNotFound, ErrFileDirectory)
		}
	}
	if _, err := body.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("%w: %w", ErrFileSeek, err)
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("%w: %w", ErrFileSeek, err)
	}
	return nil
}

// savedHeaders holds the values the headers [ServeFile] and
// [http.ServeContent] write had before.
type savedHeaders struct {
	fixed [len(fileHeaders)]savedHeader
	extra []savedHeader // the names of File.Header
}

type savedHeader struct {
	name   string
	values []string
	had    bool
}

func (s *savedHeaders) save(h, extra http.Header) {
	for i, name := range fileHeaders {
		values, had := h[name]
		s.fixed[i] = savedHeader{name, values, had}
	}
	for key := range extra {
		name := textproto.CanonicalMIMEHeaderKey(key)
		values, had := h[name]
		s.extra = append(s.extra, savedHeader{name, values, had})
	}
}

// restore writes the saved values back. A slice that addHeader appended to
// in place gets its old length back, which hides what it appended.
func (s *savedHeaders) restore(h http.Header) {
	for _, v := range s.fixed {
		v.restore(h)
	}
	for _, v := range s.extra {
		v.restore(h)
	}
}

func (v savedHeader) restore(h http.Header) {
	if v.had {
		h[v.name] = v.values
	} else {
		delete(h, v.name)
	}
}

// passFileThrough makes every writer between w and the server that edits
// response bodies, such as the one offline.Middleware installs, pass the file
// through unchanged: its bytes, its length and its ranges are the file's.
// Such a writer has a PassThrough method.
func passFileThrough(w http.ResponseWriter) {
	for {
		if p, ok := w.(interface{ PassThrough() }); ok {
			p.PassThrough()
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

// addHeader writes extra into h. A name in extra replaces the values h holds
// for it, except Set-Cookie and Vary, whose values add to the ones set before.
// Vary lists the request headers the response depends on, each added by what
// made it depend on one, such as Cookie for a handler that takes the session.
// Keys that spell the same name differently are joined in sorted order,
// which writes the same values on every request.
func addHeader(h, extra http.Header) {
	replaced := make(map[string]bool, len(extra))
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		name := textproto.CanonicalMIMEHeaderKey(key)
		if name != "Set-Cookie" && name != "Vary" && !replaced[name] {
			replaced[name] = true
			delete(h, name)
		}
		h[name] = append(h[name], extra[key]...)
	}
}

// guardedBody is a Body whose Close waits for a Read or Seek under way.
// Read and Seek after Close return [os.ErrClosed] without reaching the Body.
//
// A panic in Read or Seek is recovered: [http.ServeContent] calls them on
// a goroutine that no recover covers. The panic is returned as the error of
// the call, which ends the response, and of every later call.
type guardedBody struct {
	lock     sync.Mutex
	body     io.ReadSeekCloser
	closed   bool
	panicked *datapages.PanicError
}

func (g *guardedBody) Read(p []byte) (n int, err error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if err := g.unusable(); err != nil {
		return 0, err
	}
	defer g.recoverPanic(&err)
	return g.body.Read(p)
}

func (g *guardedBody) Seek(offset int64, whence int) (n int64, err error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if err := g.unusable(); err != nil {
		return 0, err
	}
	defer g.recoverPanic(&err)
	return g.body.Seek(offset, whence)
}

// unusable returns why a Read or Seek must not reach the Body, nil if it may.
func (g *guardedBody) unusable() error {
	switch {
	case g.panicked != nil:
		return *g.panicked
	case g.closed:
		return os.ErrClosed
	}
	return nil
}

// recoverPanic keeps a panic of the Body and returns it in *err.
// It's deferred while g.lock is held.
func (g *guardedBody) recoverPanic(err *error) {
	v := recover()
	if v == nil {
		return
	}
	g.panicked = &datapages.PanicError{Value: v, Stack: debug.Stack()}
	*err = *g.panicked
}

// recovered returns the panic of a Read or Seek, nil without one.
func (g *guardedBody) recovered() *datapages.PanicError {
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.panicked
}

func (g *guardedBody) Close() error {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	return g.body.Close()
}

// cacheControl returns the Cache-Control value of c, "" for the zero value.
// It refuses fields that contradict each other and accepts ones that repeat
// each other, such as Private next to NoStore.
//
// A value without max-age, no-cache or no-store gets no-cache. A browser reuses
// a response without an explicit freshness lifetime for 1/10 of the time since
// its Last-Modified without asking, as RFC 9111 section 4.2.2 permits.
func cacheControl(c datapages.FileCache) (string, error) {
	invalid := func(reason string) (string, error) {
		return "", fmt.Errorf("%w: %s", ErrFileInvalidCache, reason)
	}
	switch {
	case c.Raw != "" && c != (datapages.FileCache{Raw: c.Raw}):
		return invalid("Raw excludes the other fields")
	case c.Raw != "":
		if strings.ContainsFunc(c.Raw, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) {
			return invalid("Raw contains a control character")
		}
		return c.Raw, nil
	case c.MaxAge < 0 || c.MaxAge > 0 && c.MaxAge < time.Second:
		return invalid("MaxAge " + c.MaxAge.String() + " is negative or under a second")
	case c.Immutable && c.MaxAge == 0:
		return invalid("Immutable requires a positive MaxAge")
	case c.MaxAge > 0 && c.NoCache:
		return invalid("NoCache excludes MaxAge")
	case c.MaxAge > 0 && c.NoStore:
		return invalid("NoStore excludes MaxAge")
	case c == datapages.FileCache{}:
		return "", nil
	}
	directives := make([]string, 0, 4)
	if c.Private {
		directives = append(directives, "private")
	}
	if c.NoCache || c.MaxAge == 0 && !c.NoStore {
		directives = append(directives, "no-cache")
	}
	if c.NoStore {
		directives = append(directives, "no-store")
	}
	if c.MaxAge > 0 {
		directives = append(directives,
			"max-age="+strconv.FormatInt(int64(c.MaxAge/time.Second), 10))
	}
	if c.Immutable {
		directives = append(directives, "immutable")
	}
	return strings.Join(directives, ", "), nil
}

// contentDisposition returns the Content-Disposition value of d,
// "" for the zero value.
func contentDisposition(d datapages.FileDisposition) string {
	kind := "inline"
	switch {
	case d.Download:
		kind = "attachment"
	case d.Filename == "":
		return ""
	}
	if d.Filename == "" {
		return kind
	}
	return mime.FormatMediaType(kind, map[string]string{"filename": d.Filename})
}

// validETag reports whether tag holds only etagc characters of RFC 9110
// section 8.8.3, the ones an entity tag allows between its quotes.
func validETag(tag string) bool {
	for i := range len(tag) {
		if c := tag[i]; c != 0x21 && (c < 0x23 || c > 0x7e) && c < 0x80 {
			return false
		}
	}
	return true
}

// IsDocumentRequest reports whether r loads a document the browser shows,
// as a link opened in a tab does. An img element or a fetch loads something else,
// and a browser never shows what they receive.
func IsDocumentRequest(r *http.Request) bool {
	return r.Header.Get("Sec-Fetch-Dest") == "document"
}
