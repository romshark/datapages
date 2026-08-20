package httpread_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/httpread"
)

// cookieOracle is what net/http answers for the same header.
func cookieOracle(t *testing.T, header, name string) (string, bool) {
	t.Helper()
	r := &http.Request{Header: http.Header{"Cookie": []string{header}}}
	c, err := r.Cookie(name)
	if err != nil {
		return "", false
	}
	return c.Value, true
}

func TestCookieValue(t *testing.T) {
	t.Parallel()

	const name = "sessiontoken"
	for label, header := range map[string]string{
		"plain":              name + "=abc",
		"among other pairs":  "theme=dark; " + name + "=abc; lang=en",
		"quoted value":       name + `="abc"`,
		"space before name":  " " + name + "=abc",
		"space after name":   name + " =abc",
		"tab around pair":    "\t" + name + "=abc\t",
		"invalid value byte": name + `=a"b`,
		"invalid then valid": name + `=a"b; ` + name + "=abc",
		"another cookie":     "theme=dark",
		"empty value":        name + "=",
		"no equals sign":     name,
		"empty header":       "",
		"only separators":    ";;;",
		"value with space":   name + "=a b",
		"repeated":           name + "=first; " + name + "=second",
	} {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			wantValue, wantOK := cookieOracle(t, header, name)
			r := &http.Request{Header: http.Header{"Cookie": []string{header}}}
			value, ok := httpread.CookieValue(r, name)
			require.Equal(t, wantOK, ok)
			require.Equal(t, wantValue, value)
		})
	}
}

// TestCookieValueManyPairs covers the handover to net/http,
// which applies a limit of its own to the number of cookies.
func TestCookieValueManyPairs(t *testing.T) {
	t.Parallel()

	const name = "sessiontoken"
	header := ""
	for range 200 {
		header += "a=b; "
	}
	header += name + "=abc"

	wantValue, wantOK := cookieOracle(t, header, name)
	r := &http.Request{Header: http.Header{"Cookie": []string{header}}}
	value, ok := httpread.CookieValue(r, name)
	require.Equal(t, wantOK, ok)
	require.Equal(t, wantValue, value)
}

// TestCookieValueSeveralLines covers a request carrying more than one
// Cookie header, which net/http reads as one sequence of pairs.
func TestCookieValueSeveralLines(t *testing.T) {
	t.Parallel()

	const name = "sessiontoken"
	r := &http.Request{Header: http.Header{
		"Cookie": []string{"theme=dark", name + "=abc"},
	}}
	value, ok := httpread.CookieValue(r, name)
	require.True(t, ok)
	require.Equal(t, "abc", value)
}

func TestIsCookieName(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{
		"sessiontoken": true,
		"a-b_c.d":      true,
		"":             false,
		"a b":          false,
		"a=b":          false,
		"a;b":          false,
		"a,b":          false,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, want, httpread.IsCookieName(name))
		})
	}
}

func FuzzCookieValue(f *testing.F) {
	for _, seed := range []string{
		"sessiontoken=abc", `sessiontoken="abc"`, "a=b; sessiontoken=abc",
		"sessiontoken=a\"b", ";;", " sessiontoken = abc ", "sessiontoken",
	} {
		f.Add(seed, "sessiontoken")
	}
	f.Fuzz(func(t *testing.T, header, name string) {
		if !httpread.IsCookieName(name) {
			t.Skip("net/http reads no cookie of such a name")
		}
		r := &http.Request{Header: http.Header{"Cookie": []string{header}}}
		wantValue, wantOK := "", false
		if c, err := r.Cookie(name); err == nil {
			wantValue, wantOK = c.Value, true
		}
		value, ok := httpread.CookieValue(r, name)
		if ok != wantOK || value != wantValue {
			t.Fatalf("header %q name %q: got (%q,%v), want (%q,%v)",
				header, name, value, ok, wantValue, wantOK)
		}
	})
}

func TestQueryValue(t *testing.T) {
	t.Parallel()

	for label, rawQuery := range map[string]string{
		"plain":             "term=x",
		"plus is a space":   "term=a+b",
		"escaped key":       "te%72m=x",
		"first value wins":  "term=1&term=2",
		"semicolon pair":    "term=a;b",
		"bad escape":        "term=%zz&term=ok",
		"empty pairs":       "&&term=y",
		"escaped value":     "term=%2Fx",
		"key without value": "term",
		"value carries =":   "term=a=b",
		"plus in key":       "te+rm=x",
		"empty key first":   "=x&term=y",
		"other key":         "x=1",
		"empty query":       "",
	} {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			values, _ := url.ParseQuery(rawQuery)
			require.Equal(t, values.Get("term"), httpread.QueryValue(rawQuery, "term"))
			require.Equal(t, values.Has("term"), httpread.QueryHas(rawQuery, "term"))
		})
	}
}

func FuzzQueryValue(f *testing.F) {
	for _, seed := range []string{
		"term=x", "term=a+b", "te%72m=x", "term=1&term=2", "term=a;b",
		"term=%zz&term=ok", "&&term=y", "term", "",
	} {
		f.Add(seed, "term")
	}
	f.Fuzz(func(t *testing.T, rawQuery, key string) {
		values, _ := url.ParseQuery(rawQuery)
		if got, want := httpread.QueryValue(rawQuery, key), values.Get(key); got != want {
			t.Fatalf("query %q key %q: value = %q, want %q", rawQuery, key, got, want)
		}
		if got, want := httpread.QueryHas(rawQuery, key), values.Has(key); got != want {
			t.Fatalf("query %q key %q: has = %v, want %v", rawQuery, key, got, want)
		}
	})
}
