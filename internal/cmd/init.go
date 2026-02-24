package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/romshark/datapages/generator/skeleton"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newInitCmd(stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Datapages project",
		Long: `Create a new Datapages project with the standard directory structure.

By default, init runs interactively and prompts for project settings.
Use -n/--non-interactive to disable prompts; when a value is needed
that would normally be prompted for, pass it via --name or --module.

If not inside a git repository, a new one is created. If not inside
a Go module, a new one is initialized. Missing datapages.yaml and
app/app.go files are generated. Code generation is run, and finally
go mod tidy resolves all dependencies.`,
	}
	nonInteractive := cmd.Flags().BoolP("non-interactive", "n", false,
		"Disable interactive prompts (requires --name/--module when applicable)")
	name := cmd.Flags().String("name", "",
		"Project name (used as directory name)")
	module := cmd.Flags().String("module", "",
		"Go module path")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		// Use accessible mode for non-terminal input (tests, piped input).
		// When stdin is a real terminal, pass nil so huh uses its TUI.
		var in io.Reader
		if _, ok := c.InOrStdin().(*os.File); !ok {
			in = c.InOrStdin()
		}
		return runInit(in, c.OutOrStdout(), stderr, *nonInteractive, *name, *module)
	}
	return cmd
}

// oneByteReader wraps an io.Reader to return at most one byte per Read call.
// This prevents bufio.Scanner from buffering ahead when multiple huh fields
// share the same underlying reader in accessible mode.
type oneByteReader struct{ r io.Reader }

func (o oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

// runField runs a huh field. When in is non-nil, it uses accessible mode
// (line-based I/O). Otherwise it uses the full TUI.
func runField(f huh.Field, in io.Reader, out io.Writer) error {
	if in != nil {
		return f.RunAccessible(out, in)
	}
	return f.Run()
}

func runInit(in io.Reader, out, stderr io.Writer, nonInteractive bool, dir, module string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Wrap non-terminal reader so each huh field's internal bufio.Scanner
	// only consumes exactly the bytes it needs.
	if in != nil {
		in = oneByteReader{in}
	}

	// Step 1: Ensure git repository.
	var projectDir string
	var created bool
	if gitDir := findGitDir(cwd); gitDir == "" {
		dirName, err := resolveGitDir(in, out, nonInteractive, dir)
		if err != nil {
			return err
		}
		projectDir = filepath.Join(cwd, dirName)
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			return fmt.Errorf("creating project directory: %w", err)
		}
		if err := gitInit(projectDir); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Initialized git repository in %s\n", projectDir)
		created = true
	} else {
		projectDir = cwd
	}

	// Step 2: Ensure Go module.
	goModPath := filepath.Join(projectDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		modulePath, err := resolveModulePath(in, out, projectDir, nonInteractive, module)
		if err != nil {
			return err
		}
		if err := goModInit(projectDir, modulePath); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Initialized Go module %s\n", modulePath)
		created = true
	}

	// Step 3: Write datapages.yaml if missing.
	if wrote, err := writeDefaultConfigIfMissing(projectDir, out); err != nil {
		return err
	} else if wrote {
		created = true
	}

	// Step 4: Write app/app.go if missing.
	if wrote, err := writeAppGoIfMissing(projectDir, out); err != nil {
		return err
	} else if wrote {
		created = true
	}

	if !created {
		_, _ = fmt.Fprintln(out, "Project already initialized.")
		return nil
	}

	// Step 5: Write .env with random secrets if missing.
	if _, err := writeEnvIfMissing(projectDir, out); err != nil {
		return err
	}

	// Step 6: Append .env to .gitignore.
	if err := gitignoreEnv(projectDir); err != nil {
		return err
	}

	// Step 7: Write compose.yaml if missing.
	if _, err := writeComposeIfMissing(projectDir, out); err != nil {
		return err
	}

	// Step 8: Write Makefile if missing.
	if _, err := writeMakefileIfMissing(projectDir, out); err != nil {
		return err
	}

	// Step 9: Run go mod tidy to resolve app package dependencies
	// (e.g. templ) so the parser can type-check before code generation.
	if err := goModTidy(projectDir); err != nil {
		return err
	}

	// Step 10: Run templ generate to produce _templ.go files from .templ
	// sources so the parser can type-check before code generation.
	if err := templGenerate(projectDir); err != nil {
		return err
	}

	// Step 11: Run code generation so all imports exist for the final tidy.
	config, _, err := loadConfig(projectDir)
	if err != nil {
		return err
	}
	if err := runGen(projectDir, config, stderr); err != nil {
		return err
	}

	// Step 12: Run go mod tidy again to resolve generated code dependencies.
	if err := goModTidy(projectDir); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Project initialized successfully.")
	return nil
}

