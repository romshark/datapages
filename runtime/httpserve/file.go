package httpserve

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/romshark/datapages"
)

var (
	// ErrFileNoBody reports a [datapages.File] without a Body.
	ErrFileNoBody = errors.New("datapages.File has no Body")

	// ErrFileNoType reports a [datapages.File] without a Type.
	ErrFileNoType = errors.New("datapages.File has no Type")
)

// ServeFile writes f as the response to r and closes its Body.
// It writes nothing and returns [ErrFileNoBody] or [ErrFileNoType] for an incomplete f.
func ServeFile(w http.ResponseWriter, r *http.Request, f datapages.File) error {
	if c, ok := f.Body.(io.Closer); ok {
		defer func() { _ = c.Close() }()
	}
	switch {
	case f.Body == nil:
		return ErrFileNoBody
	case f.Type == "":
		return ErrFileNoType
	}
	h := w.Header()
	h.Set("Content-Type", f.Type)
	h.Set("X-Content-Type-Options", "nosniff")
	if f.CacheControl != "" {
		h.Set("Cache-Control", f.CacheControl)
	}
	if f.Filename != "" {
		// FormatMediaType returns "" for a name it cannot encode.
		disposition := mime.FormatMediaType("attachment", map[string]string{
			"filename": f.Filename,
		})
		if disposition == "" {
			disposition = "attachment"
		}
		h.Set("Content-Disposition", disposition)
	}
	http.ServeContent(w, r, "", f.ModTime, f.Body)
	return nil
}

// IsDocumentRequest reports whether r loads a document the browser shows,
// as a link opened in a tab does. An img element or a fetch loads something else,
// and a browser never shows what they receive.
func IsDocumentRequest(r *http.Request) bool {
	return r.Header.Get("Sec-Fetch-Dest") == "document"
}
