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

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/parser/model"
)

const TypeNameComponent = "github.com/romshark/datapages.Component"

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
	require.Nil(app.RecoverError)
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
		requireExprLineCol(t, app, app.GlobalHeadGenerator.Expr, "app.go", 25, 13)
		require.False(app.GlobalHeadGenerator.InputSession)
	}
	{
		require.NotNil(app.RecoverError)
		requireExprLineCol(t, app, app.RecoverError, "app.go", 29, 13)
	}
	{
		p := app.Pages[3]
		require.NotNil(p)
		require.Equal("PageIndex", p.TypeName)
		require.Equal("/", p.Route)
		requireExprLineCol(t, app, p.Expr, "app.go", 13, 6)
		require.Empty(p.EventHandlers)
		require.Empty(p.Embeds)
		require.Empty(p.Actions)
		require.Equal(model.PageTypeIndex, p.PageSpecialization)
		{
			get := p.GET
			require.NotNil(get.Handler)
			requireExprLineCol(t, app, get.Expr, "app.go", 15, 18)
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
		requireExprLineCol(t, app, app.PageError404.Expr, "app.go", 37, 6)
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
			requireExprLineCol(t, app, get.Expr, "app.go", 39, 21)
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
		requireExprLineCol(t, app, app.PageError500.Expr, "app.go", 44, 6)
		require.Equal("/the-internal-error-page", app.PageError500.Route)
		require.Empty(app.PageError500.EventHandlers)
		require.Empty(app.PageError500.Embeds)
		require.Empty(app.PageError500.Actions)
		require.Equal(model.PageTypeError500, app.PageError500.PageSpecialization)
		{
			get := app.PageError500.GET
			require.NotNil(get.Handler)
			requireExprLineCol(t, app, get.Expr, "app.go", 46, 21)
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
		requireExprLineCol(t, app, p.Expr, "app.go", 51, 6)
		require.Empty(p.EventHandlers)
		require.Empty(p.Embeds)
		require.Empty(p.Actions)
		require.Zero(p.PageSpecialization)
		require.NotNil(p.GET)
		require.NotNil(p.GET.Handler)
		requireExprLineCol(t, app, p.GET.Expr, "app.go", 53, 20)
		require.NotNil(p.GET.OutputBody)
		// Return values are matched by their type, not by their names.
		require.Equal("view", p.GET.OutputBody.Name)
		require.Equal(TypeNameComponent,
			p.GET.OutputBody.Type.Resolved.String())
		require.NotNil(p.GET.OutputHead)
		require.Equal("meta", p.GET.OutputHead.Name)
		require.Equal("github.com/romshark/datapages.Head",
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
		require.Len(p.Actions, 9)
		require.Len(p.EventHandlers, 1)

		// Actions at same path as page
		for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
			a := findAction(p.Actions, "SamePath")
			require.NotNil(a, "missing %sSamePath", method)
			require.Equal("/actions", a.Route)
		}

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

		// PATCH with SSE
		patchWith := findActionByMethod(p.Actions, "PATCH", "WithSSE")
		require.NotNil(patchWith)
		require.Equal("PATCH", patchWith.HTTPMethod)
		require.NotNil(patchWith.InputSSE)

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

func TestParse_StreamHooks(t *testing.T) {
	app, err := parse(t, "stream_hooks")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)
	require.NotNil(app.PageIndex)

	p := app.PageIndex
	require.NotNil(p.StreamOpen)
	require.NotNil(p.StreamClose)

	require.Equal("StreamOpen", p.StreamOpen.Name)
	require.NotNil(p.StreamOpen.InputRequest)
	require.NotNil(p.StreamOpen.InputStreamID)
	require.NotNil(p.StreamOpen.InputSSE)
	require.NotNil(p.StreamOpen.InputSession)
	require.NotNil(p.StreamOpen.InputSignals)
	require.Len(p.StreamOpen.InputDispatches, 1)
	require.NotNil(p.StreamOpen.OutputErr)

	require.Equal("StreamClose", p.StreamClose.Name)
	require.NotNil(p.StreamClose.InputRequest)
	require.NotNil(p.StreamClose.InputStreamID)
	require.Nil(p.StreamClose.InputSSE)
	require.NotNil(p.StreamClose.InputSession)
	require.Nil(p.StreamClose.InputSignals)
	require.Len(p.StreamClose.InputDispatches, 1)
	require.NotNil(p.StreamClose.OutputErr)

	{ // PageStreamMin: only required params (r, streamID), no error return
		p := findPage(app, "PageStreamMin")
		require.NotNil(p)

		require.NotNil(p.StreamOpen)
		require.Equal("StreamOpen", p.StreamOpen.Name)
		require.NotNil(p.StreamOpen.InputRequest)
		require.NotNil(p.StreamOpen.InputStreamID)
		// The parameter is matched by its type, not by its name.
		require.Equal("id", p.StreamOpen.InputStreamID.Name)
		require.Nil(p.StreamOpen.InputSSE)
		require.Nil(p.StreamOpen.InputSession)
		require.Nil(p.StreamOpen.InputSignals)
		require.Empty(p.StreamOpen.InputDispatches)
		require.Nil(p.StreamOpen.OutputErr)

		require.NotNil(p.StreamClose)
		require.Equal("StreamClose", p.StreamClose.Name)
		require.NotNil(p.StreamClose.InputRequest)
		require.NotNil(p.StreamClose.InputStreamID)
		require.Nil(p.StreamClose.InputSSE)
		require.Nil(p.StreamClose.InputSession)
		require.Nil(p.StreamClose.InputSignals)
		require.Empty(p.StreamClose.InputDispatches)
		require.Nil(p.StreamClose.OutputErr)
	}

	{ // PageStreamMax: all optional params directly
		p := findPage(app, "PageStreamMax")
		require.NotNil(p)

		require.NotNil(p.StreamOpen)
		require.Equal("StreamOpen", p.StreamOpen.Name)
		require.NotNil(p.StreamOpen.InputRequest)
		require.NotNil(p.StreamOpen.InputStreamID)
		require.NotNil(p.StreamOpen.InputSSE)
		require.NotNil(p.StreamOpen.InputSession)
		require.NotNil(p.StreamOpen.InputSignals)
		require.Len(p.StreamOpen.InputDispatches, 1)
		require.NotNil(p.StreamOpen.OutputErr)

		require.NotNil(p.StreamClose)
		require.Equal("StreamClose", p.StreamClose.Name)
		require.NotNil(p.StreamClose.InputRequest)
		require.NotNil(p.StreamClose.InputStreamID)
		require.Nil(p.StreamClose.InputSSE)
		require.NotNil(p.StreamClose.InputSession)
		require.Nil(p.StreamClose.InputSignals)
		require.Len(p.StreamClose.InputDispatches, 1)
		require.NotNil(p.StreamClose.OutputErr)

		// Event handler with streamID
		require.Len(p.EventHandlers, 1)
		eh := p.EventHandlers[0]
		require.Equal("Ping", eh.Name)
		require.NotNil(eh.InputEvent)
		require.NotNil(eh.InputSSE)
		require.NotNil(eh.InputStreamID)
		require.Nil(eh.InputSession)
	}
}

func TestParse_ErrStreamHooks(t *testing.T) {
	_, err := parse(t, "err_stream_hooks")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSignatureMissingReq,
		parser.ErrSignatureMissingStreamID,
		parser.ErrSignatureMissingStreamID,  // StreamOpen with streamID string
		parser.ErrSignatureUnsupportedInput, // StreamClose with signals
		parser.ErrSignatureStreamHookReturnMustBeError,
		parser.ErrSignatureUnsupportedInput, // StreamClose with sse
		parser.ErrSignatureUnsupportedInput, // StreamOpen with path
		parser.ErrSignatureUnsupportedInput, // StreamClose with path
		parser.ErrSignatureUnsupportedInput, // StreamOpen with query
		parser.ErrSignatureUnsupportedInput, // StreamClose with query
		parser.ErrSignatureUnsupportedInput, // action handler with streamID
		parser.ErrSignatureMissingStreamID,  // StreamOpen with streamID int
		parser.ErrSignatureUnsupportedInput, // StreamOpen with an untyped dispatcher
		parser.ErrDispatchDuplicate,         // StreamClose with two of one type
	)
}

func TestParse_ErrUnsupportedMethod(t *testing.T) {
	_, err := parse(t, "err_unsupported_method")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrUnsupportedMethod, // Foo
		parser.ErrUnsupportedMethod, // Helper
	)
}