// resolveGitDir prompts for or defaults the directory name for a new git repo.
// If dir is non-empty, it is used directly without prompting.
func resolveGitDir(in io.Reader, out io.Writer, nonInteractive bool, dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if nonInteractive {
		return "", fmt.Errorf("--name is required in non-interactive mode " +
			"when not inside a git repository")
	}

	ok := true
	if err := runField(
		huh.NewConfirm().
			Title("Not inside a git repository. Create one?").
			Value(&ok),
		in, out,
	); err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("cannot initialize project without a git repository")
	}

	var name string
	if err := runField(
		huh.NewInput().
			Title("Directory name").
			Value(&name).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("directory name is required")
				}
				return nil
			}),
		in, out,
	); err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("directory name is required")
	}
	return name, nil
}

// resolveModulePath prompts for or defaults the Go module path.
// If module is non-empty, it is used directly without prompting.
func resolveModulePath(
	in io.Reader, out io.Writer, projectDir string, nonInteractive bool, module string,
) (string, error) {
	if module != "" {
		return module, nil
	}
	if nonInteractive {
		return "", fmt.Errorf("--module is required in non-interactive mode " +
			"when not inside a Go module")
	}

	defaultPath := gitRemoteModulePath(projectDir)
	if defaultPath == "" {
		defaultPath = filepath.Base(projectDir)
	}

	ok := true
	if err := runField(
		huh.NewConfirm().
			Title("No go.mod found. Initialize a Go module?").
			Value(&ok),
		in, out,
	); err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("cannot initialize project without a Go module")
	}

	modulePath := defaultPath
	if err := runField(
		huh.NewInput().
			Title("Module path").
			Value(&modulePath).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("module path is required")
				}
				return nil
			}),
		in, out,
	); err != nil {
		return "", err
	}
	if modulePath == "" {
		return "", fmt.Errorf("module path is required")
	}
	return modulePath, nil
}

// gitRemoteModulePath runs "git remote get-url origin" and converts
// the URL to a Go module path. If dir is a subdirectory of the git root,
// the relative path is appended. Returns empty string on any error.
func gitRemoteModulePath(dir string) string {
	c := exec.Command("git", "remote", "get-url", "origin")
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return ""
	}
	modulePath := remoteURLToModulePath(strings.TrimSpace(string(out)))

	// If project dir is a subdirectory of the git root, append the
	// relative path so the module path is unique within the repo.
	c = exec.Command("git", "rev-parse", "--show-toplevel")
	c.Dir = dir
	topOut, err := c.Output()
	if err != nil {
		return modulePath
	}
	gitRoot := strings.TrimSpace(string(topOut))
	rel, err := filepath.Rel(gitRoot, dir)
	if err != nil || rel == "." {
		return modulePath
	}
	return modulePath + "/" + filepath.ToSlash(rel)
}

// remoteURLToModulePath converts a git remote URL to a Go module path.
//
//	https://github.com/user/repo.git -> github.com/user/repo
//	git@github.com:user/repo.git    -> github.com/user/repo
func remoteURLToModulePath(rawURL string) string {
	if s, ok := strings.CutPrefix(rawURL, "git@"); ok {
		rawURL = strings.Replace(s, ":", "/", 1)
	}
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.TrimRight(rawURL, "/")
	return rawURL
}

