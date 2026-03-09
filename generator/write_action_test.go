package generator_test

import (
	"go/ast"
	"go/format"
	"go/importer"
	goparser "go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser/model"
	"github.com/stretchr/testify/require"
)

func TestWritePkgAction(t *testing.T) {
	tests := map[string]struct {
		app    *model.App
		golden string
	}{
		"no actions": {
			app:    &model.App{},
			golden: "action_no_actions.go.txt",
		},
		"simple page action post": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageLogin",
					actionHandler("post", "Submit", "/login/submit/{$}", nil)),
			}},
			golden: "action_simple_page_post.go.txt",
		},
		"put method": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageSettings",
					actionHandler("put", "Save", "/settings/save/{$}", nil)),
			}},
			golden: "action_put_method.go.txt",
		},
		"delete method": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PagePost",
					actionHandler("delete", "Remove", "/post/remove/{$}", nil)),
			}},
			golden: "action_delete_method.go.txt",
		},
		"page action with path variable": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PagePost",
					actionHandler("post", "SendMessage",
						"/post/{slug}/send-message/{$}", nil)),
			}},
			golden: "action_page_path_variable.go.txt",
		},
		"page action with multiple path variables": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageOrg",
					actionHandler("post", "Invite",
						"/org/{org}/member/{id}/invite/{$}", nil)),
			}},
			golden: "action_page_multiple_path_variables.go.txt",
		},
		"page action with query": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageMessages",
					actionHandler("post", "Read", "/messages/read/{$}",
						hrefStruct(
							hrefFieldDef{
								"MessageID", types.Typ[types.String],
								`query:"msgid"`,
							},
						))),
			}},
			golden: "action_page_query.go.txt",
		},
		"page action with query multiple fields": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageMessages",
					actionHandler("post", "MarkRead", "/messages/mark-read/{$}",
						hrefStruct(
							hrefFieldDef{
								"MessageID", types.Typ[types.String],
								`query:"msgid"`,
							},
							hrefFieldDef{
								"Chat", types.Typ[types.String],
								`query:"chat"`,
							},
						))),
			}},
			golden: "action_page_query_multiple_fields.go.txt",
		},
		"page action with path and query": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PagePost",
					actionHandler("post", "Reply", "/post/{slug}/reply/{$}",
						hrefStruct(
							hrefFieldDef{
								"ParentID", types.Typ[types.String],
								`query:"pid"`,
							},
						))),
			}},
			golden: "action_page_path_and_query.go.txt",
		},
		"app-level action": {
			app: &model.App{Actions: []*model.Handler{
				actionHandler("post", "SignOut", "/sign-out/{$}", nil),
			}},
			golden: "action_app_level.go.txt",
		},
		"app actions before page actions": {
			app: &model.App{
				Actions: []*model.Handler{
					actionHandler("post", "SignOut", "/sign-out/{$}", nil),
				},
				Pages: []*model.Page{
					actionPage("PageLogin",
						actionHandler("post", "Submit",
							"/login/submit/{$}", nil)),
				},
			},
			golden: "action_app_before_page.go.txt",
		},
		"actions sorted alphabetically": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageSettings",
					actionHandler("post", "Zebra",
						"/settings/zebra/{$}", nil),
					actionHandler("post", "Alpha",
						"/settings/alpha/{$}", nil)),
			}},
			golden: "action_sorted.go.txt",
		},
		"multiple pages": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageLogin",
					actionHandler("post", "Submit",
						"/login/submit/{$}", nil)),
				actionPage("PageSettings",
					actionHandler("post", "Save",
						"/settings/save/{$}", nil)),
			}},
			golden: "action_multiple_pages.go.txt",
		},
		"app action with query": {
			app: &model.App{Actions: []*model.Handler{
				actionHandler("post", "Search", "/search/{$}",
					hrefStruct(
						hrefFieldDef{
							"Term", types.Typ[types.String],
							`query:"t"`,
						},
					)),
			}},
			golden: "action_app_action_with_query.go.txt",
		},
		"root route action": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageIndex",
					actionHandler("post", "Refresh", "/{$}", nil)),
			}},
			golden: "action_root_route.go.txt",
		},
		"path and query multiple fields": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PagePost",
					actionHandler("post", "Comment",
						"/post/{slug}/comment/{$}",
						hrefStruct(
							hrefFieldDef{
								"ParentID", types.Typ[types.String],
								`query:"pid"`,
							},
							hrefFieldDef{
								"Draft", types.Typ[types.String],
								`query:"draft"`,
							},
						))),
			}},
			golden: "action_path_and_query_multiple_fields.go.txt",
		},
		"query with int field": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageIndex",
					actionHandler("post", "Add", "/add/{$}",
						hrefStruct(
							hrefFieldDef{
								"Delta", types.Typ[types.Int64],
								`query:"delta"`,
							},
						))),
			}},
			golden: "action_query_with_int_field.go.txt",
		},
		"path and query with int field": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PagePost",
					actionHandler("post", "Rate",
						"/post/{slug}/rate/{$}",
						hrefStruct(
							hrefFieldDef{
								"Score", types.Typ[types.Int64],
								`query:"score"`,
							},
						))),
			}},
			golden: "action_path_and_query_with_int_field.go.txt",
		},
		"path and query multiple path vars": {
			app: &model.App{Pages: []*model.Page{
				actionPage("PageOrg",
					actionHandler("post", "Update",
						"/org/{org}/team/{team}/update/{$}",
						hrefStruct(
							hrefFieldDef{
								"Role", types.Typ[types.String],
								`query:"role"`,
							},
						))),
			}},
			golden: "action_path_and_query_multiple_path_vars.go.txt",
		},
	}

	w := generator.Writer{Buf: make([]byte, 2*1024*1024)}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var err error
			w.Reset()
			w.WritePkgAction(tt.app)
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