func TestParse_ErrActionHandlerNoName(t *testing.T) {
	_, err := parse(t, "err_action_handler_no_name")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrActionNameMissing, // POST
		parser.ErrActionNameMissing, // DELETE
		parser.ErrActionNameMissing, // PATCH
		parser.ErrActionNameMissing, // PUT
	)
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

func TestParse_ErrPageIndexPath(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_page_index_path")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrPageIndexPathMustBeRoot,
	)
}

func TestParse_ErrRouteDuplicatePage(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_route_duplicate_page")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrRouteConflict,
	)
}

func TestParse_ErrRouteDuplicateAction(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_route_duplicate_action")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrRouteConflict,
	)
}

func TestParse_ErrRouteWildcardStream(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_route_wildcard_stream")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrRouteWildcardStream,
	)
}

func TestParse_ErrPages(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_pages")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
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

	requireParseErrors(
		t, err,
		parser.ErrSignatureMissingReq,
		parser.ErrSignatureMultiErrRet,
		parser.ErrSignatureUnsupportedInput,
		parser.ErrSignatureUnsupportedInput,
		parser.ErrSignatureUnsupportedInput, // asd
		parser.ErrSignatureUnsupportedInput, // asd2
		parser.ErrSignatureGETMissingBody,
		parser.ErrSignatureDuplicateOutput,
		parser.ErrSignatureDuplicateOutput,
	)
}

