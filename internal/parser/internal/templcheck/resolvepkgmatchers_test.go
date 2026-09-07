package templcheck

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// parseFiles parses src keyed by filename into a package of the given module.
func parseFiles(t *testing.T, src map[string]string) *packages.Package {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	for name, code := range src {
		f, err := goparser.ParseFile(fset, name, code, goparser.ImportsOnly)
		require.NoError(t, err, name)
		files = append(files, f)
	}
	return &packages.Package{
		Fset:   fset,
		Syntax: files,
		Module: &packages.Module{Path: "example.com/myapp"},
	}
}

// TestResolvePkgMatchers tests the local name the generated href and action
// packages are called under, resolved per file rather than per package:
// two files  of one package may import it under different aliases,
// and a blank import means no calls at all.
func TestResolvePkgMatchers(t *testing.T) {
	t.Parallel()

	const href = "example.com/myapp/datapagesgen/href"
	for name, tc := range map[string]struct {
		files map[string]string
		want  map[string]string // .templ filename -> localName
	}{
		"default": {
			files: map[string]string{
				"app_templ.go": "package app\nimport \"" + href + "\"\n",
			},
			want: map[string]string{"app.templ": "href"},
		},
		"alias": {
			files: map[string]string{
				"app_templ.go": "package app\nimport myhref \"" + href + "\"\n",
			},
			want: map[string]string{"app.templ": "myhref"},
		},
		// Two files of one package importing it under different names.
		// One matcher for the package recognized the calls of one file only.
		"one alias per file": {
			files: map[string]string{
				"a_templ.go": "package app\nimport \"" + href + "\"\n",
				"b_templ.go": "package app\nimport h \"" + href + "\"\n",
			},
			want: map[string]string{"a.templ": "href", "b.templ": "h"},
		},
		// A blank import means side effects only, no calls.
		"blank import": {
			files: map[string]string{
				"app_templ.go": "package app\nimport _ \"" + href + "\"\n",
			},
			want: map[string]string{},
		},
		// A dot import without type info resolves no exports.
		"dot import": {
			files: map[string]string{
				"app_templ.go": "package app\nimport . \"" + href + "\"\n",
			},
			want: map[string]string{},
		},
		"non-templ file is ignored": {
			files: map[string]string{
				"app.go": "package app\nimport \"" + href + "\"\n",
			},
			want: map[string]string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := resolvePkgMatchers(parseFiles(t, tc.files), "/href", "href")
			require.Len(t, got, len(tc.want))
			for file, localName := range tc.want {
				m := got[file]
				require.NotNil(t, m, file)
				require.Equal(t, localName, m.localName)
				require.Nil(t, m.exports, "exports are only resolved for a dot import")
			}
		})
	}
}
