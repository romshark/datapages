package parser_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/model"

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
		requireExprLineCol(t, app, app.PageIndex.Expr, "app.go", 14, 6)
		p := app.PageIndex
		require.Equal("/", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.Handler)
		require.Equal("GET", p.GET.HTTPMethod)

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
			requireExprLineCol(t, app, get.Expr, "app.go", 16, 18)
			require.NotNil(get.InputRequest)
			require.Equal("r", get.InputRequest.Name)
			require.Equal("err", get.OutputErr.Name)
			require.Equal("error", get.OutputErr.Type.Resolved.String())
			require.NotNil(get.OutputBody)
			require.Equal("body", get.OutputBody.Name)
		}
	}
	{
		require.NotNil(app.PageError404)
		requireExprLineCol(t, app, app.PageError404.Expr, "app.go", 38, 6)
		require.Equal("/the-not-found-page", app.PageError404.Route)
		require.NotNil(app.PageError404.GET.Handler)
		require.Equal("r", app.PageError404.GET.InputRequest.Name)
		require.Empty(app.PageError404.EventHandlers)
		require.Empty(app.PageError404.Embeds)
		require.Empty(app.PageError404.Actions)
		require.Equal(model.PageTypeError404, app.PageError404.PageSpecialization)
		{
			get := app.PageError404.GET
			require.NotNil(get.Handler)
			requireExprLineCol(t, app, get.Expr, "app.go", 40, 21)
			require.NotNil(get.InputRequest)
			require.Equal("r", get.InputRequest.Name)
			require.Equal("err", get.OutputErr.Name)
			require.Equal("error", get.OutputErr.Type.Resolved.String())
			require.NotNil(get.OutputBody)
			require.Equal("body", get.OutputBody.Name)
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
			requireExprLineCol(t, app, get.Expr, "app.go", 47, 21)
			require.NotNil(get.InputRequest)
			require.Equal("r", get.InputRequest.Name)
			require.Equal("err", get.OutputErr.Name)
			require.Equal("error", get.OutputErr.Type.Resolved.String())
			require.NotNil(get.OutputBody)
			require.Equal("body", get.OutputBody.Name)
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
		requireExprLineCol(t, app, p.GET.Expr, "app.go", 54, 20)
		require.NotNil(p.GET.OutputBody)
		require.Equal("body", p.GET.OutputBody.Name)
		require.Equal(TypeNameTemplComponent,
			p.GET.OutputBody.Type.Resolved.String())
		require.NotNil(p.GET.OutputHead)
		require.Equal("head", p.GET.OutputHead.Name)
		require.Equal(TypeNameTemplComponent,
			p.GET.OutputHead.Type.Resolved.String())
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

func TestParse_ActionHandlerSSE(t *testing.T) {
	app, err := parse(t, "action_handler")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	{ // Verify PageIndex - GET without SSE
		require.NotNil(app.PageIndex)
		p := app.PageIndex
		require.Equal("/", p.Route)
		require.NotNil(p.GET)
		require.Nil(p.GET.InputSSE)
	}

	{ // Verify PageActions has action handlers with and without SSE
		p := findPage(app, "PageActions")
		require.NotNil(p)
		require.Equal("/actions", p.Route)
		require.Len(p.Actions, 4)
		require.Len(p.EventHandlers, 1)

		// POST without SSE
		actionWithout := findAction(p.Actions, "WithoutSSE")
		require.NotNil(actionWithout)
		require.Equal("POST", actionWithout.HTTPMethod)
		require.Nil(actionWithout.InputSSE)

		// POST with SSE
		actionWith := findAction(p.Actions, "WithSSE")
		require.NotNil(actionWith)
		require.Equal("POST", actionWith.HTTPMethod)
		require.NotNil(actionWith.InputSSE)
		require.Equal("sse", actionWith.InputSSE.Name)

		// PUT with SSE
		putWith := findActionByMethod(p.Actions, "PUT", "WithSSE")
		require.NotNil(putWith)
		require.Equal("PUT", putWith.HTTPMethod)
		require.NotNil(putWith.InputSSE)

		// DELETE without SSE
		deleteWithout := findActionByMethod(p.Actions, "DELETE", "WithoutSSE")
		require.NotNil(deleteWithout)
		require.Equal("DELETE", deleteWithout.HTTPMethod)
		require.Nil(deleteWithout.InputSSE)

		// Event handler MUST have SSE
		evHandler := p.EventHandlers[0]
		require.Equal("EventFoo", evHandler.Name)
		require.NotNil(evHandler.InputSSE)
		require.Equal("sse", evHandler.InputSSE.Name)
	}
}

func TestParse_SyntaxErr(t *testing.T) {
	require := require.New(t)

	tmp := t.TempDir()

	// Minimal module + package with a syntax error.
	err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(
		"module example.com/syntaxerr\n\ngo 1.22\n",
	), 0o644)
	require.NoError(err)

	err = os.WriteFile(filepath.Join(tmp, "app.go"), []byte(
		"package app\n\nfunc Broken( { }\n",
	), 0o644)
	require.NoError(err)

	require.NoError(err)
	app, errs := parser.Parse(tmp)
	require.Nil(app)
	require.NotZero(errs.Error())
	require.GreaterOrEqual(errs.Len(), 1)
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

func TestParse_ErrGET(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_get")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrSignatureMissingReq,
		parser.ErrSignatureMultiErrRet,
		parser.ErrSignatureUnknownInput,
		parser.ErrSignatureGETMissingBody,
		parser.ErrSignatureGETBodyWrongName,
		parser.ErrSignatureGETHeadWrongName,
	)
}

func TestParse_ErrEvents(t *testing.T) {
	_, err := parse(t, "err_events")
	require.NotZero(t, err.Error())

	requireParseErrors(t, err,
		parser.ErrEventCommMissing,
		parser.ErrEventSubjectInvalid,
		parser.ErrSignatureEvHandFirstArgNotEvent,
		parser.ErrSignatureSecondArgNotSSE,
		parser.ErrSignatureEvHandFirstArgTypeNotEvent,
		parser.ErrSignatureSecondArgNotSSE,
		parser.ErrSignatureSecondArgNotSSE,
		parser.ErrEvHandDuplicate,
		parser.ErrSignatureSecondArgNotSSE,
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
		parser.ErrSignatureSecondArgNotSSE,
		parser.ErrSignatureUnknownInput,
		parser.ErrSignatureSecondArgNotSSE,
		parser.ErrSignatureEvHandReturnMustBeError,
		parser.ErrSignatureEvHandReturnMustBeError,
		parser.ErrSignatureEvHandReturnMustBeError,
		parser.ErrSignatureEvHandReturnMustBeError,
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

	requireParseErrors(t, err, parser.ErrPageConflictingGETEmbed)

	pos, _ := err.Entry(0)
	requirePosEqual(t, "app.go", 15, 2, pos)
}

func TestParse_Path(t *testing.T) {
	app, err := parse(t, "path")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	// PageIndex - no path param
	{
		require.NotNil(app.PageIndex)
		require.Nil(app.PageIndex.GET.InputPath)
	}

	// PageItem - GET with path struct
	{
		p := findPage(app, "PageItem")
		require.NotNil(p)
		require.Equal("/item/{id}", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputPath)
		require.Equal("path", p.GET.InputPath.Name)

		// Action with path struct
		require.Len(p.Actions, 1)
		action := p.Actions[0]
		require.Equal("POST", action.HTTPMethod)
		require.NotNil(action.InputPath)
		require.Equal("path", action.InputPath.Name)
	}
}

func TestParse_ErrPath(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_path")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrPathParamNotStruct,
		parser.ErrPathFieldUnexported,
		parser.ErrPathFieldNotString,
		parser.ErrPathFieldMissingTag,
		parser.ErrPathFieldNotInRoute,
		parser.ErrPathMissingRouteVar,
	)
}

func TestParse_Query(t *testing.T) {
	app, err := parse(t, "query")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	// PageIndex - no query param
	{
		require.NotNil(app.PageIndex)
		require.Nil(app.PageIndex.GET.InputQuery)
	}

	// PageSearch - GET with query struct (mixed types)
	{
		p := findPage(app, "PageSearch")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputQuery)
		require.Equal("query", p.GET.InputQuery.Name)

		// Action with query struct
		require.Len(p.Actions, 1)
		action := p.Actions[0]
		require.Equal("POST", action.HTTPMethod)
		require.NotNil(action.InputQuery)
		require.Equal("query", action.InputQuery.Name)
	}
}

