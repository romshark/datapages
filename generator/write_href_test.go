package generator_test

import (
	"flag"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser/model"
)

var update = flag.Bool("update", false, "update golden files")

func TestWritePkgHref(t *testing.T) {
	tests := map[string]struct {
		app    *model.App
		golden string
	}{
		"no pages": {
			app:    &model.App{},
			golden: "href_no_pages.go.txt",
		},
		"simple one-liner": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageFoo", "/foo/{$}", nil),
				},
			},
			golden: "href_simple_one_liner.go.txt",
		},
		"index page": {
			app: &model.App{
				Pages: []*model.Page{{
					TypeName:           "PageIndex",
					Route:              "/",
					PageSpecialization: model.PageTypeIndex,
					GET:                &model.HandlerGET{Handler: &model.Handler{}},
				}},
			},
			golden: "href_index_page.go.txt",
		},
		"error404 included": {
			app: &model.App{
				Pages: []*model.Page{{
					TypeName:           "PageError404",
					Route:              "/not-found/{$}",
					PageSpecialization: model.PageTypeError404,
					GET:                &model.HandlerGET{Handler: &model.Handler{}},
				}},
			},
			golden: "href_error404_included.go.txt",
		},
		"error500 skipped": {
			app: &model.App{
				Pages: []*model.Page{
					{
						TypeName:           "PageError500",
						Route:              "/error/",
						PageSpecialization: model.PageTypeError500,
						GET:                &model.HandlerGET{Handler: &model.Handler{}},
					},
					hrefPage("PageFoo", "/foo/{$}", nil),
				},
			},
			golden: "href_error500_skipped.go.txt",
		},
		"page without GET skipped": {
			app: &model.App{
				Pages: []*model.Page{
					{TypeName: "PageNoGET", Route: "/noget/"},
					hrefPage("PageFoo", "/foo/{$}", nil),
				},
			},
			golden: "href_page_without_get_skipped.go.txt",
		},
		"path variable": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PagePost", "/post/{slug}/{$}", nil),
				},
			},
			golden: "href_path_variable.go.txt",
		},
		"multiple path variables": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageItem", "/org/{org}/item/{id}/{$}", nil),
				},
			},
			golden: "href_multiple_path_variables.go.txt",
		},
		"query string fields only": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageMessages", "/messages/{$}", hrefStruct(
						hrefFieldDef{"Chat", types.Typ[types.String], `query:"chat"`},
					)),
				},
			},
			golden: "href_query_string_fields_only.go.txt",
		},
		"query with int fields": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageSearch", "/search/{$}", hrefStruct(
						hrefFieldDef{"Term", types.Typ[types.String], `query:"t"`},
						hrefFieldDef{"PriceMin", types.Typ[types.Int64], `query:"pmin"`},
					)),
				},
			},
			golden: "href_query_with_int_fields.go.txt",
		},
		"path and query combined": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageUserPost", "/user/{name}/post/{$}", hrefStruct(
						hrefFieldDef{"Sort", types.Typ[types.String], `query:"sort"`},
					)),
				},
			},
			golden: "href_path_and_query_combined.go.txt",
		},
		"path and query with int": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageCatalog", "/catalog/{category}/{$}", hrefStruct(
						hrefFieldDef{"Page", types.Typ[types.Int64], `query:"p"`},
						hrefFieldDef{"Sort", types.Typ[types.String], `query:"sort"`},
					)),
				},
			},
			golden: "href_path_and_query_with_int.go.txt",
		},
		"multiple pages mixed": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageFoo", "/foo/{$}", nil),
					hrefPage("PageBar", "/bar/{slug}/{$}", nil),
					hrefPage("PageBaz", "/baz/{$}", hrefStruct(
						hrefFieldDef{"Q", types.Typ[types.String], `query:"q"`},
					)),
				},
			},
			golden: "href_multiple_pages_mixed.go.txt",
		},
		"path variable int32": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPageWithPath("PageItem", "/item/{id}/{$}",
						hrefStruct(
							hrefFieldDef{"ID", types.Typ[types.Int32], `path:"id"`},
						), nil),
				},
			},
			golden: "href_path_variable_int32.go.txt",
		},
		"path variable naming conflict": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPageWithPath("PageItem", "/item/{value}/{s_value}/{s_s_value}/{$}",
						hrefStruct(
							hrefFieldDef{"Value", types.Typ[types.Int32], `path:"value"`},
							hrefFieldDef{"SValue", types.Typ[types.Int32], `path:"s_value"`},
							hrefFieldDef{"SSValue", types.Typ[types.String], `path:"s_s_value"`},
						), nil),
				},
			},
			golden: "href_path_variable_naming_conflict.go.txt",
		},
		"path variable uint64": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPageWithPath("PageItem", "/item/{id}/{$}",
						hrefStruct(
							hrefFieldDef{"ID", types.Typ[types.Uint64], `path:"id"`},
						), nil),
				},
			},
			golden: "href_path_variable_uint64.go.txt",
		},
		"path variable float64": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPageWithPath("PageCoord", "/coord/{lat}/{$}",
						hrefStruct(
							hrefFieldDef{"Lat", types.Typ[types.Float64], `path:"lat"`},
						), nil),
				},
			},
			golden: "href_path_variable_float64.go.txt",
		},
		"path variable bool": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPageWithPath("PageToggle", "/toggle/{on}/{$}",
						hrefStruct(
							hrefFieldDef{"On", types.Typ[types.Bool], `path:"on"`},
						), nil),
				},
			},
			golden: "href_path_variable_bool.go.txt",
		},
		"query with bool field": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageSearch", "/search/{$}", hrefStruct(
						hrefFieldDef{"Active", types.Typ[types.Bool], `query:"active"`},
					)),
				},
			},
			golden: "href_query_with_bool_field.go.txt",
		},
		"query with float64 field": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageSearch", "/search/{$}", hrefStruct(
						hrefFieldDef{"Price", types.Typ[types.Float64], `query:"price"`},
					)),
				},
			},
			golden: "href_query_with_float64_field.go.txt",
		},
		"query with uint32 field": {
			app: &model.App{
				Pages: []*model.Page{
					hrefPage("PageSearch", "/search/{$}", hrefStruct(
						hrefFieldDef{"Count", types.Typ[types.Uint32], `query:"count"`},
					)),
				},
			},
			golden: "href_query_with_uint32_field.go.txt",
		},
	}

	w := generator.Writer{Buf: make([]byte, 2*1024*1024)}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var err error
			w.Reset()
			w.WritePkgHref(tt.app)
			w.Buf, err = format.Source(w.Buf)
			require.NoError(t, err,
				"generated code is not valid Go:\n%s", string(w.Buf))

			goldenPath := filepath.Join("testdata", tt.golden)
			if *update {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(goldenPath, w.Buf, 0o644))
				return
			}

			want, err := os.ReadFile(goldenPath)
			require.NoError(t, err)
			require.Equal(t, string(want), string(w.Buf))
		})
	}
}

