package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/romshark/templier/engine"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/serverscan"
)

// TestGenWatcherCmd tests the shell line the gen watcher runs.
// Templier looks the first word up on PATH and hands the line to `sh -c`,
// hence a path carrying a space, a quote or a "$" has to survive both.
func TestGenWatcherCmd(t *testing.T) {
	dir := t.TempDir()
	for name, exeName := range map[string]string{
		"plain":  "datapages",
		"space":  "data pages",
		"quote":  "data'pages",
		"dollar": "data$pages",
	} {
		t.Run(name, func(t *testing.T) {
			// A script standing in for the CLI, which echoes what it was given.
			exe := filepath.Join(dir, exeName)
			require.NoError(t, os.WriteFile(
				exe, []byte("#!/bin/sh\necho \"ran $1\"\n"), 0o755,
			))

			// The config the watch command builds, not the helper alone:
			// nothing else ties the quoting to what runs.
			cmd := genWatcher(exe, serverscan.Result{}).Cmd
			first, _, _ := strings.Cut(cmd, " ")
			_, err := exec.LookPath(first)
			require.NoError(t, err, "templier looks up %q", first)

			out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
			require.NoError(t, err, "%s", out)
			require.Equal(t, "ran gen\n", string(out))
		})
	}
}

// TestNewWatchLogger tests the levels the watch engine logs at for each
// watch.log.level: erronly logs errors, verbose adds info and warnings,
// and debug adds debug.
func TestNewWatchLogger(t *testing.T) {
	all := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	for name, tc := range map[string]struct {
		level config.LogLevel
		want  []slog.Level
	}{
		"erronly": {level: config.LogLevelErrOnly, want: all[3:]},
		"verbose": {level: config.LogLevelVerbose, want: all[1:]},
		"debug":   {level: config.LogLevelDebug, want: all},
	} {
		t.Run(name, func(t *testing.T) {
			l := newWatchLogger(tc.level, io.Discard)
			var got []slog.Level
			for _, lvl := range all {
				if l.Enabled(context.Background(), lvl) {
					got = append(got, lvl)
				}
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestParseAppHost tests that the app host needs an http or https scheme
// and a host, which "localhost:8080" lacks.
func TestParseAppHost(t *testing.T) {
	for name, tc := range map[string]struct {
		in       string
		wantHost string
		wantErr  string
	}{
		"http":  {in: "http://localhost:8080", wantHost: "localhost:8080"},
		"https": {in: "https://example.test:8443", wantHost: "example.test:8443"},
		"no scheme": {
			in: "localhost:8080",
			wantErr: `watch.app-host "localhost:8080" must be an http or https URL ` +
				"with a host, such as http://localhost:8080",
		},
		"other scheme": {
			in: "ftp://localhost:21",
			wantErr: `watch.app-host "ftp://localhost:21" must be an http or https URL ` +
				"with a host, such as http://localhost:8080",
		},
		"no host": {
			in: "http:///app",
			wantErr: `watch.app-host "http:///app" must be an http or https URL ` +
				"with a host, such as http://localhost:8080",
		},
		"unparsable": {
			in:      "http://[::1",
			wantErr: `parsing watch.app-host: parse "http://[::1": missing ']' in host`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			u, err := parseAppHost(tc.in)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantHost, u.Host)
		})
	}
}

// TestSplitFlags tests splitting a flags string from the config into argv.
// Runs of whitespace collapse and an empty string yields no arguments,
// not 1 empty one, which the go tool would reject.
// TestRunInReportsTheCause tests a program that cannot be started.
// The combined output is empty then, and the exec error is the whole report.
func TestRunInReportsTheCause(t *testing.T) {
	err := runIn(t.TempDir(), "git init", "definitely-not-a-program-xyz")
	require.Error(t, err)
	require.ErrorContains(t, err, "git init")
	require.ErrorContains(t, err, "definitely-not-a-program-xyz")
}

func TestSplitFlags(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  []string
	}{
		"empty":    {input: "", want: nil},
		"single":   {input: "single", want: []string{"single"}},
		"multiple": {input: "-v -count=1", want: []string{"-v", "-count=1"}},
		"extra whitespace": {
			input: "  -a   -b  ",
			want:  []string{"-a", "-b"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, splitFlags(tc.input))
		})
	}
}

// TestBuildCompilerConfig tests translating the config's compiler section into
// the go build flags templier passes on. A nil section stays nil rather than
// becoming an empty config, and a field left unset contributes no flag.
func TestBuildCompilerConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		input *config.WatchCompiler
		want  *engine.CompilerConfig
	}{
		"nil": {input: nil, want: nil},
		"all fields": {
			input: &config.WatchCompiler{
				Gcflags:  "-N -l",
				Ldflags:  "-s -w",
				Asmflags: "-trimpath",
				Tags:     []string{"integration", "e2e"},
				Race:     true,
				Trimpath: true,
				Msan:     true,
				P:        4,
				Env:      map[string]string{"CGO_ENABLED": "1"},
			},
			want: &engine.CompilerConfig{
				Flags: []string{
					"-gcflags", "-N -l",
					"-ldflags", "-s -w",
					"-asmflags", "-trimpath",
					"-tags", "integration,e2e",
					"-race",
					"-trimpath",
					"-msan",
					"-p", "4",
				},
				Env: []string{"CGO_ENABLED=1"},
			},
		},
		"partial": {
			input: &config.WatchCompiler{Race: true},
			want: &engine.CompilerConfig{
				Flags: []string{"-race"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := buildCompilerConfig(tc.input)
			if tc.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.ElementsMatch(t, tc.want.Flags, got.Flags)
			require.ElementsMatch(t, tc.want.Env, got.Env)
		})
	}
}

