package datapagestest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/csrfscript"
)

// TestParseEvents tests Datastar framing and standard SSE fields.
func TestParseEvents(t *testing.T) {
	tests := map[string]struct {
		body string
		want []Event
	}{
		"sdk terminator": {
			// The SDK ends each event with a blank line and one more "\n".
			body: "event: datastar-patch-elements\ndata: elements <p id=\"a\">1</p>\n\n\n" +
				"event: datastar-patch-signals\ndata: signals {\"n\":1}\n\n\n",
			want: []Event{
				{
					Type: PatchElements,
					Data: map[string]string{"elements": `<p id="a">1</p>`},
				},
				{
					Type: PatchSignals,
					Data: map[string]string{"signals": `{"n":1}`},
				},
			},
		},
		"repeated field": {
			body: "event: datastar-patch-elements\ndata: selector #x\ndata: mode inner\n" +
				"data: elements <li>1</li>\ndata: elements <li>2</li>\n\n",
			want: []Event{{Type: PatchElements, Data: map[string]string{
				"selector": "#x", "mode": "inner", "elements": "<li>1</li>\n<li>2</li>",
			}}},
		},
		"empty line in elements": {
			body: "event: datastar-patch-elements\ndata: elements <pre>a\n" +
				"data: elements \ndata: elements b</pre>\n\n",
			want: []Event{{Type: PatchElements, Data: map[string]string{
				"elements": "<pre>a\n\nb</pre>",
			}}},
		},
		"crlf comments and ids": {
			body: ": keepalive\r\nid: 7\r\nretry: 1000\r\nevent: datastar-patch-signals\r\n" +
				"data: signals {\"url\":\"http://x/y\"}\r\n\r\n",
			want: []Event{{Type: PatchSignals, Data: map[string]string{
				"signals": `{"url":"http://x/y"}`,
			}, Retry: time.Second}},
		},
		"unfinished event": {
			body: "event: datastar-patch-elements\ndata: elements <p id=\"a\">1</p>\n\n" +
				"event: datastar-patch-elements\ndata: elements <p id=\"b\">",
			want: []Event{
				{Type: PatchElements, Data: map[string]string{"elements": `<p id="a">1</p>`}},
			},
		},
		"blank lines only": {body: "\n\n\n"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, parseEvents(tt.body))
		})
	}
}

