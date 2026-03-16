package templcheck_test

import (
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/parser/internal/templcheck"
	"github.com/romshark/datapages/parser/model"
)

func loadPkg(tb testing.TB, fixtureName string) *packages.Package {
	tb.Helper()
	dir := filepath.Join("testdata", fixtureName)
	absDir, err := filepath.Abs(dir)
	require.NoError(tb, err)
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax |
			packages.NeedModule,
		Dir: absDir,
	}
	pkgs, err := packages.Load(cfg, ".")
	require.NoError(tb, err)
	require.Len(tb, pkgs, 1)
	return pkgs[0]
}

type posErr struct {
	pos token.Position
	err error
}

type posError struct {
	line, col int
	err       any
}

func toPosErrors(errs []posErr) []posError {
	out := make([]posError, len(errs))
	for i, pe := range errs {
		out[i] = posError{pe.pos.Line, pe.pos.Column, derefErr(pe.err)}
	}
	return out
}

// derefErr returns the dereferenced (value) form of known error pointer types
// so that require.Equal compares by value rather than pointer identity.
func derefErr(err error) any {
	switch e := err.(type) {
	case *templcheck.ErrorHrefRelative:
		return *e
	case *templcheck.ErrorHrefUnverifiable:
		return *e
	case *templcheck.ErrorHrefExternalIsRelative:
		return *e
	case *templcheck.ErrorActionHardcoded:
		return *e
	case *templcheck.ErrorActionUnverifiable:
		return *e
	case *templcheck.ErrorActionUnverifiableWithPrefix:
		return *e
	case *templcheck.ErrorActionUnverifiableWithSuffix:
		return *e
	case *templcheck.ErrorFormAction:
		return *e
	case *templcheck.ErrorActionContext:
		return *e
	case *templcheck.ErrorHrefContext:
		return *e
	case *templcheck.ErrorActionWrongPage:
		return *e
	default:
		return err
	}
}

func requireNoErrs(t *testing.T, errs []posErr) {
	t.Helper()
	for _, pe := range errs {
		t.Errorf("unexpected error at %s: %v", pe.pos, pe.err)
	}
	require.Empty(t, errs)
}

func check(t *testing.T, fixtureName string, app *model.App) []posErr {
	t.Helper()
	pkg := loadPkg(t, fixtureName)
	var errs []posErr
	templcheck.Check(pkg, app, func(pos token.Position, err error) {
		errs = append(errs, posErr{pos: pos, err: err})
	})
	return errs
}