// TestMapLogLevel tests the mapping from the config's log level to templier's.
func TestMapLogLevel(t *testing.T) {
	for name, tc := range map[string]struct {
		input config.LogLevel
		want  engine.LogLevel
	}{
		"erronly": {input: config.LogLevelErrOnly, want: engine.LogLevelError},
		"verbose": {input: config.LogLevelVerbose, want: engine.LogLevelVerbose},
		"debug":   {input: config.LogLevelDebug, want: engine.LogLevelDebug},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, mapLogLevel(tc.input))
		})
	}
}

// TestMapLogClear tests the mapping from the config's log-clearing setting to templier's.
func TestMapLogClear(t *testing.T) {
	for name, tc := range map[string]struct {
		input config.LogClear
		want  engine.LogClearOn
	}{
		"disabled":    {input: config.LogClearDisabled, want: engine.LogClearNever},
		"restart":     {input: config.LogClearOnRestart, want: engine.LogClearOnRestart},
		"file change": {input: config.LogClearOnFileChange, want: engine.LogClearOnFileChange},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, mapLogClear(tc.input))
		})
	}
}

// TestMapWatcherRequires tests the mapping from what a custom watcher requires
// to the action templier takes.
func TestMapWatcherRequires(t *testing.T) {
	for name, tc := range map[string]struct {
		input config.WatcherRequires
		want  engine.ActionType
	}{
		"none":    {input: config.WatcherRequiresNone, want: engine.ActionNone},
		"reload":  {input: config.WatcherRequiresReload, want: engine.ActionReload},
		"restart": {input: config.WatcherRequiresRestart, want: engine.ActionRestart},
		"rebuild": {input: config.WatcherRequiresRebuild, want: engine.ActionRebuild},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, mapWatcherRequires(tc.input))
		})
	}
}