func TestParse_ErrEvents(t *testing.T) {
	_, err := parse(t, "err_events")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventCommMissing,
		parser.ErrEventSubjectInvalid,
		parser.ErrSignatureEvHandMissingEvent,   // OnArgWrongType: int is no event type
		parser.ErrSignatureEvHandMissingSSE,     // OnArgWrongType: no SSE param
		parser.ErrSignatureUnsupportedInput,     // OnArgWrongType: int is unsupported
		parser.ErrSignatureEvHandMissingSSE,     // OnFirstDuplicate: no SSE param
		parser.ErrEvHandDuplicate,               // OnSecondDuplicate
		parser.ErrSignatureEvHandMissingSSE,     // OnSecondDuplicate: no SSE param
		parser.ErrSignatureEvHandMultipleEvents, // OnBoth: two event parameters
		parser.ErrSignatureUnsupportedInput,     // OnBoth: the second event
		parser.ErrEventFieldUnexported,
		parser.ErrEventFieldUnexported,
		parser.ErrEventFieldMissingTag,
		parser.ErrEventFieldDuplicateTag,
		parser.ErrEventCommInvalid,
		parser.ErrEventCommInvalid,
		parser.ErrEventSubjectInvalid,
		parser.ErrEventSubjectInvalid,
		parser.ErrEventFieldEmptyTag,   // EventEmptyJSONTag: json:"" should be rejected
		parser.ErrEventFieldUnexported, // same-module subpkg.BadFields
	)
}

func TestParse_ErrEventHandler(t *testing.T) {
	_, err := parse(t, "err_event_handler")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSignatureEvHandMissingSSE, // OnEventQux: no SSE param
		parser.ErrSignatureUnsupportedInput, // OnEventCorge: unknownParam
		parser.ErrSignatureEvHandMissingSSE, // OnEventQuux: no SSE param
		parser.ErrSignatureUnsupportedInput, // OnEventQuux: notSSE is unsupported
		parser.ErrSignatureEvHandReturnMustBeError,
		parser.ErrSignatureEvHandReturnMustBeError,
		parser.ErrSignatureEvHandReturnMustBeError,
		parser.ErrSignatureEvHandReturnMustBeError,
	)
}

func TestParse_ErrEventSubjectUserNoSession(t *testing.T) {
	_, err := parse(t, "err_event_target_no_session")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventSubjectUserNoSession,
	)
}

func TestParse_ErrEventSubjectAfterPayload(t *testing.T) {
	_, err := parse(t, "err_event_subj_after_payload")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventSubjectAfterPayload,
	)
}

func TestParse_ErrEventSubjectOverlap(t *testing.T) {
	_, err := parse(t, "err_event_subject_overlap")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventSubjectOverlap,
	)
}

func TestParse_ErrEventSubjectDuplicateSignal(t *testing.T) {
	_, err := parse(t, "err_event_subj_duplicate_signal")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventSubjectDuplicateSignal,
	)
}

func TestParse_ErrEventSubjectSignalInvalid(t *testing.T) {
	_, err := parse(t, "err_event_subj_signal_invalid")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventSubjectSignalInvalid,
	)
}

func TestParse_ErrEventSubjectUserSignal(t *testing.T) {
	_, err := parse(t, "err_event_subj_user_signal")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventSubjectUserSignal,
	)
}

func TestParse_ErrEventSubjectPrefixedField(t *testing.T) {
	_, err := parse(t, "err_event_subj_prefixed")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventSubjectPrefixedField,
	)
}

func TestParse_ErrEventSubjectFieldUnexported(t *testing.T) {
	_, err := parse(t, "err_event_subj_unexported")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
		parser.ErrEventFieldUnexported,
	)
}

