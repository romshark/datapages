package parser_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/parser/model"
)

// TestParse_LoadCache compares cached and uncached output for every test app.
func TestParse_LoadCache(t *testing.T) {
	if testing.Short() {
		t.Skip("loads every fixture 3 times")
	}
	t.Setenv("DATAPAGES_LOADCACHE", t.TempDir())
	var hits, retries, total int
	for _, dir := range loadCacheDirs(t) {
		total++
		rel, _ := filepath.Rel(repoRoot(t), dir)
		t.Run(rel, func(t *testing.T) {
			wantApp, wantErrs, _, _ := parser.ParseLoad(dir, false)
			want := dumpParse(wantApp, wantErrs)
			for _, run := range []string{"fill", "read"} {
				app, errs, hit, retried := parser.ParseLoad(dir, true)
				require.Equal(t, want, dumpParse(app, errs), run)
				if run == "read" && hit {
					hits++
				}
				if retried {
					retries++
				}
			}
		})
	}
	t.Logf("%d of %d packages read from the cache, %d outputs made again from source",
		hits, total, retries)
}

// loadCacheDirs returns every fixture, example and acceptance app package.
func loadCacheDirs(t *testing.T) []string {
	root := repoRoot(t)
	var dirs []string
	fixtures, err := filepath.Glob(
		filepath.Join(root, "internal", "parser", "testdata", "*", "go.mod"),
	)
	require.NoError(t, err)
	for _, f := range fixtures {
		dirs = append(dirs, filepath.Dir(f))
	}
	for _, top := range []string{"example", filepath.Join("internal", "acceptance")} {
		err := filepath.WalkDir(
			filepath.Join(root, top), func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() && d.Name() == "datapagesgen" {
					dirs = append(dirs, filepath.Dir(path))
					return filepath.SkipDir
				}
				if d.IsDir() && (d.Name() == "node_modules" ||
					strings.HasPrefix(d.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			},
		)
		require.NoError(t, err)
	}
	slices.Sort(dirs)
	return dirs
}

func repoRoot(t *testing.T) string {
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "..", "..")
}

// dumpParse renders parser output with stable types and source positions.
func dumpParse(app *model.App, errs parser.Errors) string {
	var b strings.Builder
	for i := range errs.Len() {
		pos, err := errs.Entry(i)
		fmt.Fprintf(&b, "error %s: %v\n", pos, err)
	}
	if app == nil {
		b.WriteString("model nil\n")
		return b.String()
	}
	d := modelDumper{fset: app.Fset, seen: map[uintptr]int{}}
	d.value(&b, reflect.ValueOf(app))
	return b.String()
}

type modelDumper struct {
	fset *token.FileSet
	seen map[uintptr]int
}

var (
	typeType = reflect.TypeFor[types.Type]()
	nodeType = reflect.TypeFor[ast.Node]()
)

func (d *modelDumper) value(b *strings.Builder, v reflect.Value) {
	if !v.IsValid() {
		b.WriteString("invalid")
		return
	}
	if (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer ||
		v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil() {
		b.WriteString("nil")
		return
	}
	switch x := v.Interface().(type) {
	case *token.FileSet:
		b.WriteString("fset")
		return
	case token.Pos:
		fmt.Fprintf(b, "pos(%s)", d.fset.Position(x))
		return
	}
	if v.Type().Implements(typeType) {
		b.WriteString(types.TypeString(v.Interface().(types.Type), nil))
		return
	}
	if v.Type().Implements(nodeType) {
		n := v.Interface().(ast.Node)
		fmt.Fprintf(b, "%T@%s", n, d.fset.Position(n.Pos()))
		if e, ok := n.(ast.Expr); ok {
			fmt.Fprintf(b, "(%s)", types.ExprString(e))
		}
		return
	}
	// A go/types object, which holds unexported fields.
	t := v.Type()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.PkgPath() == "go/types" {
		fmt.Fprintf(b, "%v", v.Interface())
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if id, ok := d.seen[v.Pointer()]; ok {
			fmt.Fprintf(b, "ref#%d", id)
			return
		}
		d.seen[v.Pointer()] = len(d.seen)
		fmt.Fprintf(b, "#%d&", d.seen[v.Pointer()])
		d.value(b, v.Elem())
	case reflect.Interface:
		d.value(b, v.Elem())
	case reflect.Struct:
		fmt.Fprintf(b, "%s{", v.Type())
		for i := range v.NumField() {
			fmt.Fprintf(b, "%s:", v.Type().Field(i).Name)
			d.value(b, v.Field(i))
			b.WriteString(" ")
		}
		b.WriteString("}")
	case reflect.Slice, reflect.Array:
		b.WriteString("[")
		for i := range v.Len() {
			d.value(b, v.Index(i))
			b.WriteString(",")
		}
		b.WriteString("]")
	case reflect.Map:
		keys := v.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int {
			return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
		})
		b.WriteString("map[")
		for _, k := range keys {
			fmt.Fprintf(b, "%v:", k)
			d.value(b, v.MapIndex(k))
			b.WriteString(",")
		}
		b.WriteString("]")
	default:
		fmt.Fprintf(b, "%#v", v.Interface())
	}
}