// TestMapCustomWatchers tests translating the configured custom watchers.
// An empty list becomes nil rather than an empty slice, which is what templier
// reads as no watchers.
func TestMapCustomWatchers(t *testing.T) {
	for name, tc := range map[string]struct {
		input []config.WatchCustomWatcher
		want  []engine.CustomWatcherConfig
	}{
		"nil":   {input: nil, want: nil},
		"empty": {input: []config.WatchCustomWatcher{}, want: nil},
		"single": {
			input: []config.WatchCustomWatcher{{
				Name:        "templ",
				Cmd:         "templ generate",
				Include:     []string{"**/*.templ"},
				Exclude:     []string{"vendor/**"},
				Debounce:    100 * time.Millisecond,
				FailOnError: true,
				Requires:    config.WatcherRequiresRebuild,
			}},
			want: []engine.CustomWatcherConfig{{
				Name:      "templ",
				Cmd:       "templ generate",
				Include:   []string{"**/*.templ"},
				Exclude:   []string{"vendor/**"},
				Debounce:  100 * time.Millisecond,
				FailOnErr: true,
				Requires:  engine.ActionRebuild,
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, mapCustomWatchers(tc.input))
		})
	}
}

// TestCheckCmdPackage tests the check on the entry point directory before gen
// writes a main.go there: whether it exists and whether it is a main package.
func TestCheckCmdPackage(t *testing.T) {
	for name, tc := range map[string]struct {
		setup     func(t *testing.T) string
		wantExist bool
		wantErr   bool
	}{
		"dir not exist": {
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			wantExist: false,
		},
		"main package": {
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "cmd")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "main.go"),
					[]byte("package main\n"),
					0o644,
				))
				return dir
			},
			wantExist: true,
		},
		"non-main package": {
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "cmd")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "lib.go"),
					[]byte("package lib\n"),
					0o644,
				))
				return dir
			},
			wantExist: true,
			wantErr:   true,
		},
		"no go files": {
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "cmd")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "README.md"),
					[]byte("# hello\n"),
					0o644,
				))
				return dir
			},
			wantExist: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := tc.setup(t)
			exists, err := checkCmdPackage(dir)
			require.Equal(t, tc.wantExist, exists)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUpgradeGoMod tests raising the datapages requirement in a user's go.mod to
// the version of the CLI doing the generating. A generator and a runtime that
// disagree produce code that does not compile.
func TestUpgradeGoMod(t *testing.T) {
	const goModTemplate = `module example.com/myapp

go 1.27.3

require (
	github.com/romshark/datapages %s
)
`

	writeGoMod := func(t *testing.T, dir, version string) {
		t.Helper()
		err := os.WriteFile(
			filepath.Join(dir, "go.mod"),
			fmt.Appendf(nil, goModTemplate, version),
			0o644,
		)
		require.NoError(t, err)
	}

	readGoMod := func(t *testing.T, dir string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		require.NoError(t, err)
		return string(data)
	}

	for name, tc := range map[string]struct {
		goModVersion string // version in go.mod
		runVersion   string // running datapages version
		wantVersion  string // expected version in go.mod after call
		wantErr      string // expected error substring, empty if no error
	}{
		"upgrades older version": {
			goModVersion: "v0.7.1",
			runVersion:   "0.8.0",
			wantVersion:  "v0.8.0",
		},
		"no-op when already current": {
			goModVersion: "v0.8.0",
			runVersion:   "0.8.0",
			wantVersion:  "v0.8.0",
		},
		"error when go.mod is newer": {
			goModVersion: "v0.9.0",
			runVersion:   "0.8.0",
			wantVersion:  "v0.9.0",
			wantErr:      "go install " + datapagesCommandPath + "@v0.9.0",
		},
		"no-op for dev build": {
			goModVersion: "v0.7.1",
			runVersion:   "",
			wantVersion:  "v0.7.1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeGoMod(t, dir, tc.goModVersion)

			err := upgradeGoMod(dir, tc.runVersion)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			content := readGoMod(t, dir)
			require.Contains(t, content, datapagesModulePath+" "+tc.wantVersion)
		})
	}
}

