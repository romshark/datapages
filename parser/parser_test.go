package parser_test

import (
	"datapages/parser"
	"datapages/parser/model"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
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

	// packages.Load typically reports parse/type errors here.
	// We don't assert exact error text because it varies across Go versions.
	var hasAny bool
	for _, e := range err.All() {
		if e != nil {
			hasAny = true
			break
		}
	}
	require.True(hasAny, "expected at least one error collected")
}

func TestParse_Minimal(t *testing.T) {
	app, err := parse(t, "minimal")
	require := require.New(t)
	require.Zero(err.Error())
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
	require.Zero(err.Error())
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

	require.Equal(2, err.Len())
	require.ErrorIs(err.At(0), parser.ErrMissingTypeApp)
	require.ErrorIs(err.At(1), parser.ErrMissingTypePageIndex)
}

func TestParse_ErrSignatures(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_signatures")
	require.NotZero(err.Error())

	require.Equal(5, err.Len())
	require.ErrorIs(err.At(0), parser.ErrMissingPageFieldApp)
	require.ErrorIs(err.At(1), parser.ErrMissingPageFieldApp)
	require.ErrorIs(err.At(2), parser.ErrSignatureMultiErrRet)
	require.ErrorIs(err.At(3), parser.ErrSignatureMissingReq)
	require.ErrorIs(err.At(4), parser.ErrPageMissingGET)
}
