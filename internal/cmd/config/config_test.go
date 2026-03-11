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

func TestValidateAssetsURLPrefix(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		wantErr error
	}{
		"valid /static/":          {input: "/static/"},
		"valid /assets/":          {input: "/assets/"},
		"valid /a/b/c/":           {input: "/a/b/c/"},
		"root slash":              {input: "/", wantErr: ErrAssetsURLPrefixRoot},
		"no leading slash":        {input: "static/", wantErr: ErrAssetsURLPrefixNoLeadingSlash},
		"no trailing slash":       {input: "/static", wantErr: ErrAssetsURLPrefixNoTrailingSlash},
		"double slash":            {input: "/static//css/", wantErr: ErrAssetsURLPrefixDoubleSlash},
		"query string":            {input: "/static/?v=1/", wantErr: ErrAssetsURLPrefixQueryString},
		"fragment":                {input: "/static/#top/", wantErr: ErrAssetsURLPrefixFragment},
		"dot segment":             {input: "/static/../secret/", wantErr: ErrAssetsURLPrefixDotSegment},
		"dot segment current dir": {input: "/static/./css/", wantErr: ErrAssetsURLPrefixDotSegment},
		"backslash":               {input: `/static\css/`, wantErr: ErrAssetsURLPrefixBackslash},
		"encoded dot":             {input: "/static/%2e%2e/", wantErr: ErrAssetsURLPrefixEncodedTraversal},
		"encoded slash":           {input: "/static/%2f/", wantErr: ErrAssetsURLPrefixEncodedTraversal},
		"encoded backslash":       {input: "/static/%5C/", wantErr: ErrAssetsURLPrefixEncodedTraversal},
		"valid percent encoding":  {input: "/my%20files/"},
		"space":                   {input: "/my static/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"control char":            {input: "/static/\x00/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"non-ascii":               {input: "/données/", wantErr: ErrAssetsURLPrefixInvalidChar},
		"angle bracket":           {input: "/static</", wantErr: ErrAssetsURLPrefixInvalidChar},
		"valid with hyphens":      {input: "/my-static/"},
		"valid with underscores":  {input: "/my_static/"},
		"valid with digits":       {input: "/static-v2/"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateAssetsURLPrefix(tc.input)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
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
					filepath.Join(dir, "datapages.yaml"), []byte("app: myapp\n"), 0o644,
				))
				return dir
			},
			wantFound: true,
		},
		"yml only": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yml"), []byte("app: myapp\n"), 0o644,
				))
				return dir
			},
			wantFound: true,
		},
		"both ambiguous": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"), []byte("app: myapp\n"), 0o644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yml"), []byte("app: myapp\n"), 0o644,
				))
				return dir
			},
			wantErr: "ambiguous",
		},
		"assets both fields": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"),
					[]byte("app: app\nassets:\n  url-prefix: /static/\n  dir: ./app/static/\n"),
					0o644,
				))
				return dir
			},
			wantFound: true,
		},
		"assets missing url-prefix": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"),
					[]byte("app: app\nassets:\n  dir: ./app/static/\n"),
					0o644,
				))
				return dir
			},
			wantErr: "url-prefix is required",
		},
		"assets missing dir": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"),
					[]byte("app: app\nassets:\n  url-prefix: /static/\n"),
					0o644,
				))
				return dir
			},
			wantErr: "dir is required",
		},
		"assets invalid url-prefix": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"),
					[]byte("app: app\nassets:\n  url-prefix: static/\n  dir: ./app/static/\n"),
					0o644,
				))
				return dir
			},
			wantErr: "must start with '/'",
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