// TestRemoteURLToModulePath tests the module path init suggests from the git remote,
// for both URL forms git uses. An empty remote yields an empty path rather than a guess.
func TestRemoteURLToModulePath(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  string
	}{
		"https": {
			input: "https://github.com/user/repo",
			want:  "github.com/user/repo",
		},
		"https with .git": {
			input: "https://github.com/user/repo.git",
			want:  "github.com/user/repo",
		},
		"ssh": {
			input: "git@github.com:user/repo.git",
			want:  "github.com/user/repo",
		},
		"ssh without .git": {
			input: "git@github.com:user/repo",
			want:  "github.com/user/repo",
		},
		"trailing slash": {
			input: "https://github.com/user/repo/",
			want:  "github.com/user/repo",
		},
		"empty": {
			input: "",
			want:  "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, remoteURLToModulePath(tc.input))
		})
	}
}

// TestSelectApp tests picking the application to act on. 1 application needs
// no --app flag, several do, and the error lists their directories.
// The flag takes the directory of the app package, not its last element.
func TestSelectApp(t *testing.T) {
	one := serverscan.Result{Apps: []serverscan.App{
		{Dir: "app"},
	}}
	two := serverscan.Result{Apps: []serverscan.App{
		{Dir: filepath.Join("app", "admin")},
		{Dir: filepath.Join("app", "frontend")},
	}}
	shared := serverscan.Result{Apps: []serverscan.App{
		{Dir: filepath.Join("admin", "app")},
		{Dir: filepath.Join("web", "app")},
	}}
	for name, tt := range map[string]struct {
		result  serverscan.Result
		flag    string
		wantDir string
		wantErr string
	}{
		"1 app needs no flag": {
			result: one, wantDir: "app",
		},
		"1 app passed anyway": {
			result: one, flag: "app", wantDir: "app",
		},
		"2 apps need the flag": {
			result: two,
			wantErr: "this module builds 2 applications (app/admin, app/frontend), " +
				"pass the directory of the one to run with --app",
		},
		"2 apps by directory": {
			result: two, flag: "app/admin",
			wantDir: filepath.Join("app", "admin"),
		},
		"last element": {
			result: two, flag: "frontend",
			wantErr: `no app package in "frontend", ` +
				"the module builds: app/admin, app/frontend",
		},
		"shared last element": {
			result: shared, flag: "app",
			wantErr: `no app package in "app", ` +
				"the module builds: admin/app, web/app",
		},
		"shared last element by directory": {
			result: shared, flag: "web/app",
			wantDir: filepath.Join("web", "app"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, err := selectApp(tt.result, tt.flag)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantDir, a.Dir)
		})
	}
}

// TestWatchInclude tests that watch re-runs gen when an app package or a
// package calling NewServer changes.
func TestWatchInclude(t *testing.T) {
	app := func(dir string, callDirs ...string) serverscan.App {
		a := serverscan.App{Dir: dir}
		for _, d := range callDirs {
			a.Calls = append(a.Calls, serverscan.Call{Dir: d})
		}
		return a
	}
	cmdDir := filepath.Join("cmd", "server")
	for name, tt := range map[string]struct {
		apps []serverscan.App
		want []string
	}{
		"no call": {
			apps: []serverscan.App{app("app")},
			want: []string{"app/**/*.go", "datapages.yaml", "datapages.yml"},
		},
		"call in a command": {
			apps: []serverscan.App{app("app", cmdDir)},
			want: []string{
				"app/**/*.go", "cmd/server/*.go", "datapages.yaml", "datapages.yml",
			},
		},
		"call in the module root": {
			apps: []serverscan.App{app("app", ".")},
			want: []string{"app/**/*.go", "*.go", "datapages.yaml", "datapages.yml"},
		},
		"2 apps built in 1 command": {
			apps: []serverscan.App{
				app(filepath.Join("app", "admin"), cmdDir),
				app(filepath.Join("app", "frontend"), cmdDir),
			},
			want: []string{
				"app/admin/**/*.go", "cmd/server/*.go", "app/frontend/**/*.go",
				"datapages.yaml", "datapages.yml",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, watchInclude(serverscan.Result{Apps: tt.apps}))
		})
	}
}

