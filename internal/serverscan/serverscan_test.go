package serverscan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/serverscan"
)

const modulePath = "example.com/mod"

// write lays out a module from a path -> content map and returns its root.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module " + modulePath + "\n\ngo 1.27.1\n"
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return root
}

// mainGo renders a command calling NewServer for the app package at appDir.
// targs is the type argument list, args is appended to the call arguments.
func mainGo(appDir, targs, args string) string {
	return `package main

import (
	"github.com/romshark/datapages"
	"example.com/mod/` + appDir + `"
	gen "example.com/mod/` + appDir + `/datapagesgen"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

var _ = gen.Server{}

func main() {
	s, err := datapages.NewServer[` + targs + `](nil, inmem.New(8)` + args + `)
	_, _ = s, err
}
`
}

// genPkg renders a generated destination package.
func genPkg(header bool) string {
	h := ""
	if header {
		h = serverscan.GeneratedHeader + "\n\n"
	}
	return h + "package datapagesgen\n\ntype Server struct{}\n"
}

// TestScan tests finding the app package and the generated package from the
// datapages.NewServer calls in a module, and the fallback for a module that has
// no such call yet, which is what the first run sees.
func TestScan(t *testing.T) {
	for name, tt := range map[string]struct {
		files      map[string]string
		appDirs    []string
		hasSession bool
		fallback   bool
	}{
		"no call falls back": {
			files:    map[string]string{"app/app.go": "package app\n"},
			appDirs:  []string{"app"},
			fallback: true,
		},
		"blank import falls back": {
			files: map[string]string{
				"app/app.go": "package app\n",
				"cmd/server/main.go": `package main

import _ "github.com/romshark/datapages"

func main() {}
`,
			},
			appDirs:  []string{"app"},
			fallback: true,
		},
		"without sessions": {
			files: map[string]string{
				"app/app.go":                  "package app\n\ntype App struct{}\n",
				"app/datapagesgen/app_gen.go": genPkg(true),
				"cmd/server/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
			},
			appDirs: []string{"app"},
		},
		"with sessions": {
			files: map[string]string{
				"app/app.go": "package app\n\ntype App struct{}\n" +
					"type SessionData struct{}\n",
				"app/datapagesgen/app_gen.go": genPkg(true),
				"cmd/server/main.go": mainGo("app",
					"app.App, app.SessionData, datapages.DisablePrometheus, gen.Server",
					", datapages.WithSessionManager[app.SessionData](nil)"),
			},
			appDirs:    []string{"app"},
			hasSession: true,
		},
		"destination not generated yet": {
			files: map[string]string{
				"app/app.go": "package app\n",
				"cmd/server/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
			},
			appDirs: []string{"app"},
		},
		"nested app package": {
			files: map[string]string{
				"app/frontend/app.go":                  "package frontend\n",
				"app/frontend/datapagesgen/app_gen.go": genPkg(true),
				"cmd/frontend/main.go": mainGo("app/frontend",
					"frontend.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
			},
			appDirs: []string{filepath.Join("app", "frontend")},
		},
		"nested module is skipped": {
			files: map[string]string{
				"app/app.go":      "package app\n",
				"sub/go.mod":      "module example.com/other\n\ngo 1.27.1\n",
				"sub/cmd/main.go": mainGo("app", "app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
			},
			appDirs:  []string{"app"},
			fallback: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := write(t, tt.files)
			r, err := serverscan.Scan(root, modulePath)
			require.NoError(t, err)
			require.Equal(t, tt.fallback, r.Fallback)
			require.Len(t, r.Apps, len(tt.appDirs))

			a := r.Apps[0]
			require.Equal(t, tt.appDirs[0], a.Dir)
			require.Equal(t, filepath.Base(tt.appDirs[0]), a.Name)
			require.Equal(t,
				filepath.Join(tt.appDirs[0], serverscan.GenSubdir), a.GenDir)
			require.Equal(t,
				modulePath+"/"+filepath.ToSlash(tt.appDirs[0]), a.Import)
			require.Equal(t, a.Import+"/"+serverscan.GenSubdir, a.GenImport)

			if tt.fallback {
				require.Empty(t, a.Calls)
				return
			}
			require.Len(t, a.Calls, 1)
			require.Equal(t, tt.hasSession, a.Calls[0].HasSession)
			require.True(t, a.Calls[0].Main)
		})
	}
}

// TestScanMultipleApps tests a module that builds more than one application.
func TestScanMultipleApps(t *testing.T) {
	root := write(t, map[string]string{
		"app/admin/app.go":                     "package admin\n",
		"app/admin/datapagesgen/app_gen.go":    genPkg(true),
		"app/frontend/app.go":                  "package frontend\n",
		"app/frontend/datapagesgen/app_gen.go": genPkg(true),
		"cmd/admin/main.go": mainGo("app/admin",
			"admin.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
		"cmd/frontend/main.go": mainGo("app/frontend",
			"frontend.App, frontend.SessionData, datapages.DisablePrometheus, gen.Server",
			", datapages.WithSessionManager[frontend.SessionData](nil)"),
	})
	r, err := serverscan.Scan(root, modulePath)
	require.NoError(t, err)
	require.False(t, r.Fallback)
	require.Equal(t, []string{"admin", "frontend"}, r.Names())

	admin, ok := r.Find("admin")
	require.True(t, ok)
	require.Equal(t, filepath.Join("app", "admin"), admin.Dir)
	require.False(t, admin.HasSession)
	cmd, ok := admin.Cmd()
	require.True(t, ok)
	require.Equal(t, filepath.Join("cmd", "admin"), cmd)

	// The directory addresses an app just as its name does.
	front, ok := r.Find("app/frontend")
	require.True(t, ok)
	require.True(t, front.HasSession)
	require.Equal(t, "frontend.SessionData", front.SessionData.Src)
	require.Equal(t,
		modulePath+"/app/frontend/datapagesgen", front.GenImport)

	_, ok = r.Find("nothing")
	require.False(t, ok)
}

// TestScanErr tests what the scan refuses: a destination package datapages did not write,
// and one outside the app package. Overwriting either would destroy hand-written code.
func TestScanErr(t *testing.T) {
	for name, tt := range map[string]struct {
		files map[string]string
		msg   string
	}{
		"destination is not generated": {
			files: map[string]string{
				"app/app.go":                 "package app\n",
				"app/datapagesgen/server.go": genPkg(false),
				"cmd/server/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
			},
			msg: "which datapages did not generate",
		},
		"destination outside the app package": {
			files: map[string]string{
				"app/app.go":              "package app\n",
				"datapagesgen/app_gen.go": genPkg(true),
				"cmd/server/main.go": `package main

import (
	"github.com/romshark/datapages"
	"example.com/mod/app"
	gen "example.com/mod/datapagesgen"
)

func main() {
	s, err := datapages.NewServer[app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server](nil, nil)
	_, _ = s, err
}
`,
			},
			msg: "generates into gen.Server, but app generates into " +
				filepath.Join("app", "datapagesgen"),
		},
		"too few type arguments": {
			files: map[string]string{
				"app/app.go": "package app\n",
				"cmd/server/main.go": mainGo("app",
					"app.App, datapages.DisableSessions", ""),
			},
			msg: "datapages.NewServer needs four type arguments, got 2",
		},
		"call without a metrics mode": {
			files: map[string]string{
				"app/app.go": "package app\n",
				"cmd/server/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, gen.Server", ""),
			},
			msg: "datapages.NewServer needs four type arguments, got 3",
		},
		"metrics mode is another type": {
			files: map[string]string{
				"app/app.go": "package app\n",
				"cmd/server/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, app.Metrics, gen.Server", ""),
			},
			msg: "names Metrics app.Metrics, which is neither " +
				"datapages.EnablePrometheus nor datapages.DisablePrometheus",
		},
		"one app with two metrics modes": {
			files: map[string]string{
				"app/app.go":                  "package app\n",
				"app/datapagesgen/app_gen.go": genPkg(true),
				"cmd/a/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
				"cmd/b/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, datapages.EnablePrometheus, gen.Server", ""),
			},
			msg: "names Metrics",
		},
		"one app with two session types": {
			files: map[string]string{
				"app/app.go":                  "package app\n",
				"app/datapagesgen/app_gen.go": genPkg(true),
				"cmd/a/main.go": mainGo("app",
					"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
				"cmd/b/main.go": mainGo("app",
					"app.App, app.SessionData, datapages.DisablePrometheus, gen.Server", ""),
			},
			msg: "names SessionData",
		},
		"app type in another module": {
			files: map[string]string{
				"cmd/server/main.go": `package main

import (
	"github.com/romshark/datapages"
	"example.com/other/app"
	gen "example.com/other/app/datapagesgen"
)

func main() {
	s, err := datapages.NewServer[app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server](nil, nil)
	_, _ = s, err
}
`,
			},
			msg: "lives in another module",
		},
		"app type in a generated package": {
			files: map[string]string{
				"cmd/server/main.go": `package main

import (
	"github.com/romshark/datapages"
	gen "example.com/mod/app/datapagesgen"
)

func main() {
	s, err := datapages.NewServer[gen.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server](nil, nil)
	_, _ = s, err
}
`,
			},
			msg: "lives in a generated package",
		},
		"dot import": {
			files: map[string]string{
				"app/app.go":                  "package app\n",
				"app/datapagesgen/app_gen.go": genPkg(true),
				"cmd/server/main.go": `package main

import (
	. "github.com/romshark/datapages"
	"example.com/mod/app"
	gen "example.com/mod/app/datapagesgen"
)

func main() {
	s, err := NewServer[app.App, DisableSessions, DisablePrometheus, gen.Server](nil, nil)
	_, _ = s, err
}
`,
			},
			msg: "the datapages package must not be dot-imported",
		},
		"dot import without a call": {
			files: map[string]string{
				"app/app.go": "package app\n",
				"cmd/server/main.go": `package main

import . "github.com/romshark/datapages"

var _ Component

func main() {}
`,
			},
			msg: "the datapages package must not be dot-imported",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := write(t, tt.files)
			_, err := serverscan.Scan(root, modulePath)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.msg)
		})
	}
}

