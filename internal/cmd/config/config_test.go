package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUnmarshalLogLevel tests the config values the flag accepts. An empty value
// is the default rather than an error, since an absent key unmarshals as one.
func TestUnmarshalLogLevel(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		want    LogLevel
		wantErr bool
	}{
		"empty":   {input: "", want: LogLevelErrOnly},
		"erronly": {input: "erronly", want: LogLevelErrOnly},
		"verbose": {input: "verbose", want: LogLevelVerbose},
		"debug":   {input: "debug", want: LogLevelDebug},
		"invalid": {input: "trace", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var l LogLevel
			err := l.UnmarshalText([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, l)
			}
		})
	}
}

// TestUnmarshalLogClear tests the same for the log-clearing setting.
func TestUnmarshalLogClear(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		want    LogClear
		wantErr bool
	}{
		"empty":       {input: "", want: LogClearDisabled},
		"restart":     {input: "restart", want: LogClearOnRestart},
		"file-change": {input: "file-change", want: LogClearOnFileChange},
		"invalid":     {input: "always", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var l LogClear
			err := l.UnmarshalText([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, l)
			}
		})
	}
}

// TestUnmarshalWatcherRequires tests the same for what a custom watcher asks the
// watch command to do.
func TestUnmarshalWatcherRequires(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		want    WatcherRequires
		wantErr bool
	}{
		"empty":   {input: "", want: WatcherRequiresNone},
		"reload":  {input: "reload", want: WatcherRequiresReload},
		"restart": {input: "restart", want: WatcherRequiresRestart},
		"rebuild": {input: "rebuild", want: WatcherRequiresRebuild},
		"invalid": {input: "reboot", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var r WatcherRequires
			err := r.UnmarshalText([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, r)
			}
		})
	}
}

// TestLoad tests finding the config file next to the module: either extension is
// accepted, both at once is refused rather than one silently winning,
// and an unknown key is an error rather than a setting that quietly does nothing.
func TestLoad(t *testing.T) {
	for name, tc := range map[string]struct {
		setup     func(t *testing.T) string
		wantFound bool
		wantErr   string
	}{
		"neither": {
			setup:     func(t *testing.T) string { return t.TempDir() },
			wantFound: false,
		},
		"yaml only": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"), []byte("cmd: cmd/server\n"), 0o644,
				))
				return dir
			},
			wantFound: true,
		},
		"yml only": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yml"), []byte("cmd: cmd/server\n"), 0o644,
				))
				return dir
			},
			wantFound: true,
		},
		"both ambiguous": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"), []byte("cmd: cmd/server\n"), 0o644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yml"), []byte("cmd: cmd/server\n"), 0o644,
				))
				return dir
			},
			wantErr: "ambiguous",
		},
		"unknown field": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"),
					[]byte("app: app\n"), 0o644,
				))
				return dir
			},
			wantErr: "field app not found",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := tc.setup(t)
			_, found, err := Load(dir)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantFound, found)
		})
	}
}
