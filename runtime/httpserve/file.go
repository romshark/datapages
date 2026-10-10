package httpserve

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
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
func ServeFile(w http.ResponseWriter, r *http.Request, f datapages.File) error {
	if f.Body == nil {
		return ErrFileNoBody
	}
	defer func() { _ = f.Body.Close() }()
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
		// Without Cache-Control, a browser reuses a response with Last-Modified
		// for a tenth of its age without asking: RFC 9111 section 4.2.2.
		h.Set("Cache-Control", "no-cache")
	}
	if v := contentDisposition(f.Disposition); v != "" {
		h.Set("Content-Disposition", v)
	}
	for name, values := range f.Header {
		h.Del(name)
		for _, v := range values {
			h.Add(name, v)
		}
	}
	http.ServeContent(w, r, "", f.ModTime, f.Body)
	return nil
}

// cacheControl returns the Cache-Control value of c, "" for the zero value.
// It refuses fields that contradict each other and accepts ones that repeat
// each other, such as Private next to NoStore.
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
	}
	directives := make([]string, 0, 4)
	if c.Private {
		directives = append(directives, "private")
	}
	if c.NoCache {
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
