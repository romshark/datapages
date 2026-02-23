package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/romshark/templier/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		input *watchCompiler
		want  *engine.CompilerConfig
	}{
		"nil": {input: nil, want: nil},
		"all fields": {
			input: &watchCompiler{
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
			input: &watchCompiler{Race: true},
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
		input logLevel
		want  engine.LogLevel
	}{
		"erronly": {input: logLevelErrOnly, want: engine.LogLevelError},
		"verbose": {input: logLevelVerbose, want: engine.LogLevelVerbose},
		"debug":   {input: logLevelDebug, want: engine.LogLevelDebug},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, mapLogLevel(tc.input))
		})
	}
}

func TestMapLogClear(t *testing.T) {
	for name, tc := range map[string]struct {
		input logClear
		want  engine.LogClearOn
	}{
		"disabled":    {input: logClearDisabled, want: engine.LogClearNever},
		"restart":     {input: logClearOnRestart, want: engine.LogClearOnRestart},
		"file change": {input: logClearOnFileChange, want: engine.LogClearOnFileChange},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, mapLogClear(tc.input))
		})
	}
}

func TestMapWatcherRequires(t *testing.T) {
	for name, tc := range map[string]struct {
		input watcherRequires
		want  engine.ActionType
	}{
		"none":    {input: watcherRequiresNone, want: engine.ActionNone},
		"reload":  {input: watcherRequiresReload, want: engine.ActionReload},
		"restart": {input: watcherRequiresRestart, want: engine.ActionRestart},
		"rebuild": {input: watcherRequiresRebuild, want: engine.ActionRebuild},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, mapWatcherRequires(tc.input))
		})
	}
}

func TestMapCustomWatchers(t *testing.T) {
	for name, tc := range map[string]struct {
		input []watchCustomWatcher
		want  []engine.CustomWatcherConfig
	}{
		"nil":   {input: nil, want: nil},
		"empty": {input: []watchCustomWatcher{}, want: nil},
		"single": {
			input: []watchCustomWatcher{{
				Name:        "templ",
				Cmd:         "templ generate",
				Include:     []string{"**/*.templ"},
				Exclude:     []string{"vendor/**"},
				Debounce:    100 * time.Millisecond,
				FailOnError: true,
				Requires:    watcherRequiresRebuild,
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

func TestUnmarshalLogLevel(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		want    logLevel
		wantErr bool
	}{
		"empty":   {input: "", want: logLevelErrOnly},
		"erronly": {input: "erronly", want: logLevelErrOnly},
		"verbose": {input: "verbose", want: logLevelVerbose},
		"debug":   {input: "debug", want: logLevelDebug},
		"invalid": {input: "trace", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var l logLevel
			err := l.UnmarshalText([]byte(tc.input))
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, l)
			}
		})
	}
}

func TestUnmarshalLogClear(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		want    logClear
		wantErr bool
	}{
		"empty":       {input: "", want: logClearDisabled},
		"restart":     {input: "restart", want: logClearOnRestart},
		"file-change": {input: "file-change", want: logClearOnFileChange},
		"invalid":     {input: "always", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var l logClear
			err := l.UnmarshalText([]byte(tc.input))
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, l)
			}
		})
	}
}

func TestUnmarshalWatcherRequires(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		want    watcherRequires
		wantErr bool
	}{
		"empty":   {input: "", want: watcherRequiresNone},
		"reload":  {input: "reload", want: watcherRequiresReload},
		"restart": {input: "restart", want: watcherRequiresRestart},
		"rebuild": {input: "rebuild", want: watcherRequiresRebuild},
		"invalid": {input: "reboot", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var r watcherRequires
			err := r.UnmarshalText([]byte(tc.input))
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, r)
			}
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
