package hrefcheck_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/hrefcheck"
)

func TestIsAllowedNonRelativeHref(t *testing.T) {
	tests := map[string]struct {
		input string
		want  bool
	}{
		"empty":              {input: "", want: false},
		"whitespace":         {input: "   ", want: false},
		"query_only":         {input: "?tab=settings", want: false},
		"root_relative":      {input: "/login", want: false},
		"root_relative_deep": {input: "/static/style.css", want: false},
		"dot_relative":       {input: "./page", want: false},
		// The scheme decides, the rest of the URL is not parsed.
		"space_in_url":       {input: "https://exa mple.com", want: true},
		"bad_escape":         {input: "https://example.com/%zz", want: true},
		"dotdot_relative":    {input: "../page", want: false},
		"bare_relative":      {input: "page", want: false},
		"bare_relative_path": {input: "foo/bar", want: false},
		"javascript":         {input: "javascript:void(0)", want: false},
		"javascript_upper":   {input: "JavaScript:void(0)", want: false},

		"fragment":            {input: "#section", want: true},
		"fragment_empty":      {input: "#", want: true},
		"fragment_with_query": {input: "#frag?query=param", want: true},
		"protocol_relative":   {input: "//cdn.example.com/lib.js", want: true},
		"https":               {input: "https://example.com", want: true},
		"http":                {input: "http://example.com", want: true},
		"mailto":              {input: "mailto:test@example.com", want: true},
		"tel":                 {input: "tel:+1234567890", want: true},
		"sms":                 {input: "sms:+1234567890", want: true},
		"ftp":                 {input: "ftp://files.example.com", want: true},
		"data":                {input: "data:text/html,<h1>Hi</h1>", want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := hrefcheck.IsAllowedNonRelativeHref(tt.input)
			require.Equal(t, tt.want, got, "IsAllowedNonRelativeHref(%q)", tt.input)
		})
	}
}

func TestAssetPath(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		prefix, p, want string
	}{
		"plain":        {"/static/", "style.css", "/static/style.css"},
		"subdirectory": {"/static/", "css/app.css", "/static/css/app.css"},
		"empty":        {"/static/", "", "/static"},
		"dot":          {"/static/", ".", "/static"},
		"absolute":     {"/static/", "/etc/passwd", "/static/etc/passwd"},
		// path.Join normalizes, it does not confine: this leaves the prefix.
		"escape":        {"/static/", "../secret", "/secret"},
		"unclean":       {"/static/", "css//app.css", "/static/css/app.css"},
		"trailing dots": {"/static/", "a/./b", "/static/a/b"},
		"other prefix":  {"/assets/", "logo.svg", "/assets/logo.svg"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, hrefcheck.AssetPath(tc.prefix, tc.p))
		})
	}
}
