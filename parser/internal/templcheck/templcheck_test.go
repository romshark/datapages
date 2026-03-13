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
	hardcodedAction := templcheck.ErrHardcodedAction
	unverifiable := templcheck.ErrHrefUnverifiable
	extInternal := templcheck.ErrExternalWithInternal
	expect := []expectEntry{
		{hardcodedAction, "/submit", 11, 8},
		{hardcoded, "/login", 34, 5},
		{hardcoded, "/profile", 36, 5},
		{hardcoded, "/static/style.css", 38, 5},
		{hardcoded, "/settings", 40, 12},
		{hardcoded, "/set", 42, 12},
		{unverifiable, `"/set" + dynamicValue`, 44, 12},
		{unverifiable, `templ.SafeURL("/about")`, 46, 12},
		{unverifiable, `templ.SafeURL(ConstantStringNOTOK)`, 48, 12},
		{unverifiable, `templ.SafeURL("https://data-star.dev")`, 50, 12},
		{hardcoded, "/c", 51, 12},
		{hardcoded, "notok", 52, 12},
		{hardcoded, "", 54, 5},
		{hardcoded, "?tab=settings", 56, 5},
		{hardcoded, "relative", 58, 5},
		{hardcoded, "javascript:void(0)", 60, 5},
		{hardcoded, "/nested", 64, 7},
		{unverifiable, `loginHref()`, 68, 12},
		{unverifiable, `someOtherFunc()`, 70, 12},
		{unverifiable, `buildURL(id)`, 72, 12},
		{unverifiable, `fmt.Sprintf("mailto:%s", "test@example.com")`, 74, 12},
		{extInternal, "/login", 76, 12},
		{extInternal, "/internal", 78, 12},
	}

	var got []expectEntry
	for _, pe := range errs {
		if h, ok := errors.AsType[*templcheck.ErrorHardcodedHref](pe.err); ok {
			got = append(got,
				expectEntry{hardcoded, h.URL, pe.pos.Line, pe.pos.Column})
			continue
		}
		if a, ok := errors.AsType[*templcheck.ErrorHardcodedAction](pe.err); ok {
			got = append(got,
				expectEntry{hardcodedAction, a.URL, pe.pos.Line, pe.pos.Column})
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

	type expectEntry struct {
		actionFunc string
		pageType   string
		ownerPage  string
		line       int
		col        int
	}
	expect := map[string]expectEntry{
		"POSTPageProfileSave-PageSettings": {
			actionFunc: "POSTPageProfileSave",
			pageType:   "PageSettings",
			ownerPage:  "PageProfile",
			line:       25,
			col:        17,
		},
	}

	found := map[string]bool{}
	for _, pe := range errs {
		var e *templcheck.ErrorActionWrongPage
		if !errors.As(pe.err, &e) {
			continue
		}
		key := e.ActionFunc + "-" + e.PageType
		found[key] = true
		want, ok := expect[key]
		require.True(t, ok, "unexpected cross-page action error: %s in %s", e.ActionFunc, e.PageType)
		require.Equal(t, want.actionFunc, e.ActionFunc)
		require.Equal(t, want.pageType, e.PageType)
		require.Equal(t, want.ownerPage, e.OwnerPage)
		require.Equal(t, want.line, pe.pos.Line, "wrong line for %s", key)
		require.Equal(t, want.col, pe.pos.Column, "wrong column for %s", key)
		require.ErrorIs(t, pe.err, templcheck.ErrActionWrongPage)
	}
	require.Len(t, found, len(expect))
	for key := range expect {
		require.Contains(t, found, key)
	}
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
		"action-POSTPageIndexSubmit": {
			attrName:   "action",
			actionFunc: "POSTPageIndexSubmit",
			line:       12,
			col:        17,
		},
		"data-only-POSTPageIndexSubmit": {
			attrName:   "data-only",
			actionFunc: "POSTPageIndexSubmit",
			line:       40,
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
			line:     30,
			col:      26,
		},
		"data-on:submit-PageIndex": {
			attrName: "data-on:submit",
			hrefFunc: "PageIndex",
			line:     32,
			col:      25,
		},
		"data-init-PageIndex": {
			attrName: "data-init",
			hrefFunc: "PageIndex",
			line:     34,
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
		// ErrHrefUnverifiable is expected for action.XXX() in href attrs
		// (checkHrefExpr also flags these).
		if errors.Is(pe.err, templcheck.ErrHrefUnverifiable) {
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

func TestCheck_OKHref(t *testing.T) {
	errs := check(t, "ok_templ_href", nil)
	for _, pe := range errs {
		t.Errorf("unexpected error at %s: %v", pe.pos, pe.err)
	}
	require.Empty(t, errs)
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
