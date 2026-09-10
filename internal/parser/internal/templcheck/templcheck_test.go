package templcheck_test

import (
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/internal/parser/internal/templcheck"
	"github.com/romshark/datapages/internal/parser/model"
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
	case *templcheck.HrefRelativeError:
		return *e
	case *templcheck.HrefUnverifiableError:
		return *e
	case *templcheck.HrefExternalIsRelativeError:
		return *e
	case *templcheck.ActionHardcodedError:
		return *e
	case *templcheck.ActionUnverifiableError:
		return *e
	case *templcheck.ActionUnverifiableWithPrefixError:
		return *e
	case *templcheck.ActionUnverifiableWithSuffixError:
		return *e
	case *templcheck.FormActionError:
		return *e
	case *templcheck.ActionContextError:
		return *e
	case *templcheck.HrefContextError:
		return *e
	case *templcheck.ActionWrongPageError:
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

// TestCheck_ErrHref tests every href the linter refuses in the
// err_templ_href fixture: a relative URL written by hand instead of through the
// generated href package, an expression it cannot verify, and an external href
// handed a relative URL. Positions are asserted, since the position is what
// sends the developer to the line.
func TestCheck_ErrHref(t *testing.T) {
	errs := check(t, "err_templ_href", nil)

	expect := []posError{
		{31, 5, templcheck.HrefRelativeError{URL: "/login"}},
		{33, 5, templcheck.HrefRelativeError{URL: "/profile"}},
		{35, 5, templcheck.HrefRelativeError{URL: "/static/style.css"}},
		{37, 12, templcheck.HrefRelativeError{URL: "/settings"}},
		{39, 12, templcheck.HrefRelativeError{URL: "/set"}},
		{41, 12, templcheck.HrefUnverifiableError{Expr: `"/set" + dynamicValue`}},
		{43, 12, templcheck.HrefUnverifiableError{Expr: `templ.SafeURL("/about")`}},
		{45, 12, templcheck.HrefUnverifiableError{
			Expr: `templ.SafeURL(ConstantStringNOTOK)`,
		}},
		{47, 12, templcheck.HrefUnverifiableError{
			Expr: `templ.SafeURL("https://data-star.dev")`,
		}},
		{48, 12, templcheck.HrefRelativeError{URL: "/c"}},
		{49, 12, templcheck.HrefRelativeError{URL: "notok"}},
		{51, 5, templcheck.HrefRelativeError{URL: ""}},
		{53, 5, templcheck.HrefRelativeError{URL: "?tab=settings"}},
		{55, 5, templcheck.HrefRelativeError{URL: "relative"}},
		{57, 5, templcheck.HrefRelativeError{URL: "javascript:void(0)"}},
		{61, 7, templcheck.HrefRelativeError{URL: "/nested"}},
		{65, 12, templcheck.HrefUnverifiableError{Expr: `loginHref()`}},
		{67, 12, templcheck.HrefUnverifiableError{Expr: `someOtherFunc()`}},
		{69, 12, templcheck.HrefUnverifiableError{Expr: `buildURL(id)`}},
		{71, 12, templcheck.HrefUnverifiableError{
			Expr: `fmt.Sprintf("mailto:%s", "test@example.com")`,
		}},
		{73, 12, templcheck.HrefExternalIsRelativeError{URL: "/login"}},
		{75, 12, templcheck.HrefExternalIsRelativeError{URL: "/internal"}},
		{77, 5, templcheck.HrefRelativeError{URL: "/should-error"}},
		{79, 12, templcheck.HrefRelativeError{URL: "/login-imported"}},
		{81, 12, templcheck.HrefExternalIsRelativeError{URL: "/internal-imported"}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

// TestCheck_ErrActionWrongPage tests an action used in a template of a page that
// does not own it, which would post to a route that page's URL cannot produce.
// Ownership is checked even where a nolint suppresses the element-level checks.
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
	// action.POSTPageProfileSave(), which belongs to PageProfile,
	// not PageSettings.
	// action.POSTPageSettingsUpdate() in settingsPage is OK (own page).
	// action.POSTAppGlobal() in settingsActions is OK (app-level).
	// action.POSTPageProfileSave() in profilePage is OK (own page).
	// The nolinted PageProfile.Save.POST() at line 33 is still flagged:
	// nolint suppresses element-level checks but NOT ownership checks.

	expect := []posError{
		{11, 17, templcheck.FormActionError{}},
		{17, 17, templcheck.FormActionError{}},
		{25, 17, templcheck.FormActionError{}},
		{25, 17, templcheck.ActionWrongPageError{
			ActionFunc: "PageProfile.Save.POST",
			PageType:   "PageSettings",
			OwnerPage:  "PageProfile",
		}},
		{28, 17, templcheck.FormActionError{}},
		{33, 17, templcheck.ActionWrongPageError{
			ActionFunc: "PageProfile.Save.POST",
			PageType:   "PageSettings",
			OwnerPage:  "PageProfile",
		}},
	}

	require.ElementsMatch(t, expect, toPosErrors(errs))
}

// TestCheck_ErrContext tests an action or href used in the wrong attribute: an
// action in href, an href in a data-on expression. It also tests an action expression
// concatenated with something else, which is reported apart by whether the extra part
// is a prefix, a suffix or neither, since only the first two can be suggested a fix.
func TestCheck_ErrContext(t *testing.T) {
	errs := check(t, "err_templ_context", nil)

	expect := []posError{
		{10, 12, templcheck.ActionContextError{
			AttrName: "href", ActionFunc: "POSTPageIndexSubmit",
		}},
		{26, 26, templcheck.HrefContextError{
			AttrName: "data-on:click", HrefFunc: "PageIndex",
		}},
		{28, 25, templcheck.HrefContextError{
			AttrName: "data-on:submit", HrefFunc: "PageIndex",
		}},
		{30, 19, templcheck.HrefContextError{
			AttrName: "data-init", HrefFunc: "PageIndex",
		}},
		{36, 19, templcheck.ActionContextError{
			AttrName: "data-only", ActionFunc: "POSTPageIndexSubmit",
		}},
		{40, 26, templcheck.ActionUnverifiableWithPrefixError{
			Expr:       `"$_fresh = true; " + action.POSTPageIndexSubmit()`,
			ActionFunc: "POSTPageIndexSubmit",
			Prefix:     `"$_fresh = true; "`,
		}},
		{44, 26, templcheck.ActionUnverifiableWithSuffixError{
			Expr:       `action.POSTPageIndexSubmit() + "; $_fresh = true"`,
			ActionFunc: "POSTPageIndexSubmit",
			Suffix:     `"; $_fresh = true"`,
		}},
		{49, 19, templcheck.ActionUnverifiableError{
			Expr: `action.POSTPageIndexSubmit() + action.POSTPageIndexSubmit()`,
		}},
		{55, 19, templcheck.ActionUnverifiableError{
			Expr: `action.POSTPageIndexSubmit() + action.POSTPageIndexReset()`,
		}},
		{61, 19, templcheck.ActionUnverifiableWithPrefixError{
			Expr:       `"$a; " + "$b; " + action.POSTPageIndexSubmit()`,
			ActionFunc: "POSTPageIndexSubmit",
			Prefix:     `"$a; " + "$b; "`,
		}},
		{67, 19, templcheck.ActionUnverifiableError{
			Expr: `action.POSTPageIndexSubmit() + "; $a" + "; $b"`,
		}},
		{73, 19, templcheck.ActionUnverifiableWithPrefixError{
			Expr:       "action.POSTPageIndexSubmit() +\n\t\t\t\"; $a; \" +\n\t\t\taction.POSTPageIndexReset()",
			ActionFunc: "POSTPageIndexReset",
			Prefix:     `action.POSTPageIndexSubmit() + "; $a; "`,
		}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

// TestCheck_ErrFormAction tests a form action attribute, which Datapages has no
// route for: an action is reached through a Datastar expression, never a form submit.
func TestCheck_ErrFormAction(t *testing.T) {
	errs := check(t, "err_templ_form_action", nil)

	expect := []posError{
		{7, 8, templcheck.FormActionError{}},
		{11, 17, templcheck.FormActionError{}},
		{15, 17, templcheck.FormActionError{}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

// TestCheck_ErrActionHardcoded tests an action URL written by hand into a
// Datastar attribute rather than taken from the generated action package,
// across every attribute that carries one.
func TestCheck_ErrActionHardcoded(t *testing.T) {
	errs := check(t, "err_templ_hardcoded_action", nil)

	expect := []posError{
		{7, 10, templcheck.ActionHardcodedError{URL: "/login/submit"}},
		{9, 7, templcheck.ActionHardcodedError{URL: "/api/data"}},
		{11, 8, templcheck.ActionHardcodedError{URL: "/profile/save"}},
		{13, 10, templcheck.ActionHardcodedError{URL: "/resource"}},
		{15, 10, templcheck.ActionHardcodedError{URL: "/resource"}},
		{17, 10, templcheck.ActionHardcodedError{URL: "/resource"}},
		{19, 7, templcheck.ActionHardcodedError{URL: "/lazy"}},
		{21, 7, templcheck.ActionHardcodedError{URL: "/poll"}},
		{23, 7, templcheck.ActionHardcodedError{URL: "/sync"}},
		{25, 7, templcheck.ActionHardcodedError{URL: "/init"}},
		{27, 10, templcheck.ActionHardcodedError{URL: "/custom"}},
		{29, 10, templcheck.ActionHardcodedError{URL: "/mixed"}},
		{31, 10, templcheck.ActionHardcodedError{URL: "/debounced"}},
		{33, 7, templcheck.ActionHardcodedError{URL: "/intersect-once"}},
		{35, 7, templcheck.ActionHardcodedError{URL: "/init-once"}},
		{37, 26, templcheck.ActionHardcodedError{URL: "/expr-literal"}},
		{39, 26, templcheck.ActionHardcodedError{URL: "/backtick"}},
		{41, 26, templcheck.ActionHardcodedError{URL: "/const-action"}},
		{43, 26, templcheck.ActionHardcodedError{URL: "/imported-action"}},
		{45, 26, templcheck.ActionUnverifiableError{Expr: `"@post" + "('/concat')"`}},
		{47, 26, templcheck.ActionUnverifiableError{Expr: `buildAction()`}},
		{49, 26, templcheck.ActionUnverifiableError{Expr: `dynamicVar`}},
	}

	require.Equal(t, expect, toPosErrors(errs))
}

// TestCheck_OKHref tests the fixture that uses the generated href package
// correctly throughout: the linter must report nothing.
func TestCheck_OKHref(t *testing.T) {
	errs := check(t, "ok_templ_href", nil)
	requireNoErrs(t, errs)
}

// TestCheck_OKHrefAlias tests the same through an import alias.
func TestCheck_OKHrefAlias(t *testing.T) {
	errs := check(t, "ok_templ_href_alias", nil)
	requireNoErrs(t, errs)
}

// TestCheck_OKHrefDot tests the same for a template package in a subdirectory of
// the app package.
func TestCheck_OKHrefDot(t *testing.T) {
	errs := check(t, "ok_templ_href_dot/template", nil)
	requireNoErrs(t, errs)
}

// BenchmarkCheck_ErrHref measures the linter on a package where every href is a finding,
// which is its worst case.
func BenchmarkCheck_ErrHref(b *testing.B) {
	pkg := loadPkg(b, "err_templ_href")
	noop := func(token.Position, error) {}

	for b.Loop() {
		templcheck.Check(pkg, nil, noop)
	}
}

// BenchmarkCheck_OKHref measures the linter on a clean package,
// which is what every build pays.
func BenchmarkCheck_OKHref(b *testing.B) {
	pkg := loadPkg(b, "ok_templ_href")
	noop := func(token.Position, error) {}

	for b.Loop() {
		templcheck.Check(pkg, nil, noop)
	}
}
