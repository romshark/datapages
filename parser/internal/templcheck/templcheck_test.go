package templcheck_test

import (
	"errors"
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

	type expectEntry struct {
		sentinel  error
		val       string
		line, col int
	}
	hardcoded := templcheck.ErrHardcodedHref
	unverifiable := templcheck.ErrHrefUnverifiable
	extInternal := templcheck.ErrExternalWithInternal
	expect := []expectEntry{
		{hardcoded, "/login", 31, 5},
		{hardcoded, "/profile", 33, 5},
		{hardcoded, "/static/style.css", 35, 5},
		{hardcoded, "/settings", 37, 12},
		{hardcoded, "/set", 39, 12},
		{unverifiable, `"/set" + dynamicValue`, 41, 12},
		{unverifiable, `templ.SafeURL("/about")`, 43, 12},
		{unverifiable, `templ.SafeURL(ConstantStringNOTOK)`, 45, 12},
		{unverifiable, `templ.SafeURL("https://data-star.dev")`, 47, 12},
		{hardcoded, "/c", 48, 12},
		{hardcoded, "notok", 49, 12},
		{hardcoded, "", 51, 5},
		{hardcoded, "?tab=settings", 53, 5},
		{hardcoded, "relative", 55, 5},
		{hardcoded, "javascript:void(0)", 57, 5},
		{hardcoded, "/nested", 61, 7},
		{unverifiable, `loginHref()`, 65, 12},
		{unverifiable, `someOtherFunc()`, 67, 12},
		{unverifiable, `buildURL(id)`, 69, 12},
		{unverifiable, `fmt.Sprintf("mailto:%s", "test@example.com")`, 71, 12},
		{extInternal, "/login", 73, 12},
		{extInternal, "/internal", 75, 12},
		{hardcoded, "/should-error", 77, 5},
		{hardcoded, "/login-imported", 79, 12},
		{extInternal, "/internal-imported", 81, 12},
	}

	var got []expectEntry
	for _, pe := range errs {
		if h, ok := errors.AsType[*templcheck.ErrorHardcodedHref](pe.err); ok {
			got = append(got,
				expectEntry{hardcoded, h.URL, pe.pos.Line, pe.pos.Column})
			continue
		}
		if u, ok := errors.AsType[*templcheck.ErrorHrefUnverifiable](pe.err); ok {
			got = append(got,
				expectEntry{unverifiable, u.Expr, pe.pos.Line, pe.pos.Column})
			continue
		}
		if e, ok := errors.AsType[*templcheck.ErrorExternalWithInternal](pe.err); ok {
			got = append(got,
				expectEntry{extInternal, e.URL, pe.pos.Line, pe.pos.Column})
			continue
		}
		t.Errorf("unexpected error at %s: %v", pe.pos, pe.err)
	}
	require.Equal(t, expect, got)
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

	type expectEntry struct {
		actionFunc string
		pageType   string
		ownerPage  string
		line       int
		col        int
	}
	expect := []expectEntry{
		{
			actionFunc: "POSTPageProfileSave",
			pageType:   "PageSettings",
			ownerPage:  "PageProfile",
			line:       25,
			col:        17,
		},
		{
			actionFunc: "POSTPageProfileSave",
			pageType:   "PageSettings",
			ownerPage:  "PageProfile",
			line:       33,
			col:        17,
		},
	}

	var got []expectEntry
	for _, pe := range errs {
		var e *templcheck.ErrorActionWrongPage
		if !errors.As(pe.err, &e) {
			continue
		}
		require.ErrorIs(t, pe.err, templcheck.ErrActionWrongPage)
		got = append(got, expectEntry{
			actionFunc: e.ActionFunc,
			pageType:   e.PageType,
			ownerPage:  e.OwnerPage,
			line:       pe.pos.Line,
			col:        pe.pos.Column,
		})
	}
	require.ElementsMatch(t, expect, got)
}

