package paramvalidation

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/parser/model"
)

// typeCheckSrc parses and type-checks Go source, returning
// the AST file and type information.
func typeCheckSrc(t *testing.T, src string) (*ast.File, *types.Info) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	pkgPath := "test"
	if f.Name.Name == "datapages" {
		pkgPath = "github.com/romshark/datapages"
	}
	_, err = (&types.Config{}).Check(
		pkgPath, fset, []*ast.File{f}, info,
	)
	require.NoError(t, err)
	return f, info
}

// firstFuncParam returns the i-th parameter field from the
// first function declaration.
func firstFuncParam(t *testing.T, f *ast.File, i int) *ast.Field {
	t.Helper()

	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		return fd.Type.Params.List[i]
	}
	t.Fatal("no function declaration found")
	return nil
}

// namedType parses src, type-checks it, and returns the
// types.Type for the type named "P".
func namedType(t *testing.T, src string) types.Type {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	pkg, err := (&types.Config{}).Check("test", fset, []*ast.File{f}, info)
	require.NoError(t, err)
	obj := pkg.Scope().Lookup("P")
	require.NotNil(t, obj)
	return obj.Type()
}

// fakeStructInfo returns an *ast.Field whose Type is
// an *ast.StructType, paired with a types.Info that maps
// that expression to a non-struct type. This triggers the
// defensive second Underlying() check.
func fakeStructInfo() (*ast.Field, *types.Info) {
	st := &ast.StructType{
		Fields: &ast.FieldList{},
	}
	f := &ast.Field{
		Names: []*ast.Ident{{Name: "x"}},
		Type:  st,
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{
			st: {Type: types.Typ[types.Int]},
		},
	}
	return f, info
}

// wrapperSrc declares the datapages input types and a function taking one
// parameter of each, plus one that is none of them. It is type-checked under
// the import path of the datapages package, which is what the predicates match.
const wrapperSrc = `package datapages

type Path[Values any] struct{ Values Values }
type Query[Values any] struct{ Values Values }
type Signals[Values any] struct{ Values Values }
type Session[Data any] struct{ data Data }

func f(
	p Path[struct{}],
	q Query[struct{}],
	s Signals[struct{}],
	x int,
	sess Session[struct{}],
) {}`

func TestIsSessionParam(t *testing.T) {
	t.Parallel()
	f, info := typeCheckSrc(t, wrapperSrc)
	require.True(t, IsSessionParam(firstFuncParam(t, f, 4), info))
	require.False(t, IsSessionParam(firstFuncParam(t, f, 3), info))
}

func TestIsPathParam(t *testing.T) {
	t.Parallel()
	f, info := typeCheckSrc(t, wrapperSrc)
	require.True(t, IsPathParam(firstFuncParam(t, f, 0), info))
	require.False(t, IsPathParam(firstFuncParam(t, f, 1), info))
	require.False(t, IsPathParam(firstFuncParam(t, f, 3), info))
}

func TestIsQueryParam(t *testing.T) {
	t.Parallel()
	f, info := typeCheckSrc(t, wrapperSrc)
	require.True(t, IsQueryParam(firstFuncParam(t, f, 1), info))
	require.False(t, IsQueryParam(firstFuncParam(t, f, 0), info))
	require.False(t, IsQueryParam(firstFuncParam(t, f, 3), info))
}

func TestIsSignalsParam(t *testing.T) {
	t.Parallel()
	f, info := typeCheckSrc(t, wrapperSrc)
	require.True(t, IsSignalsParam(firstFuncParam(t, f, 2), info))
	require.False(t, IsSignalsParam(firstFuncParam(t, f, 0), info))
	require.False(t, IsSignalsParam(firstFuncParam(t, f, 3), info))
}