// TestParse_LoadCacheColumns tests source retries for positions without columns.
func TestParse_LoadCacheColumns(t *testing.T) {
	t.Setenv("DATAPAGES_LOADCACHE", t.TempDir())
	for name, page := range map[string]string{
		// QueryFieldMissingTagError at the first field of http.Cookie.
		"error at a field in GOROOT": `
func (PageIndex) GET(
	r *http.Request, query datapages.Query[http.Cookie],
) (body datapages.Component, err error) {
	return body, err
}`,
		// go/types quotes the type parameter of slices.Clone.
		"type error quoting GOROOT": `
func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	_ = slices.Clone(nil)
	return body, err
}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeAppModule(t, `package app

import (
	"net/http"
	"slices"

	"github.com/romshark/datapages"
)

var _ = slices.Index[[]int]

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }
`+page)
			wantApp, wantErrs, _, _ := parser.ParseLoad(dir, false)
			want := dumpParse(wantApp, wantErrs)
			require.NotZero(t, wantErrs.Len())

			_, _, hit, _ := parser.ParseLoad(dir, true)
			require.False(t, hit)

			app, errs, hit, retried := parser.ParseLoad(dir, true)
			require.Equal(t, want, dumpParse(app, errs))
			require.True(t, hit)
			require.True(t, retried)

			app, errs, hit, retried = parser.ParseLoad(dir, true)
			require.Equal(t, want, dumpParse(app, errs))
			require.True(t, hit)
			require.False(t, retried)
		})
	}
}

// writeAppModule writes a fixture module containing src.
func writeAppModule(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	writeModuleFiles(t, dir, src)
	return dir
}

// writeModuleFiles writes a fixture module to dir.
func writeModuleFiles(t *testing.T, dir, src string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	fixture := filepath.Join("testdata", "minimal")
	gomod, err := os.ReadFile(filepath.Join(fixture, "go.mod"))
	require.NoError(t, err)
	gomod = []byte(strings.Replace(string(gomod),
		"=> ../../../..", "=> "+repoRoot(t), 1))
	gosum, err := os.ReadFile(filepath.Join(fixture, "go.sum"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), gomod, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), gosum, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.go"), []byte(src), 0o644))
}

// TestParse_LoadCacheStamp tests cache invalidation against uncached output.
func TestParse_LoadCacheStamp(t *testing.T) {
	const appGo = `package app

import (
	"net/http"

	"github.com/romshark/datapages"

	"datapagestest/fixture/minimal/lib"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	_ = lib.F()
	return body, err
}
`
	write := func(t *testing.T, path, src string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	}
	appendTo := func(t *testing.T, path, src string) {
		t.Helper()
		old, err := os.ReadFile(path)
		require.NoError(t, err)
		write(t, path, string(old)+src)
	}
	for name, tc := range map[string]struct {
		edit func(t *testing.T, dir string)
		hit  bool
	}{
		"nothing": {
			edit: func(t *testing.T, dir string) {},
			hit:  true,
		},
		"declaration below the imports": {
			edit: func(t *testing.T, dir string) {
				appendTo(t, filepath.Join(dir, "app.go"), "\nfunc f() int { return 1 }\n")
			},
			hit: true,
		},
		"type error below the imports": {
			edit: func(t *testing.T, dir string) {
				appendTo(t, filepath.Join(dir, "app.go"), "\nvar _ int = \"\"\n")
			},
			hit: true,
		},
		"function body of an imported package of the module": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "lib", "lib.go"),
					"package lib\n\nfunc F() string { return \"\" }\n")
			},
			hit: true,
		},
		"file other than Go": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "notes.md"), "notes\n")
			},
			hit: true,
		},
		"import": {
			edit: func(t *testing.T, dir string) {
				src, err := os.ReadFile(filepath.Join(dir, "app.go"))
				require.NoError(t, err)
				write(t, filepath.Join(dir, "app.go"), strings.Replace(string(src),
					`"net/http"`, `"net/http"`+"\n\t_ \"strings\"", 1))
			},
		},
		"build constraint": {
			edit: func(t *testing.T, dir string) {
				src, err := os.ReadFile(filepath.Join(dir, "app.go"))
				require.NoError(t, err)
				write(t, filepath.Join(dir, "app.go"), "//go:build ignore\n\n"+string(src))
			},
		},
		"new Go file": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "b.go"), "package app\n")
			},
		},
		"Go file of another package name": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "b.go"), "package other\n")
			},
		},
		"imports of an imported package of the module": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "lib", "lib.go"),
					"package lib\n\nimport \"strings\"\n\n"+
						"func F() string { return strings.TrimSpace(\"\") }\n")
			},
		},
		"removed package of the module": {
			edit: func(t *testing.T, dir string) {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, "lib")))
			},
		},
		"go.mod": {
			edit: func(t *testing.T, dir string) {
				appendTo(t,
					filepath.Join(dir, "go.mod"),
					"\nreplace example.com/unused => ./unused\n")
			},
		},
		"go.work above the module": {
			edit: func(t *testing.T, dir string) {
				write(t,
					filepath.Join(filepath.Dir(dir), "go.work"),
					"go 1.27.3\n\nuse ./"+filepath.Base(dir)+"\n")
			},
		},
		"environment": {
			edit: func(t *testing.T, dir string) {
				t.Setenv("GOFLAGS", "-tags=loadcache")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATAPAGES_LOADCACHE", t.TempDir())
			dir := filepath.Join(t.TempDir(), "module")
			writeModuleFiles(t, dir, appGo)
			write(t, filepath.Join(dir, "lib", "lib.go"), "package lib\n\nfunc F() int { return 1 }\n")

			_, _, hit, _ := parser.ParseLoad(dir, true)
			require.False(t, hit)
			tc.edit(t, dir)
			wantApp, wantErrs, _, _ := parser.ParseLoad(dir, false)
			app, errs, hit, _ := parser.ParseLoad(dir, true)
			require.Equal(t, tc.hit, hit)
			require.Equal(t, dumpParse(wantApp, wantErrs), dumpParse(app, errs))
		})
	}
}

// TestParse_LoadCacheStampEmbed tests //go:embed cache invalidation.
func TestParse_LoadCacheStampEmbed(t *testing.T) {
	const appGo = `package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}
