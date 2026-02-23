package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/romshark/datapages/internal/cmd"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	for name, tc := range map[string]struct {
		args       []string
		wantCode   int
		wantStderr string
	}{
		"no command": {
			args:       []string{"datapages"},
			wantCode:   1,
			wantStderr: "usage:",
		},
		"unknown command": {
			args:       []string{"datapages", "foobar"},
			wantCode:   1,
			wantStderr: "unknown command: foobar",
		},
		"-h": {
			args:       []string{"datapages", "-h"},
			wantCode:   0,
			wantStderr: "usage:",
		},
		"--help": {
			args:       []string{"datapages", "--help"},
			wantCode:   0,
			wantStderr: "usage:",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cmd.Run(
				tc.args, nil, &stdout, &stderr,
				"", "", "",
			)
			require.Equal(t, tc.wantCode, code)
			if tc.wantStderr != "" {
				require.Contains(t, stderr.String(), tc.wantStderr)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	for name, tc := range map[string]struct {
		args                  []string
		version, commit, date string
		want                  string
	}{
		"short": {
			args:    []string{"datapages", "version"},
			version: "1.2.3",
			commit:  "abc1234",
			date:    "2025-01-15",
			want:    "1.2.3\n",
		},
		"full": {
			args:    []string{"datapages", "version", "-full"},
			version: "1.2.3",
			commit:  "abc1234",
			date:    "2025-01-15",
			want: "datapages 1.2.3\n" +
				"  commit: abc1234\n" +
				"  built:  2025-01-15\n" +
				"  go:     " + runtime.Version() + "\n" +
				"  os:     " + runtime.GOOS + "/" + runtime.GOARCH + "\n" +
				"\ndependencies:\n",
		},
		"full unset": {
			args: []string{"datapages", "version", "-full"},
			want: "datapages \n" +
				"  commit: \n" +
				"  built:  \n" +
				"  go:     " + runtime.Version() + "\n" +
				"  os:     " + runtime.GOOS + "/" + runtime.GOARCH + "\n" +
				"\ndependencies:\n",
		},
		"empty version": {
			args: []string{"datapages", "version"},
			want: "\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cmd.Run(
				tc.args, nil, &stdout, &stderr,
				tc.version, tc.commit, tc.date,
			)
			require.Zero(t, code)
			require.Empty(t, stderr.String())
			out := stdout.String()
			require.Equal(t, tc.want, out[:len(tc.want)])
		})
	}
}

// setupProject creates a temporary Go module with a datapages app package.
// It copies the given app source file from testdata into a temporary directory,
// changes the working directory to the project root and returns
// the project directory path.
func copyTestdata(t *testing.T, dst, src string) {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	require.NoError(t, os.WriteFile(dst, data, 0o644))
}

func setupProject(t *testing.T, appGoFile string) string {
	t.Helper()

	dir := t.TempDir()

	copyTestdata(t, filepath.Join(dir, "go.mod"), filepath.Join("testdata", "project", "go.mod"))
	copyTestdata(t, filepath.Join(dir, "datapages.yaml"), filepath.Join("testdata", "project", "datapages.yaml"))
	copyTestdata(t, filepath.Join(dir, "app", "app.go"), filepath.Join("testdata", "app", appGoFile))

	// findModuleDir uses os.Getwd, so we must chdir.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	out, err := exec.Command("go", "mod", "tidy").CombinedOutput()
	require.NoError(t, err, "go mod tidy: %s", out)

	return dir
}

func TestWatch(t *testing.T) {
	t.Run("no module", func(t *testing.T) {
		dir := t.TempDir()
		origDir, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(dir))
		t.Cleanup(func() { _ = os.Chdir(origDir) })

		var stdout, stderr bytes.Buffer
		code := cmd.Run(
			[]string{"datapages", "watch"}, nil, &stdout, &stderr,
			"", "", "",
		)
		require.Equal(t, 1, code)
		require.Contains(t, stderr.String(), "no go.mod found")
	})
}

func TestGenAndLint(t *testing.T) {
	for name, tc := range map[string]struct {
		command    string
		appGoFile string
		wantOK    bool
		wantGen   []string // files expected after gen
	}{
		"lint ok": {
			command:   "lint",
			appGoFile: "valid.go",
			wantOK:    true,
		},
		"lint error": {
			command:   "lint",
			appGoFile: "invalid.go",
			wantOK:    false,
		},
		"gen ok": {
			command:   "gen",
			appGoFile: "valid.go",
			wantOK:    true,
			wantGen: []string{
				"datapagesgen/app_gen.go",
				"datapagesgen/action/action_gen.go",
				"datapagesgen/href/href_gen.go",
				"cmd/server/main.go",
			},
		},
		"gen error": {
			command:   "gen",
			appGoFile: "invalid.go",
			wantOK:    false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := setupProject(t, tc.appGoFile)

			var stdout, stderr bytes.Buffer
			code := cmd.Run(
				[]string{"datapages", tc.command}, nil, &stdout, &stderr,
				"", "", "",
			)

			if tc.wantOK {
				require.Zero(t, code, "stderr: %s", stderr.String())
				for _, f := range tc.wantGen {
					_, err := os.Stat(filepath.Join(dir, f))
					require.NoError(t, err, "expected generated file %s", f)
				}
			} else {
				require.Equal(t, 1, code)
				require.NotEmpty(t, stderr.String())
			}
		})
	}
}