func gitInit(dir string) error {
	c := exec.Command("git", "init")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func goModInit(dir, modulePath string) error {
	c := exec.Command("go", "mod", "init", modulePath)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod init: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func templGenerate(dir string) error {
	c := exec.Command(
		"go", "run", "github.com/a-h/templ/cmd/templ@latest",
		"generate", "./app/",
	)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("templ generate: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func goModTidy(dir string) error {
	c := exec.Command("go", "mod", "tidy")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func writeDefaultConfigIfMissing(projectDir string, w io.Writer) (bool, error) {
	for _, name := range []string{"datapages.yml", "datapages.yaml"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err == nil {
			return false, nil
		}
	}
	if err := writeDefaultConfig(projectDir); err != nil {
		return false, err
	}
	_, _ = fmt.Fprintln(w, "Created datapages.yaml")
	return true, nil
}

func writeAppGoIfMissing(projectDir string, w io.Writer) (bool, error) {
	appDir := filepath.Join(projectDir, "app")
	appFile := filepath.Join(appDir, "app.go")
	if _, err := os.Stat(appFile); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return false, fmt.Errorf("creating app directory: %w", err)
	}
	if err := os.WriteFile(appFile, []byte(skeleton.AppGo), 0o644); err != nil {
		return false, fmt.Errorf("writing app/app.go: %w", err)
	}
	_, _ = fmt.Fprintln(w, "Created app/app.go")
	templFile := filepath.Join(appDir, "app.templ")
	if err := os.WriteFile(templFile, []byte(skeleton.AppTempl), 0o644); err != nil {
		return false, fmt.Errorf("writing app/app.templ: %w", err)
	}
	_, _ = fmt.Fprintln(w, "Created app/app.templ")
	return true, nil
}

func writeEnvIfMissing(projectDir string, w io.Writer) (bool, error) {
	envFile := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envFile); err == nil {
		return false, nil
	}
	csrfSecret, err := randomHex(32)
	if err != nil {
		return false, fmt.Errorf("generating CSRF secret: %w", err)
	}
	sessKey, err := randomHex(16)
	if err != nil {
		return false, fmt.Errorf("generating session encryption key: %w", err)
	}
	content := "NATS_URL=nats://localhost:4222\n" +
		"CSRF_SECRET=" + csrfSecret + "\n" +
		"SESSION_ENCRYPTION_KEY=" + sessKey + "\n"
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("writing .env: %w", err)
	}
	_, _ = fmt.Fprintln(w, "Created .env")
	return true, nil
}

func writeComposeIfMissing(projectDir string, w io.Writer) (bool, error) {
	composePath := filepath.Join(projectDir, "compose.yaml")
	if _, err := os.Stat(composePath); err == nil {
		return false, nil
	}
	if err := os.WriteFile(composePath, []byte(skeleton.ComposeYAML), 0o644); err != nil {
		return false, fmt.Errorf("writing compose.yaml: %w", err)
	}
	_, _ = fmt.Fprintln(w, "Created compose.yaml")
	return true, nil
}

func writeMakefileIfMissing(projectDir string, w io.Writer) (bool, error) {
	makefilePath := filepath.Join(projectDir, "Makefile")
	if _, err := os.Stat(makefilePath); err == nil {
		return false, nil
	}
	if err := os.WriteFile(makefilePath, []byte(skeleton.Makefile), 0o644); err != nil {
		return false, fmt.Errorf("writing Makefile: %w", err)
	}
	_, _ = fmt.Fprintln(w, "Created Makefile")
	return true, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// gitignoreEnv ensures .env is listed in .gitignore.
func gitignoreEnv(projectDir string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == ".env" {
			return nil
		}
	}
	f, err := os.OpenFile(gitignorePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	entry := ".env\n"
	if len(content) > 0 && content[len(content)-1] != '\n' {
		entry = "\n" + entry
	}
	_, writeErr := f.WriteString(entry)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("writing .gitignore: %w", writeErr)
	}
	return closeErr
}
