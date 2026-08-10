package generator_test

import (
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser"
)

func TestGenerateClassifieds(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "example", "classifieds", "app"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		Prometheus:      true,
		AssetsURLPrefix: "/static/",
		AssetsDir:       "static",
		AppDir:          "app",
		GenImport:       "github.com/romshark/datapages/example/classifieds/datapagesgen",
	})
	require.NoError(t, err)

	compareFile(t, "app_gen.go",
		filepath.Join(tmpDir, "app_gen.go"),
		filepath.Join("..", "example", "classifieds",
			"datapagesgen", "app_gen.go"))
	compareFile(t, "action/action_gen.go",
		filepath.Join(tmpDir, "action", "action_gen.go"),
		filepath.Join("..", "example", "classifieds",
			"datapagesgen", "action", "action_gen.go"))
	compareFile(t, "href/href_gen.go",
		filepath.Join(tmpDir, "href", "href_gen.go"),
		filepath.Join("..", "example", "classifieds",
			"datapagesgen", "href", "href_gen.go"))
	compareFile(t, "assets/assets_gen.go",
		filepath.Join(tmpDir, "assets", "assets_gen.go"),
		filepath.Join("..", "example", "classifieds",
			"datapagesgen", "assets", "assets_gen.go"))
}

// TestGenerateStateful covers the per-tab state runtime and the routing of
// state-id-scoped events (events that carry SubjectStateID).
//
// Four places in the generated code must use the same subject:
// the prefix constant, the page's subscription, the publish call in the dispatch closure,
// and the match in the stream loop. If any two of them disagree, the event is never
// delivered and nothing reports it. Each place is checked on its own below.
// A failure then names the place that is wrong.
func TestGenerateStateful(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state_subject_id"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state_subject_id/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	// The page subscribes to the prefix plus the state id.
	// The wildcard constant EvSubjFiltersUpdated ("filters.updated.*") is
	// the stream filter. It must not be used to build a subscription.
	require.Contains(t, got, `EvSubjPrefFiltersUpdated = "filters.updated."`,
		"state-id-scoped events need a subject prefix constant")
	require.Contains(t, got, "EvSubjPrefFiltersUpdated + stateID",
		"subscription must be built from the prefix, not the wildcard constant")

	// The dispatch closure publishes to that same subject.
	require.Contains(t, got, `subj := "filters.updated." + p0`,
		"dispatch must publish to the subject the page subscribes to")

	// Incoming events are matched by prefix. The real subject ends with the state id.
	// No equality test against a constant can match it.
	require.Contains(t,
		got, "case strings.HasPrefix(msg.Subject, EvSubjPrefFiltersUpdated):",
		"state-id-scoped events must be matched by subject prefix")

	// The instance id is used as one subject token, exactly as it is.
	// Its separator must not be ".". A "." would split the id into two tokens
	// and break single-"*" filters like the one MessageBrokerStreamSubjects exports.
	// The separator also stays outside the base64url alphabet to
	// keep the split point unambiguous.
	require.Contains(t, got, "const stateInstanceIDSep = '~'",
		"the instance id must stay a single message-broker subject token")
}

// TestGenerateGenericAbstract covers pages that embed a generic abstract page,
// such as PageA embedding Base[StateA].
//
// The page constructor must instantiate the embedded type.
// A generic type cannot appear in a composite literal without its type argument,
// and the argument differs per embed site.
func TestGenerateGenericAbstract(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state_generic_abstract"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state_generic_abstract/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)

	// Collect the embed initialization lines. Asserting on those keeps a
	// failure readable instead of printing the whole generated file.
	var embedLines []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "Base:") {
			embedLines = append(embedLines, strings.TrimSpace(line))
		}
	}
	require.NotEmpty(t, embedLines, "no embed initialization generated")
	embeds := strings.Join(embedLines, "\n")

	require.NotContains(t, embeds, ".Base{",
		"the embedded generic type needs a type argument")
	require.Contains(t, embeds, "StateA]{",
		"PageA must construct its embed as Base[StateA]")
	require.Contains(t, embeds, "StateB]{",
		"PageB must construct its embed as Base[StateB]")
}

