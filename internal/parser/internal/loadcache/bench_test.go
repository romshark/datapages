package loadcache

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"go/types"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// BenchmarkLoad measures cached loading of LOADCACHE_BENCH_DIR under js/wasm.
// A native datapages gen must first fill DATAPAGES_LOADCACHE.
func BenchmarkLoad(b *testing.B) {
	dir := os.Getenv("LOADCACHE_BENCH_DIR")
	if dir == "" {
		b.Skip("LOADCACHE_BENCH_DIR is unset")
	}
	c := Open(dir)
	m, _, err := c.readMeta()
	if err != nil {
		b.Fatal(err)
	}
	all := map[string]bool{}
	for _, pm := range m.Pkgs {
		all[pm.ID] = true
	}
	withBodies := func([]string) ParseFunc {
		return func(
			fset *token.FileSet, filename string, src []byte,
		) (*ast.File, error) {
			return goparser.ParseFile(
				fset, filename, src, goparser.AllErrors|goparser.ParseComments,
			)
		}
	}
	report := func(b *testing.B, pkgs []*packages.Package) {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		b.ReportMetric(float64(ms.HeapAlloc)/(1<<20), "heap-MB")
		runtime.KeepAlive(pkgs)
	}
	for _, bc := range []struct {
		name   string
		parse  func([]string) ParseFunc
		source map[string]bool
	}{
		{"all from source", withBodies, all},
		{"all from source without dependency bodies", withoutBodies, all},
		{"dependencies from export data", withoutBodies, nil},
	} {
		b.Run(bc.name, func(b *testing.B) {
			var pkgs []*packages.Package
			for b.Loop() {
				pkgs, _, _, err = c.build(m, bc.parse, bc.source)
				if err != nil {
					b.Fatal(err)
				}
			}
			report(b, pkgs)
		})
	}
	b.Run("source packages again, dependencies in memory", func(b *testing.B) {
		pkgs, _, _, err := c.build(m, withoutBodies, nil)
		if err != nil {
			b.Fatal(err)
		}
		sizes := types.SizesFor("gc", m.Arch)
		parse := withoutBodies(m.Roots)
		for b.Loop() {
			// Copy each source package and its copied dependencies.
			copies := map[*packages.Package]*packages.Package{}
			for i, pm := range m.Pkgs {
				if !pm.Source {
					continue
				}
				old := pkgs[i]
				np := &packages.Package{
					ID: old.ID, Name: old.Name, PkgPath: old.PkgPath, Dir: old.Dir,
					GoFiles: old.GoFiles, CompiledGoFiles: old.CompiledGoFiles,
					Imports: map[string]*packages.Package{}, Module: old.Module,
					Fset: old.Fset, TypesSizes: sizes,
					Types: types.NewPackage(old.PkgPath, old.Name),
				}
				for path, imp := range old.Imports {
					if cp, ok := copies[imp]; ok {
						imp = cp
					}
					np.Imports[path] = imp
				}
				if err := checkSource(np, parse, sizes, m.Warn); err != nil {
					b.Fatal(err)
				}
				copies[old] = np
			}
		}
		report(b, pkgs)
	})
}

// withoutBodies matches the parser's cold-load parsing.
func withoutBodies(roots []string) ParseFunc {
	return func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		f, err := goparser.ParseFile(
			fset, filename, src, goparser.AllErrors|goparser.ParseComments,
		)
		if f != nil && slices.ContainsFunc(roots, func(r string) bool {
			return strings.HasPrefix(filename, r)
		}) {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok {
					fd.Body = nil
				}
			}
		}
		return f, err
	}
}
