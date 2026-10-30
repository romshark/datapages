package httpserve

import (
	"errors"
	"fmt"
	"html"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/romshark/datapages"
)

var (
	// ErrDocumentAttrsInvalidName reports a [datapages.DocumentAttrs]
	// with a name that it does not accept.
	ErrDocumentAttrsInvalidName = errors.New(
		"datapages.DocumentAttrs has an invalid attribute name")

	// ErrDocumentAttrsReserved reports a [datapages.DocumentAttrs] that sets
	// data-nonce, which [datapages.WithCSPNonce] writes.
	ErrDocumentAttrsReserved = errors.New(
		"datapages.DocumentAttrs sets the reserved attribute data-nonce")
)

// CSRFScriptWriter writes the script that adds a CSRF token to state-changing
// Datastar requests.
// It is implemented by the session manager of the generated server.
type CSRFScriptWriter interface {
	WriteCSRFScript(w io.Writer, sessionToken, cspNonce string) error
}

// HTMLDocument is the page [Core.WriteHTML] writes. Every field may be zero.
type HTMLDocument struct {
	// Status is the status code [Core.WriteHTML] writes after the headers it sets.
	// 0 leaves it to the first write, which sends 200.
	Status int

	// CSRF writes the CSRF script into the head.
	// Nil for an application that declares no session type.
	CSRF CSRFScriptWriter

	// SessionToken is the token the CSRF script sends with actions from this document.
	// It must match the session cookie those actions will send. Leave it empty when
	// the actions will send no session cookie or when visitors share the document.
	// A document with a token is sent with Cache-Control: private,
	// unless the response already has a Cache-Control header.
	SessionToken string

	// HeadGeneric is what the application-wide head generator renders.
	// It goes before Head, which is the head of the page itself.
	HeadGeneric, Head datapages.Head

	// DocumentAttrs holds the attributes of the html element.
	DocumentAttrs datapages.DocumentAttrs

	// WriteHeadPrologue writes into the head between the opening tags and the
	// Datastar script tag. It is the seam for a script that has to run before
	// Datastar does, which a deferred module script cannot: the stateful-page
	// fetch wrapper installs itself here so no request on the page escapes it.
	// Nil when nothing needs that seam, which writes the head in a single piece.
	WriteHeadPrologue func(w io.Writer) error

	Body            datapages.Component
	WriteBodyAttrs  func(w http.ResponseWriter)
	WriteBodySuffix func(w http.ResponseWriter)
}

// WriteHTML writes doc as a complete HTML document.
// For doc.DocumentAttrs with an invalid or reserved name it writes
// 500 Internal Server Error instead and returns [ErrDocumentAttrsInvalidName]
// or [ErrDocumentAttrsReserved].
//
// The attribute writers of doc open with their own separating space, which is
// why the body and the template tag are written without one. 2 attributes
// written next to each other would otherwise be 1 HTML parse error,
// recovered from by every browser and refused by every validator.
func (c *Core) WriteHTML(
	w http.ResponseWriter, r *http.Request, doc HTMLDocument,
) error {
	attrs, err := htmlAttrs(doc.DocumentAttrs)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return err
	}
	if doc.SessionToken != "" && w.Header().Get("Cache-Control") == "" {
		// The CSRF script carries a token derived from the session.
		// A shared cache would serve it to the next visitor, whose actions
		// it fails with 403.
		w.Header().Set("Cache-Control", "private")
	}
	if doc.Status != 0 {
		w.WriteHeader(doc.Status)
	}
	head, datastarScript := c.htmlOpening(r, attrs)
	if doc.WriteHeadPrologue == nil {
		if _, err := io.WriteString(w, head+datastarScript); err != nil {
			return err
		}
	} else {
		if _, err := io.WriteString(w, head); err != nil {
			return err
		}
		if err := doc.WriteHeadPrologue(w); err != nil {
			return err
		}
		if _, err := io.WriteString(w, datastarScript); err != nil {
			return err
		}
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
		err := doc.CSRF.WriteCSRFScript(w, doc.SessionToken, c.CSPNonce(r))
		if err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "</head><body"); err != nil {
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
		if _, err := io.WriteString(w, "<template"); err != nil {
			return err
		}
		doc.WriteBodySuffix(w)
		if _, err := io.WriteString(w, "></template>"); err != nil {
			return err
		}
	}
	_, err = io.WriteString(w, "</body></html>")
	return err
}

// htmlAttrs returns the attributes of the html element sorted by name,
// each with a leading space.
func htmlAttrs(d datapages.DocumentAttrs) (string, error) {
	if len(d) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(d)) {
		switch {
		case !validAttrName(name):
			return "", fmt.Errorf("%w: %q", ErrDocumentAttrsInvalidName, name)
		case name == "data-nonce":
			return "", ErrDocumentAttrsReserved
		}
		writeAttr(&b, name, d[name])
	}
	return b.String(), nil
}

// validAttrName reports whether name starts with a lowercase ASCII letter
// followed by lowercase ASCII letters, digits, '-', '_', '.' and ':'.
// HTML accepts more, but it compares names without case,
// which makes "Data-X" and "data-x" 1 attribute written 2 times.
func validAttrName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		switch c := name[i]; {
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9',
			c == '-', c == '_', c == '.', c == ':':
		default:
			return false
		}
	}
	return true
}

// writeAttr writes the attribute name="value" with a leading space.
func writeAttr(b *strings.Builder, name, value string) {
	b.WriteByte(' ')
	b.WriteString(name)
	b.WriteString(`="`)
	b.WriteString(html.EscapeString(value))
	b.WriteByte('"')
}
