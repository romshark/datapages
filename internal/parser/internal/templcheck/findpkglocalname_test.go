package templcheck

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestResolvePkgMatcher_DotImport(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import . "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	require.NoError(t, err)

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	// Dot-import without type info available returns nil (no exports to resolve).
	require.Nil(t, resolvePkgMatcher(pkg, "/href", "href"))
}

func TestResolvePkgMatcher_BlankImport(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import _ "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	require.NoError(t, err)

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	// A blank import is skipped: "_" means side effects only, no calls.
	require.Nil(t, resolvePkgMatcher(pkg, "/href", "href"))
}

func TestResolvePkgMatcher_Alias(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import myhref "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	require.NoError(t, err)

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	got := resolvePkgMatcher(pkg, "/href", "href")
	require.NotNil(t, got, "aliased import")
	require.Equal(t, "myhref", got.localName)
	require.Nil(t, got.exports, "exports are only resolved for a dot import")
}

func TestResolvePkgMatcher_Default(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	require.NoError(t, err)

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	got := resolvePkgMatcher(pkg, "/href", "href")
	require.NotNil(t, got, "default import")
	require.Equal(t, "href", got.localName)
}

func TestResolvePkgMatcher_NonTemplFile(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app.go",
		`package app
import "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	require.NoError(t, err)

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	// Imports in non-_templ.go files should be ignored.
	require.Nil(t, resolvePkgMatcher(pkg, "/href", "href"))
}
