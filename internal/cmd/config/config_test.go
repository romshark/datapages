package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, r)
			}
		})
	}
}

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
