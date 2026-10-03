package validate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAssetsURLPrefix tests the prefix an application may serve assets under:
// an absolute directory path of letters, digits, '-', '.', '_', '~' and '/',
// without dot segments.
func TestAssetsURLPrefix(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		wantErr error
	}{
		"valid /static/":          {input: "/static/"},
		"valid /assets/":          {input: "/assets/"},
		"valid /a/b/c/":           {input: "/a/b/c/"},
		"root slash":              {input: "/", wantErr: ErrAssetsURLPrefixRoot},
		"no leading slash":        {input: "static/", wantErr: ErrAssetsURLPrefixNoLeadingSlash},
		"no trailing slash":       {input: "/static", wantErr: ErrAssetsURLPrefixNoTrailingSlash},
		"double slash":            {input: "/static//css/", wantErr: ErrAssetsURLPrefixDoubleSlash},
		"query string":            {input: "/static/?v=1/", wantErr: ErrAssetsURLPrefixQueryString},
		"fragment":                {input: "/static/#top/", wantErr: ErrAssetsURLPrefixFragment},
		"dot segment":             {input: "/static/../secret/", wantErr: ErrAssetsURLPrefixDotSegment},
		"dot segment current dir": {input: "/static/./css/", wantErr: ErrAssetsURLPrefixDotSegment},
		"backslash":               {input: `/static\css/`, wantErr: ErrAssetsURLPrefixBackslash},
		"encoded dot":             {input: "/static/%2e%2e/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"encoded slash":           {input: "/static/%2f/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"encoded backslash":       {input: "/static/%5C/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"encoded space":           {input: "/my%20files/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"quote":                   {input: `/my"files/`, wantErr: ErrAssetsURLPrefixInvalidChar},
		"apostrophe":              {input: "/my'files/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"valid with dot":          {input: "/static/v1.2/"},
		"valid with tilde":        {input: "/~assets/"},
		"valid dot-led segment":   {input: "/.well-known/"},
		"space":                   {input: "/my static/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"control char":            {input: "/static/\x00/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"non-ascii":               {input: "/données/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"angle bracket":           {input: "/static</", wantErr: ErrAssetsURLPrefixInvalidChar},
		"valid with hyphens":      {input: "/my-static/"},
		"valid with underscores":  {input: "/my_static/"},
		"valid with digits":       {input: "/static-v2/"},
	} {
		t.Run(name, func(t *testing.T) {
			err := AssetsURLPrefix(tc.input)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
