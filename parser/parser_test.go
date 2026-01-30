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

const TypeNameTemplComponent = "github.com/a-h/templ.Component"

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
		require.NotNil(p.GET.Handler)
		require.Equal("GET", p.GET.Handler.HTTPMethod)

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
		requireExprLineCol(t, app, app.PageIndex.Expr, "app.go", 14, 6)
		p := app.PageIndex
		require.Equal("/", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.Handler)
		require.Equal("GET", p.GET.Handler.HTTPMethod)

		require.Empty(p.Actions)
		require.Empty(p.EventHandlers)
		require.Empty(p.Embeds)
	}
	require.Contains(app.Pages, app.PageIndex)
	require.Len(app.Pages, 4)

	require.Empty(app.Events)
	{
		require.NotNil(app.GlobalHeadGenerator)
		requireExprLineCol(t, app, app.GlobalHeadGenerator, "app.go", 26, 13)
	}
	{
		require.NotNil(app.Recover500)
		requireExprLineCol(t, app, app.Recover500, "app.go", 30, 13)
	}
	{
		p := app.Pages[3]
		require.NotNil(p)
		require.Equal("PageIndex", p.TypeName)
		require.Equal("/", p.Route)
		requireExprLineCol(t, app, p.Expr, "app.go", 14, 6)
		require.Empty(p.EventHandlers)
		require.Empty(p.Embeds)
		require.Empty(p.Actions)
		require.Equal(model.PageTypeIndex, p.PageSpecialization)
		{
			get := p.GET
			require.NotNil(get.Handler)
			requireExprLineCol(t, app, get.Handler.Expr, "app.go", 16, 18)
			require.NotNil(get.Handler.InputRequest)
			require.Equal("r", get.Handler.InputRequest.Name)
			require.Equal("err", get.Handler.OutputErr.Name)
			require.Equal("error", get.Handler.OutputErr.Type.Resolved.String())
			require.NotNil(get.OutputBody)
			require.Equal("body", get.OutputBody.Output.Name)
		}
	}
	{
		require.NotNil(app.PageError404)
		requireExprLineCol(t, app, app.PageError404.Expr, "app.go", 38, 6)
		require.Equal("/the-not-found-page", app.PageError404.Route)
		require.NotNil(app.PageError404.GET.Handler)
		require.Equal("r", app.PageError404.GET.Handler.InputRequest.Name)
		require.Empty(app.PageError404.EventHandlers)
		require.Empty(app.PageError404.Embeds)
		require.Empty(app.PageError404.Actions)
		require.Equal(model.PageTypeError404, app.PageError404.PageSpecialization)
		{
			get := app.PageError404.GET
			require.NotNil(get.Handler)
			requireExprLineCol(t, app, get.Handler.Expr, "app.go", 40, 21)
			require.NotNil(get.Handler.InputRequest)
			require.Equal("r", get.Handler.InputRequest.Name)
			require.Equal("err", get.Handler.OutputErr.Name)
			require.Equal("error", get.Handler.OutputErr.Type.Resolved.String())
			require.NotNil(get.OutputBody)
			require.Equal("body", get.OutputBody.Output.Name)
		}
	}
	{
		require.NotNil(app.PageError500)
		requireExprLineCol(t, app, app.PageError500.Expr, "app.go", 45, 6)
		require.Equal("/the-internal-error-page", app.PageError500.Route)
		require.Empty(app.PageError500.EventHandlers)
		require.Empty(app.PageError500.Embeds)
		require.Empty(app.PageError500.Actions)
		require.Equal(model.PageTypeError500, app.PageError500.PageSpecialization)
		{
			get := app.PageError500.GET
			require.NotNil(get.Handler)
			requireExprLineCol(t, app, get.Handler.Expr, "app.go", 47, 21)
			require.NotNil(get.Handler.InputRequest)
			require.Equal("r", get.Handler.InputRequest.Name)
			require.Equal("err", get.Handler.OutputErr.Name)
			require.Equal("error", get.Handler.OutputErr.Type.Resolved.String())
			require.NotNil(get.OutputBody)
			require.Equal("body", get.OutputBody.Output.Name)
		}
	}
	{
		p := app.Pages[2]
		require.NotNil(p)
		require.Equal("PageExample", p.TypeName)
		require.Equal("/example", p.Route)
		requireExprLineCol(t, app, p.Expr, "app.go", 52, 6)
		require.Empty(p.EventHandlers)
		require.Empty(p.Embeds)
		require.Empty(p.Actions)
		require.Zero(p.PageSpecialization)
		require.NotNil(p.GET)
		require.NotNil(p.GET.Handler)
		requireExprLineCol(t, app, p.GET.Handler.Expr, "app.go", 54, 20)
		require.NotNil(p.GET.OutputBody)
		require.Equal("body", p.GET.OutputBody.Output.Name)
		require.Equal(TypeNameTemplComponent,
			p.GET.OutputBody.Output.Type.Resolved.String())
		require.NotNil(p.GET.OutputHead)
		require.Equal("head", p.GET.OutputHead.Output.Name)
		require.Equal(TypeNameTemplComponent,
			p.GET.OutputHead.Output.Type.Resolved.String())
	}
}