// hrefFieldDef defines a struct field for test query parameter construction.
type hrefFieldDef struct {
	Name string
	Type types.Type
	Tag  string
}

// hrefStruct constructs a *types.Struct from field definitions.
func hrefStruct(fields ...hrefFieldDef) *types.Struct {
	vars := make([]*types.Var, len(fields))
	tags := make([]string, len(fields))
	for i, f := range fields {
		vars[i] = types.NewVar(token.NoPos, nil, f.Name, f.Type)
		tags[i] = f.Tag
	}
	return types.NewStruct(vars, tags)
}

func TestWritePkgHrefTypeChecks(t *testing.T) {
	allIntTypes := []struct {
		name string
		typ  types.Type
	}{
		{"int", types.Typ[types.Int]},
		{"int8", types.Typ[types.Int8]},
		{"int16", types.Typ[types.Int16]},
		{"int32", types.Typ[types.Int32]},
		{"int64", types.Typ[types.Int64]},
		{"uint", types.Typ[types.Uint]},
		{"uint8", types.Typ[types.Uint8]},
		{"uint16", types.Typ[types.Uint16]},
		{"uint32", types.Typ[types.Uint32]},
		{"uint64", types.Typ[types.Uint64]},
	}

	tests := map[string]*model.App{
		"query with bool": {Pages: []*model.Page{
			hrefPage("PageSearch", "/search/{$}", hrefStruct(
				hrefFieldDef{"Active", types.Typ[types.Bool], `query:"active"`},
			)),
		}},
		"query with float32": {Pages: []*model.Page{
			hrefPage("PageSearch", "/search/{$}", hrefStruct(
				hrefFieldDef{"Ratio", types.Typ[types.Float32], `query:"ratio"`},
			)),
		}},
		"query with float64": {Pages: []*model.Page{
			hrefPage("PageSearch", "/search/{$}", hrefStruct(
				hrefFieldDef{"Price", types.Typ[types.Float64], `query:"price"`},
			)),
		}},
		"path with bool": {Pages: []*model.Page{
			hrefPageWithPath("PageToggle", "/toggle/{on}/{$}",
				hrefStruct(
					hrefFieldDef{"On", types.Typ[types.Bool], `path:"on"`},
				), nil),
		}},
		"path with float64": {Pages: []*model.Page{
			hrefPageWithPath("PageCoord", "/coord/{lat}/{$}",
				hrefStruct(
					hrefFieldDef{"Lat", types.Typ[types.Float64], `path:"lat"`},
				), nil),
		}},
		"path naming conflict": {Pages: []*model.Page{
			hrefPageWithPath("PageItem", "/item/{value}/{s_value}/{s_s_value}/{$}",
				hrefStruct(
					hrefFieldDef{"Value", types.Typ[types.Int32], `path:"value"`},
					hrefFieldDef{"SValue", types.Typ[types.Int32], `path:"s_value"`},
					hrefFieldDef{"SSValue", types.Typ[types.String], `path:"s_s_value"`},
				), nil),
		}},
	}

	// Add a test for each integer type as query param.
	for _, it := range allIntTypes {
		tests["query with "+it.name] = &model.App{Pages: []*model.Page{
			hrefPage("PageSearch", "/search/{$}", hrefStruct(
				hrefFieldDef{"Val", it.typ, `query:"val"`},
			)),
		}}
	}

	// Add a test for each integer type as path param.
	for _, it := range allIntTypes {
		tests["path with "+it.name] = &model.App{Pages: []*model.Page{
			hrefPageWithPath("PageItem", "/item/{id}/{$}",
				hrefStruct(
					hrefFieldDef{"ID", it.typ, `path:"id"`},
				), nil),
		}}
	}

	w := generator.Writer{Buf: make([]byte, 2*1024*1024)}
	for name, app := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.WritePkgHref(app)
			src, err := format.Source(w.Buf)
			require.NoError(t, err,
				"generated code is not valid Go syntax:\n%s", string(w.Buf))
			typeCheckGenerated(t, src)
		})
	}
}

// hrefPage constructs a *model.Page for AppendPkgHref testing.
// If querySt is non-nil, the page will have query parameters.
func hrefPage(typeName, route string, querySt *types.Struct) *model.Page {
	return hrefPageWithPath(typeName, route, nil, querySt)
}

// hrefPageWithPath constructs a *model.Page with explicit typed path and query structs.
func hrefPageWithPath(
	typeName, route string, pathSt, querySt *types.Struct,
) *model.Page {
	h := &model.Handler{}
	if pathSt != nil {
		h.InputPath = &model.Input{
			Name: "path",
			Type: model.Type{Resolved: pathSt},
		}
	}
	if querySt != nil {
		h.InputQuery = &model.Input{
			Name: "query",
			Type: model.Type{Resolved: querySt},
		}
	}
	return &model.Page{
		TypeName: typeName,
		Route:    route,
		GET:      &model.HandlerGET{Handler: h},
	}
}
