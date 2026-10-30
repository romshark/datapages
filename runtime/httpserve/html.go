package httpserve

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"

	"github.com/romshark/datapages"
)

var (
	// ErrDocumentAttrsInvalidName reports a [datapages.DocumentAttrs]
	// with a name that it does not accept.
	ErrDocumentAttrsInvalidName = errors.New(
		"datapages.DocumentAttrs has an invalid attribute name",
	)

	// ErrDocumentAttrsReserved reports a [datapages.DocumentAttrs] that sets
	// data-nonce, which [datapages.WithCSPNonce] writes.
	ErrDocumentAttrsReserved = errors.New(
		"datapages.DocumentAttrs sets the reserved attribute data-nonce",
	)
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
	// A nonce or attributes make the prefix differ per response.
	// It's assembled in a pooled buffer and written in 1 piece,
	// since the offline middleware sniffs the type from the first write.
	// Without either, the prebuilt prefix is written.
	nonce := c.CSPNonce(r)
	var bp *[]byte
	if nonce != "" || len(doc.DocumentAttrs) > 0 {
		bp = htmlBufPool.Get().(*[]byte)
		defer func() {
			if cap(*bp) <= maxPooledHTMLBuf {
				*bp = (*bp)[:0]
				htmlBufPool.Put(bp)
			}
		}()
		var err error
		*bp, err = c.appendHTMLHead((*bp)[:0], nonce, doc.DocumentAttrs)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			return err
		}
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
	if err := c.writeHTMLPrefix(w, bp, nonce, doc.WriteHeadPrologue); err != nil {
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
	_, err := io.WriteString(w, "</body></html>")
	return err
}

// htmlBufPool holds the buffers [Core.WriteHTML] assembles a prefix in.
var htmlBufPool = sync.Pool{New: func() any {
	b := make([]byte, 0, 2<<10)
	return &b
}}

// maxPooledHTMLBuf caps the buffers htmlBufPool keeps.
// A large attribute value grows a buffer past it, and the pool drops that buffer.
const maxPooledHTMLBuf = 64 << 10

// writeHTMLPrefix writes the document prefix through the Datastar script tag,
// with prologue between the head and the script tag. bp holds the head
// [Core.appendHTMLHead] assembled, or is nil for the prebuilt head.
func (c *Core) writeHTMLPrefix(
	w io.Writer, bp *[]byte, nonce string, prologue func(io.Writer) error,
) error {
	if bp == nil {
		if prologue == nil {
			_, err := io.WriteString(w, c.htmlPrefix)
			return err
		}
		if _, err := io.WriteString(w, c.htmlHead); err != nil {
			return err
		}
		if err := prologue(w); err != nil {
			return err
		}
		_, err := io.WriteString(w, c.htmlDatastar)
		return err
	}
	if prologue == nil {
		*bp = c.appendDatastarScript(*bp, nonce)
		_, err := w.Write(*bp)
		return err
	}
	if _, err := w.Write(*bp); err != nil {
		return err
	}
	if err := prologue(w); err != nil {
		return err
	}
	*bp = c.appendDatastarScript((*bp)[:0], nonce)
	_, err := w.Write(*bp)
	return err
}

// appendHTMLHead appends the document prefix up to the Datastar script tag to b:
// the html element with attrs and the nonce, and the build script with the nonce.
// It reports an invalid or reserved name in attrs.
func (c *Core) appendHTMLHead(
	b []byte, nonce string, attrs datapages.DocumentAttrs,
) ([]byte, error) {
	b = append(b, htmlDoctype+"<html"...)
	b, err := appendDocumentAttrs(b, attrs)
	if err != nil {
		return b, err
	}
	if nonce != "" {
		b = append(b, ` data-nonce="`...)
		b = append(b, nonce...)
		b = append(b, '"')
	}
	b = append(b, ">"+htmlHeadStart...)
	if c.buildID == "" {
		return b, nil
	}
	b = append(b, c.htmlBuildMeta...)
	if nonce == "" {
		b = append(b, "<script>"...)
	} else {
		b = append(b, `<script nonce="`...)
		b = append(b, nonce...)
		b = append(b, `">`...)
	}
	return append(b, buildScript+"</script>"...), nil
}

// appendDatastarScript appends the Datastar script tag with the nonce to b.
func (c *Core) appendDatastarScript(b []byte, nonce string) []byte {
	b = append(b, "\n\t\t"+`<script type="module"`...)
	if nonce != "" {
		b = append(b, ` nonce="`...)
		b = append(b, nonce...)
		b = append(b, '"')
	}
	b = append(b, ` src="`...)
	b = append(b, c.datastarJSSrcEsc...)
	return append(b, `"></script>`...)
}

// appendDocumentAttrs appends the attributes of attrs to b sorted by name,
// each with a leading space. It reports an invalid or reserved name.
func appendDocumentAttrs(b []byte, attrs datapages.DocumentAttrs) ([]byte, error) {
	if len(attrs) == 0 {
		return b, nil
	}
	// Up to 8 names sort without an allocation.
	var buf [8]string
	names := buf[:0]
	for name := range attrs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		switch {
		case !validAttrName(name):
			return b, fmt.Errorf("%w: %q", ErrDocumentAttrsInvalidName, name)
		case name == "data-nonce":
			return b, ErrDocumentAttrsReserved
		}
		b = append(b, ' ')
		b = append(b, name...)
		b = append(b, `="`...)
		b = appendEscaped(b, attrs[name])
		b = append(b, '"')
	}
	return b, nil
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

// appendEscaped appends s to b escaped as [html.EscapeString] escapes it,
// without the string html.EscapeString allocates for an s it changes.
func appendEscaped(b []byte, s string) []byte {
	last := 0
	for i := 0; i < len(s); i++ {
		var esc string
		switch s[i] {
		case '<':
			esc = "&lt;"
		case '>':
			esc = "&gt;"
		case '&':
			esc = "&amp;"
		case '\'':
			esc = "&#39;"
		case '"':
			esc = "&#34;"
		default:
			continue
		}
		b = append(b, s[last:i]...)
		b = append(b, esc...)
		last = i + 1
	}
	return append(b, s[last:]...)
}
