package generator

import (
	"go/types"
	"maps"
	"slices"
	"strconv"

	"github.com/romshark/datapages/internal/parser/model"
)

// appFixedImports maps every import path app_gen.go's own block carries to the
// identifier it binds. [Writer.writeAppHeader] writes that block.
//
// A package the model brings along is named against this: one whose path is
// here keeps the identifier the block already gives it, and one whose declared
// name is taken by a different path needs an alias, or a single identifier
// would name two packages and the generated file would not compile.
//
// Listed rather than read back from the writer, since the writer emits some of
// them only for the models that need them while a name is taken for good once
// any model can take it.
//
// TestAppFixedImportsCoverTheHeader fails when the writer emits an import this
// table does not carry. Every entry also needs a type in the pkgname_foreign
// fixture and in internal/acceptance/pkgnames to be exercised under a
// colliding package name; neither is enforced.
var appFixedImports = map[string]string{
	"bufio":                "bufio",
	"context":              "context",
	"encoding/json":        "json",
	"errors":               "errors",
	"fmt":                  "fmt",
	"github.com/a-h/templ": "templ",
	"github.com/prometheus/client_golang/prometheus":          "prometheus",
	"github.com/prometheus/client_golang/prometheus/promhttp": "promhttp",
	"github.com/romshark/datapages":                           "datapages",
	"github.com/romshark/datapages/modules/csrf":              "csrf",
	"github.com/romshark/datapages/modules/messaging":         "messaging",
	"github.com/romshark/datapages/modules/sessions":          "sessions",
	"github.com/romshark/datapages/runtime/actionexpr":        "actionexpr",
	"github.com/romshark/datapages/runtime/auth":              "auth",
	"github.com/romshark/datapages/runtime/htmlattr":          "htmlattr",
	"github.com/romshark/datapages/runtime/httpread":          "httpread",
	"github.com/romshark/datapages/runtime/httpserve":         "httpserve",
	"github.com/romshark/datapages/runtime/prom":              "prom",
	"github.com/romshark/datapages/runtime/sse":               "dpsse",
	"github.com/romshark/datapages/runtime/stream":            "stream",
	"github.com/romshark/datapages/runtime/subject":           "subject",
	"github.com/starfederation/datastar-go/datastar":          "datastar",
	"golang.org/x/sync/errgroup":                              "errgroup",
	"io":                                                      "io",
	"log/slog":                                                "slog",
	"net":                                                     "net",
	"net/http":                                                "http",
	"os":                                                      "os",
	"slices":                                                  "slices",
	"strconv":                                                 "strconv",
	"strings":                                                 "strings",
	"sync":                                                    "sync",
	"sync/atomic":                                             "atomic",
	"time":                                                    "time",
}

// appGenSubpkgIdents are the identifiers the generated subpackages bind.
// Their paths follow the generated package rather than being fixed, which keeps
// them out of [appFixedImports]. A model type cannot come from either,
// hence only the names have to be kept out of the way of.
var appGenSubpkgIdents = []string{"assets", "href"}

// genImport is a package a generated file imports for a type the model names,
// under the identifier that file qualifies it by.
type genImport struct {
	Path  string
	Ident string
	// Aliased reports whether Ident differs from the name the package
	// declares, which is when the import has to spell it out.
	Aliased bool
}

// genImports decides what every package a generated file names is called in it.
// A package keeps the name it declares while that name is free,
// and takes a "dp" prefix once a fixed import or an earlier package holds it.
type genImports struct {
	byPath map[string]string
	list   []genImport
}

// Qualifier names a package the way the generated file imports it.
// Every renderer of a model type qualifies by this.
func (g genImports) Qualifier() func(*types.Package) string {
	return func(p *types.Package) string {
		if ident, ok := g.byPath[p.Path()]; ok {
			return ident
		}
		// A package no entry type reached is one the file does not name.
		// The name it declares is the honest answer.
		return p.Name()
	}
}

// Extra returns the imports the model brings along, ordered by path.
// The fixed block is written whatever the model is; these are not.
func (g genImports) Extra() []genImport { return g.list }

// newGenImports names every package in pkgs, keeping out of the way of taken
// and of the fixed identifiers a caller seeded it with. fixed maps a path the
// file imports itself to the identifier it already uses, which is how the app
// package keeps [appPkgQual].
//
// Naming runs over the paths in order, which keeps the result independent of
// the order the writers emit types in.
func newGenImports(
	pkgs []*types.Package, taken map[string]bool, fixed map[string]string,
) genImports {
	g := genImports{byPath: maps.Clone(fixed)}
	if g.byPath == nil {
		g.byPath = map[string]string{}
	}

	byPath := map[string]*types.Package{}
	for _, p := range pkgs {
		if _, ok := fixed[p.Path()]; ok {
			continue
		}
		byPath[p.Path()] = p
	}

	for _, path := range slices.Sorted(maps.Keys(byPath)) {
		name := byPath[path].Name()
		ident := name
		if taken[ident] {
			base := "dp" + upperFirst(name)
			ident = base
			for n := 2; taken[ident]; n++ {
				ident = base + strconv.Itoa(n)
			}
		}
		taken[ident] = true
		g.byPath[path] = ident
		g.list = append(g.list, genImport{
			Path:    path,
			Ident:   ident,
			Aliased: ident != name,
		})
	}
	return g
}