func TestParse_ErrQuery(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_query")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrQueryParamNotStruct,
		parser.ErrQueryFieldUnexported,
		parser.ErrQueryFieldMissingTag,
	)
}

func TestParse_Signals(t *testing.T) {
	app, err := parse(t, "signals")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	// PageIndex - no signals
	{
		require.NotNil(app.PageIndex)
		require.Nil(app.PageIndex.GET.InputSignals)
	}

	// PageForm - action with signals
	{
		p := findPage(app, "PageForm")
		require.NotNil(p)
		require.Nil(p.GET.InputSignals)
		require.Len(p.Actions, 1)
		action := p.Actions[0]
		require.NotNil(action.InputSignals)
		require.Equal("signals", action.InputSignals.Name)
	}

	// PageSearch - GET with query + signals + reflectsignal
	{
		p := findPage(app, "PageSearch")
		require.NotNil(p)
		require.NotNil(p.GET.InputQuery)
		require.NotNil(p.GET.InputSignals)
		require.Equal("signals", p.GET.InputSignals.Name)

		// Action with both query and signals
		require.Len(p.Actions, 1)
		action := p.Actions[0]
		require.NotNil(action.InputQuery)
		require.NotNil(action.InputSignals)
	}
}

func TestParse_Dispatch(t *testing.T) {
	app, err := parse(t, "dispatch")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	p := app.PageIndex
	require.NotNil(p)
	require.Len(p.Actions, 3)

	// POSTSingle - single event dispatch
	{
		a := findAction(p.Actions, "Single")
		require.NotNil(a)
		require.Equal("POST", a.HTTPMethod)
		require.NotNil(a.InputDispatch)
		require.Equal("dispatch", a.InputDispatch.Name)
		require.Equal(
			[]string{"EventFoo"},
			a.InputDispatch.EventTypeNames,
		)
	}

	// POSTMulti - multi event dispatch
	{
		a := findAction(p.Actions, "Multi")
		require.NotNil(a)
		require.Equal("POST", a.HTTPMethod)
		require.NotNil(a.InputDispatch)
		require.Equal(
			[]string{"EventFoo", "EventBar"},
			a.InputDispatch.EventTypeNames,
		)
	}

	// POSTWithSignals - signals before dispatch
	{
		a := findAction(p.Actions, "WithSignals")
		require.NotNil(a)
		require.Equal("POST", a.HTTPMethod)
		require.NotNil(a.InputSignals)
		require.NotNil(a.InputDispatch)
		require.Equal(
			[]string{"EventFoo"},
			a.InputDispatch.EventTypeNames,
		)
	}
}

