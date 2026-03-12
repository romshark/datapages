package hrefcheck_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/hrefcheck"
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
		"dotdot_relative":    {input: "../page", want: false},
		"bare_relative":      {input: "page", want: false},
		"bare_relative_path": {input: "foo/bar", want: false},
		"javascript":         {input: "javascript:void(0)", want: false},
		"javascript_upper":   {input: "JavaScript:void(0)", want: false},

		"fragment":          {input: "#section", want: true},
		"fragment_empty":    {input: "#", want: true},
		"protocol_relative": {input: "//cdn.example.com/lib.js", want: true},
		"https":             {input: "https://example.com", want: true},
		"http":              {input: "http://example.com", want: true},
		"mailto":            {input: "mailto:test@example.com", want: true},
		"tel":               {input: "tel:+1234567890", want: true},
		"sms":               {input: "sms:+1234567890", want: true},
		"ftp":               {input: "ftp://files.example.com", want: true},
		"data":              {input: "data:text/html,<h1>Hi</h1>", want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := hrefcheck.IsAllowedNonRelativeHref(tt.input)
			require.Equal(t, tt.want, got, "IsAllowedNonRelativeHref(%q)", tt.input)
		})
	}
}
