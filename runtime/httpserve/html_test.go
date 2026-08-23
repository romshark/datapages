package httpserve_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// renderer renders s and fails with err when err is non-nil.
type renderer struct {
	s   string
	err error
}

func (c renderer) Render(_ context.Context, w io.Writer) error {
	if c.err != nil {
		return c.err
	}
	_, err := io.WriteString(w, c.s)
	return err
}

// csrfScript writes what a session manager would.
type csrfScript struct{ err error }

func (c csrfScript) WriteCSRFScript(w io.Writer, userID, token string) error {
	if c.err != nil {
		return c.err
	}
	_, err := io.WriteString(w, "<csrf "+userID+" "+token+">")
	return err
}

func builtCore(t *testing.T) *httpserve.Core {
	t.Helper()
	c := httpserve.NewCore(datapages.ServerConfig{
		DatastarJS: "/ds.js",
	}, "")
	c.Build()
	return c
}

func writeHTML(
	t *testing.T, c *httpserve.Core, doc httpserve.HTMLDocument,
) (string, error) {
	t.Helper()
	w := httptest.NewRecorder()
	err := c.WriteHTML(w, httptest.NewRequest(http.MethodGet, "/", nil), doc)
	return w.Body.String(), err
}

func TestWriteHTML(t *testing.T) {
	t.Parallel()

	c := builtCore(t)

	for name, tc := range map[string]struct {
		doc  httpserve.HTMLDocument
		want string
	}{
		"empty": {
			httpserve.HTMLDocument{},
			c.HTMLPrefix() + "</head><body ></body></html>",
		},
		"head and body": {
			httpserve.HTMLDocument{
				Head: renderer{s: "<title>t</title>"},
				Body: renderer{s: "<p>b</p>"},
			},
			c.HTMLPrefix() +
				"<title>t</title></head><body ><p>b</p></body></html>",
		},
		"generic head goes first": {
			httpserve.HTMLDocument{
				HeadGeneric: renderer{s: "<meta g>"},
				Head:        renderer{s: "<meta p>"},
			},
			c.HTMLPrefix() + "<meta g><meta p></head><body ></body></html>",
		},
		"csrf follows the head": {
			httpserve.HTMLDocument{
				CSRF:         csrfScript{},
				UserID:       "u1",
				SessionToken: "tok",
				Head:         renderer{s: "<meta p>"},
			},
			c.HTMLPrefix() +
				"<meta p><csrf u1 tok></head><body ></body></html>",
		},
		"body attributes and suffix": {
			httpserve.HTMLDocument{
				WriteBodyAttrs: func(w http.ResponseWriter) {
					_, _ = io.WriteString(w, `class="x"`)
				},
				WriteBodySuffix: func(w http.ResponseWriter) {
					_, _ = io.WriteString(w, `data-x`)
				},
			},
			c.HTMLPrefix() +
				`</head><body class="x"><template data-x></template></body></html>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := writeHTML(t, c, tc.doc)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestWriteHTMLError(t *testing.T) {
	t.Parallel()

	c := builtCore(t)
	errRender := errors.New("render failed")

	for name, doc := range map[string]httpserve.HTMLDocument{
		"generic head": {HeadGeneric: renderer{err: errRender}},
		"head":         {Head: renderer{err: errRender}},
		"body":         {Body: renderer{err: errRender}},
		"csrf":         {CSRF: csrfScript{err: errRender}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := writeHTML(t, c, doc)
			require.ErrorIs(t, err, errRender)
		})
	}
}

func TestCheckDatastarRequest(t *testing.T) {
	t.Parallel()

	c := builtCore(t)

	t.Run("datastar", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Datastar-Request", "true")
		require.True(t, c.CheckDatastarRequest(w, r))
		require.Empty(t, w.Body.String())
	})

	t.Run("plain", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, c.CheckDatastarRequest(w, r))
		require.Equal(t, http.StatusNotAcceptable, w.Code)
	})
}

func TestHTTPErrBad(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	builtCore(t).HTTPErrBad(w, "reading signals", errors.New("eof"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "reading signals", strings.TrimSpace(w.Body.String()))
}