// TestGenerateStateFromGenericEmbedOnly covers a page that reaches its state
// type only through the type argument of an embedded generic abstract page.
//
// The state runtime is emitted per state type. A page bound this way needs
// the same slot type, pool and allocation as a page that names the state
// type in one of its own handlers.
func TestGenerateStateFromGenericEmbedOnly(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state_generic_embed_only"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state_generic_embed_only/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	// Asserting with require.True keeps a failure short.
	// require.Contains would print the whole generated file.
	mustContain := func(sub, why string) {
		t.Helper()
		require.True(t, strings.Contains(got, sub), "%s, missing: %s", why, sub)
	}
	mustContain("type stateSlotTabState struct",
		"the state runtime must be emitted for the embedded state type")
	mustContain("var slot *stateSlotTabState",
		"the stream handler must declare the slot it passes to handlers")
	mustContain("s.allocateTabState(instanceID, streamID)",
		"the stream handler must allocate the slot on open")
}

// TestGenerateStateSlotLifecycle covers the window between finding a slot and using it.
//
// A slot is found through a map, then locked. The grace timer runs on its own
// goroutine and can return the state to the pool in between.
// Every user of a slot must therefore re-check under the lock that the slot is
// still alive, and the timer must leave slots alone that a stream reattached to.
func TestGenerateStateSlotLifecycle(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	mustContain := func(sub, why string) {
		t.Helper()
		require.True(t, strings.Contains(got, sub), "%s, missing: %s", why, sub)
	}

	mustContain("dead     bool",
		"a slot must stay recognizable after its state went back to the pool")
	mustContain("if slot.streamID != 0 || slot.dead {",
		"the grace timer must skip a reattached or already released slot")
	mustContain("if slot.dead {\n\t\treturn nil, false\n\t}",
		"reconnect must refuse a released slot")

	// Action handlers hold the lock for the whole call.
	// The check belongs after the lock and before the state reaches the user handler.
	mustContain("slot.mu.Lock()\n\tdefer slot.mu.Unlock()\n\tif slot.dead {",
		"a stateful action must reject a released slot")

	// StreamClose runs on the watchdog goroutine, outside net/http. A panic
	// there is not recovered and ends the process.
	mustContain("if !slot.dead {",
		"StreamClose must not read state of a released slot")

	// The stream loop keeps its slot across events. It outlives the grace timer.
	mustContain("if slot.dead {\n\t\t\t\t\t\tslot.mu.Unlock()\n\t\t\t\t\t\tcontinue",
		"the stream loop must skip a released slot")
}

// TestGenerateStateSlotOwnership covers which stream may end a slot's life.
//
// One tab can hold two SSE streams at once. A browser opens the new one before
// the server notices that the old one is gone, and both carry the same instance id.
// The slot then belongs to the newer stream.
// A close arriving from the older stream must leave it alone.
func TestGenerateStateSlotOwnership(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	mustContain := func(sub, why string) {
		t.Helper()
		require.True(t, strings.Contains(got, sub), "%s, missing: %s", why, sub)
	}

	mustContain(
		"func (s *Server) closeStreamStateIndex(id string, streamID uint64) {",
		"closing a slot must name the stream that closes",
	)
	mustContain("if slot.streamID != streamID {",
		"only the attached stream may detach the slot and start the timer")
	mustContain("s.closeStreamStateIndex(instanceID, streamID)",
		"the close hook must pass the stream it belongs to")

	// An id outlives the slot behind it. A new stream can allocate a fresh
	// slot under the same id while the previous one is still being released.
	mustContain("stateInstancesStateIndex.CompareAndDelete(id, slot)",
		"releasing a slot must not drop a newer slot under the same id")
}

// TestGenerateSharedStateType covers two pages bound to the same state type,
// which happens as soon as they embed the same stateful abstract page.
//
// The state runtime belongs to the state type, not to the page. One slot type,
// one pool and one instance map serve every page that uses that state.
func TestGenerateSharedStateType(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state_shared"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state_shared/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	for _, decl := range []string{
		"type stateSlotTabContext struct",
		"var statePoolTabContext",
		"var stateInstancesTabContext",
		"func (s *Server) allocateTabContext(",
		"func (s *Server) lookupTabContext(",
		"func (s *Server) reconnectTabContext(",
		"func (s *Server) closeStreamTabContext(",
	} {
		require.Equal(t, 1, strings.Count(got, decl),
			"declared once for the state type, not once per page: %s", decl)
	}
}