// TestModuleVersionExistsEndsWithTheContext tests that a
// stalled module query returns when its context expires.
func TestModuleVersionExistsEndsWithTheContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
	))
	t.Cleanup(srv.Close)

	t.Setenv("GOFLAGS", "")
	t.Setenv("GOPROXY", srv.URL)
	t.Setenv("GOSUMDB", "off")
	// An empty cache leaves the proxy as the only source of an answer.
	t.Setenv("GOMODCACHE", t.TempDir())

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/pending\n\ngo 1.24\n"), 0o644))

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err := moduleVersionExists(ctx, dir, "v1.2.3")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestAcquireWatchLock tests the record contents, heartbeat, and release.
func TestAcquireWatchLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envWatchLockDir, dir)

	moduleDir := filepath.Join(t.TempDir(), "module")
	release, err := acquireWatchLock(moduleDir, "localhost:7331")
	require.NoError(t, err)

	path := filepath.Join(dir, watchLockName(moduleDir))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t,
		fmt.Sprintf("%d\t%s\tlocalhost:7331\n", os.Getpid(), moduleDir),
		string(content))

	stale := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(path, stale, stale))
	require.Eventually(t, func() bool {
		info, err := os.Stat(path)
		return err == nil && time.Since(info.ModTime()) < watchLockTTL
	}, 3*watchLockHeartbeat, 50*time.Millisecond,
		"the heartbeat must refresh the lock file")

	release()
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestWatchLockNamePerModule tests that equivalent module paths share a lock
// name and distinct module paths do not.
func TestWatchLockNamePerModule(t *testing.T) {
	dir := t.TempDir()
	require.Equal(t, watchLockName(dir), watchLockName(dir+string(filepath.Separator)))
	require.Equal(t, watchLockName(dir), watchLockName(filepath.Join(dir, "sub", "..")))
	require.NotEqual(t, watchLockName(dir), watchLockName(filepath.Join(dir, "sub")))
}

// WatchLockHeld reports whether a watch lock is held to the external test package.
func WatchLockHeld() bool { return watchLockHeld() }

// TestWatchLockHeld tests that fresh lock files are held and stale files are ignored.
func TestWatchLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envWatchLockDir, dir)
	require.False(t, watchLockHeld(), "empty directory")

	path := filepath.Join(dir, "0123456789abcdef.lock")
	require.NoError(t, os.WriteFile(path, []byte("1\t/tmp/mod\tlocalhost:7331\n"), 0o666))
	require.True(t, watchLockHeld(), "fresh lock file")

	stale := time.Now().Add(-2 * watchLockTTL)
	require.NoError(t, os.Chtimes(path, stale, stale))
	require.False(t, watchLockHeld(), "expired lock file")

	require.NoError(t, os.Remove(path))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), nil, 0o666))
	require.False(t, watchLockHeld(), "file that is no lock")
}

// TestHasVisibleEntries tests which directories init suggests creating
// a project subdirectory in.
func TestHasVisibleEntries(t *testing.T) {
	for name, tc := range map[string]struct {
		entries []string // a trailing slash makes a directory
		want    bool
	}{
		"empty":         {nil, false},
		"dotfiles only": {[]string{".git/", ".DS_Store"}, false},
		"file":          {[]string{".git/", "notes.txt"}, true},
		"directory":     {[]string{"Documents/"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, e := range tc.entries {
				p := filepath.Join(dir, e)
				if strings.HasSuffix(e, "/") {
					require.NoError(t, os.Mkdir(p, 0o755))
					continue
				}
				require.NoError(t, os.WriteFile(p, nil, 0o644))
			}
			require.Equal(t, tc.want, hasVisibleEntries(dir))
		})
	}
}