func TestParse_ErrDispatch(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_dispatch")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrDispatchParamNotFunc,
		parser.ErrDispatchReturnCount,
		parser.ErrDispatchMustReturnError,
		parser.ErrDispatchNoParams,
		parser.ErrDispatchParamNotEvent,
	)
}

func TestParse_Session(t *testing.T) {
	app, err := parse(t, "session")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)
	require.NotNil(app.Session)

	// PageIndex - no session
	{
		p := app.PageIndex
		require.NotNil(p)
		require.Nil(p.GET.InputSession)
	}

	// PageProfile - GET with session (no sessionToken)
	{
		p := findPage(app, "PageProfile")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.Nil(p.GET.InputSessionToken)
		require.NotNil(p.GET.InputSession)
		require.Equal("session", p.GET.InputSession.Name)

		// POSTUpdate - action with session
		update := findAction(p.Actions, "Update")
		require.NotNil(update)
		require.Nil(update.InputSessionToken)
		require.NotNil(update.InputSession)
		require.Equal("session", update.InputSession.Name)
		require.Nil(update.InputSSE)

		// POSTNotify - action with SSE + session
		notify := findAction(p.Actions, "Notify")
		require.NotNil(notify)
		require.NotNil(notify.InputSSE)
		require.Nil(notify.InputSessionToken)
		require.NotNil(notify.InputSession)

		// Event handler with session
		require.Len(p.EventHandlers, 1)
		evh := p.EventHandlers[0]
		require.Nil(evh.InputSessionToken)
		require.NotNil(evh.InputSession)
		require.Equal("session", evh.InputSession.Name)
	}

	// PageSettings - sessionToken + session
	{
		p := findPage(app, "PageSettings")
		require.NotNil(p)

		// GET with sessionToken and session.
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputSessionToken)
		require.Equal("sessionToken", p.GET.InputSessionToken.Name)
		require.NotNil(p.GET.InputSession)

		// POSTClose - action with sessionToken + session
		close := findAction(p.Actions, "Close")
		require.NotNil(close)
		require.NotNil(close.InputSessionToken)
		require.Equal("sessionToken", close.InputSessionToken.Name)
		require.NotNil(close.InputSession)

		// Event handler with sessionToken + session
		require.Len(p.EventHandlers, 1)
		evh := p.EventHandlers[0]
		require.NotNil(evh.InputSessionToken)
		require.Equal("sessionToken", evh.InputSessionToken.Name)
		require.NotNil(evh.InputSession)
	}
}

