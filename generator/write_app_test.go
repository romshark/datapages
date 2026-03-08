package generator

import (
	"flag"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/romshark/datapages/parser/model"
	"github.com/stretchr/testify/require"
)

const (
	testAppPkgPath = "example.com/testapp"
	testAppPkg     = "testapp"
)

// namedAppType constructs a named type in the test app package.
func namedAppType(name string, st *types.Struct) model.Type {
	pkg := types.NewPackage(testAppPkgPath, testAppPkg)
	tn := types.NewTypeName(token.NoPos, pkg, name, nil)
	named := types.NewNamed(tn, st, nil)
	return model.Type{Resolved: named}
}

// shouldUpdate returns true when -update is passed on the command line.
// The flag is registered in append_href_test.go (package generator_test).
func shouldUpdate() bool {
	f := flag.Lookup("update")
	return f != nil && f.Value.String() == "true"
}

// compareGolden compares got against a golden file.
// When -update is set, it writes the golden file instead.
func compareGolden(t *testing.T, golden string, got []byte) {
	t.Helper()
	goldenPath := filepath.Join("testdata", golden)
	if shouldUpdate() {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}

// testFieldDef defines a struct field for test type construction.
type testFieldDef struct {
	Name string
	Type types.Type
	Tag  string
}

// testStruct constructs a *types.Struct from field definitions.
func testStruct(fields ...testFieldDef) *types.Struct {
	vars := make([]*types.Var, len(fields))
	tags := make([]string, len(fields))
	for i, f := range fields {
		vars[i] = types.NewVar(token.NoPos, nil, f.Name, f.Type)
		tags[i] = f.Tag
	}
	return types.NewStruct(vars, tags)
}

func testEvent(typeName, subject string, private bool) *model.Event {
	return &model.Event{
		TypeName:         typeName,
		Subject:          subject,
		HasTargetUserIDs: private,
	}
}

func testEventHandler(
	name, eventTypeName string, opts ...func(*model.EventHandler),
) *model.EventHandler {
	eh := &model.EventHandler{
		Name:          name,
		EventTypeName: eventTypeName,
		InputEvent:    &model.Input{Name: "e"},
		InputSSE:      &model.Input{Name: "sse"},
	}
	for _, o := range opts {
		o(eh)
	}
	return eh
}

func withEHSession(eh *model.EventHandler)   { eh.InputSession = &model.Input{Name: "sess"} }
func withEHErr(eh *model.EventHandler)       { eh.OutputErr = &model.Output{Name: "err"} }
func withEHSignals(eh *model.EventHandler)   { eh.InputSignals = &model.Input{Name: "signals"} }
func withEHSessToken(eh *model.EventHandler) { eh.InputSessionToken = &model.Input{Name: "sessToken"} }

func TestWriteEvSubjPageFuncs(t *testing.T) {
	privateEvent := testEvent("EventMessagingSent", "messaging.sent", true)
	publicEvent := testEvent("EventPostsArchived", "posts.archived", false)

	tests := map[string]struct {
		pages    []*model.Page
		eventMap map[string]*model.Event
		golden   string
	}{
		"private only": {
			pages: []*model.Page{{
				TypeName: "PageMessaging",
				Route:    "/messaging/",
				EventHandlers: []*model.EventHandler{
					testEventHandler("MessagingSent", "EventMessagingSent", withEHSession),
				},
			}},
			eventMap: map[string]*model.Event{
				"EventMessagingSent": privateEvent,
			},
			golden: "app_evsubj_private_only.txt",
		},
		"public only": {
			pages: []*model.Page{{
				TypeName: "PageFeed",
				Route:    "/feed/",
				EventHandlers: []*model.EventHandler{
					testEventHandler("PostsArchived", "EventPostsArchived"),
				},
			}},
			eventMap: map[string]*model.Event{
				"EventPostsArchived": publicEvent,
			},
			golden: "app_evsubj_public_only.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.eventMap = tt.eventMap
			w.writeEvSubjPageFuncs(tt.pages)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWriteAppErrHelpers(t *testing.T) {
	tests := map[string]struct {
		app    *model.App
		golden string
	}{
		"without recover500": {
			app:    &model.App{PkgPath: testAppPkgPath},
			golden: "app_err_helpers_no_recover.txt",
		},
		"with recover500": {
			app: &model.App{
				PkgPath:      testAppPkgPath,
				Recover500:   &ast.Ident{Name: "Recover500"},
				PageError500: &model.Page{TypeName: "PageError500"},
			},
			golden: "app_err_helpers_with_recover.txt",
		},
	}

	w := Writer{prometheus: true}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.writeAppErrHelpers(tt.app, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWriteAppActionHandler(t *testing.T) {
	querySt := testStruct(
		testFieldDef{"Term", types.Typ[types.String], `query:"t"`},
		testFieldDef{"Page", types.Typ[types.Int64], `query:"p"`},
	)
	queryNonInt64St := testStruct(
		testFieldDef{"Count", types.Typ[types.Int], `query:"n"`},
		testFieldDef{"Offset", types.Typ[types.Uint32], `query:"o"`},
	)
	pathSt := testStruct(
		testFieldDef{"Slug", types.Typ[types.String], `path:"slug"`},
	)
	signalsSt := testStruct(
		testFieldDef{"Message", types.Typ[types.String], `json:"message"`},
	)
	publicEvent := testEvent("EventPostsArchived", "posts.archived", false)

	tests := map[string]struct {
		handler *model.Handler
		app     *model.App
		golden  string
	}{
		"minimal void": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Ping", Route: "/ping/{$}",
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_minimal.txt",
		},
		"error only": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Check", Route: "/check/{$}",
				OutputErr: &model.Output{Name: "err"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_error_only.txt",
		},
		"with SSE": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Stream", Route: "/stream/{$}",
				InputSSE: &model.Input{Name: "sse"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_with_sse.txt",
		},
		"session without token": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Update", Route: "/update/{$}",
				InputSession: &model.Input{Name: "sess"},
				OutputErr:    &model.Output{Name: "err"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_session_no_token.txt",
		},
		"signals and query": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Search", Route: "/search/{$}",
				InputSession: &model.Input{Name: "sess"},
				InputSignals: &model.Input{
					Name: "signals",
					Type: namedAppType("SignalsSearch", signalsSt),
				},
				InputQuery: &model.Input{
					Name: "query",
					Type: namedAppType("QuerySearch", querySt),
				},
				OutputErr: &model.Output{Name: "err"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_signals_query.txt",
		},
		"query non-int64 int types": {
			handler: &model.Handler{
				HTTPMethod: "GET", Name: "List", Route: "/list/{$}",
				InputQuery: &model.Input{
					Name: "query",
					Type: namedAppType("QueryList", queryNonInt64St),
				},
				OutputBody: &model.TemplComponent{Output: &model.Output{Name: "body"}},
				OutputErr:  &model.Output{Name: "err"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_query_non_int64.txt",
		},
		"path and dispatch public": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Archive", Route: "/archive/{$}",
				InputSession: &model.Input{Name: "sess"},
				InputPath: &model.Input{
					Name: "path",
					Type: namedAppType("PathArchive", pathSt),
				},
				InputDispatch: &model.InputDispatch{
					Name:           "dispatch",
					EventTypeNames: []string{"EventPostsArchived"},
				},
				OutputErr: &model.Output{Name: "err"},
			},
			app: &model.App{
				PkgPath: testAppPkgPath, Fset: token.NewFileSet(),
				Events: []*model.Event{publicEvent},
			},
			golden: "app_action_path_dispatch.txt",
		},
		"redirect outputs": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Login", Route: "/login/{$}",
				InputSession:         &model.Input{Name: "sess"},
				InputSessionToken:    &model.Input{Name: "sessToken"},
				OutputCloseSession:   &model.Output{Name: "closeSession"},
				OutputRedirect:       &model.Output{Name: "redirect"},
				OutputRedirectStatus: &model.Output{Name: "redirectStatus"},
				OutputNewSession:     &model.Output{Name: "newSession"},
				OutputErr:            &model.Output{Name: "err"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_redirect_outputs.txt",
		},
		"body output": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Render", Route: "/render/{$}",
				OutputBody:           &model.TemplComponent{Output: &model.Output{Name: "body"}},
				OutputEnableBgStream: &model.Output{Name: "enableBgStream"},
				OutputDisableRefresh: &model.Output{Name: "disableRefresh"},
				OutputErr:            &model.Output{Name: "err"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_action_body_output.txt",
		},
		"body with global head": {
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Preview", Route: "/preview/{$}",
				InputSession: &model.Input{Name: "sess"},
				OutputBody:   &model.TemplComponent{Output: &model.Output{Name: "body"}},
				OutputErr:    &model.Output{Name: "err"},
			},
			app: &model.App{
				PkgPath: testAppPkgPath, Fset: token.NewFileSet(),
				Session:             &model.SessionType{},
				GlobalHeadGenerator: &ast.Ident{Name: "Head"},
			},
			golden: "app_action_body_global_head.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.buildEventMap(tt.app.Events)
			w.writeAppActionHandler(tt.handler, tt.app, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWriteSetupHandlers(t *testing.T) {
	tests := map[string]struct {
		app    *model.App
		golden string
	}{
		"mixed": {
			app: &model.App{
				PkgPath: testAppPkgPath,
				Pages: []*model.Page{
					{
						TypeName:           "PageIndex",
						Route:              "/",
						PageSpecialization: model.PageTypeIndex,
						GET: &model.HandlerGET{
							Handler:    &model.Handler{},
							OutputBody: &model.TemplComponent{Output: &model.Output{Name: "body"}},
						},
					},
					{TypeName: "PageNoGET", Route: "/noget/"},
					{
						TypeName: "PagePost",
						Route:    "/post/{slug}",
						GET: &model.HandlerGET{
							Handler:    &model.Handler{},
							OutputBody: &model.TemplComponent{Output: &model.Output{Name: "body"}},
						},
						EventHandlers: []*model.EventHandler{
							testEventHandler("PostsArchived", "EventPostsArchived"),
						},
						Actions: []*model.Handler{
							{HTTPMethod: "POST", Name: "Comment", Route: "/post/{slug}/comment"},
						},
					},
				},
				Actions: []*model.Handler{
					{HTTPMethod: "POST", Name: "SignOut", Route: "/sign-out"},
				},
				Events: []*model.Event{
					testEvent("EventPostsArchived", "posts.archived", false),
				},
			},
			golden: "app_setup_handlers_mixed.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.buildEventMap(tt.app.Events)
			w.writeSetupHandlers(tt.app)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWriteRender404(t *testing.T) {
	tests := map[string]struct {
		app    *model.App
		golden string
	}{
		"basic": {
			app: &model.App{
				PkgPath:             testAppPkgPath,
				Session:             &model.SessionType{},
				GlobalHeadGenerator: &ast.Ident{Name: "Head"},
				PageError404: &model.Page{
					TypeName:           "PageError404",
					Route:              "/not-found/{$}",
					PageSpecialization: model.PageTypeError404,
					GET: &model.HandlerGET{
						Handler: &model.Handler{
							InputSession: &model.Input{Name: "sess"},
							OutputErr:    &model.Output{Name: "err"},
						},
						OutputBody: &model.TemplComponent{
							Output: &model.Output{Name: "body"},
						},
					},
				},
			},
			golden: "app_render404_basic.txt",
		},
		"with head and redirect": {
			app: &model.App{
				PkgPath:             testAppPkgPath,
				Session:             &model.SessionType{},
				GlobalHeadGenerator: &ast.Ident{Name: "Head"},
				PageError404: &model.Page{
					TypeName:           "PageError404",
					Route:              "/not-found/{$}",
					PageSpecialization: model.PageTypeError404,
					GET: &model.HandlerGET{
						Handler: &model.Handler{
							InputRequest:   &model.Input{Name: "r"},
							InputSession:   &model.Input{Name: "sess"},
							OutputRedirect: &model.Output{Name: "redirect"},
							OutputErr:      &model.Output{Name: "err"},
						},
						OutputBody: &model.TemplComponent{
							Output: &model.Output{Name: "body"},
						},
						OutputHead: &model.TemplComponent{
							Output: &model.Output{Name: "head"},
						},
					},
				},
			},
			golden: "app_render404_head_redirect.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.writeRender404(tt.app, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWriteAppWriteHTML(t *testing.T) {
	tests := map[string]struct {
		app    *model.App
		golden string
	}{
		"with global head": {
			app: &model.App{
				PkgPath:             testAppPkgPath,
				Session:             &model.SessionType{},
				GlobalHeadGenerator: &ast.Ident{Name: "Head"},
			},
			golden: "app_writehtml_global_head.txt",
		},
		"without global head": {
			app:    &model.App{PkgPath: testAppPkgPath},
			golden: "app_writehtml_no_global_head.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.writeAppWriteHTML(tt.app, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWriteEventSubjectConsts(t *testing.T) {
	tests := map[string]struct {
		events []*model.Event
		golden string
	}{
		"mixed public and private": {
			events: []*model.Event{
				testEvent("EventMessagingSent", "messaging.sent", true),
				testEvent("EventPostsArchived", "posts.archived", false),
			},
			golden: "app_evsubj_consts_mixed.txt",
		},
		"public only": {
			events: []*model.Event{
				testEvent("EventPostsArchived", "posts.archived", false),
			},
			golden: "app_evsubj_consts_public_only.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.writeEventSubjectConsts(tt.events)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWritePageGETHandler(t *testing.T) {
	tests := map[string]struct {
		page   *model.Page
		app    *model.App
		golden string
	}{
		"with session token": {
			page: &model.Page{
				TypeName: "PageSettings",
				Route:    "/settings/",
				GET: &model.HandlerGET{
					Handler: &model.Handler{
						InputSession:      &model.Input{Name: "sess"},
						InputSessionToken: &model.Input{Name: "sessToken"},
						OutputErr:         &model.Output{Name: "err"},
					},
					OutputBody: &model.TemplComponent{
						Output: &model.Output{Name: "body"},
					},
				},
			},
			app: &model.App{
				PkgPath:             testAppPkgPath,
				Fset:                token.NewFileSet(),
				Session:             &model.SessionType{},
				GlobalHeadGenerator: &ast.Ident{Name: "Head"},
			},
			golden: "app_page_get_session_token.txt",
		},
		"with redirect and status": {
			page: &model.Page{
				TypeName: "PageOldRoute",
				Route:    "/old-route/",
				GET: &model.HandlerGET{
					Handler: &model.Handler{
						InputRequest:         &model.Input{Name: "r"},
						OutputRedirect:       &model.Output{Name: "redirect"},
						OutputRedirectStatus: &model.Output{Name: "redirectStatus"},
						OutputErr:            &model.Output{Name: "err"},
					},
					OutputBody: &model.TemplComponent{
						Output: &model.Output{Name: "body"},
					},
				},
			},
			app: &model.App{
				PkgPath:             testAppPkgPath,
				Fset:                token.NewFileSet(),
				GlobalHeadGenerator: &ast.Ident{Name: "Head"},
			},
			golden: "app_page_get_redirect_status.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.writePageGETHandler(tt.page, tt.app, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWritePageGETStreamHandler(t *testing.T) {
	publicEvent := testEvent("EventPostsCreated", "posts.created", false)
	privateEvent := testEvent("EventMessagingSent", "messaging.sent", true)

	tests := map[string]struct {
		page     *model.Page
		app      *model.App
		eventMap map[string]*model.Event
		golden   string
	}{
		"public only events": {
			page: &model.Page{
				TypeName: "PageFeed",
				Route:    "/feed/",
				EventHandlers: []*model.EventHandler{
					testEventHandler("PostsCreated", "EventPostsCreated", withEHErr),
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
				Events:  []*model.Event{publicEvent},
			},
			eventMap: map[string]*model.Event{
				"EventPostsCreated": publicEvent,
			},
			golden: "app_stream_public_only.txt",
		},
		"handler without error": {
			page: &model.Page{
				TypeName: "PageFeed",
				Route:    "/feed/",
				EventHandlers: []*model.EventHandler{
					testEventHandler("PostsCreated", "EventPostsCreated"),
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
				Events:  []*model.Event{publicEvent},
			},
			eventMap: map[string]*model.Event{
				"EventPostsCreated": publicEvent,
			},
			golden: "app_stream_handler_no_err.txt",
		},
		"handler with signals and session token": {
			page: &model.Page{
				TypeName: "PageChat",
				Route:    "/chat/",
				EventHandlers: []*model.EventHandler{
					testEventHandler("MessagingSent", "EventMessagingSent",
						withEHSession, withEHSessToken, withEHSignals, withEHErr),
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
				Events:  []*model.Event{privateEvent},
			},
			eventMap: map[string]*model.Event{
				"EventMessagingSent": privateEvent,
			},
			golden: "app_stream_signals_sesstoken.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.eventMap = tt.eventMap
			w.writePageGETStreamHandler(tt.page, tt.app, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWritePageGETStreamAnonHandler(t *testing.T) {
	privateEvent := testEvent("EventMessagingSent", "messaging.sent", true)
	publicEvent := testEvent("EventPostsCreated", "posts.created", false)
	publicEvent2 := testEvent("EventPostsArchived", "posts.archived", false)

	tests := map[string]struct {
		page     *model.Page
		eventMap map[string]*model.Event
		golden   string
	}{
		"single public handler": {
			page: &model.Page{
				TypeName: "PageFeed",
				Route:    "/feed/",
				EventHandlers: []*model.EventHandler{
					testEventHandler("MessagingSent", "EventMessagingSent",
						withEHSession, withEHErr),
					testEventHandler("PostsCreated", "EventPostsCreated", withEHErr),
				},
			},
			eventMap: map[string]*model.Event{
				"EventMessagingSent": privateEvent,
				"EventPostsCreated":  publicEvent,
			},
			golden: "app_stream_anon_single_public.txt",
		},
		"multiple public handlers": {
			page: &model.Page{
				TypeName: "PageFeed",
				Route:    "/feed/",
				EventHandlers: []*model.EventHandler{
					testEventHandler("MessagingSent", "EventMessagingSent",
						withEHSession, withEHErr),
					testEventHandler("PostsCreated", "EventPostsCreated", withEHErr),
					testEventHandler("PostsArchived", "EventPostsArchived", withEHErr),
				},
			},
			eventMap: map[string]*model.Event{
				"EventMessagingSent": privateEvent,
				"EventPostsCreated":  publicEvent,
				"EventPostsArchived": publicEvent2,
			},
			golden: "app_stream_anon_multi_public.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.eventMap = tt.eventMap
			w.writePageGETStreamAnonHandler(tt.page, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWritePageActionHandler(t *testing.T) {
	tests := map[string]struct {
		page    *model.Page
		handler *model.Handler
		app     *model.App
		golden  string
	}{
		"void no error": {
			page: &model.Page{TypeName: "PageSettings", Route: "/settings/"},
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Reset", Route: "/settings/reset/{$}",
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_page_action_void_no_err.txt",
		},
		"void with error": {
			page: &model.Page{TypeName: "PageSettings", Route: "/settings/"},
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Reset", Route: "/settings/reset/{$}",
				InputSSE:  &model.Input{Name: "sse"},
				OutputErr: &model.Output{Name: "err"},
			},
			app:    &model.App{PkgPath: testAppPkgPath, Fset: token.NewFileSet()},
			golden: "app_page_action_void_with_err.txt",
		},
		"body with global head": {
			page: &model.Page{TypeName: "PageSettings", Route: "/settings/"},
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Preview", Route: "/settings/preview/{$}",
				InputSession: &model.Input{Name: "sess"},
				InputSSE:     &model.Input{Name: "sse"},
				OutputBody:   &model.TemplComponent{Output: &model.Output{Name: "body"}},
				OutputErr:    &model.Output{Name: "err"},
			},
			app: &model.App{
				PkgPath:             testAppPkgPath,
				Fset:                token.NewFileSet(),
				Session:             &model.SessionType{},
				GlobalHeadGenerator: &ast.Ident{Name: "Head"},
			},
			golden: "app_page_action_body_global_head.txt",
		},
		"body without session": {
			page: &model.Page{TypeName: "PageDashboard", Route: "/dashboard/"},
			handler: &model.Handler{
				HTTPMethod: "POST", Name: "Refresh", Route: "/dashboard/refresh/{$}",
				InputSSE:   &model.Input{Name: "sse"},
				OutputBody: &model.TemplComponent{Output: &model.Output{Name: "body"}},
				OutputErr:  &model.Output{Name: "err"},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
			},
			golden: "app_page_action_body_no_session.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.writePageActionHandler(tt.page, tt.handler, tt.app, testAppPkg)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}

func TestWriteGETBodyAttrs(t *testing.T) {
	privateEvent := testEvent("EventMessagingSent", "messaging.sent", true)
	publicEvent := testEvent("EventPostsCreated", "posts.created", false)

	tests := map[string]struct {
		page   *model.Page
		app    *model.App
		golden string
	}{
		"anon stream static path": {
			page: &model.Page{
				TypeName: "PageFeed",
				Route:    "/feed/",
				GET: &model.HandlerGET{
					Handler: &model.Handler{
						InputSession: &model.Input{Name: "sess"},
					},
				},
				EventHandlers: []*model.EventHandler{
					testEventHandler("MessagingSent", "EventMessagingSent",
						withEHSession),
					testEventHandler("PostsCreated", "EventPostsCreated"),
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
				Events: []*model.Event{
					privateEvent, publicEvent,
				},
			},
			golden: "app_body_attrs_anon_static.txt",
		},
		"public only stream static path": {
			page: &model.Page{
				TypeName: "PageFeed",
				Route:    "/feed/",
				GET: &model.HandlerGET{
					Handler: &model.Handler{},
				},
				EventHandlers: []*model.EventHandler{
					testEventHandler("PostsCreated", "EventPostsCreated"),
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
				Events:  []*model.Event{publicEvent},
			},
			golden: "app_body_attrs_public_only.txt",
		},
		"enable bg stream": {
			page: &model.Page{
				TypeName: "PageChat",
				Route:    "/chat/",
				GET: &model.HandlerGET{
					Handler: &model.Handler{
						InputSession:         &model.Input{Name: "sess"},
						OutputEnableBgStream: &model.Output{Name: "enableBgStream"},
					},
				},
				EventHandlers: []*model.EventHandler{
					testEventHandler("MessagingSent", "EventMessagingSent",
						withEHSession),
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
				Events:  []*model.Event{privateEvent},
			},
			golden: "app_body_attrs_enable_bg_stream.txt",
		},
		"reflect signals int": {
			page: &model.Page{
				TypeName: "PageSearch",
				Route:    "/search/",
				GET: &model.HandlerGET{
					Handler: &model.Handler{
						InputQuery: &model.Input{
							Name: "query",
							Type: model.Type{Resolved: testStruct(
								testFieldDef{
									"Page", types.Typ[types.Int64],
									`query:"p" reflectsignal:"page"`,
								},
							)},
						},
					},
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
			},
			golden: "app_body_attrs_reflect_int.txt",
		},
		"reflect signals root route": {
			page: &model.Page{
				TypeName:           "PageIndex",
				Route:              "/",
				PageSpecialization: model.PageTypeIndex,
				GET: &model.HandlerGET{
					Handler: &model.Handler{
						InputQuery: &model.Input{
							Name: "query",
							Type: model.Type{Resolved: testStruct(
								testFieldDef{
									"Term", types.Typ[types.String],
									`query:"q" reflectsignal:"term"`,
								},
							)},
						},
					},
				},
			},
			app: &model.App{
				PkgPath: testAppPkgPath,
				Fset:    token.NewFileSet(),
			},
			golden: "app_body_attrs_reflect_root.txt",
		},
	}

	w := Writer{}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w.Reset()
			w.buildEventMap(tt.app.Events)
			w.writeGETBodyAttrs(tt.page)
			compareGolden(t, tt.golden, w.Buf)
		})
	}
}