func TestParse_SignalSubjectFields(t *testing.T) {
	app, errs := parse(t, "signal_subject")
	require := require.New(t)
	requireParseErrors(t, errs)
	require.NotNil(app)

	require.Len(app.Events, 5)
	events := map[string]*model.Event{}
	for _, e := range app.Events {
		events[e.TypeName] = e
	}

	for name, tc := range map[string]struct {
		subject        string
		subjectFields  []model.SubjectField
		isPrivate      bool
		isSignalScoped bool
	}{
		"EventSingular": {
			subject: "calc.updated",
			subjectFields: []model.SubjectField{
				{FieldName: "Instance", Kind: model.SubjectKindValue, SignalName: "instance_id"},
			},
			isPrivate:      false,
			isSignalScoped: true,
		},
		"EventPluralUser": {
			subject: "chat.sent",
			subjectFields: []model.SubjectField{
				{FieldName: "Recipient", Kind: model.SubjectKindUser},
			},
			isPrivate:      true,
			isSignalScoped: false,
		},
		"EventSingularUser": {
			subject: "dm.sent",
			subjectFields: []model.SubjectField{
				{FieldName: "Recipient", Kind: model.SubjectKindUser},
			},
			isPrivate:      true,
			isSignalScoped: false,
		},
		"EventMixed": {
			subject: "mixed",
			subjectFields: []model.SubjectField{
				{FieldName: "Recipient", Kind: model.SubjectKindUser},
				{FieldName: "Instance", Kind: model.SubjectKindValue, SignalName: "instance_id"},
			},
			isPrivate:      true,
			isSignalScoped: true,
		},
		"EventThreeField": {
			subject: "three",
			subjectFields: []model.SubjectField{
				{FieldName: "Recipient", Kind: model.SubjectKindUser},
				{FieldName: "Room", Kind: model.SubjectKindValue},
				{FieldName: "Calc", Kind: model.SubjectKindValue, SignalName: "calc_id"},
			},
			isPrivate:      true,
			isSignalScoped: true,
		},
	} {
		e, ok := events[name]
		require.True(ok, "missing event: %s", name)
		require.Equal(tc.subject, e.Subject, "event %s subject", name)
		require.Equal(tc.isPrivate, e.IsPrivate(), "event %s IsPrivate", name)
		require.Equal(tc.isSignalScoped, e.IsSignalScoped(), "event %s IsSignalScoped", name)
		require.Equal(tc.subjectFields, e.SubjectFields, "event %s SubjectFields", name)
	}
}

func TestParse_ErrEmbedDuplicateEventHandler(t *testing.T) {
	_, err := parse(t, "err_embed_duplicate_event_handler")
	require.NotZero(t, err.Error())

	requireParseErrors(
		t, err,
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
		// The parameter is matched by its type, not by its name.
		require.Equal("vars", action.InputPath.Name)
	}

	// PageNamed - the path values are a named struct type
	{
		p := findPage(app, "PageNamed")
		require.NotNil(p)
		require.NotNil(p.GET.InputPath)
		require.Equal(
			"datapagestest/fixture/path.NamedPath",
			p.GET.InputPath.Type.Resolved.String(),
		)
	}
}

func TestParse_ErrPath(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_path")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrPathParamNotStruct,
		parser.ErrPathFieldUnexported,
		parser.ErrPathFieldUnsupportedType,
		parser.ErrPathFieldMissingTag,
		parser.ErrPathFieldNotInRoute,
		parser.ErrPathMissingRouteVar,
		parser.ErrPathFieldDuplicateTag,
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
		// The parameter is matched by its type, not by its name.
		require.Equal("params", action.InputQuery.Name)
	}
}

func TestParse_ErrQuery(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_query")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrQueryParamNotStruct,
		parser.ErrQueryFieldUnexported,
		parser.ErrQueryFieldUnsupportedType,
		parser.ErrQueryFieldMissingTag,
		parser.ErrQueryFieldDuplicateTag,
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
		// The parameter is matched by its type, not by its name.
		require.Equal("form", action.InputSignals.Name)
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
		require.Len(a.InputDispatches, 1)
		require.Equal("dispatch", a.InputDispatches[0].Name)
		require.Equal("EventFoo", a.InputDispatches[0].EventTypeName)
	}

	// POSTMulti - one dispatcher per event type
	{
		a := findAction(p.Actions, "Multi")
		require.NotNil(a)
		require.Equal("POST", a.HTTPMethod)
		require.Len(a.InputDispatches, 2)
		require.Equal("dispatchFoo", a.InputDispatches[0].Name)
		require.Equal("EventFoo", a.InputDispatches[0].EventTypeName)
		require.Equal("dispatchBar", a.InputDispatches[1].Name)
		require.Equal("EventBar", a.InputDispatches[1].EventTypeName)
	}

	// POSTWithSignals - signals before dispatch
	{
		a := findAction(p.Actions, "WithSignals")
		require.NotNil(a)
		require.Equal("POST", a.HTTPMethod)
		require.NotNil(a.InputSignals)
		require.Len(a.InputDispatches, 1)
		require.Equal("EventFoo", a.InputDispatches[0].EventTypeName)
	}
}

func TestParse_ErrDispatch(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_dispatch")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSignatureUnsupportedInput, // PageFuncParam: plain func type
		parser.ErrSignatureUnsupportedInput, // PageDispatchInt: dispatch int
		parser.ErrDispatchParamNotEvent,
		parser.ErrDispatchDuplicate,
	)
}