func TestParse_Embed(t *testing.T) {
	app, err := parse(t, "embed")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)

	require.NotNil(app)

	// PageConcrete
	// - Own: OnC
	// - Level2: OnB
	// - Level1: OnA
	{
		p := findPage(app, "PageConcrete")
		require.NotNil(p)

		// Ensure exact set of handlers.
		handlerNames := getHandlerNames(p.EventHandlers)
		require.ElementsMatch([]string{"C", "B", "A"}, handlerNames)

		// Should have inherited GET
		require.NotNil(p.GET)
		// Should have no other actions
		require.Empty(p.Actions)
	}

	// PageOverride
	// - Own: OnA (override)
	// - Level1: OnA (shadowed) -> we expect only 1 handler for A
	{
		p := findPage(app, "PageOverride")
		require.NotNil(p)

		// Ensure exact set of handlers.
		handlerNames := getHandlerNames(p.EventHandlers)
		require.ElementsMatch([]string{"A"}, handlerNames)

		// Should have its own GET
		require.NotNil(p.GET)
		require.Empty(p.Actions)
	}

	// PageOverrideEvent
	// - Own: OnNewA (handles A)
	// - Level1: OnA (handles A) -> shadowed by Event Type
	{
		p := findPage(app, "PageOverrideEvent")
		require.NotNil(p)

		// Expectation: logic says "handledEvents[h.EventTypeName]".
		// OnNewA handles A. So OnA should NOT be imported.
		handlerNames := getHandlerNames(p.EventHandlers)
		require.ElementsMatch([]string{"NewA"}, handlerNames)

		require.NotNil(p.GET)
		require.Empty(p.Actions)
	}

	// PageMulti
	// - Level1: OnA
	// - Level3: OnD
	{
		p := findPage(app, "PageMulti")
		require.NotNil(p)

		handlerNames := getHandlerNames(p.EventHandlers)
		require.ElementsMatch([]string{"A", "D"}, handlerNames)

		require.NotNil(p.GET)
		require.Empty(p.Actions)
	}
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

	p := parser.New()
	app, err := p.Parse(tmp)
	require.Nil(app)
	require.NotZero(err.Error())
	require.GreaterOrEqual(err.Len(), 1)
}

func TestParse_ErrMissingPageIndex(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_missing_essentials")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrAppMissingTypeApp,
		parser.ErrAppMissingPageIndex)
}

func TestParse_ErrPages(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_pages")
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

func TestParse_ErrEvents(t *testing.T) {
	_, err := parse(t, "err_events")
	require.NotZero(t, err.Error())

	requireParseErrors(t, err,
		parser.ErrEventCommMissing,
		parser.ErrEventSubjectInvalid,
		parser.ErrEvHandFirstArgNotEvent,
		parser.ErrEvHandFirstArgTypeNotEvent,
		parser.ErrEvHandDuplicate,
		parser.ErrEventFieldUnexported,
		parser.ErrEventFieldMissingTag,
		parser.ErrEventFieldUnexported,
		parser.ErrEventCommInvalid,
		parser.ErrEventCommInvalid,
		parser.ErrEventSubjectInvalid,
		parser.ErrEventSubjectInvalid,
	)
}

func TestParse_ErrEventHandler(t *testing.T) {
	_, err := parse(t, "err_event_handler")
	require.NotZero(t, err.Error())

	requireParseErrors(t, err,
		parser.ErrEvHandReturnMustBeError,
		parser.ErrEvHandReturnMustBeError,
		parser.ErrEvHandReturnMustBeError,
		parser.ErrEvHandReturnMustBeError,
	)
}

func TestParse_ErrEmbedDuplicateEventHandler(t *testing.T) {
	_, err := parse(t, "err_embed_duplicate_event_handler")
	require.NotZero(t, err.Error())

	requireParseErrors(t, err,
		parser.ErrEvHandDuplicateEmbed,
	)
}

func TestParse_ErrEmbedConflictingGET(t *testing.T) {
	_, err := parse(t, "err_embed_conflicting_get")
	require.NotZero(t, err.Error())

	requireParseErrors(t, err,
		parser.ErrPageConflictingGETEmbed,
	)

	pos, _ := err.Entry(0)
	requirePosEqual(t, "app.go", 11, 2, pos)
}

func requireExprLineCol(
	t *testing.T, app *model.App, e ast.Expr, wantFile string, wantLine, wantCol int,
) token.Position {
	t.Helper()
	p := app.Fset.Position(e.Pos())
	requirePosEqual(t, wantFile, wantLine, wantCol, p)
	return p
}

func requirePosEqual(
	t *testing.T, wantFile string, wantLine, wantCol int, p token.Position,
) {
	t.Helper()
	fName := filepath.Base(p.Filename)
	require.True(t, wantFile == fName && wantLine == p.Line && wantCol == p.Column,
		"expected %s:%d:%d; received %s:%d:%d",
		wantFile, wantLine, wantCol, fName, p.Line, p.Column)
}

func fixtureDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func parse(t *testing.T, fixtureName string) (*model.App, parser.Errors) {
	t.Helper()
	dir := fixtureDir(t, fixtureName)
	p := parser.New()
	return p.Parse(dir)
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

func getHandlerNames(hs []*model.EventHandler) []string {
	names := make([]string, 0, len(hs))
	for _, h := range hs {
		names = append(names, h.Name)
	}
	return names
}

func findPage(app *model.App, name string) *model.Page {
	for _, p := range app.Pages {
		if p.TypeName == name {
			return p
		}
	}
	return nil
}