`
	const assetsGo = `package app

import "embed"

// AssetsFS is /static/
//
//go:embed static/*
var AssetsFS embed.FS
`
	write := func(t *testing.T, path, src string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	}
	for name, tc := range map[string]struct {
		edit func(t *testing.T, dir string)
		hit  bool
	}{
		"content of an embedded file": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "static", "style.css"), "p{color:blue}\n")
			},
			hit: true,
		},
		"content of a file in a subdirectory": {
			edit: func(t *testing.T, dir string) {
				write(t,
					filepath.Join(dir, "sub", "sub.go"),
					"package sub\n\nvar X = 2\n")
			},
			hit: true,
		},
		"new file in a subdirectory": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "sub", "b.go"), "package sub\n")
			},
		},
		"new embedded file": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "static", "app.js"), "\n")
			},
		},
		"removed embedded directory": {
			edit: func(t *testing.T, dir string) {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, "static")))
			},
		},
		"edit in the file importing embed": {
			edit: func(t *testing.T, dir string) {
				write(t, filepath.Join(dir, "assets.go"), assetsGo+"\nvar x = 1\n")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATAPAGES_LOADCACHE", t.TempDir())
			dir := filepath.Join(t.TempDir(), "module")
			writeModuleFiles(t, dir, appGo)
			write(t, filepath.Join(dir, "assets.go"), assetsGo)
			write(t, filepath.Join(dir, "static", "style.css"), "p{color:red}\n")
			write(t, filepath.Join(dir, "sub", "sub.go"), "package sub\n\nvar X = 1\n")

			_, _, hit, _ := parser.ParseLoad(dir, true)
			require.False(t, hit)
			tc.edit(t, dir)
			wantApp, wantErrs, _, _ := parser.ParseLoad(dir, false)
			app, errs, hit, _ := parser.ParseLoad(dir, true)
			require.Equal(t, tc.hit, hit)
			require.Equal(t, dumpParse(wantApp, wantErrs), dumpParse(app, errs))
		})
	}
}