// TestScanStubDestination tests a destination a failed run left as stubs.
// A stub carries the generated header, which makes the next run generate over
// it instead of reading it as a package datapages did not write.
func TestScanStubDestination(t *testing.T) {
	root := write(t, map[string]string{
		"app/app.go": "package app\n",
		"app/datapagesgen/app_gen.go": serverscan.GeneratedHeader +
			"\n\npackage datapagesgen\n",
		"cmd/server/main.go": mainGo("app",
			"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
	})
	r, err := serverscan.Scan(root, modulePath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join("app", "datapagesgen"), r.Apps[0].GenDir)
}

// TestScanAnonymousSessionData tests an app whose session carries no data of its own,
// which names struct{} as the SessionData type argument.
func TestScanAnonymousSessionData(t *testing.T) {
	root := write(t, map[string]string{
		"app/app.go":                  "package app\n",
		"app/datapagesgen/app_gen.go": genPkg(true),
		"cmd/server/main.go": mainGo("app",
			"app.App, struct{}, datapages.DisablePrometheus, gen.Server",
			", datapages.WithSessionManager[struct{}](nil)"),
	})
	r, err := serverscan.Scan(root, modulePath)
	require.NoError(t, err)
	require.True(t, r.Apps[0].HasSession)
	require.Equal(t, "struct{}", r.Apps[0].SessionData.Src)
}

// TestCheckSessionData tests the session type argument of a NewServer call
// against the app package: whether the app declares sessions has to match what
// the call asks for.
func TestCheckSessionData(t *testing.T) {
	for name, tt := range map[string]struct {
		call       serverscan.Call
		hasSession bool
		msg        string
	}{
		"ok without sessions": {
			call: serverscan.Call{},
		},
		"ok with sessions": {
			call: serverscan.Call{
				HasSession:  true,
				SessionData: serverscan.TypeArg{Src: "app.SessionData"},
			},
			hasSession: true,
		},
		"app declares a session type": {
			call:       serverscan.Call{},
			hasSession: true,
			msg: "datapages.NewServer with SessionData datapages.DisableSessions, " +
				"but app declares a session type",
		},
		"app declares no session type": {
			call: serverscan.Call{
				HasSession:  true,
				SessionData: serverscan.TypeArg{Src: "app.SessionData"},
			},
			msg: "but app declares no session type",
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := serverscan.App{Dir: "app", Calls: []serverscan.Call{tt.call}}
			err := serverscan.CheckSessionData(a, tt.hasSession)
			if tt.msg == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.msg)
		})
	}
}

