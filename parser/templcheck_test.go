package parser_test

import (
	"errors"
	"testing"

	"github.com/romshark/datapages/parser"

	"github.com/stretchr/testify/require"
)

func TestCheckTemplFiles_HardcodedHref(t *testing.T) {
	errs := parser.CheckTemplFiles(fixtureDir(t, "templ_hardcoded_href"))
	require.Equal(t, 3, errs.Len())

	expect := map[string]error{
		"/login":   parser.ErrTemplHardcodedHref,
		"/profile": parser.ErrTemplHardcodedHref,
		"/submit":  parser.ErrTemplHardcodedAction,
	}

	for i, err := range errs.All() {
		pos, inner := errs.Entry(i)
		require.Equal(t, "app.templ", pos.Filename)

		var h *parser.ErrorTemplHardcodedHref
		var a *parser.ErrorTemplHardcodedAction
		switch {
		case errors.As(inner, &h):
			sentinel, ok := expect[h.URL]
			require.True(t, ok, "unexpected URL: %s", h.URL)
			require.ErrorIs(t, err, sentinel)
			delete(expect, h.URL)
		case errors.As(inner, &a):
			sentinel, ok := expect[a.URL]
			require.True(t, ok, "unexpected URL: %s", a.URL)
			require.ErrorIs(t, err, sentinel)
			delete(expect, a.URL)
		default:
			t.Fatalf("unexpected error type: %T", inner)
		}
	}
	require.Empty(t, expect, "not all expected errors were found")
}

func TestCheckTemplFiles_OK(t *testing.T) {
	errs := parser.CheckTemplFiles(fixtureDir(t, "templ_ok"))
	require.Equal(t, 0, errs.Len())
}

func TestCheckTemplFiles_Nolint(t *testing.T) {
	errs := parser.CheckTemplFiles(fixtureDir(t, "templ_nolint"))
	// Only the last href should error; the nolint suppresses the element after it.
	require.Equal(t, 1, errs.Len())

	_, inner := errs.Entry(0)
	var h *parser.ErrorTemplHardcodedHref
	require.ErrorAs(t, inner, &h)
	require.Equal(t, "/should-error", h.URL)
}

func TestCheckTemplFiles_NoDir(t *testing.T) {
	errs := parser.CheckTemplFiles(fixtureDir(t, "nonexistent"))
	require.Equal(t, 0, errs.Len())
}