// TestGenerateStateFetchWrapper covers the inline script that replaces
// globalThis.fetch on a stateful page and adds the instance id to every
// same-origin Datastar request.
//
// The script is the only client-side part of the state feature.
// It reads and changes every request the app makes, the id it holds stands
// for one tab's server-side state, and text that reaches it runs as page JavaScript.
func TestGenerateStateFetchWrapper(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	mustContain := func(sub, why string) {
		t.Helper()
		require.True(t, strings.Contains(got, sub), "%s, missing: %s", why, sub)
	}

	// The value reaches the page as a JavaScript string literal.
	// Only a value the server signed may get there.
	mustContain("s.verifyStateInstanceID(id)",
		"the embedded id must be verified before it reaches the page")

	// The id is a bearer token for one tab.
	// A shared cache must not hand the same page to a second visitor.
	mustContain(`w.Header().Set("Cache-Control", "no-store")`,
		"a stateful page response must not be stored")

	// The wrapper replaces fetch.
	// Anything that fetches before it is installed sends no instance header.
	require.Less(t,
		strings.Index(got, "__dpInstance"), strings.Index(got, "+s.datastarJSSrc+"),
		"the wrapper must be written before the Datastar bundle")
	mustContain("<script>(() => {",
		"the wrapper must run at parse time, which a module script does not")

	// The header is a credential. It belongs to this origin only.
	mustContain("location.origin",
		"the instance header must stay on same-origin requests")

	// A rejected action reloads the page.
	// A reload that changes nothing must not reload again.
	mustContain("sessionStorage",
		"a reload after 409 must happen at most once per document")
}

// TestGenerateStateRouteKey covers the value that names a tab in message broker subjects.
//
// The Datapages-Instance id is what a request presents to claim a tab's state.
// Subjects travel further than the request does: into broker logs,
// stream storage, traces and metrics.
// The value used there is derived from the id and grants nothing on its own.
func TestGenerateStateRouteKey(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state_subject_id"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state_subject_id/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	mustContain := func(sub, why string) {
		t.Helper()
		require.True(t, strings.Contains(got, sub), "%s, missing: %s", why, sub)
	}

	mustContain("func (s *Server) stateRouteKey(id string) string {",
		"subjects need a value that is not the id itself")
	mustContain("stateID := s.stateRouteKey(instanceID)",
		"the stateID a handler receives is the derived value")

	// The subscription subject and the value handed to handlers must be the
	// same derived value. A handler dispatches events with what it receives.
	mustContain("evSubjPageIndex(stateID)",
		"the subscription must use the derived value")
	require.NotContains(t, got, "evSubjPageIndex(instanceID)",
		"the id must not reach a subject")
	require.NotContains(t, got, "slot.state, instanceID",
		"the id must not reach a handler")
}

// TestGenerateStateConfigRequired covers what happens when an app with
// stateful pages starts without WithStateConfig.
//
// Without it the server has no key to sign instance identifiers with.
// Every stateful page then fails, and it fails on request rather than on start.
func TestGenerateStateConfigRequired(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	require.True(t, strings.Contains(got, "if s.stateConf == nil {"),
		"NewServer must reject a stateful app without a state config")
	require.True(t, strings.Contains(got, `panic("missing state config`),
		"the failure must name what is missing, like the other prerequisites")
}