func TestValidatePathStruct(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		src     string
		wantErr error
	}{
		"valid single field": {
			src: `package test
func f(path struct {
	ID string ` + "`" + `path:"id"` + "`" + `
}) {}`,
		},
		"valid multiple fields": {
			src: `package test
func f(path struct {
	Name string ` + "`" + `path:"name"` + "`" + `
	Slug string ` + "`" + `path:"slug"` + "`" + `
}) {}`,
		},
		"valid int field": {
			src: `package test
func f(path struct {
	ID int ` + "`" + `path:"id"` + "`" + `
}) {}`,
		},
		"valid int32 field": {
			src: `package test
func f(path struct {
	ID int32 ` + "`" + `path:"id"` + "`" + `
}) {}`,
		},
		"valid int64 field": {
			src: `package test
func f(path struct {
	ID int64 ` + "`" + `path:"id"` + "`" + `
}) {}`,
		},
		"valid uint field": {
			src: `package test
func f(path struct {
	ID uint ` + "`" + `path:"id"` + "`" + `
}) {}`,
		},
		"valid float64 field": {
			src: `package test
func f(path struct {
	Score float64 ` + "`" + `path:"score"` + "`" + `
}) {}`,
		},
		"valid bool field": {
			src: `package test
func f(path struct {
	Active bool ` + "`" + `path:"active"` + "`" + `
}) {}`,
		},
		"valid TextUnmarshaler pointer receiver": {
			src: `package test
type MyID struct{ V string }
func f(path struct {
	ID MyID ` + "`" + `path:"id"` + "`" + `
}) {}
func (m *MyID) UnmarshalText(text []byte) error {
	m.V = string(text)
	return nil
}`,
		},
		"valid TextUnmarshaler value receiver": {
			src: `package test
type MyID string
func f(path struct {
	ID MyID ` + "`" + `path:"id"` + "`" + `
}) {}
func (m MyID) UnmarshalText(text []byte) error {
	return nil
}`,
		},
		"empty struct": {
			src: `package test
func f(path struct{}) {}`,
		},
		"not a struct": {
			src: `package test
func f(path string) {}`,
			wantErr: ErrPathParamNotStruct,
		},
		"unexported field": {
			src: `package test
func f(path struct {
	id string ` + "`" + `path:"id"` + "`" + `
}) {}`,
			wantErr: ErrPathFieldUnexported,
		},
		"unsupported type": {
			src: `package test
func f(path struct {
	ID []byte ` + "`" + `path:"id"` + "`" + `
}) {}`,
			wantErr: ErrPathFieldUnsupportedType,
		},
		"missing tag": {
			src: `package test
func f(path struct {
	ID string
}) {}`,
			wantErr: ErrPathFieldMissingTag,
		},
		"empty tag": {
			src: `package test
func f(path struct {
	ID string ` + "`" + `path:""` + "`" + `
}) {}`,
			wantErr: ErrPathFieldEmptyTag,
		},
		"duplicate tag": {
			src: `package test
func f(path struct {
	ID    string ` + "`" + `path:"id"` + "`" + `
	Other string ` + "`" + `path:"id"` + "`" + `
}) {}`,
			wantErr: ErrPathFieldDuplicateTag,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, info := typeCheckSrc(t, tt.src)
			p := firstFuncParam(t, f, 0)
			err := ValidatePathStruct(
				p.Type, info, "Recv", "Method",
			)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}

	t.Run("resolved type not struct", func(t *testing.T) {
		t.Parallel()
		f, info := fakeStructInfo()
		err := ValidatePathStruct(
			f.Type, info, "Recv", "Method",
		)
		require.ErrorIs(t, err, ErrPathParamNotStruct)
	})
}

