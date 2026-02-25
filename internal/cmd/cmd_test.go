package cmd_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/romshark/datapages/internal/cmd"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	for name, tc := range map[string]struct {
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		"no command": {
			args:       []string{"datapages"},
			wantCode:   0,
			wantStdout: "Available Commands:",
		},
		"unknown command": {
			args:       []string{"datapages", "foobar"},
			wantCode:   1,
			wantStderr: `unknown command "foobar"`,
		},
		"-h": {
			args:       []string{"datapages", "-h"},
			wantCode:   0,
			wantStdout: "Available Commands:",
		},
		"--help": {
			args:       []string{"datapages", "--help"},
			wantCode:   0,
			wantStdout: "Available Commands:",
		},
		"help command": {
			args:       []string{"datapages", "help"},
			wantCode:   0,
			wantStdout: "Available Commands:",
		},
		"subcommand help": {
			args:       []string{"datapages", "gen", "--help"},
			wantCode:   0,
			wantStdout: "Type-safe URL helpers",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cmd.Run(context.Background(), tc.args,
				nil, &stdout, &stderr,
				"0.0.0", "xxxxxxx", "2026-2-23",
			)
			require.Equal(t, tc.wantCode, code)
			if tc.wantStdout != "" {
				require.Contains(t, stdout.String(), tc.wantStdout)
			}
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
			date:    "2026-2-23",
			want:    "1.2.3\n",
		},
		"full": {
			args:    []string{"datapages", "version", "--full"},
			version: "1.2.3",
			commit:  "abc1234",
			date:    "2026-2-23",
			want: "datapages 1.2.3\n" +
				"  commit: abc1234\n" +
				"  built:  2026-2-23\n" +
				"  go:     " + runtime.Version() + "\n" +
				"  os:     " + runtime.GOOS + "/" + runtime.GOARCH + "\n" +
				"\ndependencies:\n",
		},
		"full unset": {
			args: []string{"datapages", "version", "--full"},
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
				context.Background(), tc.args,
				nil, &stdout, &stderr,
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

// hashDir returns a SHA-256 digest of the directory tree rooted at dir.
// It hashes file paths and contents so any added, removed, or modified
// file changes the result.
func hashDir(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s %d %t\n", rel, info.Size(), info.IsDir())
		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write(data)
		}
		return nil
	})
	require.NoError(t, err)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func setupProject(t *testing.T, appGoFile string) string {
	t.Helper()

	dir := t.TempDir()

	copyTestdata(t,
		filepath.Join(dir, "go.mod"),
		filepath.Join("testdata", "project", "go.mod.txt"))
	copyTestdata(t,
		filepath.Join(dir, "datapages.yaml"),
		filepath.Join("testdata", "project", "datapages.yaml"))
	copyTestdata(t,
		filepath.Join(dir, "app", "app.go"),
		filepath.Join("testdata", "app", appGoFile))

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
			context.Background(), []string{"datapages", "watch"},
			nil, &stdout, &stderr,
			"0.0.0", "xxxxxxx", "2026-2-23",
		)
		require.Equal(t, 1, code)
		require.Contains(t, stderr.String(), "no go.mod found")
	})

	t.Run("runs gen", func(t *testing.T) {
		setupProject(t, "invalid.go")

		var stdout, stderr bytes.Buffer
		code := cmd.Run(
			context.Background(), []string{"datapages", "watch"},
			nil, &stdout, &stderr,
			"0.0.0", "xxxxxxx", "2026-2-23",
		)
		require.Equal(t, 1, code)
		require.Contains(t, stderr.String(), "parsing app package")
	})

	t.Run("writes default config", func(t *testing.T) {
		dir := setupProject(t, "invalid.go")
		require.NoError(t, os.Remove(filepath.Join(dir, "datapages.yaml")))

		var stdout, stderr bytes.Buffer
		code := cmd.Run(
			context.Background(), []string{"datapages", "watch"},
			nil, &stdout, &stderr,
			"0.0.0", "xxxxxxx", "2026-2-23",
		)
		require.Equal(t, 1, code)
		_, err := os.Stat(filepath.Join(dir, "datapages.yaml"))
		require.NoError(t, err, "expected default datapages.yaml")
	})

	t.Run("generates code", func(t *testing.T) {
		dir := setupProject(t, "valid.go")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var stdout, stderr bytes.Buffer
		done := make(chan int, 1)
		go func() {
			done <- cmd.Run(
				ctx, []string{"datapages", "watch"},
				nil, &stdout, &stderr,
				"0.0.0", "xxxxxxx", "2026-2-23",
			)
		}()

		// Gen runs synchronously before the engine; wait for generated files.
		require.Eventually(t, func() bool {
			for _, f := range []string{
				"datapagesgen/app_gen.go",
				"datapagesgen/action/action_gen.go",
				"datapagesgen/href/href_gen.go",
				"cmd/server/main.go",
			} {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					return false
				}
			}
			return true
		}, 30*time.Second, 100*time.Millisecond)

		cancel()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("watch did not stop after cancel")
		}
	})
}

