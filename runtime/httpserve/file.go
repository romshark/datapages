package httpserve

import (
	"errors"
	"fmt"
	"io"
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
)

// reservedHeaders are the headers [ServeFile] and [http.ServeContent] write,
// by canonical name. A field of [datapages.File] is the one way to set each.
var reservedHeaders = map[string]bool{
	"Accept-Ranges":          true,
	"Cache-Control":          true,
	"Content-Disposition":    true,
	"Content-Length":         true,
	"Content-Range":          true,
	"Content-Type":           true,
	"Etag":                   true,
	"Last-Modified":          true,
	"X-Content-Type-Options": true,
}

// ServeFile writes f as the response to r and closes its Body. It writes
// nothing and returns [ErrFileNoBody], [ErrFileNoType], [ErrFileInvalidETag],
// [ErrFileInvalidCache] or [ErrFileReservedHeader] for an incomplete or
// invalid f.
//
// A panic in Body panics ServeFile. For a multi-range request, [http.ServeContent]
// reads Body on a goroutine of its own. ServeFile recovers a panic there and
// panics with a [datapages.PanicError] that carries the stack of that goroutine,
// once Body is closed.
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

// addHeader writes extra into h. A name in extra replaces the values h holds
// for it, except Set-Cookie and Vary, whose values add to the ones set before.
// Vary lists the request headers the response depends on, each added by what
// made it depend on one, such as Cookie for a handler that takes the session.
// Keys that spell one name differently are joined in sorted order,
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
// a response without an explicit freshness lifetime for a tenth of the time since
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
