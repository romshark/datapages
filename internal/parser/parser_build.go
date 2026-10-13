package parser

import (
	"bufio"
	"errors"
	"go/build"
	"go/build/constraint"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

var ErrBuildConstrainedFile = errors.New(
	"app package file must not have a build constraint",
)

// checkBuildConstraints reports each file of the app package that a build
// constraint restricts, by a //go:build line or by a name ending in an operating
// system or an architecture, as _windows.go does. The model is read for the host
// platform. A page, action or event declared in a file the host excludes is
// missing from the generated code without an error, and one declared in a file
// another platform excludes breaks the build there.
//
// A file under //go:build ignore is in no build. A test file declares nothing
// generated code reads.
func checkBuildConstraints(pkg *packages.Package, errs *Errors) {
	check := func(path string, excluded bool) {
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return
		}
		h := readBuildHeader(path)
		if h.expr != nil && neverBuilt(h.expr) {
			return
		}
		if !excluded && h.expr == nil && !nameConstrained(path) {
			return
		}
		errs.ErrAt(token.Position{Filename: path, Line: h.line, Column: 1},
			ErrBuildConstrainedFile)
	}
	for _, path := range pkg.GoFiles {
		check(path, false)
	}
	for _, path := range pkg.IgnoredFiles {
		check(path, true)
	}
}

// buildHeader is what [readBuildHeader] finds above the package clause.
type buildHeader struct {
	expr constraint.Expr // nil without a constraint line
	line int             // of the constraint line, else of the package clause
}

// readBuildHeader finds the constraint line of the file at path. An unreadable
// file has none, and the compiler reports it when the build includes it.
func readBuildHeader(path string) buildHeader {
	h := buildHeader{line: 1}
	f, err := os.Open(path)
	if err != nil {
		return h
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			if h.expr == nil {
				h.line = n
			}
			break
		}
		if h.expr != nil ||
			(!constraint.IsGoBuild(line) && !constraint.IsPlusBuild(line)) {
			continue
		}
		if x, err := constraint.Parse(line); err == nil {
			h.expr, h.line = x, n
		}
	}
	return h
}

// neverBuilt reports whether no build includes a file under the constraint x.
// "ignore" is the tag no build sets, and any other tag may be set or not. An
// expression of more than 16 tags counts as built rather than trying them all.
func neverBuilt(x constraint.Expr) bool {
	var tags []string
	var collect func(constraint.Expr)
	collect = func(x constraint.Expr) {
		switch x := x.(type) {
		case *constraint.TagExpr:
			if x.Tag != "ignore" && !slices.Contains(tags, x.Tag) {
				tags = append(tags, x.Tag)
			}
		case *constraint.NotExpr:
			collect(x.X)
		case *constraint.AndExpr:
			collect(x.X)
			collect(x.Y)
		case *constraint.OrExpr:
			collect(x.X)
			collect(x.Y)
		}
	}
	collect(x)
	if len(tags) > 16 {
		return false
	}
	for set := range 1 << len(tags) {
		if x.Eval(func(tag string) bool {
			i := slices.Index(tags, tag)
			return i >= 0 && set&(1<<i) != 0
		}) {
			return false
		}
	}
	return true
}

// nameConstrained reports whether the name of the file at path restricts it to
// an operating system or an architecture, as _windows.go and _arm64.go do.
// [build.Context.MatchFile] applies Go's list of both, and for a platform named
// "-" it refuses exactly the names that carry one.
func nameConstrained(path string) bool {
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH = "-", "-"
	// The real content would add the constraint line, which [readBuildHeader] reads.
	ctx.OpenFile = func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("package p\n")), nil
	}
	ok, err := ctx.MatchFile(filepath.Dir(path), filepath.Base(path))
	return err == nil && !ok
}
