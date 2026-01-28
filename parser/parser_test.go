package parser_test

import (
	"datapages/parser"
	"datapages/parser/model"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireExprLineCol(
	t *testing.T, app *model.App, e ast.Expr, wantFile string, wantLine, wantCol int,
) token.Position {
	t.Helper()
	p := app.Fset.Position(e.Pos())
	fName := filepath.Base(p.Filename)
	require.True(t, wantFile == fName && wantLine == p.Line && wantCol == p.Column,
		"expected %s:%d:%d; received %s:%d:%d",
		wantFile, wantLine, wantCol, fName, p.Line, p.Column)
	return p
}

func fixtureDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func parse(t *testing.T, fixtureName string) (*model.App, parser.Errors) {
	t.Helper()
	dir := fixtureDir(t, fixtureName)
	return parser.Parse(dir)
}

func requireParseErrors(t *testing.T, got parser.Errors, want ...error) {
	t.Helper()

	// Build pretty lists.
	wantLines := make([]string, 0, len(want))
	for i, w := range want {
		wantLines = append(wantLines, fmt.Sprintf("%2d) %s", i, errLabel(w)))
	}

	gotLines := make([]string, got.Len())
	for i := 0; i < got.Len(); i++ {
		pos, err := got.Entry(i)
		gotLines[i] = fmt.Sprintf("%2d) %s:%d:%d %s", i,
			pos.Filename, pos.Line, pos.Column, errLabel(err))
	}

	// Compare length first with a readable dump.
	if got.Len() != len(want) {
		require.Failf(t, "unexpected number of errors",
			"want=%d got=%d\n\nEXPECTED:\n%s\n\nACTUAL:\n%s\n",
			len(want), got.Len(),
			strings.Join(wantLines, "\n"),
			strings.Join(gotLines, "\n"),
		)
		return
	}

	// Per-index mismatch report.
	var mismatches []string
	for i, w := range want {
		_, a := got.Entry(i)
		if !errors.Is(a, w) {
			mismatches = append(mismatches, fmt.Sprintf(
				"%2d) want Is(%s) got %s",
				i, errLabel(w), errLabel(a),
			))
		}
	}
	if len(mismatches) > 0 {
		require.Failf(t, "error mismatch",
			"\nMISMATCHES:\n%s\n\nEXPECTED:\n%s\n\nACTUAL:\n%s\n",
			strings.Join(mismatches, "\n"),
			strings.Join(wantLines, "\n"),
			strings.Join(gotLines, "\n"),
		)
	}
}

func errLabel(err error) string {
	if err == nil {
		return "<nil>"
	}
	// Keep the concrete message, but also include the type for quick scanning.
	return fmt.Sprintf("%T: %q", err, err.Error())
}

func TestParse_SyntaxErr(t *testing.T) {
	require := require.New(t)

	tmp := t.TempDir()

	// Minimal module + package with a syntax error.
	require.NoError(os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(
		"module example.com/syntaxerr\n\ngo 1.22\n",
	), 0o644))

	require.NoError(os.WriteFile(filepath.Join(tmp, "app.go"), []byte(
		"package app\n\nfunc Broken( { }\n",
	), 0o644))

	app, err := parser.Parse(tmp)
	require.Nil(app)
	require.NotZero(err.Error())
	require.GreaterOrEqual(err.Len(), 1)
}

func TestParse_Minimal(t *testing.T) {
	app, err := parse(t, "minimal")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	{
		require.NotNil(app.PageIndex)
		p := app.PageIndex
		require.Equal("/", p.Route)
		require.NotNil(p.GET)
		require.Equal("GET", p.GET.HTTPMethod)

		require.Empty(p.Actions)
		require.Empty(p.EventHandlers)
	}
	require.Contains(app.Pages, app.PageIndex)
	require.Len(app.Pages, 1)

	require.Empty(app.Events)
	require.Nil(app.PageError404)
	require.Nil(app.PageError500)
	require.Nil(app.Recover500)
	require.Nil(app.GlobalHeadGenerator)
}

func TestParse_Basic(t *testing.T) {
	app, err := parse(t, "basic")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	{
		require.NotNil(app.PageIndex)
		requireExprLineCol(t, app, app.PageIndex.Expr, "app.go", 13, 6)
		p := app.PageIndex
		require.Equal("/", p.Route)
		require.NotNil(p.GET)
		require.Equal("GET", p.GET.HTTPMethod)

		require.Empty(p.Actions)
		require.Empty(p.EventHandlers)
		require.Empty(p.Embeds)
	}
	require.Contains(app.Pages, app.PageIndex)
	require.Len(app.Pages, 3)

	require.Empty(app.Events)
	{
		require.NotNil(app.GlobalHeadGenerator)
		requireExprLineCol(t, app, app.GlobalHeadGenerator, "app.go", 19, 13)
	}
	{
		require.NotNil(app.Recover500)
		requireExprLineCol(t, app, app.Recover500, "app.go", 23, 13)
	}
	{
		require.NotNil(app.PageError404)
		requireExprLineCol(t, app, app.PageError404.Expr, "app.go", 31, 6)
		require.Equal("/the-not-found-page", app.PageError404.Route)
		require.Empty(app.PageError404.EventHandlers)
		require.Empty(app.PageError404.Embeds)
		require.Empty(app.PageError404.Actions)
		requireExprLineCol(t, app, app.PageError404.GET.Expr, "app.go", 33, 21)
	}
	{
		require.NotNil(app.PageError500)
		requireExprLineCol(t, app, app.PageError500.Expr, "app.go", 38, 6)
		require.Equal("/the-internal-error-page", app.PageError500.Route)
		require.Empty(app.PageError500.EventHandlers)
		require.Empty(app.PageError500.Embeds)
		require.Empty(app.PageError500.Actions)
		requireExprLineCol(t, app, app.PageError500.GET.Expr, "app.go", 40, 21)
	}
}

func TestParse_MissingPageIndex(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_missing_essentials")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrAppMissingTypeApp,
		parser.ErrAppMissingPageIndex)
}

func TestParse_Errors(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "errors")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrSignatureMultiErrRet,
		parser.ErrPageMissingFieldApp,
		parser.ErrSignatureMissingReq,
		parser.ErrPageMissingGET,
		parser.ErrPageHasExtraFields,
		parser.ErrSignatureMissingReq,
		parser.ErrPageNameInvalid,
		parser.ErrPageNameInvalid,
		parser.ErrPageNameInvalid,
		parser.ErrPageNameInvalid,
		parser.ErrPageInvalidPathComm,
		parser.ErrPageMissingPathComm,
		parser.ErrPageMissingGET,
		parser.ErrActionMissingPathComm,
		parser.ErrActionNameInvalid,
		parser.ErrActionInvalidPathComm,
		parser.ErrActionPathNotUnderPage,
	)
}