func TestValidateQueryStruct(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		src     string
		wantErr error
	}{
		"valid": {
			src: `package test
func f(query struct {
	Search string ` + "`" + `query:"search"` + "`" + `
}) {}`,
		},
		"valid int field": {
			src: `package test
func f(query struct {
	Limit int ` + "`" + `query:"l"` + "`" + `
}) {}`,
		},
		"valid int64 field": {
			src: `package test
func f(query struct {
	Offset int64 ` + "`" + `query:"o"` + "`" + `
}) {}`,
		},
		"valid uint field": {
			src: `package test
func f(query struct {
	Page uint ` + "`" + `query:"p"` + "`" + `
}) {}`,
		},
		"valid float64 field": {
			src: `package test
func f(query struct {
	Price float64 ` + "`" + `query:"price"` + "`" + `
}) {}`,
		},
		"valid bool field": {
			src: `package test
func f(query struct {
	Active bool ` + "`" + `query:"active"` + "`" + `
}) {}`,
		},
		"valid TextUnmarshaler": {
			src: `package test
type Filter struct{ V string }
func f(query struct {
	F Filter ` + "`" + `query:"f"` + "`" + `
}) {}
func (fl *Filter) UnmarshalText(text []byte) error {
	fl.V = string(text)
	return nil
}`,
		},
		"empty struct": {
			src: `package test
func f(query struct{}) {}`,
		},
		"valid named type": {
			src: `package test
type SearchParams struct {
	Term string ` + "`" + `query:"t"` + "`" + `
}
func f(query SearchParams) {}`,
		},
		"unsupported type": {
			src: `package test
func f(query struct {
	Data []byte ` + "`" + `query:"d"` + "`" + `
}) {}`,
			wantErr: ErrQueryFieldUnsupportedType,
		},
		"not a struct": {
			src: `package test
func f(query string) {}`,
			wantErr: ErrQueryParamNotStruct,
		},
		"unexported field": {
			src: `package test
func f(query struct {
	search string ` + "`" + `query:"search"` + "`" + `
}) {}`,
			wantErr: ErrQueryFieldUnexported,
		},
		"missing tag": {
			src: `package test
func f(query struct {
	Search string
}) {}`,
			wantErr: ErrQueryFieldMissingTag,
		},
		"empty tag": {
			src: `package test
func f(query struct {
	Search string ` + "`" + `query:""` + "`" + `
}) {}`,
			wantErr: ErrQueryFieldEmptyTag,
		},
		"duplicate tag": {
			src: `package test
func f(query struct {
	Term  string ` + "`" + `query:"q"` + "`" + `
	Other string ` + "`" + `query:"q"` + "`" + `
}) {}`,
			wantErr: ErrQueryFieldDuplicateTag,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, info := typeCheckSrc(t, tt.src)
			p := firstFuncParam(t, f, 0)
			err := ValidateQueryStruct(
				p.Type, info, "Recv", "Method",
			)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}

	t.Run("resolved type not struct", func(t *testing.T) {
		t.Parallel()
		f, info := fakeStructInfo()
		err := ValidateQueryStruct(
			f.Type, info, "Recv", "Method",
		)
		require.ErrorIs(t, err, ErrQueryParamNotStruct)
	})
}

func TestValidateSignalsStruct(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		src     string
		wantErr error
	}{
		"valid": {
			src: `package test
func f(signals struct {
	Count int ` + "`" + `json:"count"` + "`" + `
}) {}`,
		},
		"empty struct": {
			src: `package test
func f(signals struct{}) {}`,
		},
		"valid named type": {
			src: `package test
type Signals struct {
	Count int ` + "`" + `json:"count"` + "`" + `
}
func f(signals Signals) {}`,
		},
		"not a struct": {
			src: `package test
func f(signals string) {}`,
			wantErr: ErrSignalsParamNotStruct,
		},
		"unexported field": {
			src: `package test
func f(signals struct {
	count int ` + "`" + `json:"count"` + "`" + `
}) {}`,
			wantErr: ErrSignalsFieldUnexported,
		},
		"missing tag": {
			src: `package test
func f(signals struct {
	Count int
}) {}`,
			wantErr: ErrSignalsFieldMissingTag,
		},
		"empty tag": {
			src: `package test
func f(signals struct {
	Count int ` + "`" + `json:""` + "`" + `
}) {}`,
			wantErr: ErrSignalsFieldEmptyTag,
		},
		"duplicate tag": {
			src: `package test
func f(signals struct {
	Name  string ` + "`" + `json:"name"` + "`" + `
	Other string ` + "`" + `json:"name"` + "`" + `
}) {}`,
			wantErr: ErrSignalsFieldDuplicateTag,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, info := typeCheckSrc(t, tt.src)
			p := firstFuncParam(t, f, 0)
			err := ValidateSignalsStruct(
				p.Type, info, "Recv", "Method",
			)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}

	t.Run("resolved type not struct", func(t *testing.T) {
		t.Parallel()
		f, info := fakeStructInfo()
		err := ValidateSignalsStruct(
			f.Type, info, "Recv", "Method",
		)
		require.ErrorIs(
			t, err, ErrSignalsParamNotStruct,
		)
	})
}