// TestScanPrometheus tests the Metrics type argument,
// which decides the metrics instrumentation, and the option, which does not.
func TestScanPrometheus(t *testing.T) {
	for name, tt := range map[string]struct {
		main string
		want bool
	}{
		"no metrics": {
			main: mainGo("app",
				"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
		},
		"metrics": {
			main: mainGo("app",
				"app.App, datapages.DisableSessions, datapages.EnablePrometheus, gen.Server",
				", datapages.WithPrometheus(datapages.PrometheusConfig{})"),
			want: true,
		},
		"the option alone selects nothing": {
			main: mainGo("app",
				"app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server",
				", datapages.WithPrometheus(datapages.PrometheusConfig{})"),
		},
		"options built elsewhere": {
			main: `package main

import (
	"os"

	"github.com/romshark/datapages"
	"example.com/mod/app"
	gen "example.com/mod/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	var opts []datapages.ServerOption
	if os.Getenv("METRICS") != "" {
		opts = append(opts, datapages.WithPrometheus(datapages.PrometheusConfig{}))
	}
	s, err := datapages.NewServer[
		app.App, datapages.DisableSessions, datapages.EnablePrometheus, gen.Server,
	](
		new(app.App), inmem.New(8), opts...,
	)
	_, _ = s, err
}
`,
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := write(t, map[string]string{
				"app/app.go":                  "package app\n",
				"app/datapagesgen/app_gen.go": genPkg(true),
				"cmd/server/main.go":          tt.main,
			})
			r, err := serverscan.Scan(root, modulePath)
			require.NoError(t, err)
			require.Equal(t, tt.want, r.Apps[0].Prometheus)
		})
	}
}

