package datapagestest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseExpr tests the handler expressions Click runs: generated action calls
// with their options and before and after code, and assignments.
func TestParseExpr(t *testing.T) {
	type stmt struct {
		Action   string // method and URL
		Form     bool
		Selector string
		Include  string
		Exclude  string
		Headers  map[string]string
		Assign   string // the signal assigned
		Value    any
		From     string
		Not      bool
	}
	tests := map[string]struct {
		expr string
		want []stmt
	}{
		"call": {
			expr: `@post('/add/')`,
			want: []stmt{{Action: "POST /add/"}},
		},
		"escaped URL": {
			expr: `@get('/a\'b\\c?x=1')`,
			want: []stmt{{Action: `GET /a'b\c?x=1`}},
		},
		"options": {
			expr: `@put('/x', {contentType: 'form', selector: '#f', ` +
				`headers: {'X-A': '1'}, ` +
				`filterSignals: {include: /^a;b$/, exclude: /_[/]x/i}, retry: 'never'})`,
			want: []stmt{{
				Action:   "PUT /x",
				Form:     true,
				Selector: "#f",
				Include:  "^a;b$",
				Exclude:  "(?i)_[/]x", Headers: map[string]string{"X-A": "1"},
			}},
		},
		"before and after": {
			expr: "$_busy = true; @delete('/item/1/'); $showModal=false;$newTitle=''",
			want: []stmt{
				{Assign: "_busy", Value: true},
				{Action: "DELETE /item/1/"},
				{Assign: "showModal", Value: false},
				{Assign: "newTitle", Value: ""},
			},
		},
		"signals": {
			expr: "\n\t$a.b = !$c; $d = $e; $n = 1.5; $s = \"x;y\"\n",
			want: []stmt{
				{Assign: "a.b", From: "c", Not: true},
				{Assign: "d", From: "e"},
				{Assign: "n", Value: 1.5},
				{Assign: "s", Value: "x;y"},
			},
		},
		"empty": {expr: " ; ", want: nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sts, err := parseExpr(tc.expr)
			require.NoError(t, err)
			var got []stmt
			for _, st := range sts {
				if st.action == nil {
					got = append(got, stmt{
						Assign: st.path,
						Value:  st.value,
						From:   st.from,
						Not:    st.not,
					})
					continue
				}
				a := st.action
				s := stmt{
					Action:   a.method + " " + a.url,
					Form:     a.form,
					Selector: a.selector,
					Headers:  a.headers,
				}
				if a.filter.include != nil {
					s.Include = a.filter.include.String()
				}
				if a.filter.exclude != nil {
					s.Exclude = a.filter.exclude.String()
				}
				got = append(got, s)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestParseExprRefuses tests that JavaScript other than
// actions and assignments is refused.
func TestParseExprRefuses(t *testing.T) {
	for name, expr := range map[string]string{
		"function call":      "alert(1)",
		"comparison":         "$a == 1",
		"expression value":   "$a = $b + 1",
		"computed URL":       "@get(window.location.pathname)",
		"trailing code":      "@post('/x').then(f)",
		"unterminated":       "@post('/x)",
		"unbalanced":         "@post('/x'))",
		"unknown content":    "@post('/x', {contentType: 'xml'})",
		"payload":            "@post('/x', {payload: {a: 1}})",
		"filter not a regex": "@post('/x', {filterSignals: {include: 'a'}})",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseExpr(expr)
			require.Error(t, err)
		})
	}
}

// TestCSSIdent tests the unescaping of CSS identifiers in #id selectors.
func TestCSSIdent(t *testing.T) {
	for in, want := range map[string][2]string{
		"todo-1":    {"todo-1", ""},
		`a\'b\\c`:   {`a'b\c`, ""},
		`\31 23`:    {"123", ""},
		`\31 23 .x`: {"123", " .x"},
		"list > li": {"list", " > li"},
		"café-id":   {"café-id", ""},
	} {
		ident, rest := cssIdent(in)
		require.Equal(t, want, [2]string{ident, rest}, in)
	}
}

// TestEvalExpr tests the JavaScript subset the tab evaluates in Datastar attributes.
func TestEvalExpr(t *testing.T) {
	p := &page{
		signals: map[string]any{
			"a":    1.0,
			"s":    "x",
			"t":    true,
			"o":    map[string]any{"b": 2.0},
			"list": []any{},
		},
		computed: map[string]string{"twice": "$a + $a", "loop": "$loop"},
	}
	for expr, want := range map[string]any{
		"$a":                    1.0,
		"!$a":                   false,
		"$o.b > $a":             true,
		"$a == 1 && $s === 'x'": true,
		"$a != 1 || $missing":   nil,
		"$a == '1'":             false,
		"'n=' + $a":             "n=1",
		"-$a + 3":               2.0,
		"$t ? 'yes' : 'no'":     "yes",
		"($a + 1) * 2 == 4":     errNeedsBrowser,
		"$twice >= 2":           true,
		"$loop":                 errNeedsBrowser,
		"$list.length":          errNeedsBrowser,
		"$s.trim()":             errNeedsBrowser,
		"null":                  nil,
		"undefined == null":     true,
		"el.checked":            errNeedsBrowser,
		"$a = 2":                errNeedsBrowser,
	} {
		v, err := p.evalExpr(expr)
		if e, ok := want.(error); ok {
			require.ErrorIs(t, err, e, expr)
			continue
		}
		require.NoError(t, err, expr)
		require.Equal(t, want, v, expr)
	}
}