// TestScriptRedirect tests each redirect script Datapages emits.
func TestScriptRedirect(t *testing.T) {
	tests := map[string]struct {
		script string
		want   string
	}{
		"before sse": {`window.location = "/a b?q=\"x\"";`, `/a b?q="x"`},
		"page cache": {`window.location="/x/";`, "/x/"},
		"page cache worker": {
			`(function(){var go=function(){window.location="/x/";};` +
				`if(!navigator.serviceWorker){go();return;}})();`,
			"/x/",
		},
		"event": {
			`<script data-effect="el.remove()">` +
				`setTimeout(() => window.location.href = "/y/<z>")</script>`,
			"/y/<z>",
		},
		"json escapes": {`window.location.href = "/\u00e9\u2028"`, "/\u00e9\u2028"},
		"none":         {`console.log("window.location")`, ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := scriptRedirect(tt.script)
			require.Equal(t, tt.want != "", ok)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestLiteral tests supported literals and browser-only expressions.
func TestLiteral(t *testing.T) {
	tests := map[string]struct {
		expr    string
		want    any
		wantErr bool
	}{
		"single quoted":       {expr: `'all'`, want: "all"},
		"single quoted empty": {expr: `''`, want: ""},
		"escapes":             {expr: `'it\'s\\\n\ré'`, want: "it's\\\n\ré"},
		"non-ascii":           {expr: `'😀'`, want: "\U0001F600"},
		"json string":         {expr: `"x \"y\""`, want: `x "y"`},
		"number":              {expr: `42`, want: float64(42)},
		"bool":                {expr: ` false `, want: false},
		"null":                {expr: `null`, want: nil},
		"json array":          {expr: `[1, "a"]`, want: []any{float64(1), "a"}},
		"concatenation":       {expr: `'a' + 'b'`, wantErr: true},
		"signal reference":    {expr: `$other`, wantErr: true},
		"unicode escape":      {expr: `'\` + `u0041'`, wantErr: true},
		"tab escape":          {expr: `'\t'`, wantErr: true},
		"trailing backslash":  {expr: `'a\'`, wantErr: true},
		"object literal":      {expr: `{a: 1}`, wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := literal(tt.expr)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestCSRFTokenOfPage tests extracting a token from a page's CSRF script.
func TestCSRFTokenOfPage(t *testing.T) {
	tests := map[string]struct {
		page string
		want string
	}{
		"script": {
			page: csrfscript.Open + csrfscript.Prefix + "abc-1" + csrfscript.Suffix,
			want: "abc-1",
		},
		"none":         {page: `<p>guest</p>`},
		"unterminated": {page: csrfscript.Open + csrfscript.Prefix + "abc"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, csrfToken(tt.page))
		})
	}
}

// TestConvertCase tests the signal names Datastar derives from attribute keys.
func TestConvertCase(t *testing.T) {
	tests := map[string]struct {
		key  string
		want string
	}{
		"default camel":    {"new-title", "newTitle"},
		"nested":           {"form.new-title", "form.newTitle"},
		"kebab kept":       {"new-title__case.kebab", "new-title"},
		"snake":            {"new-title__case.snake", "new_title"},
		"pascal":           {"new-title__case.pascal", "NewTitle"},
		"digit after dash": {"row-1", "row-1"},
		"ifmissing":        {"new-title__ifmissing", "newTitle"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			n, mods := splitModifiers(tt.key)
			require.Equal(t, tt.want, convertCase(n, mods["case"]))
		})
	}
}

// TestParseCall tests the method, URL and options read from generated action
// expressions, including a URL whose JavaScript string escapes a quote of the route.
func TestParseCall(t *testing.T) {
	// parsed is a call with its filter patterns as text, which require.Equal compares.
	type parsed struct {
		method, path     string
		form, payload    bool
		headers          map[string]string
		include, exclude string
	}
	tests := map[string]struct {
		expr string
		want parsed
	}{
		"post":  {`@post('/form/submit/')`, parsed{method: "POST", path: "/form/submit/"}},
		"get":   {`@get('/files/1/')`, parsed{method: "GET", path: "/files/1/"}},
		"patch": {`@patch('/p/')`, parsed{method: "PATCH", path: "/p/"}},
		"query": {
			`@query('/input/?btn=18&paste=true')`,
			parsed{method: "QUERY", path: "/input/?btn=18&paste=true"},
		},
		"percent-escaped": {`@delete('/row/a%27b/')`, parsed{method: "DELETE", path: "/row/a%27b/"}},
		"quote escape":    {`@post('/it\'s/\\x/')`, parsed{method: "POST", path: `/it's/\x/`}},
		"form": {
			`@post('/start/', {contentType: 'form'})`,
			parsed{method: "POST", path: "/start/", form: true},
		},
		"json": {`@post('/x/', {contentType: 'json'})`, parsed{method: "POST", path: "/x/"}},
		"payload": {
			`@post('/x/', {payload: evt.detail})`,
			parsed{method: "POST", path: "/x/", payload: true},
		},
		"headers, filter, before and after": {
			`$busy = true; @post('/x/', {filterSignals: {include: /^a}'/, exclude: /b/i}, ` +
				`headers: {'X-Note': 'it\'s }', 'X-Id': '7'}}); $busy = false`,
			parsed{
				method: "POST", path: "/x/",
				headers: map[string]string{"X-Note": "it's }", "X-Id": "7"},
				include: "^a}'", exclude: "(?i)b",
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := parseCall(t, tt.expr)
			got := parsed{
				method: c.method, path: c.path, form: c.form, payload: c.payload,
				headers: c.headers,
			}
			if c.include != nil {
				got.include = c.include.String()
			}
			if c.exclude != nil {
				got.exclude = c.exclude.String()
			}
			require.Equal(t, tt.want, got)
		})
	}
}
