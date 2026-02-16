package generator_test

import (
	"flag"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser/model"
	"github.com/stretchr/testify/require"
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

// hrefPage constructs a *model.Page for AppendPkgHref testing.
// If querySt is non-nil, the page will have query parameters.
func hrefPage(typeName, route string, querySt *types.Struct) *model.Page {
	h := &model.Handler{}
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
