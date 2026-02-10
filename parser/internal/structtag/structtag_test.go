package structtag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"datapages/parser/model"

	"github.com/stretchr/testify/require"
)

func TestJSONTagValue(t *testing.T) {
	tests := map[string]struct {
		tag  string
		want string
	}{
		"simple":         {`json:"name"`, "name"},
		"omitempty":      {`json:"name,omitempty"`, "name"},
		"comma only":     {`json:","`, ""},
		"empty value":    {`json:""`, ""},
		"wrong prefix":   {`query:"x"`, ""},
		"empty string":   {"", ""},
		"unclosed quote": {`json:"name`, ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(
				t, tt.want, JSONTagValue(tt.tag),
			)
		})
	}
}

func TestReflectSignalTagValue(t *testing.T) {
	tests := map[string]struct {
		tag  string
		want string
	}{
		"simple":         {`reflectsignal:"count"`, "count"},
		"multi-tag":      {`query:"x" reflectsignal:"count"`, "count"},
		"wrong prefix":   {`query:"x"`, ""},
		"empty string":   {"", ""},
		"unclosed quote": {`reflectsignal:"count`, ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(
				t, tt.want,
				ReflectSignalTagValue(tt.tag),
			)
		})
	}
}

func TestPathTagValue(t *testing.T) {
	tests := map[string]struct {
		tag  string
		want string
	}{
		"simple":         {`path:"id"`, "id"},
		"multi-tag":      {`json:"x" path:"slug"`, "slug"},
		"wrong prefix":   {`json:"x"`, ""},
		"empty string":   {"", ""},
		"unclosed quote": {`path:"id`, ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(
				t, tt.want, PathTagValue(tt.tag),
			)
		})
	}
}

// namedType parses src, type-checks it, and returns the
// types.Type for the named type n.
func namedType(t *testing.T, src, n string) types.Type {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	pkg, err := (&types.Config{}).Check(
		"test", fset, []*ast.File{f}, info,
	)
	require.NoError(t, err)
	obj := pkg.Scope().Lookup(n)
	require.NotNil(t, obj)
	return obj.Type()
}

func TestValidateReflectSignal(t *testing.T) {
	sigSrc := `package test
type S struct {
	Count int ` + "`" + `json:"count"` + "`" + `
	Name  string ` + "`" + `json:"name"` + "`" + `
}`

	tests := map[string]struct {
		handler *model.Handler
		wantErr error
	}{
		"nil query": {
			handler: &model.Handler{},
		},
		"nil signals": {
			handler: &model.Handler{
				InputQuery: &model.Input{
					Type: model.Type{
						Resolved: types.Typ[types.Int],
					},
				},
			},
		},
		"query not struct": {
			handler: &model.Handler{
				InputQuery: &model.Input{
					Type: model.Type{
						Resolved: types.Typ[types.Int],
					},
				},
				InputSignals: &model.Input{
					Type: model.Type{
						Resolved: types.Typ[types.Int],
					},
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateReflectSignal(
				tt.handler, "Recv", "Method",
			)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}

	sigType := namedType(t, sigSrc, "S")

	t.Run("signals not struct", func(t *testing.T) {
		h := &model.Handler{
			InputQuery: &model.Input{
				Type: model.Type{Resolved: sigType},
			},
			InputSignals: &model.Input{
				Type: model.Type{
					Resolved: types.Typ[types.Int],
				},
			},
		}
		require.NoError(
			t,
			ValidateReflectSignal(h, "Recv", "Method"),
		)
	})

	t.Run("matching reflectsignal", func(t *testing.T) {
		querySrc := `package test
type Q struct {
	Search string ` + "`" +
			`query:"search" reflectsignal:"count"` +
			"`" + `
}`
		h := &model.Handler{
			InputQuery: &model.Input{
				Type: model.Type{
					Resolved: namedType(
						t, querySrc, "Q",
					),
				},
			},
			InputSignals: &model.Input{
				Type: model.Type{Resolved: sigType},
			},
		}
		require.NoError(
			t,
			ValidateReflectSignal(h, "Recv", "Method"),
		)
	})

	t.Run("no reflectsignal tags", func(t *testing.T) {
		querySrc := `package test
type Q struct {
	Search string ` + "`" + `query:"search"` + "`" + `
}`
		h := &model.Handler{
			InputQuery: &model.Input{
				Type: model.Type{
					Resolved: namedType(
						t, querySrc, "Q",
					),
				},
			},
			InputSignals: &model.Input{
				Type: model.Type{Resolved: sigType},
			},
		}
		require.NoError(
			t,
			ValidateReflectSignal(h, "Recv", "Method"),
		)
	})

	t.Run("reflectsignal not in signals", func(t *testing.T) {
		querySrc := `package test
type Q struct {
	Search string ` + "`" +
			`query:"search" reflectsignal:"missing"` +
			"`" + `
}`
		h := &model.Handler{
			InputQuery: &model.Input{
				Type: model.Type{
					Resolved: namedType(
						t, querySrc, "Q",
					),
				},
			},
			InputSignals: &model.Input{
				Type: model.Type{Resolved: sigType},
			},
		}
		err := ValidateReflectSignal(
			h, "Recv", "Method",
		)
		require.ErrorIs(
			t, err,
			ErrQueryReflectSignalNotInSignals,
		)
	})
}
