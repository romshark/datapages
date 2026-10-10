package httpserve

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
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
	// whose fields exclude each other.
	ErrFileInvalidCache = errors.New("datapages.File has an invalid Cache")
)

// ServeFile writes f as the response to r and closes its Body. It writes
// nothing and returns [ErrFileNoBody], [ErrFileNoType], [ErrFileInvalidETag]
// or [ErrFileInvalidCache] for an incomplete or invalid f.
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
	h := w.Header()
	h.Set("Content-Type", f.Type)
	h.Set("X-Content-Type-Options", "nosniff")
	if f.ETag != "" {
		// [http.ServeContent] reads it back for the conditional requests.
		h.Set("ETag", `"`+f.ETag+`"`)
	}
	if cacheControl != "" {
		h.Set("Cache-Control", cacheControl)
	}
	if v := contentDisposition(f.Disposition); v != "" {
		h.Set("Content-Disposition", v)
	}
	http.ServeContent(w, r, "", f.ModTime, f.Body)
	return nil
}

// cacheControl returns the Cache-Control value of c, "" for the zero value.
func cacheControl(c datapages.FileCache) (string, error) {
	invalid := func(reason string) (string, error) {
		return "", fmt.Errorf("%w: %s", ErrFileInvalidCache, reason)
	}
	typed := c.MaxAge != 0 || c.Immutable || c.Private || c.NoCache
	switch {
	case c.Raw != "" && (typed || c.NoStore):
		return invalid("Raw excludes the other fields")
	case c.Raw != "":
		if strings.ContainsFunc(c.Raw, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) {
			return invalid("Raw contains a control character")
		}
		return c.Raw, nil
	case c.NoStore && typed:
		return invalid("NoStore excludes the other fields")
	case c.NoStore:
		return "no-store", nil
	case c.MaxAge < 0 || c.MaxAge > 0 && c.MaxAge < time.Second:
		return invalid("MaxAge " + c.MaxAge.String() + " is negative or under a second")
	case c.Immutable && c.MaxAge == 0:
		return invalid("Immutable requires a positive MaxAge")
	case c.Immutable && c.NoCache:
		return invalid("Immutable excludes NoCache")
	}
	directives := make([]string, 0, 4)
	if c.Private {
		directives = append(directives, "private")
	}
	if c.NoCache {
		directives = append(directives, "no-cache")
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
	// FormatMediaType returns "" for a name it cannot encode.
	if v := mime.FormatMediaType(kind, map[string]string{
		"filename": d.Filename,
	}); v != "" {
		return v
	}
	return kind
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
