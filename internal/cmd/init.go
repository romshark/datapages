package cmd

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const defaultAutoDir = "datapages-app"

//go:embed default_app.go.txt
var defaultAppGo string

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Datapages project",
		Long: `Create a new Datapages project with the standard directory structure.

By default, init runs interactively and prompts for project settings.
Use --auto for non-interactive mode with sensible defaults.

If not inside a git repository, a new one is created. If not inside
a Go module, a new one is initialized. Missing datapages.yaml and
app/app.go files are generated. Finally, go mod tidy is run.`,
	}
	auto := cmd.Flags().Bool("auto", false,
		"Non-interactive mode with default settings")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runInit(
			c.InOrStdin(),
			c.OutOrStdout(),
			*auto,
		)
	}
	return cmd
}

func runInit(stdin io.Reader, stdout io.Writer, auto bool) error {
	reader := bufio.NewReader(stdin)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Step 1: Ensure git repository.
	var projectDir string
	var created bool
	if gitDir := findGitDir(cwd); gitDir == "" {
		dirName, err := resolveGitDir(reader, stdout, auto)
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
		_, _ = fmt.Fprintf(stdout, "Initialized git repository in %s\n", projectDir)
		created = true
	} else {
		projectDir = cwd
	}

	// Step 2: Ensure Go module.
	goModPath := filepath.Join(projectDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		modulePath, err := resolveModulePath(reader, stdout, projectDir, auto)
		if err != nil {
			return err
		}
		if err := goModInit(projectDir, modulePath); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "Initialized Go module %s\n", modulePath)
		created = true
	}

	// Step 3: Write datapages.yaml if missing.
	if wrote, err := writeDefaultConfigIfMissing(projectDir, stdout); err != nil {
		return err
	} else if wrote {
		created = true
	}

	// Step 4: Write app/app.go if missing.
	if wrote, err := writeAppGoIfMissing(projectDir, stdout); err != nil {
		return err
	} else if wrote {
		created = true
	}

	// Step 5: Run go mod tidy.
	if err := goModTidy(projectDir); err != nil {
		return err
	}

	if created {
		_, _ = fmt.Fprintln(stdout, "Project initialized successfully.")
	} else {
		_, _ = fmt.Fprintln(stdout, "Project already initialized.")
	}
	return nil
}

// resolveGitDir prompts for or defaults the directory name for a new git repo.
func resolveGitDir(reader *bufio.Reader, w io.Writer, auto bool) (string, error) {
	if auto {
		return defaultAutoDir, nil
	}
	ok, err := promptYesNo(reader, w,
		"Not inside a git repository. Create one?", true)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("cannot initialize project without a git repository")
	}
	name, err := prompt(reader, w, "Directory name", "")
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("directory name is required")
	}
	return name, nil
}

// resolveModulePath prompts for or defaults the Go module path.
func resolveModulePath(
	reader *bufio.Reader, w io.Writer, projectDir string, auto bool,
) (string, error) {
	defaultPath := gitRemoteModulePath(projectDir)
	if defaultPath == "" {
		defaultPath = filepath.Base(projectDir)
	}

	if auto {
		return defaultPath, nil
	}

	ok, err := promptYesNo(reader, w,
		"No go.mod found. Initialize a Go module?", true)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("cannot initialize project without a Go module")
	}

	modulePath, err := prompt(reader, w, "Module path", defaultPath)
	if err != nil {
		return "", err
	}
	if modulePath == "" {
		return "", fmt.Errorf("module path is required")
	}
	return modulePath, nil
}

// prompt asks a question and returns the answer.
// If the user provides an empty response, the default is returned.
func prompt(
	reader *bufio.Reader, w io.Writer, question, defaultVal string,
) (string, error) {
	if defaultVal != "" {
		_, _ = fmt.Fprintf(w, "%s [%s]: ", question, defaultVal)
	} else {
		_, _ = fmt.Fprintf(w, "%s: ", question)
	}
	answer, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return defaultVal, nil
	}
	return answer, nil
}

// promptYesNo asks a yes/no question.
// defaultYes controls the default when the user presses Enter.
func promptYesNo(
	reader *bufio.Reader, w io.Writer, question string, defaultYes bool,
) (bool, error) {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	_, _ = fmt.Fprintf(w, "%s [%s]: ", question, hint)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading input: %w", err)
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" {
		return defaultYes, nil
	}
	return answer == "y" || answer == "yes", nil
}

// gitRemoteModulePath runs "git remote get-url origin" and converts
// the URL to a Go module path. Returns empty string on any error.
func gitRemoteModulePath(dir string) string {
	c := exec.Command("git", "remote", "get-url", "origin")
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return remoteURLToModulePath(strings.TrimSpace(string(out)))
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
	if err := os.WriteFile(appFile, []byte(defaultAppGo), 0o644); err != nil {
		return false, fmt.Errorf("writing app/app.go: %w", err)
	}
	_, _ = fmt.Fprintln(w, "Created app/app.go")
	return true, nil
}
