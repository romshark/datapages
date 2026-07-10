package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/romshark/templier/engine"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/cmd/config"
)

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

func TestUpgradeGoMod(t *testing.T) {
	const goModTemplate = `module example.com/myapp

go 1.26.5

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
			wantErr:      "go install github.com/romshark/datapages@v0.9.0",
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