func TestParse_Session(t *testing.T) {
	app, err := parse(t, "session")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)
	require.NotNil(app.Session)

	// Head with session
	{
		require.NotNil(app.GlobalHeadGenerator)
		require.True(app.GlobalHeadGenerator.InputSession)
	}

	// PageIndex - no session
	{
		p := app.PageIndex
		require.NotNil(p)
		require.Nil(p.GET.InputSession)
	}

	// PageProfile - GET with session
	{
		p := findPage(app, "PageProfile")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputSession)
		require.Equal("session", p.GET.InputSession.Name)

		// POSTUpdate - action with session
		update := findAction(p.Actions, "Update")
		require.NotNil(update)
		require.NotNil(update.InputSession)
		// The parameter is matched by its type, not by its name.
		require.Equal("sess", update.InputSession.Name)
		require.Nil(update.InputSSE)

		// POSTNotify - action with SSE + session
		notify := findAction(p.Actions, "Notify")
		require.NotNil(notify)
		require.NotNil(notify.InputSSE)
		require.NotNil(notify.InputSession)

		// Event handler with session
		require.Len(p.EventHandlers, 1)
		evh := p.EventHandlers[0]
		require.NotNil(evh.InputSession)
		require.Equal("session", evh.InputSession.Name)
	}

	// PageSettings - session
	{
		p := findPage(app, "PageSettings")
		require.NotNil(p)

		// GET with session.
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputSession)

		// POSTClose - action with session
		close := findAction(p.Actions, "Close")
		require.NotNil(close)
		require.NotNil(close.InputSession)

		// Event handler with session
		require.Len(p.EventHandlers, 1)
		evh := p.EventHandlers[0]
		require.NotNil(evh.InputSession)
	}
}

func TestParse_ErrSession(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_session")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSignatureUnsupportedInput,
	)
}

func TestParse_SessionCloseOnly(t *testing.T) {
	app, err := parse(t, "session_close_only")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)

	// closeSession alone puts sessions in play. No handler names the Data type,
	// so it defaults to the empty struct.
	require.NotNil(app.Session)
	require.Equal("struct{}", app.Session.Data.Resolved.String())
}

func TestParse_ErrSessionTypeConflict(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_session_conflict")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSessionTypeConflict,
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
	}

	// PageLogin - GET with redirect only
	{
		p := findPage(app, "PageLogin")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputRedirect)
		require.Equal("redirect", p.GET.OutputRedirect.Name)

		// POSTSignIn - action with redirect
		require.Len(p.Actions, 1)
		a := p.Actions[0]
		require.NotNil(a.OutputRedirect)
		require.Equal("redirect", a.OutputRedirect.Name)
	}
}

func TestParse_ErrRedirect(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_redirect")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSignatureUnsupportedOutput,
		parser.ErrSignatureUnsupportedOutput,
	)
}

