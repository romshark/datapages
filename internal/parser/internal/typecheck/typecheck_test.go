package typecheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/internal/parser/internal/typecheck"
)

// checkPkg type-checks src as a package of its own and returns it the way
// go/packages hands one over: syntax, type information and the path.
func checkPkg(t *testing.T, path, src string) *packages.Package {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	require.NoError(t, err)
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	conf := types.Config{}
	tp, err := conf.Check(path, fset, []*ast.File{f}, info)
	require.NoError(t, err)
	return &packages.Package{
		PkgPath: path, Types: tp, TypesInfo: info,
		Syntax: []*ast.File{f}, Fset: fset,
		Imports: map[string]*packages.Package{},
	}
}

func typeNameOf(t *testing.T, pkg *packages.Package, name string) *types.TypeName {
	t.Helper()
	obj, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
	require.True(t, ok, "%s is no type in %s", name, pkg.PkgPath)
	return obj
}

// TestTypeDeclOf tests the declaration [typecheck.TypeDeclOf] finds for a type
// and the doc comment it returns with it.
//
// An event declared outside the app package is read through this, and an alias
// re-exporting one puts the declaration further out than the app package imports,
// which is why the search follows the imports of the imports.
func TestTypeDeclOf(t *testing.T) {
	t.Parallel()

	deep := checkPkg(t, "example.com/deep", `package deep

// EventDeep is "deep"
type EventDeep struct{ Text string }

// Grouped holds two types under one doc comment.
type (
	Grouped struct{ N int }
)
`)
	mid := checkPkg(t, "example.com/mid", `package mid

type Own struct{}
`)
	app := checkPkg(t, "example.com/app", `package app

type Local struct{}
`)

	t.Run("in the package itself", func(t *testing.T) {
		t.Parallel()
		declPkg, ts, doc, ok := typecheck.TypeDeclOf(typeNameOf(t, app, "Local"), app)
		require.True(t, ok)
		require.Equal(t, app, declPkg)
		require.Equal(t, "Local", ts.Name.Name)
		require.Nil(t, doc)
	})

	t.Run("in a direct import", func(t *testing.T) {
		t.Parallel()
		app := checkPkg(t, "example.com/app", `package app
type Local struct{}
`)
		app.Imports["example.com/deep"] = deep

		declPkg, ts, doc, ok := typecheck.TypeDeclOf(
			typeNameOf(t, deep, "EventDeep"), app,
		)
		require.True(t, ok)
		require.Equal(t, deep, declPkg)
		require.Equal(t, "EventDeep", ts.Name.Name)
		require.NotNil(t, doc)
		require.Contains(t, doc.Text(), `EventDeep is "deep"`)
	})

	t.Run("behind an import of an import", func(t *testing.T) {
		t.Parallel()
		app := checkPkg(t, "example.com/app", `package app
type Local struct{}
`)
		mid := checkPkg(t, "example.com/mid", `package mid
type Own struct{}
`)
		mid.Imports["example.com/deep"] = deep
		app.Imports["example.com/mid"] = mid

		declPkg, ts, _, ok := typecheck.TypeDeclOf(
			typeNameOf(t, deep, "EventDeep"), app,
		)
		require.True(t, ok)
		require.Equal(t, deep, declPkg)
		require.Equal(t, "EventDeep", ts.Name.Name)
	})

	t.Run("doc comment of the declaration group", func(t *testing.T) {
		t.Parallel()
		_, ts, doc, ok := typecheck.TypeDeclOf(typeNameOf(t, deep, "Grouped"), deep)
		require.True(t, ok)
		require.Equal(t, "Grouped", ts.Name.Name)
		require.NotNil(t, doc)
		require.Contains(t, doc.Text(), "Grouped holds two types")
	})

	t.Run("out of reach", func(t *testing.T) {
		t.Parallel()
		_, _, _, ok := typecheck.TypeDeclOf(typeNameOf(t, mid, "Own"), app)
		require.False(t, ok, "a package nothing imports was found")
	})

	t.Run("no package", func(t *testing.T) {
		t.Parallel()
		_, _, _, ok := typecheck.TypeDeclOf(nil, app)
		require.False(t, ok)
	})
}