func TestWritePkgActionIntFieldTypeChecks(t *testing.T) {
	tests := map[string]*model.App{
		"query only with int field": {Pages: []*model.Page{
			actionPage("PageIndex",
				actionHandler("post", "Add", "/add/{$}",
					hrefStruct(
						hrefFieldDef{
							"Delta", types.Typ[types.Int64],
							`query:"delta"`,
						},
					))),
		}},
		"path and query with int field": {Pages: []*model.Page{
			actionPage("PagePost",
				actionHandler("post", "Rate", "/post/{slug}/rate/{$}",
					hrefStruct(
						hrefFieldDef{
							"Score", types.Typ[types.Int64],
							`query:"score"`,
						},
					))),
		}},
	}

	w := generator.Writer{Buf: make([]byte, 2*1024*1024)}
	for name, app := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.WritePkgAction(app)
			src, err := format.Source(w.Buf)
			require.NoError(t, err,
				"generated code is not valid Go syntax:\n%s", string(w.Buf))
			typeCheckGenerated(t, src)
		})
	}
}

func typeCheckGenerated(t *testing.T, src []byte) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "gen.go", src, 0)
	require.NoError(t, err, "parse error")

	conf := types.Config{Importer: importer.Default()}
	_, err = conf.Check("action", fset, []*ast.File{f}, nil)
	require.NoError(t, err,
		"generated code does not type-check:\n%s", string(src))
}

// actionPage constructs a *model.Page with the given actions.
func actionPage(typeName string, actions ...*model.Handler) *model.Page {
	return &model.Page{
		TypeName: typeName,
		Actions:  actions,
	}
}

// actionHandler constructs a *model.Handler for AppendPkgAction testing.
// InputPath is set automatically when the route contains path variables.
// If query is non-nil, InputQuery is set.
func actionHandler(
	method, name, route string, query *types.Struct,
) *model.Handler {
	h := &model.Handler{
		HTTPMethod: method,
		Name:       name,
		Route:      route,
	}
	// Set InputPath when route has path variables so the import check works.
	cleaned := strings.ReplaceAll(route, "{$}", "")
	if strings.Contains(cleaned, "{") {
		h.InputPath = &model.Input{Name: "path"}
	}
	if query != nil {
		h.InputQuery = &model.Input{
			Name: "query",
			Type: model.Type{Resolved: query},
		}
	}
	return h
}
