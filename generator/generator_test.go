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
// End-to-end coverage lives in example/reprod.
func TestGenerateStateful(t *testing.T) {
	app, errs := parser.Parse(
		filepath.Join("..", "example", "reprod", "app"),
	)
	require.Zero(t, errs.Len(), "unexpected parser errors: %s", errs.Error())
	require.NotNil(t, app, "parser returned nil model")

	tmpDir := t.TempDir()
	err := generator.Generate(tmpDir, "datapagesgen", app, 0o644, generator.Options{
		GenImport: "github.com/romshark/datapages/example/reprod/datapagesgen",
	})
	require.NoError(t, err)

	genPath := filepath.Join(tmpDir, "app_gen.go")
	compareFile(t, "app_gen.go", genPath,
		filepath.Join("..", "example", "reprod", "datapagesgen", "app_gen.go"))

	b, err := os.ReadFile(genPath)
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