// newAppImports builds the table app_gen.go renders every model type with.
func newAppImports(m *model.App) genImports {
	fixed := maps.Clone(appFixedImports)
	// After the clone: the app package keeps its alias even if it sits at a
	// path the block already imports.
	fixed[m.PkgPath] = appPkgQual
	return newGenImports(collectModelPkgs(m), appTakenIdents(), fixed)
}

// appTakenIdents is every identifier app_gen.go binds before the model brings
// a package of its own along.
func appTakenIdents() map[string]bool {
	taken := make(map[string]bool,
		len(appFixedImports)+len(appGenSubpkgIdents)+1)
	for _, id := range appFixedImports {
		taken[id] = true
	}
	for _, id := range appGenSubpkgIdents {
		taken[id] = true
	}
	taken[appPkgQual] = true
	return taken
}

// collectModelPkgs returns every package app_gen.go can name a type from.
// The entry types are the ones its writers render: the session data type,
// and the path, query and signals type of every handler.
//
// A named entry type is rendered by its own name while the fields of its
// underlying struct are rendered one by one by the value parsers, hence both
// the type and its underlying are walked. A package that turns out unused
// costs an import goimports drops.
func collectModelPkgs(m *model.App) []*types.Package {
	var out []*types.Package
	seen := map[*types.Package]bool{}
	add := func(t types.Type) {
		if t == nil {
			return
		}
		collectTypePkgs(t, seen, &out)
		collectTypePkgs(t.Underlying(), seen, &out)
	}
	addHandler := func(h *model.Handler) {
		if h == nil {
			return
		}
		for _, in := range []*model.Input{
			h.InputPath, h.InputQuery, h.InputSignals,
		} {
			if in != nil {
				add(in.Type.Resolved)
			}
		}
	}

	if m.Session != nil {
		add(m.Session.Data.Resolved)
	}
	// An event of another application is named by its own package.
	for _, e := range m.Events {
		collectTypePkgs(e.Type, seen, &out)
	}
	for _, h := range m.Actions {
		addHandler(h)
	}
	for _, p := range m.Pages {
		if p.GET != nil {
			addHandler(p.GET.Handler)
		}
		addHandler(p.StreamOpen)
		addHandler(p.StreamClose)
		for _, h := range p.Actions {
			addHandler(h)
		}
	}
	for _, p := range []*model.Page{m.PageError404, m.PageError500} {
		if p != nil && p.GET != nil {
			addHandler(p.GET.Handler)
		}
	}
	return out
}

// collectTypePkgs walks t the way types.TypeString prints it and records the
// package of every named type on the way. It does not descend into a named
// type's underlying: TypeString writes the name, not the definition.
func collectTypePkgs(
	t types.Type, seen map[*types.Package]bool, out *[]*types.Package,
) {
	switch t := t.(type) {
	case *types.Named:
		if p := t.Obj().Pkg(); p != nil && !seen[p] {
			seen[p] = true
			*out = append(*out, p)
		}
		if args := t.TypeArgs(); args != nil {
			for i := range args.Len() {
				collectTypePkgs(args.At(i), seen, out)
			}
		}
	case *types.Alias:
		collectTypePkgs(types.Unalias(t), seen, out)
	case *types.Pointer:
		collectTypePkgs(t.Elem(), seen, out)
	case *types.Slice:
		collectTypePkgs(t.Elem(), seen, out)
	case *types.Array:
		collectTypePkgs(t.Elem(), seen, out)
	case *types.Chan:
		collectTypePkgs(t.Elem(), seen, out)
	case *types.Map:
		collectTypePkgs(t.Key(), seen, out)
		collectTypePkgs(t.Elem(), seen, out)
	case *types.Struct:
		// The signals and query writers render field types one by one.
		for i := range t.NumFields() {
			collectTypePkgs(t.Field(i).Type(), seen, out)
		}
	}
}

// writeExtraImports writes an import line per package the model brings along,
// indented for the block [Writer.writeAppHeader] opened.
func (w *Writer) writeExtraImports(imports []genImport) {
	if len(imports) == 0 {
		return
	}
	w.Line(0, "")
	for _, imp := range imports {
		w.Byte('\t')
		if imp.Aliased {
			w.Raw(imp.Ident)
			w.Byte(' ')
		}
		w.writeQuoted(imp.Path)
		w.Byte('\n')
	}
}

// collectSessionDataPkgs returns every package the rendered session data type names.
// It is what a generated main.go has to import beyond the app package,
// since the session manager is instantiated with that type.
func collectSessionDataPkgs(m *model.App) []*types.Package {
	if m == nil || m.Session == nil {
		return nil
	}
	var out []*types.Package
	collectTypePkgs(m.Session.Data.Resolved, map[*types.Package]bool{}, &out)
	return out
}
