package templcheck

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestResolvePkgMatcher_DotImport(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import . "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	// Dot-import without type info available returns nil (no exports to resolve).
	got := resolvePkgMatcher(pkg, "/href", "href")
	if got != nil {
		t.Errorf("resolvePkgMatcher returned non-nil for dot-import without type info")
	}
}

func TestResolvePkgMatcher_BlankImport(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import _ "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	// Blank import should be skipped — "_" means side-effects only, no calls.
	got := resolvePkgMatcher(pkg, "/href", "href")
	if got != nil {
		t.Errorf("resolvePkgMatcher returned non-nil for blank import")
	}
}

func TestResolvePkgMatcher_Alias(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import myhref "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	got := resolvePkgMatcher(pkg, "/href", "href")
	if got == nil {
		t.Fatal("resolvePkgMatcher returned nil for aliased import")
	}
	if got.localName != "myhref" {
		t.Errorf("localName = %q, want %q", got.localName, "myhref")
	}
	if got.exports != nil {
		t.Errorf("exports should be nil for non-dot import")
	}
}

func TestResolvePkgMatcher_Default(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app_templ.go",
		`package app
import "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	got := resolvePkgMatcher(pkg, "/href", "href")
	if got == nil {
		t.Fatal("resolvePkgMatcher returned nil for default import")
	}
	if got.localName != "href" {
		t.Errorf("localName = %q, want %q", got.localName, "href")
	}
}

func TestResolvePkgMatcher_NonTemplFile(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "app.go",
		`package app
import "example.com/myapp/datapagesgen/href"
`, goparser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	pkg := &packages.Package{
		Fset:   fset,
		Syntax: []*ast.File{f},
		Module: &packages.Module{Path: "example.com/myapp"},
	}

	// Imports in non-_templ.go files should be ignored.
	got := resolvePkgMatcher(pkg, "/href", "href")
	if got != nil {
		t.Errorf("resolvePkgMatcher should return nil for non-_templ.go files")
	}
}
