package skeleton_test

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator/skeleton"
)

// TestCIWorkflowInstallsPinnedTools tests what the scaffolded workflow installs.
// An unpinned CLI regenerates with whatever released last,
// and the workflow fails the build when that differs from the committed code.
func TestCIWorkflowInstallsPinnedTools(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		version string
		want    string
	}{
		"release":           {version: "1.2.3", want: "datapages@v1.2.3"},
		"built from source": {version: "", want: "datapages@latest"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := skeleton.CIWorkflow(tc.version)
			require.NoError(t, err)
			require.Contains(t, got, "go install "+
				"github.com/romshark/datapages/cmd/"+tc.want)
			require.Contains(t, got, "go install "+skeleton.TemplCmd)
			require.False(t, strings.Contains(got, "templ@latest"),
				"templ must be pinned")
		})
	}
}

// TestMainGoAppImportAlias tests the identifier the scaffolded main.go refers
// to the app package by. It keeps the declared name, which is what a developer
// reads, unless one of the imports already binds it: unaliased, an app package
// named "sessions" or "http" would bind one identifier to two packages and the
// file the scaffold just wrote would not compile.
func TestMainGoAppImportAlias(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		appPkg   string
		wantRef  string
		wantLine string
	}{
		"free name": {
			appPkg:   "app",
			wantRef:  "app.App",
			wantLine: `"example.com/m/app"`,
		},
		"collides with a module import": {
			appPkg:   "sessions",
			wantRef:  "dpapp.App",
			wantLine: `dpapp "example.com/m/app"`,
		},
		"collides with the standard library": {
			appPkg:   "http",
			wantRef:  "dpapp.App",
			wantLine: `dpapp "example.com/m/app"`,
		},
		"collides with an aliased path": {
			appPkg:   "nats",
			wantRef:  "dpapp.App",
			wantLine: `dpapp "example.com/m/app"`,
		},
		"collides with the generated package": {
			appPkg:   "datapagesgen",
			wantRef:  "dpapp.App",
			wantLine: `dpapp "example.com/m/app"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := skeleton.MainGo(
				"example.com/m/app", tt.appPkg,
				"example.com/m/app/datapagesgen", "datapagesgen",
				false, false, "struct{}", nil,
			)
			require.NoError(t, err)
			require.Contains(t, string(src), tt.wantLine)
			require.Contains(t, string(src), tt.wantRef)
		})
	}
}

// TestMainGoImportsAreKnown tests that every import main.go carries is one the
// alias decision knows about. An import the table does not list binds an
// identifier nothing checks the app package against.
func TestMainGoImportsAreKnown(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		hasSession bool
		hasState   bool
	}{
		"plain":           {},
		"session":         {hasSession: true},
		"state":           {hasState: true},
		"session + state": {hasSession: true, hasState: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sessionData := ""
			if tt.hasSession {
				sessionData = "struct{}"
			}
			src, err := skeleton.MainGo(
				"example.com/m/app", "app",
				"example.com/m/app/datapagesgen", "datapagesgen",
				true, tt.hasState, sessionData, nil,
			)
			require.NoError(t, err)

			f, err := parser.ParseFile(
				token.NewFileSet(), "main.go", src, parser.ImportsOnly,
			)
			require.NoError(t, err)

			known := skeleton.MainGoImportIdents()
			for _, imp := range f.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				require.NoError(t, err)
				switch path {
				case "example.com/m/app", "example.com/m/app/datapagesgen":
					continue // The app and generated packages, not fixed imports.
				}
				require.Contains(t, known, path,
					"main.go imports %s, which the alias decision does not know", path)
			}
		})
	}
}

// TestMainGoWiresStateConfig tests the option the scaffolded entry point
// carries for an application whose handlers take datapages.State[T].
//
// datapages.NewServer refuses to build such a server without
// datapages.WithStateConfig. An entry point written without it compiles and
// then exits on its first run, which is a project the scaffold never built.
func TestMainGoWiresStateConfig(t *testing.T) {
	t.Parallel()

	for name, hasState := range map[string]bool{
		"stateful":  true,
		"stateless": false,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := skeleton.MainGo(
				"example.com/m/app", "app",
				"example.com/m/app/datapagesgen", "datapagesgen",
				false, hasState, "", nil,
			)
			require.NoError(t, err)

			for _, want := range []string{
				"datapages.WithStateConfig(",
				`os.Getenv("STATE_HMAC_KEY")`,
				"withState(&opts)",
			} {
				if hasState {
					require.Contains(t, string(src), want)
					continue
				}
				require.NotContains(t, string(src), want,
					"a stateless entry point configures the state runtime")
			}
		})
	}
}