func TestValidatePathAgainstRoute(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		route   string
		pathSrc string // empty means nil InputPath
		wantErr error
	}{
		"matching single var": {
			route: "/items/{id}",
			pathSrc: `package test
type P struct {
	ID string ` + "`" + `path:"id"` + "`" + `
}`,
		},
		"matching multiple vars": {
			route: "/users/{name}/posts/{slug}",
			pathSrc: `package test
type P struct {
	Name string ` + "`" + `path:"name"` + "`" + `
	Slug string ` + "`" + `path:"slug"` + "`" + `
}`,
		},
		"no vars no path": {
			route: "/items",
		},
		"vars but no path struct": {
			route:   "/items/{id}",
			wantErr: ErrPathMissingRouteVar,
		},
		"extra field not in route": {
			route: "/items/{id}",
			pathSrc: `package test
type P struct {
	ID   string ` + "`" + `path:"id"` + "`" + `
	Slug string ` + "`" + `path:"slug"` + "`" + `
}`,
			wantErr: ErrPathFieldNotInRoute,
		},
		"missing route var": {
			route: "/users/{name}/posts/{slug}",
			pathSrc: `package test
type P struct {
	Name string ` + "`" + `path:"name"` + "`" + `
}`,
			wantErr: ErrPathMissingRouteVar,
		},
		"field without path tag skipped": {
			route: "/items/{id}",
			pathSrc: `package test
type P struct {
	ID    string ` + "`" + `path:"id"` + "`" + `
	Extra string
}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := &model.Handler{Route: tt.route}
			if tt.pathSrc != "" {
				h.InputPath = &model.Input{
					Type: model.Type{
						Resolved: namedType(
							t, tt.pathSrc,
						),
					},
				}
			}
			err := ValidatePathAgainstRoute(
				h, "Recv", "Method",
			)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}

	t.Run("two extra fields both reported", func(t *testing.T) {
		t.Parallel()
		h := &model.Handler{
			Route: "/items/{id}",
			InputPath: &model.Input{
				Type: model.Type{
					Resolved: namedType(t, `package test
type P struct {
	Foo string `+"`"+`path:"foo"`+"`"+`
	Bar string `+"`"+`path:"bar"`+"`"+`
}`),
				},
			},
		}
		err := ValidatePathAgainstRoute(h, "Recv", "Method")
		require.ErrorIs(t, err, ErrPathFieldNotInRoute)
		require.Contains(t, err.Error(), `"foo"`)
		require.Contains(t, err.Error(), `"bar"`)
	})

	t.Run("two missing route vars both reported", func(t *testing.T) {
		t.Parallel()
		h := &model.Handler{Route: "/a/{id}/b/{slug}"}
		err := ValidatePathAgainstRoute(h, "Recv", "Method")
		require.ErrorIs(t, err, ErrPathMissingRouteVar)
		require.Contains(t, err.Error(), "{id}")
		require.Contains(t, err.Error(), "{slug}")
	})

	t.Run("resolved type not struct", func(t *testing.T) {
		t.Parallel()
		h := &model.Handler{Route: "/items"}
		h.InputPath = &model.Input{
			Type: model.Type{
				Resolved: types.Typ[types.Int],
			},
		}
		err := ValidatePathAgainstRoute(
			h, "Recv", "Method",
		)
		require.NoError(t, err)
	})
}

// typeCheckWithDatapages type-checks src against a stand-in for the datapages
// package, which is what the Dispatch parameter type resolves through.
func typeCheckWithDatapages(t *testing.T, src string) (*ast.File, *types.Info) {
	t.Helper()

	fset := token.NewFileSet()
	dpFile, err := parser.ParseFile(fset, "datapages.go", `package datapages

type Dispatcher[Event any] interface {
	Dispatch(event Event) error
	DispatchCtx(ctx any, event Event) error
}
`, 0)
	require.NoError(t, err)
	dpPkg, err := (&types.Config{}).Check(
		"github.com/romshark/datapages", fset, []*ast.File{dpFile}, nil,
	)
	require.NoError(t, err)

	f, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
	conf := &types.Config{Importer: importerFunc(func(path string) (*types.Package, error) {
		if path == "github.com/romshark/datapages" {
			return dpPkg, nil
		}
		return nil, fmt.Errorf("unexpected import %q", path)
	})}
	_, err = conf.Check("test", fset, []*ast.File{f}, info)
	require.NoError(t, err)
	return f, info
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

func TestIsDispatchParam(t *testing.T) {
	t.Parallel()
	src := `package test
import "github.com/romshark/datapages"
type EventFoo struct{}
func f(d datapages.Dispatcher[EventFoo], notDispatch string) {}`
	f, info := typeCheckWithDatapages(t, src)
	require.True(t, IsDispatchParam(firstFuncParam(t, f, 0), info))
	require.False(t, IsDispatchParam(firstFuncParam(t, f, 1), info))
}

func TestValidateDispatch(t *testing.T) {
	t.Parallel()
	eventTypes := map[string]struct{}{
		"EventFoo": {},
		"EventBar": {},
	}

	tests := map[string]struct {
		src       string
		wantErr   error
		wantEvent string
	}{
		"valid": {
			src: `package test
import "github.com/romshark/datapages"
type EventFoo struct{}
func f(d datapages.Dispatcher[EventFoo]) {}`,
			wantEvent: "EventFoo",
		},
		"valid through alias": {
			src: `package test
import "github.com/romshark/datapages"
type EventBar struct{}
type DispatchBar = datapages.Dispatcher[EventBar]
func f(d DispatchBar) {}`,
			wantEvent: "EventBar",
		},
		"type argument not an event type": {
			src: `package test
import "github.com/romshark/datapages"
func f(d datapages.Dispatcher[string]) {}`,
			wantErr: ErrDispatchParamNotEvent,
		},
		"type argument is an undeclared type": {
			src: `package test
import "github.com/romshark/datapages"
type NotAnEvent struct{}
func f(d datapages.Dispatcher[NotAnEvent]) {}`,
			wantErr: ErrDispatchParamNotEvent,
		},
		"not a dispatcher": {
			src: `package test
func f(d string) {}`,
			wantErr: ErrDispatchParamNotEvent,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var f *ast.File
			var info *types.Info
			if strings.Contains(tt.src, "romshark/datapages") {
				f, info = typeCheckWithDatapages(t, tt.src)
			} else {
				f, info = typeCheckSrc(t, tt.src)
			}
			p := firstFuncParam(t, f, 0)
			event, err := ValidateDispatch(
				p, info, eventTypes,
				"Recv", "Method",
			)
			if tt.wantErr == nil {
				require.NoError(t, err)
				require.Equal(t, tt.wantEvent, event)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidateReflectSignal(t *testing.T) {
	sigType := namedType(t, "package test\n"+
		"type P struct {\n"+
		"\tCount int `json:\"count\"`\n"+
		"\tName  string `json:\"name\"`\n"+
		"}")
	queryType := func(tag string) types.Type {
		return namedType(t, "package test\n"+
			"type P struct {\n"+
			"\tSearch string `"+tag+"`\n"+
			"}")
	}
	input := func(t types.Type) *model.Input {
		return &model.Input{Type: model.Type{Resolved: t}}
	}
	notAStruct := input(types.Typ[types.Int])

	for name, td := range map[string]struct {
		handler *model.Handler
		wantErr error
	}{
		"no query":         {handler: &model.Handler{}},
		"no signals":       {handler: &model.Handler{InputQuery: notAStruct}},
		"query not struct": {handler: &model.Handler{InputQuery: notAStruct, InputSignals: notAStruct}},
		"signals not struct": {handler: &model.Handler{
			InputQuery: input(sigType), InputSignals: notAStruct,
		}},
		"no reflectsignal tag": {handler: &model.Handler{
			InputQuery:   input(queryType(`query:"search"`)),
			InputSignals: input(sigType),
		}},
		"reflectsignal matches signal": {handler: &model.Handler{
			InputQuery:   input(queryType(`query:"search" reflectsignal:"count"`)),
			InputSignals: input(sigType),
		}},
		"reflectsignal not in signals": {
			handler: &model.Handler{
				InputQuery:   input(queryType(`query:"search" reflectsignal:"missing"`)),
				InputSignals: input(sigType),
			},
			wantErr: ErrQueryReflectSignalNotInSignals,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateReflectSignal(td.handler, "Recv", "Method")
			if td.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, td.wantErr)
		})
	}
}