// TestGenerateStateInstanceLimit covers how many instances a server keeps.
//
// A page load mints an identifier and an SSE connect turns it into a live
// instance that outlives its stream by the grace period. Both are cheap to
// ask for from outside, which puts the count under the caller's control.
func TestGenerateStateInstanceLimit(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "state"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "datapagestest/fixture/state/datapagesgen",
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	got := string(b)

	mustContain := func(sub, why string) {
		t.Helper()
		require.True(t, strings.Contains(got, sub), "%s, missing: %s", why, sub)
	}

	mustContain("MaxConcurrentInstances int",
		"the limit belongs to the server configuration")
	mustContain("var stateLiveInstances atomic.Int64",
		"the count spans all state types, since memory does too")
	mustContain("stateLiveInstances.Add(-1)",
		"releasing an instance must lower the count")

	// A refused allocation has to reach the stream open hook. It answers
	// with an error instead of a slot nothing can use.
	mustContain("if slot == nil {",
		"the stream open hook must handle a refused allocation")

	// The stream request commits 200 and the SSE headers before the open
	// hook runs. A full server has to say so before that point, while the
	// status line is still free.
	mustContain("func (s *Server) stateHasCapacity() bool {",
		"the stream handler needs to ask about capacity before it opens")
	mustContain(
		"if _, live := s.lookupStateIndex(instanceID); !live && !s.stateHasCapacity() {",
		"a reconnect needs no new instance and must pass the check")
	mustContain("http.StatusServiceUnavailable",
		"a full server is a temporary condition, not a broken request")
}

func TestGenerateCmd(t *testing.T) {
	tests := map[string]struct {
		appImport  string
		genImport  string
		genPkgName string
		hasSession bool
	}{
		"with session": {
			appImport:  "example.com/myapp/app",
			genImport:  "example.com/myapp/datapagesgen",
			genPkgName: "datapagesgen",
			hasSession: true,
		},
		"without session": {
			appImport:  "example.com/myapp/app",
			genImport:  "example.com/myapp/mygen",
			genPkgName: "mygen",
			hasSession: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			err := generator.GenerateCmd(
				tmpDir, tt.appImport, tt.genImport, tt.genPkgName,
				true, tt.hasSession, 0o644,
			)
			require.NoError(t, err)

			data, err := os.ReadFile(filepath.Join(tmpDir, "main.go"))
			require.NoError(t, err)

			src := string(data)
			require.True(t, strings.HasPrefix(src, "package main"))
			require.Contains(t, src, tt.appImport)
			require.Contains(t, src, tt.genImport)
			require.Contains(t, src, tt.genPkgName+".NewServer")

			if tt.hasSession {
				require.Contains(t, src, "sessionManager")
				require.Contains(t, src, "WithAuth")
				require.Contains(t, src, "WithCSRFProtection")
			} else {
				require.NotContains(t, src, "sessionManager")
				require.NotContains(t, src, "WithAuth")
				require.NotContains(t, src, "WithCSRFProtection")
				require.NotContains(t, src, "app.Session")
			}
		})
	}
}

// TestGenerateNoSession confirms issue #14: when the source package defines
// no Session type, the generated app_gen.go must not reference Session at all.
func TestGenerateNoSession(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", "minimal"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")
	require.Nil(t, app.Session, "minimal fixture must have no Session type")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, "app_gen.go"))
	require.NoError(t, err)
	src := string(data)

	// None of the session or CSRF symbols should appear when Session is absent.
	require.NotContains(t, src, "SessionManager",
		"generated code must not reference SessionManager when Session type is absent")
	require.NotContains(t, src, ".Session",
		"generated code must not reference .Session when Session type is absent")
	require.NotContains(t, src, "CSRFConfig",
		"generated code must not reference CSRFConfig when Session type is absent")
	require.NotContains(t, src, "WithCSRFProtection",
		"generated code must not reference WithCSRFProtection when Session type is absent")
}

func compareFile(t *testing.T, name, gotPath, wantPath string) {
	t.Helper()

	got, err := os.ReadFile(gotPath)
	require.NoError(t, err, "reading generated %s", name)

	want, err := os.ReadFile(wantPath)
	require.NoError(t, err, "reading reference %s", name)

	// Format both to normalize whitespace differences.
	gotFmt, err := format.Source(got)
	if err != nil {
		// If formatting fails, compare raw.
		gotFmt = got
	}
	wantFmt, err := format.Source(want)
	if err != nil {
		wantFmt = want
	}

	if string(gotFmt) != string(wantFmt) {
		t.Errorf("%s differs from reference.\nGenerated: %s\nReference: %s",
			name, gotPath, wantPath)
	}
}