func TestParse_ErrUnsupportedOutput(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_unsupported_output")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSignatureUnsupportedOutput,
		parser.ErrSignatureUnsupportedOutput,
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

	requireParseErrors(
		t, err,
		parser.ErrSignatureUnsupportedOutput,
		parser.ErrSignatureUnsupportedOutput,
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

	requireParseErrors(
		t, err,
		parser.ErrEnableBgStreamNotGET,
		parser.ErrDisableRefreshNotGET,
		parser.ErrSignatureUnsupportedOutput,
		parser.ErrSignatureUnsupportedOutput,
	)
}

func TestParse_ErrSignals(t *testing.T) {
	require := require.New(t)
	_, err := parse(t, "err_signals")
	require.NotZero(err.Error())

	requireParseErrors(
		t, err,
		parser.ErrSignalsParamNotStruct,
		parser.ErrSignalsFieldUnexported,
		parser.ErrSignalsFieldMissingTag,
		parser.ErrSignalsFieldDuplicateTag,
		parser.ErrQueryReflectSignalNotInSignals,
	)
}

func TestParse_ParamOrder(t *testing.T) {
	app, err := parse(t, "param_order")
	require := require.New(t)
	requireParseErrors(t, err /*none*/)
	require.NotNil(app)

	// PageSessionFirst - session before request.
	{
		p := findPage(app, "PageSessionFirst")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputRequest)
		require.NotNil(p.GET.InputSession)
		require.Equal(
			[]string{
				model.InputKindSession,
				model.InputKindRequest,
			},
			inputKinds(p.GET.OrderedInputs),
		)
	}

	// PageReversed - all params in reverse order.
	{
		p := findPage(app, "PageReversed")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputRequest)
		require.NotNil(p.GET.InputSession)
		require.NotNil(p.GET.InputPath)
		require.NotNil(p.GET.InputQuery)
		require.Equal(
			[]string{
				model.InputKindQuery,
				model.InputKindPath,
				model.InputKindSession,
				model.InputKindRequest,
			},
			inputKinds(p.GET.OrderedInputs),
		)
	}

	// PageSignalsFirst - signals before request.
	{
		p := findPage(app, "PageSignalsFirst")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.InputRequest)
		require.NotNil(p.GET.InputSignals)
		require.Equal(
			[]string{
				model.InputKindSignals,
				model.InputKindRequest,
			},
			inputKinds(p.GET.OrderedInputs),
		)
	}

	// PageErrBeforeBody - error before body in output.
	{
		p := findPage(app, "PageErrBeforeBody")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputErr)
		require.Equal(
			[]string{
				model.OutputKindErr,
				model.OutputKindBody,
			},
			outputKinds(p.GET.OrderedOutputs),
		)
	}

	// PageOutputReversed - all outputs reversed.
	{
		p := findPage(app, "PageOutputReversed")
		require.NotNil(p)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.NotNil(p.GET.OutputErr)
		require.NotNil(p.GET.OutputRedirect)
		require.Equal(
			[]string{
				model.OutputKindErr,
				model.OutputKindRedirect,
				model.OutputKindBody,
			},
			outputKinds(p.GET.OrderedOutputs),
		)
	}

	// PageActionReversed - POSTSubmit with reversed params.
	{
		p := findPage(app, "PageActionReversed")
		require.NotNil(p)
		require.Len(p.Actions, 1)
		a := p.Actions[0]
		require.NotNil(a.InputRequest)
		require.NotNil(a.InputSSE)
		require.NotNil(a.InputSession)
		require.NotNil(a.InputSignals)
		require.Equal(
			[]string{
				model.InputKindSession,
				model.InputKindSSE,
				model.InputKindSignals,
				model.InputKindRequest,
			},
			inputKinds(a.OrderedInputs),
		)
	}

	// PageEventReversed - OnEventPing with SSE before event.
	{
		p := findPage(app, "PageEventReversed")
		require.NotNil(p)
		require.Len(p.EventHandlers, 1)
		evh := p.EventHandlers[0]
		require.NotNil(evh.InputEvent)
		// The parameter is matched by its type, not by its name.
		require.Equal("ping", evh.InputEvent.Name)
		require.NotNil(evh.InputSSE)
		require.NotNil(evh.InputSession)
		require.Equal(
			[]string{
				model.InputKindSSE,
				model.InputKindSession,
				model.InputKindEvent,
			},
			inputKinds(evh.OrderedInputs),
		)
	}
}

