package skeleton_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator/skeleton"
	"github.com/romshark/datapages/runtime/httpserve"
)

// TestCIWorkflowInstallsPinnedTools tests that the scaffolded workflow installs
// templ at its fixed version and Datapages at the version required by go.mod.
func TestCIWorkflowInstallsPinnedTools(t *testing.T) {
	t.Parallel()

	const pseudo = "v0.9.5-0.20260920120806-f9d471eadce2"
	for name, tc := range map[string]struct {
		modVersion string
		want       string
	}{
		"release":          {modVersion: "v1.2.3", want: "datapages@v1.2.3"},
		"pseudo-version":   {modVersion: pseudo, want: "datapages@" + pseudo},
		"nothing required": {modVersion: "", want: "datapages@latest"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := skeleton.CIWorkflow(tc.modVersion)
			require.NoError(t, err)
			require.Contains(t, got, "go install "+
				"github.com/romshark/datapages/cmd/"+tc.want)
			require.Contains(t, got, "go install "+skeleton.TemplCmd)
			require.False(t, strings.Contains(got, "templ@latest"),
				"templ must be pinned")
		})
	}
}

// TestCIWorkflowChecksOutBeforeSetupGo tests that the scaffolded workflow
// checks out the repository before installing Go. setup-go computes its cache
// key from go.mod and restores no cache when the file is not there yet.
func TestCIWorkflowChecksOutBeforeSetupGo(t *testing.T) {
	t.Parallel()

	got, err := skeleton.CIWorkflow("v1.2.3")
	require.NoError(t, err)
	checkout := strings.Index(got, "uses: actions/checkout@")
	setupGo := strings.Index(got, "uses: actions/setup-go@")
	require.NotEqual(t, -1, checkout, "no checkout step")
	require.NotEqual(t, -1, setupGo, "no setup-go step")
	require.Less(t, checkout, setupGo, "checkout must come before setup-go")
}

// TestMainGoAppImportAlias tests the identifier the scaffolded main.go refers
// to the app package by. It keeps the declared name, which is what a developer
// reads, unless one of the imports already binds it: unaliased, an app package
// named "sessions" or "http" would bind 1 identifier to 2 packages and the
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
			src, err := skeleton.MainGo(skeleton.MainGoConfig{
				AppImport: "example.com/m/app", AppPkgName: tt.appPkg,
				GenImport: "example.com/m/app/datapagesgen", GenPkgName: "datapagesgen",
				SessionData: "struct{}",
			})
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
	}{
		"plain":   {},
		"session": {hasSession: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sessionData := ""
			if tt.hasSession {
				sessionData = "struct{}"
			}
			src, err := skeleton.MainGo(skeleton.MainGoConfig{
				AppImport: "example.com/m/app", AppPkgName: "app",
				GenImport: "example.com/m/app/datapagesgen", GenPkgName: "datapagesgen",
				Prometheus: true, SessionData: sessionData,
				AssetsVar: "StaticFS", DatastarJS: skeleton.RocketJS,
			})
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

// TestMainGoShutsDownOnSIGTERM tests that the scaffolded main.go cancels the
// context it passes to ListenAndServe on SIGTERM as well as SIGINT. Docker,
// Kubernetes and systemd stop a process with SIGTERM, whose default action ends
// the process without a graceful shutdown.
func TestMainGoShutsDownOnSIGTERM(t *testing.T) {
	t.Parallel()

	for name, sessionData := range map[string]string{
		"plain":   "",
		"session": "struct{}",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := skeleton.MainGo(skeleton.MainGoConfig{
				AppImport: "example.com/m/app", AppPkgName: "app",
				GenImport: "example.com/m/app/datapagesgen", GenPkgName: "datapagesgen",
				Prometheus: true, SessionData: sessionData,
			})
			require.NoError(t, err)

			f, err := parser.ParseFile(token.NewFileSet(), "main.go", src, 0)
			require.NoError(t, err)

			var signals []string
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || types.ExprString(call.Fun) != "signal.NotifyContext" {
					return true
				}
				for _, arg := range call.Args[1:] {
					signals = append(signals, types.ExprString(arg))
				}
				return false
			})
			require.Subset(t, signals, []string{"os.Interrupt", "syscall.SIGTERM"},
				"main.go does not cancel the server context on both signals:\n%s", src)
		})
	}
}