func TestCheck_ErrHref(t *testing.T) {
	errs := check(t, "err_templ_href", nil)

	expect := []posError{
		{31, 5, templcheck.ErrorHrefRelative{URL: "/login"}},
		{33, 5, templcheck.ErrorHrefRelative{URL: "/profile"}},
		{35, 5, templcheck.ErrorHrefRelative{URL: "/static/style.css"}},
		{37, 12, templcheck.ErrorHrefRelative{URL: "/settings"}},
		{39, 12, templcheck.ErrorHrefRelative{URL: "/set"}},
		{41, 12, templcheck.ErrorHrefUnverifiable{Expr: `"/set" + dynamicValue`}},
		{43, 12, templcheck.ErrorHrefUnverifiable{Expr: `templ.SafeURL("/about")`}},
		{45, 12, templcheck.ErrorHrefUnverifiable{
			Expr: `templ.SafeURL(ConstantStringNOTOK)`,
		}},
		{47, 12, templcheck.ErrorHrefUnverifiable{
			Expr: `templ.SafeURL("https://data-star.dev")`,
		}},
		{48, 12, templcheck.ErrorHrefRelative{URL: "/c"}},
		{49, 12, templcheck.ErrorHrefRelative{URL: "notok"}},
		{51, 5, templcheck.ErrorHrefRelative{URL: ""}},
		{53, 5, templcheck.ErrorHrefRelative{URL: "?tab=settings"}},
		{55, 5, templcheck.ErrorHrefRelative{URL: "relative"}},
		{57, 5, templcheck.ErrorHrefRelative{URL: "javascript:void(0)"}},
		{61, 7, templcheck.ErrorHrefRelative{URL: "/nested"}},
		{65, 12, templcheck.ErrorHrefUnverifiable{Expr: `loginHref()`}},
		{67, 12, templcheck.ErrorHrefUnverifiable{Expr: `someOtherFunc()`}},
		{69, 12, templcheck.ErrorHrefUnverifiable{Expr: `buildURL(id)`}},
		{71, 12, templcheck.ErrorHrefUnverifiable{
			Expr: `fmt.Sprintf("mailto:%s", "test@example.com")`,
		}},
		{73, 12, templcheck.ErrorHrefExternalIsRelative{URL: "/login"}},
		{75, 12, templcheck.ErrorHrefExternalIsRelative{URL: "/internal"}},
		{77, 5, templcheck.ErrorHrefRelative{URL: "/should-error"}},
		{79, 12, templcheck.ErrorHrefRelative{URL: "/login-imported"}},
		{81, 12, templcheck.ErrorHrefExternalIsRelative{URL: "/internal-imported"}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

func TestCheck_ErrActionWrongPage(t *testing.T) {
	// Build a minimal model.App that mirrors the fixture:
	// PageProfile owns POSTSave, PageSettings owns POSTUpdate, App owns POSTGlobal.
	app := &model.App{
		Actions: []*model.Handler{
			{HTTPMethod: "post", Name: "Global"},
		},
		Pages: []*model.Page{
			{
				TypeName: "PageIndex",
				GET:      &model.HandlerGET{Handler: &model.Handler{}},
			},
			{
				TypeName: "PageProfile",
				GET:      &model.HandlerGET{Handler: &model.Handler{}},
				Actions: []*model.Handler{
					{HTTPMethod: "post", Name: "Save"},
				},
			},
			{
				TypeName: "PageSettings",
				GET:      &model.HandlerGET{Handler: &model.Handler{}},
				Actions: []*model.Handler{
					{HTTPMethod: "post", Name: "Update"},
				},
			},
		},
	}

	errs := check(t, "err_templ_action_not_on_page", app)

	// settingsPage() calls @settingsActions() which uses
	// action.POSTPageProfileSave() — that action belongs to PageProfile,
	// not PageSettings.
	// action.POSTPageSettingsUpdate() in settingsPage is OK (own page).
	// action.POSTAppGlobal() in settingsActions is OK (app-level).
	// action.POSTPageProfileSave() in profilePage is OK (own page).
	// The nolinted POSTPageProfileSave() at line 33 is still flagged:
	// nolint suppresses element-level checks but NOT ownership checks.

	expect := []posError{
		{11, 17, templcheck.ErrorFormAction{}},
		{17, 17, templcheck.ErrorFormAction{}},
		{25, 17, templcheck.ErrorFormAction{}},
		{25, 17, templcheck.ErrorActionWrongPage{
			ActionFunc: "POSTPageProfileSave",
			PageType:   "PageSettings",
			OwnerPage:  "PageProfile",
		}},
		{28, 17, templcheck.ErrorFormAction{}},
		{33, 17, templcheck.ErrorActionWrongPage{
			ActionFunc: "POSTPageProfileSave",
			PageType:   "PageSettings",
			OwnerPage:  "PageProfile",
		}},
	}

	require.ElementsMatch(t, expect, toPosErrors(errs))
}

func TestCheck_ErrContext(t *testing.T) {
	errs := check(t, "err_templ_context", nil)

	expect := []posError{
		{10, 12, templcheck.ErrorActionContext{
			AttrName: "href", ActionFunc: "POSTPageIndexSubmit",
		}},
		{26, 26, templcheck.ErrorHrefContext{
			AttrName: "data-on:click", HrefFunc: "PageIndex",
		}},
		{28, 25, templcheck.ErrorHrefContext{
			AttrName: "data-on:submit", HrefFunc: "PageIndex",
		}},
		{30, 19, templcheck.ErrorHrefContext{
			AttrName: "data-init", HrefFunc: "PageIndex",
		}},
		{36, 19, templcheck.ErrorActionContext{
			AttrName: "data-only", ActionFunc: "POSTPageIndexSubmit",
		}},
		{40, 26, templcheck.ErrorActionUnverifiableWithPrefix{
			Expr:       `"$_fresh = true; " + action.POSTPageIndexSubmit()`,
			ActionFunc: "POSTPageIndexSubmit",
			Prefix:     `"$_fresh = true; "`,
		}},
		{44, 26, templcheck.ErrorActionUnverifiableWithSuffix{
			Expr:       `action.POSTPageIndexSubmit() + "; $_fresh = true"`,
			ActionFunc: "POSTPageIndexSubmit",
			Suffix:     `"; $_fresh = true"`,
		}},
		{49, 19, templcheck.ErrorActionUnverifiable{
			Expr: `action.POSTPageIndexSubmit() + action.POSTPageIndexSubmit()`,
		}},
		{55, 19, templcheck.ErrorActionUnverifiable{
			Expr: `action.POSTPageIndexSubmit() + action.POSTPageIndexReset()`,
		}},
		{61, 19, templcheck.ErrorActionUnverifiableWithPrefix{
			Expr:       `"$a; " + "$b; " + action.POSTPageIndexSubmit()`,
			ActionFunc: "POSTPageIndexSubmit",
			Prefix:     `"$a; " + "$b; "`,
		}},
		{67, 19, templcheck.ErrorActionUnverifiable{
			Expr: `action.POSTPageIndexSubmit() + "; $a" + "; $b"`,
		}},
		{73, 19, templcheck.ErrorActionUnverifiableWithPrefix{
			Expr: "action.POSTPageIndexSubmit() +\n\t\t\t\"; $a; \" +\n\t\t\taction.POSTPageIndexReset()",
			ActionFunc: "POSTPageIndexReset",
			Prefix:     `action.POSTPageIndexSubmit() + "; $a; "`,
		}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

func TestCheck_ErrFormAction(t *testing.T) {
	errs := check(t, "err_templ_form_action", nil)

	expect := []posError{
		{7, 8, templcheck.ErrorFormAction{}},
		{11, 17, templcheck.ErrorFormAction{}},
		{15, 17, templcheck.ErrorFormAction{}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

func TestCheck_ErrActionHardcoded(t *testing.T) {
	errs := check(t, "err_templ_hardcoded_action", nil)

	expect := []posError{
		{7, 10, templcheck.ErrorActionHardcoded{URL: "/login/submit"}},
		{9, 7, templcheck.ErrorActionHardcoded{URL: "/api/data"}},
		{11, 8, templcheck.ErrorActionHardcoded{URL: "/profile/save"}},
		{13, 10, templcheck.ErrorActionHardcoded{URL: "/resource"}},
		{15, 10, templcheck.ErrorActionHardcoded{URL: "/resource"}},
		{17, 10, templcheck.ErrorActionHardcoded{URL: "/resource"}},
		{19, 7, templcheck.ErrorActionHardcoded{URL: "/lazy"}},
		{21, 7, templcheck.ErrorActionHardcoded{URL: "/poll"}},
		{23, 7, templcheck.ErrorActionHardcoded{URL: "/sync"}},
		{25, 7, templcheck.ErrorActionHardcoded{URL: "/init"}},
		{27, 10, templcheck.ErrorActionHardcoded{URL: "/custom"}},
		{29, 10, templcheck.ErrorActionHardcoded{URL: "/mixed"}},
		{31, 10, templcheck.ErrorActionHardcoded{URL: "/debounced"}},
		{33, 7, templcheck.ErrorActionHardcoded{URL: "/intersect-once"}},
		{35, 7, templcheck.ErrorActionHardcoded{URL: "/init-once"}},
		{37, 26, templcheck.ErrorActionHardcoded{URL: "/expr-literal"}},
		{39, 26, templcheck.ErrorActionHardcoded{URL: "/backtick"}},
		{41, 26, templcheck.ErrorActionHardcoded{URL: "/const-action"}},
		{43, 26, templcheck.ErrorActionHardcoded{URL: "/imported-action"}},
		{45, 26, templcheck.ErrorActionUnverifiable{Expr: `"@post" + "('/concat')"`}},
		{47, 26, templcheck.ErrorActionUnverifiable{Expr: `buildAction()`}},
		{49, 26, templcheck.ErrorActionUnverifiable{Expr: `dynamicVar`}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

func TestCheck_OKHref(t *testing.T) {
	errs := check(t, "ok_templ_href", nil)
	requireNoErrs(t, errs)
}

func TestCheck_OKHrefAlias(t *testing.T) {
	errs := check(t, "ok_templ_href_alias", nil)
	requireNoErrs(t, errs)
}

func TestCheck_OKHrefDot(t *testing.T) {
	errs := check(t, "ok_templ_href_dot/template", nil)
	requireNoErrs(t, errs)
}

func BenchmarkCheck_ErrHref(b *testing.B) {
	pkg := loadPkg(b, "err_templ_href")
	noop := func(token.Position, error) {}

	for b.Loop() {
		templcheck.Check(pkg, nil, noop)
	}
}

func BenchmarkCheck_OKHref(b *testing.B) {
	pkg := loadPkg(b, "ok_templ_href")
	noop := func(token.Position, error) {}

	for b.Loop() {
		templcheck.Check(pkg, nil, noop)
	}
}