func TestCheck_ErrContext(t *testing.T) {
	errs := check(t, "err_templ_context", nil)

	type actionExpect struct {
		attrName   string
		actionFunc string
		line       int
		col        int
	}
	actionCases := map[string]actionExpect{
		"href-POSTPageIndexSubmit": {
			attrName:   "href",
			actionFunc: "POSTPageIndexSubmit",
			line:       10,
			col:        12,
		},
		"data-only-POSTPageIndexSubmit": {
			attrName:   "data-only",
			actionFunc: "POSTPageIndexSubmit",
			line:       36,
			col:        19,
		},
	}

	type hrefExpect struct {
		attrName string
		hrefFunc string
		line     int
		col      int
	}
	hrefCases := map[string]hrefExpect{
		"data-on:click-PageIndex": {
			attrName: "data-on:click",
			hrefFunc: "PageIndex",
			line:     26,
			col:      26,
		},
		"data-on:submit-PageIndex": {
			attrName: "data-on:submit",
			hrefFunc: "PageIndex",
			line:     28,
			col:      25,
		},
		"data-init-PageIndex": {
			attrName: "data-init",
			hrefFunc: "PageIndex",
			line:     30,
			col:      19,
		},
	}

	foundAction := map[string]bool{}
	foundHref := map[string]bool{}
	for _, pe := range errs {
		if e, ok := errors.AsType[*templcheck.ErrorActionContext](pe.err); ok {
			key := e.AttrName + "-" + e.ActionFunc
			foundAction[key] = true
			want, ok := actionCases[key]
			require.True(t, ok, "unexpected action context error: %s in %s", e.ActionFunc, e.AttrName)
			require.Equal(t, want.attrName, e.AttrName)
			require.Equal(t, want.actionFunc, e.ActionFunc)
			require.Equal(t, want.line, pe.pos.Line, "wrong line for %s", key)
			require.Equal(t, want.col, pe.pos.Column, "wrong column for %s", key)
			require.ErrorIs(t, pe.err, templcheck.ErrActionContext)
			continue
		}
		if e, ok := errors.AsType[*templcheck.ErrorHrefContext](pe.err); ok {
			key := e.AttrName + "-" + e.HrefFunc
			foundHref[key] = true
			want, ok := hrefCases[key]
			require.True(t, ok, "unexpected href context error: %s in %s", e.HrefFunc, e.AttrName)
			require.Equal(t, want.attrName, e.AttrName)
			require.Equal(t, want.hrefFunc, e.HrefFunc)
			require.Equal(t, want.line, pe.pos.Line, "wrong line for %s", key)
			require.Equal(t, want.col, pe.pos.Column, "wrong column for %s", key)
			require.ErrorIs(t, pe.err, templcheck.ErrHrefContext)
			continue
		}
		t.Errorf("unexpected error at %s: %v", pe.pos, pe.err)
	}
	require.Len(t, foundAction, len(actionCases))
	for key := range actionCases {
		require.Contains(t, foundAction, key)
	}
	require.Len(t, foundHref, len(hrefCases))
	for key := range hrefCases {
		require.Contains(t, foundHref, key)
	}
}

func TestCheck_ErrFormAction(t *testing.T) {
	errs := check(t, "err_templ_form_action", nil)

	type expectEntry struct {
		line, col int
	}
	expect := []expectEntry{
		{7, 8},
		{11, 17},
		{15, 17},
	}

	var got []expectEntry
	for _, pe := range errs {
		if !errors.Is(pe.err, templcheck.ErrFormAction) {
			t.Errorf("unexpected error at %s: %v", pe.pos, pe.err)
			continue
		}
		got = append(got, expectEntry{pe.pos.Line, pe.pos.Column})
	}
	require.Equal(t, expect, got)
}

func TestCheck_ErrHardcodedAction(t *testing.T) {
	errs := check(t, "err_templ_hardcoded_action", nil)

	hardcoded := templcheck.ErrHardcodedAction
	unverifiable := templcheck.ErrActionUnverifiable

	type expectEntry struct {
		sentinel  error
		val       string
		line, col int
	}
	expect := []expectEntry{
		{hardcoded, "/login/submit", 7, 10},
		{hardcoded, "/api/data", 9, 7},
		{hardcoded, "/profile/save", 11, 8},
		{hardcoded, "/resource", 13, 10},
		{hardcoded, "/resource", 15, 10},
		{hardcoded, "/resource", 17, 10},
		{hardcoded, "/lazy", 19, 7},
		{hardcoded, "/poll", 21, 7},
		{hardcoded, "/sync", 23, 7},
		{hardcoded, "/init", 25, 7},
		{hardcoded, "/custom", 27, 10},
		{hardcoded, "/mixed", 29, 10},
		{hardcoded, "/debounced", 31, 10},
		{hardcoded, "/intersect-once", 33, 7},
		{hardcoded, "/init-once", 35, 7},
		{hardcoded, "/expr-literal", 37, 26},
		{hardcoded, "/backtick", 39, 26},
		{hardcoded, "/const-action", 41, 26},
		{hardcoded, "/imported-action", 43, 26},
		{unverifiable, `"@post" + "('/concat')"`, 45, 26},
		{unverifiable, `buildAction()`, 47, 26},
		{unverifiable, `dynamicVar`, 49, 26},
	}

	var got []expectEntry
	for _, pe := range errs {
		if h, ok := errors.AsType[*templcheck.ErrorHardcodedAction](pe.err); ok {
			got = append(got,
				expectEntry{hardcoded, h.URL, pe.pos.Line, pe.pos.Column})
			continue
		}
		if u, ok := errors.AsType[*templcheck.ErrorActionUnverifiable](pe.err); ok {
			got = append(got,
				expectEntry{unverifiable, u.Expr, pe.pos.Line, pe.pos.Column})
			continue
		}
		t.Errorf("unexpected error at %s: %v", pe.pos, pe.err)
	}
	require.Equal(t, expect, got)
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
