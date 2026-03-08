package parser_test

import (
	"errors"
	"testing"

	"github.com/romshark/datapages/parser"

	"github.com/stretchr/testify/require"
)

func TestParse_ErrTemplHardcodedHref(t *testing.T) {
	_, errs := parse(t, "err_templ_hardcoded_href")

	expect := map[string]error{
		"/login":   parser.ErrTemplHardcodedHref,
		"/profile": parser.ErrTemplHardcodedHref,
		"/submit":  parser.ErrTemplHardcodedAction,
	}

	found := map[string]bool{}
	for i := range errs.Len() {
		_, inner := errs.Entry(i)
		var h *parser.ErrorTemplHardcodedHref
		var a *parser.ErrorTemplHardcodedAction
		switch {
		case errors.As(inner, &h):
			found[h.URL] = true
			sentinel, ok := expect[h.URL]
			require.True(t, ok, "unexpected href URL: %s", h.URL)
			require.ErrorIs(t, inner, sentinel)
		case errors.As(inner, &a):
			found[a.URL] = true
			sentinel, ok := expect[a.URL]
			require.True(t, ok, "unexpected action URL: %s", a.URL)
			require.ErrorIs(t, inner, sentinel)
		}
	}
	for url := range expect {
		require.True(t, found[url], "expected error for URL %q not found", url)
	}
}

func TestParse_ErrTemplActionWrongPage(t *testing.T) {
	_, errs := parse(t, "err_templ_action_not_on_page")

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
	}
	expect := map[string]expectEntry{
		"POSTPageProfileSave-PageSettings": {
			actionFunc: "POSTPageProfileSave",
			pageType:   "PageSettings",
			ownerPage:  "PageProfile",
		},
	}

	found := map[string]bool{}
	for i := range errs.Len() {
		_, inner := errs.Entry(i)
		var e *parser.ErrorTemplActionWrongPage
		if !errors.As(inner, &e) {
			continue
		}
		key := e.ActionFunc + "-" + e.PageType
		found[key] = true
		want, ok := expect[key]
		require.True(t, ok, "unexpected cross-page action error: %s in %s", e.ActionFunc, e.PageType)
		require.Equal(t, want.actionFunc, e.ActionFunc)
		require.Equal(t, want.pageType, e.PageType)
		require.Equal(t, want.ownerPage, e.OwnerPage)
		require.ErrorIs(t, inner, parser.ErrTemplActionWrongPage)
	}
	for key := range expect {
		require.True(t, found[key], "expected error for %q not found", key)
	}
}