func TestParse_ErrorPositions(t *testing.T) {
	type wantPos struct {
		err       error
		file      string
		line, col int
	}

	for name, tc := range map[string][]wantPos{
		"err_get": {
			{parser.ErrSignatureMissingReq, "app.go", 23, 23},
			{parser.ErrSignatureMultiErrRet, "app.go", 32, 2},
			{parser.ErrSignatureUnsupportedInput, "app.go", 43, 19},
			{parser.ErrSignatureUnsupportedInput, "app.go", 54, 5},
			{parser.ErrSignatureUnsupportedInput, "app.go", 65, 19},
			{parser.ErrSignatureUnsupportedInput, "app.go", 65, 24},
			{parser.ErrSignatureGETMissingBody, "app.go", 75, 24},
			{parser.ErrSignatureDuplicateOutput, "app.go", 86, 2},
			{parser.ErrSignatureDuplicateOutput, "app.go", 100, 2},
		},
		"err_head": {
			{parser.ErrAppHeadMustTakeRequest, "app.go", 20, 13},
		},
		"err_head_return": {
			{parser.ErrAppHeadMustReturnTemplComponent, "app.go", 20, 13},
		},
		"err_head_unsupported": {
			{parser.ErrAppHeadUnsupportedInput, "app.go", 20, 13},
		},
		"err_recover_error_no_params": {
			{parser.ErrAppRecoverErrorInvalidSignature, "app.go", 20, 13},
		},
		"err_recover_error_params": {
			{parser.ErrAppRecoverErrorInvalidSignature, "app.go", 20, 13},
		},
		"err_recover_error_return": {
			{parser.ErrAppRecoverErrorInvalidSignature, "app.go", 20, 13},
		},
		"err_dispatch": {
			{parser.ErrSignatureUnsupportedInput, "app.go", 34, 2},
			{parser.ErrSignatureUnsupportedInput, "app.go", 47, 2},
			{parser.ErrDispatchParamNotEvent, "app.go", 60, 17},
			{parser.ErrDispatchDuplicate, "app.go", 74, 19},
		},
		"err_event_subj_prefixed": {
			{parser.ErrEventSubjectPrefixedField, "app.go", 26, 2},
		},
		"err_event_subj_unexported": {
			{parser.ErrEventFieldUnexported, "app.go", 25, 2},
		},
		"err_events": {
			{parser.ErrEventCommMissing, "app.go", 30, 6},
			{parser.ErrEventSubjectInvalid, "app.go", 36, 24},
			{parser.ErrSignatureEvHandMissingEvent, "app.go", 52, 22},
			{parser.ErrSignatureEvHandMissingSSE, "app.go", 52, 22},
			{parser.ErrSignatureUnsupportedInput, "app.go", 53, 2},
			{parser.ErrSignatureEvHandMissingSSE, "app.go", 60, 22},
			{parser.ErrEvHandDuplicate, "app.go", 69, 22},
			{parser.ErrSignatureEvHandMissingSSE, "app.go", 69, 22},
			{parser.ErrSignatureEvHandMultipleEvents, "app.go", 94, 23},
			{parser.ErrSignatureUnsupportedInput, "app.go", 96, 2},
			{parser.ErrEventFieldUnexported, "app.go", 106, 2},
			{parser.ErrEventFieldUnexported, "app.go", 106, 2},
			{parser.ErrEventFieldMissingTag, "app.go", 113, 2},
			{parser.ErrEventFieldDuplicateTag, "app.go", 131, 2},
			{parser.ErrEventCommInvalid, "app.go", 136, 21},
			{parser.ErrEventCommInvalid, "app.go", 143, 4},
			{parser.ErrEventSubjectInvalid, "app.go", 150, 23},
			{parser.ErrEventSubjectInvalid, "app.go", 157, 24},
			{parser.ErrEventFieldEmptyTag, "app.go", 168, 2},
			{parser.ErrEventFieldUnexported, "subpkg.go", 7, 2},
		},
		"err_path": {
			{parser.ErrPathParamNotStruct, "app.go", 26, 24},
			{parser.ErrPathFieldUnexported, "app.go", 40, 3},
			{parser.ErrPathFieldUnsupportedType, "app.go", 55, 3},
			{parser.ErrPathFieldMissingTag, "app.go", 70, 3},
			{parser.ErrPathFieldNotInRoute, "app.go", 85, 3},
			{parser.ErrPathMissingRouteVar, "app.go", 97, 23},
			{parser.ErrPathFieldDuplicateTag, "app.go", 110, 3},
		},
		"err_query": {
			{parser.ErrQueryParamNotStruct, "app.go", 26, 25},
			{parser.ErrQueryFieldUnexported, "app.go", 40, 3},
			{parser.ErrQueryFieldUnsupportedType, "app.go", 55, 3},
			{parser.ErrQueryFieldMissingTag, "app.go", 70, 3},
			{parser.ErrQueryFieldDuplicateTag, "app.go", 86, 3},
		},
		"err_signals": {
			{parser.ErrSignalsParamNotStruct, "app.go", 26, 27},
			{parser.ErrSignalsFieldUnexported, "app.go", 40, 3},
			{parser.ErrSignalsFieldMissingTag, "app.go", 55, 3},
			{parser.ErrSignalsFieldDuplicateTag, "app.go", 71, 3},
			{parser.ErrQueryReflectSignalNotInSignals, "app.go", 85, 2},
		},
		"err_unsupported_output": {
			{parser.ErrSignatureUnsupportedOutput, "app.go", 27, 30},
			{parser.ErrSignatureUnsupportedOutput, "app.go", 36, 4},
		},
		"err_event_handler": {
			{parser.ErrSignatureEvHandMissingSSE, "app.go", 58, 18},
			{parser.ErrSignatureUnsupportedInput, "app.go", 70, 2},
			{parser.ErrSignatureEvHandMissingSSE, "app.go", 80, 18},
			{parser.ErrSignatureUnsupportedInput, "app.go", 82, 2},
			{parser.ErrSignatureEvHandReturnMustBeError, "app.go", 89, 18},
			{parser.ErrSignatureEvHandReturnMustBeError, "app.go", 100, 3},
			{parser.ErrSignatureEvHandReturnMustBeError, "app.go", 109, 3},
			{parser.ErrSignatureEvHandReturnMustBeError, "app.go", 120, 3},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, errs := parse(t, name)
			require.Equal(t, len(tc), errs.Len(),
				"unexpected number of errors for %s", name)
			for i, want := range tc {
				pos, err := errs.Entry(i)
				require.True(t, errors.Is(err, want.err),
					"%s[%d]: want Is(%v) got %T: %v", name, i, want.err, err, err)
				gotFile := filepath.Base(pos.Filename)
				require.True(
					t,
					gotFile == want.file &&
						pos.Line == want.line &&
						pos.Column == want.col,
					"%s[%d] (%v): want %s:%d:%d got %s:%d:%d",
					name, i, want.err,
					want.file, want.line, want.col,
					gotFile, pos.Line, pos.Column,
				)
			}
		})
	}
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
		require.Failf(
			t, "unexpected number of errors",
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
		require.Failf(
			t, "error mismatch",
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

func inputKinds(inputs []*model.Input) []string {
	kinds := make([]string, len(inputs))
	for i, inp := range inputs {
		kinds[i] = inp.Kind
	}
	return kinds
}

func outputKinds(outputs []*model.Output) []string {
	kinds := make([]string, len(outputs))
	for i, out := range outputs {
		kinds[i] = out.Kind
	}
	return kinds
}

func TestParse_ExampleClassifieds(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "..", "example", "classifieds", "app"),
	)
	require := require.New(t)
	requireParseErrors(t, errs)
	require.NotNil(app)

	// App-level features
	require.NotNil(app.GlobalHeadGenerator)
	require.NotNil(app.RecoverError)
	require.NotNil(app.Session)

	// App-level actions
	require.Len(app.Actions, 2)
	{
		signOut := findAction(app.Actions, "SignOut")
		require.NotNil(signOut)
		require.Equal("POST", signOut.HTTPMethod)
		require.Equal("/sign-out/{$}", signOut.Route)
		require.NotNil(signOut.InputSession)
		require.NotNil(signOut.OutputCloseSession)
		require.NotNil(signOut.OutputRedirect)

		cause500 := findAction(app.Actions, "Cause500")
		require.NotNil(cause500)
		require.Equal("POST", cause500.HTTPMethod)
		require.Equal("/cause-500-internal-error/{$}", cause500.Route)
		require.NotNil(cause500.OutputErr)
	}

	// Events (sorted alphabetically by type name)
	require.Len(app.Events, 6)
	events := map[string]*model.Event{}
	for _, e := range app.Events {
		events[e.TypeName] = e
	}
	for name, tc := range map[string]struct {
		subject        string
		hasSubjectUser bool
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
		require.Equal(tc.hasSubjectUser, e.HasSubjectUser(),
			"event %s HasSubjectUser", name)
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
		require.Len(read.InputDispatches, 1)
		require.Equal("EventMessagingRead", read.InputDispatches[0].EventTypeName)

		writing := findAction(p.Actions, "Writing")
		require.NotNil(writing)
		require.Equal("/messages/writing/{$}", writing.Route)
		require.Len(writing.InputDispatches, 1)
		require.Equal("EventMessagingWriting", writing.InputDispatches[0].EventTypeName)

		stopped := findAction(
			p.Actions, "WritingStopped",
		)
		require.NotNil(stopped)
		require.Equal("/messages/writing-stopped/{$}", stopped.Route)
		require.Len(stopped.InputDispatches, 1)
		require.Equal("EventMessagingWritingStopped",
			stopped.InputDispatches[0].EventTypeName)

		send := findAction(p.Actions, "SendMessage")
		require.NotNil(send)
		require.Equal("/messages/sendmessage/{$}", send.Route)
		require.Len(send.InputDispatches, 2)
		require.Equal(
			"EventMessagingWritingStopped",
			send.InputDispatches[0].EventTypeName,
		)
		require.Equal("EventMessagingSent", send.InputDispatches[1].EventTypeName)

		// 4 own event handlers
		// (override Base's OnMessagingSent, OnMessagingRead)
		require.Len(p.EventHandlers, 4)
		require.NotNil(findEventHandler(p.EventHandlers, "MessagingSent"))
		require.NotNil(findEventHandler(p.EventHandlers, "MessagingRead"))
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
		require.Len(send.InputDispatches, 1)
		require.Equal("EventMessagingSent", send.InputDispatches[0].EventTypeName)

		// Own OnPostArchived + inherited
		// OnMessagingSent, OnMessagingRead from Base
		require.Len(p.EventHandlers, 3)
		require.NotNil(findEventHandler(p.EventHandlers, "PostArchived"))
	}

	// PageSearch
	{
		p := findPage(app, "PageSearch")
		require.NotNil(p)
		require.Equal("/search", p.Route)
		require.NotNil(p.GET)
		require.NotNil(p.GET.OutputBody)
		require.Equal("body", p.GET.OutputBody.Name)
		require.NotNil(p.GET.InputQuery)

		require.Len(p.Actions, 1)
		a := p.Actions[0]
		require.Equal("POST", a.HTTPMethod)
		require.Equal("ParamChange", a.Name)
		require.Equal("/search/paramchange/{$}", a.Route)
		require.NotNil(a.InputSSE)
		require.NotNil(a.InputSignals)

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
		require.NotNil(closeSess.InputPath)
		require.Len(closeSess.InputDispatches, 1)
		require.NotNil(closeSess.OutputCloseSession)

		closeAll := findAction(
			p.Actions, "CloseAllSessions",
		)
		require.NotNil(closeAll)
		require.Equal("/settings/close-all-sessions/{$}", closeAll.Route)
		require.Len(closeAll.InputDispatches, 1)

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