// TestScanPrometheusPerApp tests two applications in one module,
// only one of which uses metrics.
func TestScanPrometheusPerApp(t *testing.T) {
	root := write(t, map[string]string{
		"app/admin/app.go":                     "package admin\n",
		"app/admin/datapagesgen/app_gen.go":    genPkg(true),
		"app/frontend/app.go":                  "package frontend\n",
		"app/frontend/datapagesgen/app_gen.go": genPkg(true),
		"cmd/admin/main.go": mainGo("app/admin",
			"admin.App, datapages.DisableSessions, datapages.EnablePrometheus, gen.Server",
			", datapages.WithPrometheus(datapages.PrometheusConfig{})"),
		"cmd/frontend/main.go": mainGo("app/frontend",
			"frontend.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server", ""),
	})
	r, err := serverscan.Scan(root, modulePath)
	require.NoError(t, err)

	admin, ok := r.Find("admin")
	require.True(t, ok)
	require.True(t, admin.Prometheus)
	require.Equal(t, "datapages.EnablePrometheus", admin.Metrics.Src)

	front, ok := r.Find("frontend")
	require.True(t, ok)
	require.False(t, front.Prometheus)
	require.Equal(t, "datapages.DisablePrometheus", front.Metrics.Src)
}

// TestScanAliasedTypeArgs tests two calls naming one app package under
// different import names.
//
// A file importing two packages of the same name has to alias one of them.
// Comparing type arguments as written would then read one session type as two
// and refuse the module.
func TestScanAliasedTypeArgs(t *testing.T) {
	root := write(t, map[string]string{
		"app/app.go":                  "package app\n",
		"app/datapagesgen/app_gen.go": genPkg(true),
		"cmd/server/main.go": mainGo("app",
			"app.App, app.SessionData, datapages.DisablePrometheus, gen.Server",
			", datapages.WithSessionManager[app.SessionData](nil)"),
		"serve/serve.go": `package serve

import (
	dp "github.com/romshark/datapages"
	appalias "example.com/mod/app"
	genalias "example.com/mod/app/datapagesgen"
)

func New() {
	s, err := dp.NewServer[
		appalias.App, appalias.SessionData, dp.DisablePrometheus, genalias.Server,
	](nil, nil, dp.WithSessionManager[appalias.SessionData](nil))
	_, _ = s, err
}
`,
	})
	r, err := serverscan.Scan(root, modulePath)
	require.NoError(t, err)
	require.Len(t, r.Apps, 1)
	require.Len(t, r.Apps[0].Calls, 2)
	require.True(t, r.Apps[0].HasSession)
}

// TestScanLocalTypeArgsDisagree tests two calls naming session types that
// resolve to no import. There is no import path to compare them by,
// hence they are compared as written.
func TestScanLocalTypeArgsDisagree(t *testing.T) {
	root := write(t, map[string]string{
		"app/app.go":                  "package app\n",
		"app/datapagesgen/app_gen.go": genPkg(true),
		"cmd/a/main.go": mainGo("app",
			"app.App, struct{}, datapages.DisablePrometheus, gen.Server",
			", datapages.WithSessionManager[struct{}](nil)"),
		"cmd/b/main.go": mainGo("app",
			"app.App, struct{ N int }, datapages.DisablePrometheus, gen.Server",
			", datapages.WithSessionManager[struct{ N int }](nil)"),
	})
	_, err := serverscan.Scan(root, modulePath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "names SessionData")
}