func TestParse_ErrSession(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_session")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrSessionMissingUserID,
		parser.ErrSessionParamNotSessionType,
		parser.ErrSessionTokenParamNotString,
	)
}

func TestParse_ErrSessionWrongType(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_session_wrong_type")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrSessionNotStruct,
	)
}

func TestParse_Redirect(t *testing.T) {
	app, err := parse(t, "redirect")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	// PageIndex - no redirect
	{
		p := app.PageIndex
		require.NotNil(p)
		require.Nil(p.GET.OutputRedirect)
		require.Nil(p.GET.OutputRedirectStatus)
	}

	// PageLogin - GET with redirect only
	{
		p := findPage(app, "PageLogin")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputRedirect)
		require.Equal("redirect", p.GET.OutputRedirect.Name)
		require.Nil(p.GET.OutputRedirectStatus)

		// POSTSignIn - action with redirect + redirectStatus
		require.Len(p.Actions, 1)
		a := p.Actions[0]
		require.NotNil(a.OutputRedirect)
		require.Equal("redirect", a.OutputRedirect.Name)
		require.NotNil(a.OutputRedirectStatus)
		require.Equal("redirectStatus", a.OutputRedirectStatus.Name)
	}
}

func TestParse_ErrRedirect(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_redirect")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrRedirectNotString,
		parser.ErrRedirectStatusNotInt,
		parser.ErrRedirectStatusWithoutRedirect,
	)
}

func TestParse_SessionOutput(t *testing.T) {
	app, err := parse(t, "session_output")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	// PageIndex - no newSession or closeSession
	{
		p := app.PageIndex
		require.NotNil(p)
		require.Nil(p.GET.OutputNewSession)
		require.Nil(p.GET.OutputCloseSession)
	}

	// PageLogin - GET with newSession
	{
		p := findPage(app, "PageLogin")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputNewSession)
		require.Equal("newSession", p.GET.OutputNewSession.Name)
		require.Nil(p.GET.OutputCloseSession)

		// POSTSubmit - action with newSession
		submit := findAction(p.Actions, "Submit")
		require.NotNil(submit)
		require.NotNil(submit.OutputNewSession)
		require.Equal("newSession", submit.OutputNewSession.Name)
		require.NotNil(submit.OutputRedirect)
		require.Nil(submit.OutputCloseSession)

		// POSTSignOut - action with closeSession
		signOut := findAction(p.Actions, "SignOut")
		require.NotNil(signOut)
		require.NotNil(signOut.OutputCloseSession)
		require.Equal("closeSession", signOut.OutputCloseSession.Name)
		require.NotNil(signOut.OutputRedirect)
		require.Nil(signOut.OutputNewSession)
	}
}

func TestParse_ErrSessionOutput(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_session_output")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrNewSessionNotSessionType,
		parser.ErrCloseSessionNotBool,
		parser.ErrNewSessionWithSSE,
		parser.ErrCloseSessionWithSSE,
	)
}

