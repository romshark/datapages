package paramvalidation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"datapages/parser/model"

	"github.com/stretchr/testify/require"
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
	_, err = (&types.Config{}).Check(
		"test", fset, []*ast.File{f}, info,
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

func field(name string) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{{Name: name}},
	}
}

func TestIsSessionTokenParam(t *testing.T) {
	t.Parallel()
	require.True(t, IsSessionTokenParam(field("sessionToken")))
	require.False(t, IsSessionTokenParam(field("path")))
	require.False(t, IsSessionTokenParam(&ast.Field{}))
}

func TestIsSessionParam(t *testing.T) {
	t.Parallel()
	require.True(t, IsSessionParam(field("session")))
	require.False(t, IsSessionParam(field("path")))
	require.False(t, IsSessionParam(&ast.Field{}))
}

func TestIsDispatchParam(t *testing.T) {
	t.Parallel()
	require.True(t, IsDispatchParam(field("dispatch")))
	require.False(t, IsDispatchParam(field("path")))
	require.False(t, IsDispatchParam(&ast.Field{}))
}

func TestIsPathParam(t *testing.T) {
	t.Parallel()
	require.True(t, IsPathParam(field("path")))
	require.False(t, IsPathParam(field("query")))
	require.False(t, IsPathParam(&ast.Field{}))
}

func TestIsQueryParam(t *testing.T) {
	t.Parallel()
	require.True(t, IsQueryParam(field("query")))
	require.False(t, IsQueryParam(field("path")))
	require.False(t, IsQueryParam(&ast.Field{}))
}

func TestIsSignalsParam(t *testing.T) {
	t.Parallel()
	require.True(t, IsSignalsParam(field("signals")))
	require.False(t, IsSignalsParam(field("path")))
	require.False(t, IsSignalsParam(&ast.Field{}))
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
		"field not string": {
			src: `package test
func f(path struct {
	ID int ` + "`" + `path:"id"` + "`" + `
}) {}`,
			wantErr: ErrPathFieldNotString,
		},
		"missing tag": {
			src: `package test
func f(path struct {
	ID string
}) {}`,
			wantErr: ErrPathFieldMissingTag,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, info := typeCheckSrc(t, tt.src)
			p := firstFuncParam(t, f, 0)
			err := ValidatePathStruct(
				p, info, "Recv", "Method",
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
			f, info, "Recv", "Method",
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
		"empty struct": {
			src: `package test
func f(query struct{}) {}`,
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
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, info := typeCheckSrc(t, tt.src)
			p := firstFuncParam(t, f, 0)
			err := ValidateQueryStruct(
				p, info, "Recv", "Method",
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
			f, info, "Recv", "Method",
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
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, info := typeCheckSrc(t, tt.src)
			p := firstFuncParam(t, f, 0)
			err := ValidateSignalsStruct(
				p, info, "Recv", "Method",
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
			f, info, "Recv", "Method",
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

func TestValidateDispatchFunc(t *testing.T) {
	t.Parallel()
	eventTypes := map[string]struct{}{
		"EventFoo": {},
		"EventBar": {},
	}

	tests := map[string]struct {
		src        string
		wantErr    error
		wantEvents []string
	}{
		"valid single event": {
			src: `package test
type EventFoo struct{}
func f(dispatch func(EventFoo) error) {}`,
			wantEvents: []string{"EventFoo"},
		},
		"valid multiple events": {
			src: `package test
type EventFoo struct{}
type EventBar struct{}
func f(dispatch func(EventFoo, EventBar) error) {}`,
			wantEvents: []string{
				"EventFoo", "EventBar",
			},
		},
		"valid pointer event": {
			src: `package test
type EventFoo struct{}
func f(dispatch func(*EventFoo) error) {}`,
			wantEvents: []string{"EventFoo"},
		},
		"grouped event names": {
			src: `package test
type EventFoo struct{}
func f(dispatch func(a, b EventFoo) error) {}`,
			wantEvents: []string{
				"EventFoo", "EventFoo",
			},
		},
		"not a func": {
			src: `package test
func f(dispatch string) {}`,
			wantErr: ErrDispatchParamNotFunc,
		},
		"no return": {
			src: `package test
type EventFoo struct{}
func f(dispatch func(EventFoo)) {}`,
			wantErr: ErrDispatchReturnCount,
		},
		"two returns": {
			src: `package test
type EventFoo struct{}
func f(dispatch func(EventFoo) (int, error)) {}`,
			wantErr: ErrDispatchReturnCount,
		},
		"named multiple returns": {
			src: `package test
type EventFoo struct{}
func f(dispatch func(EventFoo) (x, y error)) {}`,
			wantErr: ErrDispatchReturnCount,
		},
		"return not error": {
			src: `package test
type EventFoo struct{}
func f(dispatch func(EventFoo) int) {}`,
			wantErr: ErrDispatchMustReturnError,
		},
		"no params": {
			src: `package test
func f(dispatch func() error) {}`,
			wantErr: ErrDispatchNoParams,
		},
		"param not event type": {
			src: `package test
func f(dispatch func(int) error) {}`,
			wantErr: ErrDispatchParamNotEvent,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, info := typeCheckSrc(t, tt.src)
			p := firstFuncParam(t, f, 0)
			events, err := ValidateDispatchFunc(
				p, info, eventTypes,
				"Recv", "Method",
			)
			if tt.wantErr == nil {
				require.NoError(t, err)
				require.Equal(
					t, tt.wantEvents, events,
				)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}
