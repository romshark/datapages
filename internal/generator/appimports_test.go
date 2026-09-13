package generator

import (
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAppFixedImportsCoverTheHeader tests that appFixedImports names exactly
// the imports app_gen.go's own block carries, and gives each the identifier
// the block binds.
//
// The table is what newAppImports names a package the model brings along against.
// An import missing from it is one a foreign package can be named after,
// which leaves a single identifier naming two packages. A path with the
// wrong identifier is worse: the type renders under a name nothing imports.
//
// The header is written with every option on, since a name is taken for good
// once any model can take it.
func TestAppFixedImportsCoverTheHeader(t *testing.T) {
	t.Parallel()

	const (
		appPath = "example.com/m/app"
		genPath = "example.com/m/app/datapagesgen"
	)
	w := &Writer{
		prometheus:      true,
		assetsURLPrefix: "/static/",
		genImport:       genPath,
		appPkgQual:      appPkgQual,
	}
	w.usage.stream = true
	w.writeAppHeader("datapagesgen", appPath, true)

	f, err := parser.ParseFile(
		token.NewFileSet(), "app_gen.go", w.Buf, parser.ImportsOnly,
	)
	require.NoError(t, err)

	got := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		require.NoError(t, err)
		ident := path.Base(p)
		if imp.Name != nil {
			ident = imp.Name.Name
		}
		switch p {
		case appPath:
			require.Equal(t, appPkgQual, ident,
				"the app package is imported under its alias")
			continue
		case genPath + "/assets", genPath + "/href":
			require.Contains(t, appGenSubpkgIdents, ident)
			continue
		}
		got[p] = ident
	}

	require.Equal(t, appFixedImports, got)
}

// TestNewGenImportsAliasesATakenName tests the three answers the table gives a package:
// the app package keeps the identifier the caller fixed for it,
// a free declared name is kept, and a taken one takes the "dp" prefix.
func TestNewGenImportsAliasesATakenName(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		pkgs      []fakePkg
		wantIdent map[string]string
		wantExtra []genImport
	}{
		"free name is kept": {
			pkgs:      []fakePkg{{path: "example.com/m/mytypes", name: "mytypes"}},
			wantIdent: map[string]string{"example.com/m/mytypes": "mytypes"},
			wantExtra: []genImport{
				{Path: "example.com/m/mytypes", Ident: "mytypes"},
			},
		},
		"taken name is prefixed": {
			pkgs:      []fakePkg{{path: "example.com/m/stream", name: "stream"}},
			wantIdent: map[string]string{"example.com/m/stream": "dpStream"},
			wantExtra: []genImport{
				{Path: "example.com/m/stream", Ident: "dpStream", Aliased: true},
			},
		},
		"the app alias is taken too": {
			pkgs:      []fakePkg{{path: "example.com/m/dpapp", name: "dpapp"}},
			wantIdent: map[string]string{"example.com/m/dpapp": "dpDpapp"},
			wantExtra: []genImport{
				{Path: "example.com/m/dpapp", Ident: "dpDpapp", Aliased: true},
			},
		},
		"two packages of one name are numbered": {
			pkgs: []fakePkg{
				{path: "example.com/m/a/stream", name: "stream"},
				{path: "example.com/m/b/stream", name: "stream"},
			},
			wantIdent: map[string]string{
				"example.com/m/a/stream": "dpStream",
				"example.com/m/b/stream": "dpStream2",
			},
			wantExtra: []genImport{
				{Path: "example.com/m/a/stream", Ident: "dpStream", Aliased: true},
				{Path: "example.com/m/b/stream", Ident: "dpStream2", Aliased: true},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pkgs := make([]*types.Package, 0, len(tt.pkgs))
			for _, p := range tt.pkgs {
				pkgs = append(pkgs, types.NewPackage(p.path, p.name))
			}
			g := newGenImports(pkgs, appTakenIdents(),
				map[string]string{"example.com/m/app": appPkgQual})

			qual := g.Qualifier()
			require.Equal(t, appPkgQual,
				qual(types.NewPackage("example.com/m/app", "app")))
			for p, want := range tt.wantIdent {
				require.Equal(t, want, qual(types.NewPackage(p, "irrelevant")),
					"the table answers by path, not by declared name")
			}
			require.Equal(t, tt.wantExtra, g.Extra())
		})
	}
}

type fakePkg struct{ path, name string }