// TestMainGoServesAssetsAndDatastar tests that the scaffolded main.go passes
// the static files of the app package to WithAssets and a Datastar bundle other
// than the default to WithDatastarJS, and neither when there is none.
func TestMainGoServesAssetsAndDatastar(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		assetsVar, datastarJS string
		want, wantNot         []string
	}{
		"neither": {
			wantNot: []string{"WithAssets", "WithDatastarJS"},
		},
		"assets": {
			assetsVar: "StaticFS",
			want:      []string{"datapages.WithAssets(app.StaticFS, false)"},
			wantNot:   []string{"WithDatastarJS"},
		},
		"rocket": {
			assetsVar:  "StaticFS",
			datastarJS: skeleton.RocketJS,
			want: []string{
				"datapages.WithAssets(app.StaticFS, false)",
				`datapages.WithDatastarJS("` + skeleton.RocketJS + `")`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := skeleton.MainGo(skeleton.MainGoConfig{
				AppImport: "example.com/m/app", AppPkgName: "app",
				GenImport: "example.com/m/app/datapagesgen", GenPkgName: "datapagesgen",
				AssetsVar: tc.assetsVar, DatastarJS: tc.datastarJS,
			})
			require.NoError(t, err)
			for _, w := range tc.want {
				require.Contains(t, string(src), w)
			}
			for _, w := range tc.wantNot {
				require.NotContains(t, string(src), w)
			}
		})
	}
}

// TestRocketJSMatchesDefaultDatastar tests that the Rocket bundle comes from
// the Datastar release Datapages loads by default.
func TestRocketJSMatchesDefaultDatastar(t *testing.T) {
	t.Parallel()
	dir, ok := strings.CutSuffix(httpserve.DefaultDatastarJSSrc, "/datastar.js")
	require.True(t, ok, "not a datastar.js URL: %s", httpserve.DefaultDatastarJSSrc)
	require.Equal(t, dir+"/datastar-rocket.js", skeleton.RocketJS)
}

// TestWebComponentFiles tests the files each setup writes besides app.go and
// app.templ, and that the Rocket component imports the bundle main.go loads.
func TestWebComponentFiles(t *testing.T) {
	t.Parallel()

	for setup, want := range map[skeleton.WebComponents][]string{
		skeleton.WebComponentsNone: nil,
		skeleton.WebComponentsRocket: {
			"app/static/components/click-counter.js", "app/static/components/index.js",
		},
		skeleton.WebComponentsJavaScript: {
			"app/static/components/click-counter.js", "app/static/components/index.js",
		},
		skeleton.WebComponentsTypeScript: {
			"web/build.mjs", "web/package.json", "web/src/click-counter.ts",
			"web/src/index.ts", "web/tsconfig.json",
		},
	} {
		t.Run(string(setup), func(t *testing.T) {
			t.Parallel()
			files, err := skeleton.WebComponentFiles(setup)
			require.NoError(t, err)
			var paths []string
			for _, f := range files {
				paths = append(paths, f.Path)
				if setup == skeleton.WebComponentsRocket &&
					strings.HasSuffix(f.Path, "click-counter.js") {
					require.Contains(t, string(f.Content), `from "`+skeleton.RocketJS+`"`)
				}
			}
			require.ElementsMatch(t, want, paths)
		})
	}
}

// TestAppTemplHead tests that the head loads the components only once the
// generated assets package it refers to them through is given, and that the
// welcome page shows the example component of a setup.
func TestAppTemplHead(t *testing.T) {
	t.Parallel()

	const assets = "example.com/m/app/datapagesgen/assets"
	for name, tc := range map[string]struct {
		setup         skeleton.WebComponents
		assetsImport  string
		want, wantNot []string
	}{
		"none": {
			setup: skeleton.WebComponentsNone, assetsImport: assets,
			wantNot: []string{"import", "templ head()", "click-counter"},
		},
		"before generation": {
			setup:   skeleton.WebComponentsJavaScript,
			want:    []string{"templ head()", "<click-counter"},
			wantNot: []string{"import", "assets.Path"},
		},
		"javascript": {
			setup: skeleton.WebComponentsJavaScript, assetsImport: assets,
			want: []string{
				`import "` + assets + `"`,
				`<script type="module" src={ assets.Path("components/index.js") }></script>`,
			},
		},
		"typescript": {
			setup: skeleton.WebComponentsTypeScript, assetsImport: assets,
			want: []string{
				`<script src={ assets.Path("components.js") } defer></script>`,
				"<code>web/src/</code>",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := skeleton.AppTempl(tc.setup, tc.assetsImport)
			require.NoError(t, err)
			for _, w := range tc.want {
				require.Contains(t, string(src), w)
			}
			for _, w := range tc.wantNot {
				require.NotContains(t, string(src), w)
			}
		})
	}
}