func TestLint(t *testing.T) {
	for name, tc := range map[string]struct {
		appGoFile string
		wantOK    bool
	}{
		"ok": {
			appGoFile: "valid.go",
			wantOK:    true,
		},
		"error": {
			appGoFile: "invalid.go",
			wantOK:    false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := setupProject(t, tc.appGoFile)
			before := hashDir(t, dir)

			var stdout, stderr bytes.Buffer
			code := cmd.Run(
				context.Background(), []string{"datapages", "lint"},
				nil, &stdout, &stderr,
				"0.0.0", "xxxxxxx", "2026-2-23",
			)

			if tc.wantOK {
				require.Zero(t, code, "stderr: %s", stderr.String())
			} else {
				require.Equal(t, 1, code)
				require.NotEmpty(t, stderr.String())
			}
			require.Equal(t, before, hashDir(t, dir),
				"lint must not modify the project directory")
		})
	}
}

func TestGenBuild(t *testing.T) {
	// Resolve the datapages module root before setupProject changes cwd.
	datapagesRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	dir := setupProject(t, "valid.go")

	var stdout, stderr bytes.Buffer
	code := cmd.Run(
		context.Background(), []string{"datapages", "gen"},
		nil, &stdout, &stderr,
		"0.0.0", "xxxxxxx", "2026-2-23",
	)
	require.Zero(t, code, "gen stderr: %s", stderr.String())

	// Add a replace directive so the generated code resolves the local module.
	goModPath := filepath.Join(dir, "go.mod")
	f, err := os.OpenFile(goModPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = fmt.Fprintf(f, "\nreplace github.com/romshark/datapages => %s\n",
		datapagesRoot)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	out, err := exec.Command("go", "mod", "tidy").CombinedOutput()
	require.NoError(t, err, "go mod tidy: %s", out)

	out, err = exec.Command("go", "build", "./...").CombinedOutput()
	require.NoError(t, err, "go build: %s", out)
}

func TestGen(t *testing.T) {
	for name, tc := range map[string]struct {
		command   string
		appGoFile string
		wantOK    bool
		wantGen   []string // files expected after gen
	}{
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
				context.Background(), []string{"datapages", tc.command},
				nil, &stdout, &stderr,
				"0.0.0", "xxxxxxx", "2026-2-23",
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

// chdirTemp changes the working directory to dir and restores it on cleanup.
func chdirTemp(t *testing.T, dir string) {
	t.Helper()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })
}

func TestInit(t *testing.T) {
	for name, tc := range map[string]struct {
		setup    func(t *testing.T) string // returns dir to chdir into
		args     []string
		stdin    string
		wantCode int
		wantErr  string
		check    func(t *testing.T, startDir string, stdout string)
	}{
		"non-interactive no git no module": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"datapages", "init", "-n", "--name", "myapp", "--module", "example.com/myapp"},
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				projectDir := filepath.Join(startDir, "myapp")
				require.DirExists(t, filepath.Join(projectDir, ".git"))
				require.FileExists(t, filepath.Join(projectDir, "go.mod"))
				data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
				require.NoError(t, err)
				require.Contains(t, string(data), "example.com/myapp")
				require.FileExists(t, filepath.Join(projectDir, "datapages.yaml"))
				require.FileExists(t, filepath.Join(projectDir, "app", "app.go"))
				require.FileExists(t, filepath.Join(projectDir, ".env"))
				require.FileExists(t, filepath.Join(projectDir, "compose.yaml"))
				require.FileExists(t, filepath.Join(projectDir, "Makefile"))
				require.Contains(t, stdout, "Project initialized successfully.")
			},
		},
		"non-interactive missing name": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"datapages", "init", "-n"},
			wantCode: 1,
			wantErr:  "--name is required",
		},
		"non-interactive missing module": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				out, err := exec.Command("git", "init", dir).CombinedOutput()
				require.NoError(t, err, "git init: %s", out)
				return dir
			},
			args:     []string{"datapages", "init", "-n"},
			wantCode: 1,
			wantErr:  "--module is required",
		},
		"non-interactive in git no module": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				out, err := exec.Command("git", "init", dir).CombinedOutput()
				require.NoError(t, err, "git init: %s", out)
				return dir
			},
			args:     []string{"datapages", "init", "-n", "--module", "example.com/test"},
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				require.FileExists(t, filepath.Join(startDir, "go.mod"))
				require.FileExists(t, filepath.Join(startDir, "datapages.yaml"))
				require.FileExists(t, filepath.Join(startDir, "app", "app.go"))
				data, err := os.ReadFile(filepath.Join(startDir, "go.mod"))
				require.NoError(t, err)
				require.Contains(t, string(data), "example.com/test")
			},
		},
		"non-interactive already initialized": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				out, err := exec.Command("git", "init", dir).CombinedOutput()
				require.NoError(t, err, "git init: %s", out)
				c := exec.Command("go", "mod", "init", "example.com/existing")
				c.Dir = dir
				out, err = c.CombinedOutput()
				require.NoError(t, err, "go mod init: %s", out)
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "datapages.yaml"),
					[]byte("app: app\n"),
					0o644,
				))
				appDir := filepath.Join(dir, "app")
				require.NoError(t, os.MkdirAll(appDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(appDir, "app.go"),
					[]byte("package app\n// existing\n"),
					0o644,
				))
				return dir
			},
			args:     []string{"datapages", "init", "-n"},
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				// app.go must not be overwritten.
				data, err := os.ReadFile(filepath.Join(startDir, "app", "app.go"))
				require.NoError(t, err)
				require.Contains(t, string(data), "// existing",
					"init must not overwrite existing app/app.go")
				// go.mod module path must remain unchanged.
				modData, err := os.ReadFile(filepath.Join(startDir, "go.mod"))
				require.NoError(t, err)
				require.Contains(t, string(modData), "example.com/existing")
				require.Contains(t, stdout, "Project already initialized.")
				// No new files should be generated.
				require.NoFileExists(t, filepath.Join(startDir, ".env"))
				require.NoFileExists(t, filepath.Join(startDir, "compose.yaml"))
				require.NoFileExists(t, filepath.Join(startDir, "Makefile"))
			},
		},
		"non-interactive in git subdirectory": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				out, err := exec.Command("git", "init", dir).CombinedOutput()
				require.NoError(t, err, "git init: %s", out)
				sub := filepath.Join(dir, "sub")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				return sub
			},
			args:     []string{"datapages", "init", "-n", "--module", "example.com/sub"},
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				// Files must be in startDir (the subdirectory), not the git root.
				require.FileExists(t, filepath.Join(startDir, "go.mod"))
				require.FileExists(t, filepath.Join(startDir, "datapages.yaml"))
				require.FileExists(t, filepath.Join(startDir, "app", "app.go"))
				// Parent (git root) must not have these files.
				gitRoot := filepath.Dir(startDir)
				require.NoFileExists(t, filepath.Join(gitRoot, "datapages.yaml"))
			},
		},
		"interactive create repo": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"datapages", "init"},
			stdin:    "y\nmyapp\ny\nmymod\nn\n",
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				projectDir := filepath.Join(startDir, "myapp")
				require.DirExists(t, filepath.Join(projectDir, ".git"))
				require.FileExists(t, filepath.Join(projectDir, "go.mod"))
				data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
				require.NoError(t, err)
				require.Contains(t, string(data), "mymod")
				require.FileExists(t, filepath.Join(projectDir, "datapages.yaml"))
				require.FileExists(t, filepath.Join(projectDir, "app", "app.go"))
				require.FileExists(t, filepath.Join(projectDir, ".env"))
				require.FileExists(t, filepath.Join(projectDir, "compose.yaml"))
				require.FileExists(t, filepath.Join(projectDir, "Makefile"))
			},
		},
		"interactive decline git": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"datapages", "init"},
			stdin:    "n\n",
			wantCode: 1,
			wantErr:  "without a git repository",
		},
		"interactive decline module": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				out, err := exec.Command("git", "init", dir).CombinedOutput()
				require.NoError(t, err, "git init: %s", out)
				return dir
			},
			args:     []string{"datapages", "init"},
			stdin:    "n\n",
			wantCode: 1,
			wantErr:  "without a Go module",
		},
		"interactive empty dir name": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"datapages", "init"},
			stdin:    "y\n\n",
			wantCode: 1,
			wantErr:  "directory name is required",
		},
		"flag name no git no module": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"datapages", "init", "--name", "custom-app", "--module", "example.com/custom"},
			stdin:    "n\n",
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				projectDir := filepath.Join(startDir, "custom-app")
				require.DirExists(t, filepath.Join(projectDir, ".git"))
				require.FileExists(t, filepath.Join(projectDir, "go.mod"))
				data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
				require.NoError(t, err)
				require.Contains(t, string(data), "example.com/custom")
				require.FileExists(t, filepath.Join(projectDir, "datapages.yaml"))
				require.FileExists(t, filepath.Join(projectDir, "app", "app.go"))
				require.Contains(t, stdout, "Project initialized successfully.")
			},
		},
		"flag name skips name prompt": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"datapages", "init", "--name", "myproject"},
			stdin:    "y\nmymod\nn\n",
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				projectDir := filepath.Join(startDir, "myproject")
				require.DirExists(t, filepath.Join(projectDir, ".git"))
				require.FileExists(t, filepath.Join(projectDir, "go.mod"))
				data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
				require.NoError(t, err)
				require.Contains(t, string(data), "mymod")
			},
		},
		"flag module skips module prompt": {
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				out, err := exec.Command("git", "init", dir).CombinedOutput()
				require.NoError(t, err, "git init: %s", out)
				return dir
			},
			args:     []string{"datapages", "init", "--module", "example.com/flagmod"},
			stdin:    "n\n",
			wantCode: 0,
			check: func(t *testing.T, startDir string, stdout string) {
				require.FileExists(t, filepath.Join(startDir, "go.mod"))
				data, err := os.ReadFile(filepath.Join(startDir, "go.mod"))
				require.NoError(t, err)
				require.Contains(t, string(data), "example.com/flagmod")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := tc.setup(t)
			chdirTemp(t, dir)

			var stdin io.Reader
			if tc.stdin != "" {
				stdin = strings.NewReader(tc.stdin)
			}

			var stdout, stderr bytes.Buffer
			code := cmd.Run(
				context.Background(), tc.args,
				stdin, &stdout, &stderr,
				"0.0.0", "xxxxxxx", "2026-2-23",
			)
			require.Equal(t, tc.wantCode, code,
				"stdout: %s\nstderr: %s", stdout.String(), stderr.String())
			if tc.wantErr != "" {
				require.Contains(t, stderr.String(), tc.wantErr)
			}
			if tc.check != nil {
				tc.check(t, dir, stdout.String())
			}
		})
	}
}
