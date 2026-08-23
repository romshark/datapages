package httpserve

import (
	"io"
	"net/http"

	"github.com/romshark/datapages"
)

// CSRFScriptWriter writes what makes a page send its CSRF token back.
// It is implemented by the session manager of the generated server.
type CSRFScriptWriter interface {
	WriteCSRFScript(w io.Writer, userID, sessionToken string) error
}

// HTMLDocument is the page [Core.WriteHTML] writes. Every field may be zero.
type HTMLDocument struct {
	// CSRF writes the CSRF script into the head.
	// Nil for an application that declares no session type.
	CSRF                 CSRFScriptWriter
	UserID, SessionToken string

	// HeadGeneric is what the application-wide head generator renders.
	// It goes before Head, which is the head of the page itself.
	HeadGeneric, Head datapages.Head

	Body            datapages.Component
	WriteBodyAttrs  func(w http.ResponseWriter)
	WriteBodySuffix func(w http.ResponseWriter)
}

// WriteHTML writes doc as a complete HTML document.
func (c *Core) WriteHTML(
	w http.ResponseWriter, r *http.Request, doc HTMLDocument,
) error {
	if _, err := io.WriteString(w, c.htmlPrefix); err != nil {
		return err
	}
	if doc.HeadGeneric != nil {
		if err := doc.HeadGeneric.Render(r.Context(), w); err != nil {
			return err
		}
	}
	if doc.Head != nil {
		if err := doc.Head.Render(r.Context(), w); err != nil {
			return err
		}
	}
	if doc.CSRF != nil {
		err := doc.CSRF.WriteCSRFScript(w, doc.UserID, doc.SessionToken)
		if err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "</head><body "); err != nil {
		return err
	}
	if doc.WriteBodyAttrs != nil {
		doc.WriteBodyAttrs(w)
	}
	if _, err := io.WriteString(w, ">"); err != nil {
		return err
	}
	if doc.Body != nil {
		if err := doc.Body.Render(r.Context(), w); err != nil {
			return err
		}
	}
	if doc.WriteBodySuffix != nil {
		if _, err := io.WriteString(w, "<template "); err != nil {
			return err
		}
		doc.WriteBodySuffix(w)
		if _, err := io.WriteString(w, "></template>"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</body></html>")
	return err
}