func TestParse_GETOptions(t *testing.T) {
	app, err := parse(t, "get_options")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	{ // PageIndex - no GET options
		p := app.PageIndex
		require.NotNil(p)
		require.Nil(p.GET.OutputEnableBgStream)
		require.Nil(p.GET.OutputDisableRefresh)
	}

	{ // PageStream - enableBackgroundStreaming
		p := findPage(app, "PageStream")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputEnableBgStream)
		require.Equal("enableBackgroundStreaming", p.GET.OutputEnableBgStream.Name)
		require.Nil(p.GET.OutputDisableRefresh)
	}

	{ // PageNoRefresh - disableRefreshAfterHidden
		p := findPage(app, "PageNoRefresh")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.Nil(p.GET.OutputEnableBgStream)
		require.NotNil(p.GET.OutputDisableRefresh)
		require.Equal(
			"disableRefreshAfterHidden",
			p.GET.OutputDisableRefresh.Name,
		)
	}
}

func TestParse_ErrGETOptions(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_get_options")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrEnableBgStreamNotGET,
		parser.ErrDisableRefreshNotGET,
		parser.ErrEnableBgStreamNotBool,
		parser.ErrDisableRefreshNotBool,
	)
}

func TestParse_ErrSignals(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_signals")
	require.NotZero(err.Error())

	requireParseErrors(t, err,
		parser.ErrSignalsParamNotStruct,
		parser.ErrSignalsFieldUnexported,
		parser.ErrSignalsFieldMissingTag,
		parser.ErrQueryReflectSignalNotInSignals,
	)
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
			mismatches = append(mismatches, fmt.Sprintf("%2d) want Is(%s) got %s",
				i, errLabel(w), errLabel(a)))
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

func findAction(actions []*model.Handler, nameSuffix string) *model.Handler {
	for _, a := range actions {
		if strings.HasSuffix(a.Name, nameSuffix) {
			return a
		}
	}
	return nil
}

func findActionByMethod(actions []*model.Handler, method, nameSuffix string) *model.Handler {
	for _, a := range actions {
		if a.HTTPMethod == method && strings.HasSuffix(a.Name, nameSuffix) {
			return a
		}
	}
	return nil
}

func findEventHandler(
	hs []*model.EventHandler, name string,
) *model.EventHandler {
	for _, h := range hs {
		if h.Name == name {
			return h
		}
	}
	return nil
}

func TestParse_ExampleClassifieds(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "example", "classifieds", "app"),
	)
	require := require.New(t)
	// Known parser limitations: named types for query/signals
	// params and signals params in event handlers are not
	// yet supported.
	requireParseErrors(t, errs,
		parser.ErrSignatureUnknownInput, // OnMessagingRead signals
		parser.ErrSignatureUnknownInput, // OnMessagingSent signals
		parser.ErrQueryParamNotStruct,   // PageSearch.GET
		parser.ErrSignalsParamNotStruct, // PageSearch.POSTParamChange
	)
	require.NotNil(app)

	// App-level features
	require.NotNil(app.GlobalHeadGenerator)
	require.NotNil(app.Recover500)
	require.NotNil(app.Session)

	// Events (sorted alphabetically by type name)
	require.Len(app.Events, 6)
	events := map[string]*model.Event{}
	for _, e := range app.Events {
		events[e.TypeName] = e
	}
	for name, tc := range map[string]struct {
		subject          string
		hasTargetUserIDs bool
	}{
		"EventMessagingRead":           {"messaging.read", true},
		"EventMessagingSent":           {"messaging.sent", true},
		"EventMessagingWriting":        {"messaging.writing", true},
		"EventMessagingWritingStopped": {"messaging.writing-stopped", true},
		"EventPostArchived":            {"posts.archived", false},
		"EventSessionClosed":           {"sessions.closed", true},
	} {
		e, ok := events[name]
		require.True(ok, "missing event: %s", name)
		require.Equal(tc.subject, e.Subject, "event %s subject", name)
		require.Equal(tc.hasTargetUserIDs, e.HasTargetUserIDs,
			"event %s HasTargetUserIDs", name)
	}

	// Pages (sorted alphabetically)
	require.Len(app.Pages, 10)

	// Special pages
	require.NotNil(app.PageIndex)
	require.NotNil(app.PageError404)
	require.NotNil(app.PageError500)

	// PageError404
	{
		p := app.PageError404
		require.Equal("PageError404", p.TypeName)
		require.Equal("/not-found", p.Route)
		require.Equal(model.PageTypeError404, p.PageSpecialization)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.Equal("body", p.GET.OutputBody.Name)
		require.NotNil(p.GET.InputSession)
		require.Equal("session", p.GET.InputSession.Name)
		require.Empty(p.Actions)
		// Inherits OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 2)
	}

	// PageError500
	{
		p := app.PageError500
		require.Equal("PageError500", p.TypeName)
		require.Equal("/whoops", p.Route)
		require.Equal(model.PageTypeError500, p.PageSpecialization)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputDisableRefresh)
		require.Equal("disableRefreshAfterHidden", p.GET.OutputDisableRefresh.Name)
		require.Empty(p.Actions)
		require.Empty(p.EventHandlers) // No Base embed
	}

	// PageIndex
	{
		p := app.PageIndex
		require.Equal("PageIndex", p.TypeName)
		require.Equal("/", p.Route)
		require.Equal(model.PageTypeIndex, p.PageSpecialization)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.InputSession)
		require.Empty(p.Actions)
		// Inherits OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 2)
	}

	// PageLogin (no Base embed)
	{
		p := findPage(app, "PageLogin")
		require.NotNil(p)
		require.Equal("/login", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputRedirect)
		require.NotNil(p.GET.OutputDisableRefresh)
		require.NotNil(p.GET.InputSession)
		require.Len(p.Actions, 1)
		{
			a := p.Actions[0]
			require.Equal("POST", a.HTTPMethod)
			require.Equal("Submit", a.Name)
			require.Equal("/login/submit", a.Route)
			require.NotNil(a.InputSession)
			require.NotNil(a.InputSignals)
			require.NotNil(a.OutputRedirect)
			require.NotNil(a.OutputRedirectStatus)
			require.NotNil(a.OutputNewSession)
		}
		require.Empty(p.EventHandlers)
	}

	// PageMessages
	{
		p := findPage(app, "PageMessages")
		require.NotNil(p)
		require.Equal("/messages", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputRedirect)
		require.NotNil(p.GET.OutputEnableBgStream)
		require.Equal("enableBackgroundStreaming", p.GET.OutputEnableBgStream.Name)
		require.NotNil(p.GET.InputQuery)
		require.Equal("query", p.GET.InputQuery.Name)

		// 4 actions: POSTRead, POSTWriting,
		// POSTWritingStopped, POSTSendMessage
		require.Len(p.Actions, 4)

		read := findAction(p.Actions, "Read")
		require.NotNil(read)
		require.Equal("POST", read.HTTPMethod)
		require.Equal("/messages/read/{$}", read.Route)
		require.NotNil(read.InputSignals)
		require.NotNil(read.InputQuery)
		require.NotNil(read.InputDispatch)
		require.Equal(
			[]string{"EventMessagingRead"},
			read.InputDispatch.EventTypeNames,
		)

		writing := findAction(p.Actions, "Writing")
		require.NotNil(writing)
		require.Equal("/messages/writing/{$}", writing.Route)
		require.NotNil(writing.InputDispatch)
		require.Equal(
			[]string{"EventMessagingWriting"},
			writing.InputDispatch.EventTypeNames,
		)

		stopped := findAction(
			p.Actions, "WritingStopped",
		)
		require.NotNil(stopped)
		require.Equal("/messages/writing-stopped/{$}", stopped.Route)
		require.NotNil(stopped.InputDispatch)
		require.Equal(
			[]string{"EventMessagingWritingStopped"},
			stopped.InputDispatch.EventTypeNames,
		)

		send := findAction(p.Actions, "SendMessage")
		require.NotNil(send)
		require.Equal("/messages/sendmessage/{$}", send.Route)
		require.NotNil(send.InputDispatch)
		require.Equal(
			[]string{
				"EventMessagingWritingStopped",
				"EventMessagingSent",
			},
			send.InputDispatch.EventTypeNames,
		)

		// 4 own event handlers
		// (override Base's OnMessagingSent, OnMessagingRead)
		require.Len(p.EventHandlers, 4)
		require.NotNil(findEventHandler(p.EventHandlers, "MessagingRead"))
		require.NotNil(findEventHandler(p.EventHandlers, "MessagingSent"))
		require.NotNil(findEventHandler(p.EventHandlers, "MessagingWriting"))
		require.NotNil(findEventHandler(p.EventHandlers, "MessagingWritingStopped"))
	}

	// PageMyPosts
	{
		p := findPage(app, "PageMyPosts")
		require.NotNil(p)
		require.Equal("/my-posts", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputHead)
		require.NotNil(p.GET.OutputRedirect)
		require.Empty(p.Actions)
		// Inherits OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 2)
	}

	// PagePost
	{
		p := findPage(app, "PagePost")
		require.NotNil(p)
		require.Equal("/post/{slug}", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputHead)
		require.NotNil(p.GET.OutputRedirect)
		require.NotNil(p.GET.InputPath)
		require.Equal("path", p.GET.InputPath.Name)

		require.Len(p.Actions, 1)
		send := p.Actions[0]
		require.Equal("POST", send.HTTPMethod)
		require.Equal("SendMessage", send.Name)
		require.Equal("/post/{slug}/send-message/{$}", send.Route)
		require.NotNil(send.InputSSE)
		require.NotNil(send.InputPath)
		require.NotNil(send.InputSignals)
		require.NotNil(send.InputDispatch)
		require.Equal([]string{"EventMessagingSent"}, send.InputDispatch.EventTypeNames)

		// Own OnPostArchived + inherited
		// OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 3)
		require.NotNil(findEventHandler(p.EventHandlers, "PostArchived"))
	}

	// PageSearch
	// NOTE: GET.InputQuery and POSTParamChange.InputSignals
	// are nil due to named-type param limitation.
	{
		p := findPage(app, "PageSearch")
		require.NotNil(p)
		require.Equal("/search", p.Route)
		require.NotNil(p.GET)
		require.Nil(p.GET.OutputBody) // nil: parse err
		require.Nil(p.GET.InputQuery) // nil: named type

		require.Len(p.Actions, 1)
		a := p.Actions[0]
		require.Equal("POST", a.HTTPMethod)
		require.Equal("ParamChange", a.Name)
		require.Equal("/search/paramchange/{$}", a.Route)
		require.NotNil(a.InputSSE)
		require.Nil(a.InputSignals) // nil: named type

		// Inherits OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 2)
	}

	// PageSettings
	{
		p := findPage(app, "PageSettings")
		require.NotNil(p)
		require.Equal("/settings", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputRedirect)

		// 3 actions: POSTSave, POSTCloseSession,
		// POSTCloseAllSessions
		require.Len(p.Actions, 3)

		save := findAction(p.Actions, "Save")
		require.NotNil(save)
		require.Equal("/settings/save/{$}", save.Route)
		require.NotNil(save.InputSSE)
		require.NotNil(save.InputSignals)

		closeSess := findAction(p.Actions, "CloseSession")
		require.NotNil(closeSess)
		require.Equal("/settings/close-session/{token}/{$}", closeSess.Route)
		require.NotNil(closeSess.InputSessionToken)
		require.NotNil(closeSess.InputPath)
		require.NotNil(closeSess.InputDispatch)
		require.NotNil(closeSess.OutputCloseSession)

		closeAll := findAction(
			p.Actions, "CloseAllSessions",
		)
		require.NotNil(closeAll)
		require.Equal("/settings/close-all-sessions/{$}", closeAll.Route)
		require.NotNil(closeAll.InputDispatch)

		// Own OnSessionClosed + inherited
		// OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 3)
		require.NotNil(findEventHandler(p.EventHandlers, "SessionClosed"))
	}

	// PageUser
	{
		p := findPage(app, "PageUser")
		require.NotNil(p)
		require.Equal("/user/{name}/{$}", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputHead)
		require.NotNil(p.GET.OutputRedirect)
		require.NotNil(p.GET.InputPath)
		require.Equal("path", p.GET.InputPath.Name)
		require.Empty(p.Actions)

		// Own OnPostArchived + inherited
		// OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 3)
		require.NotNil(findEventHandler(p.EventHandlers, "PostArchived"))
	}
}
