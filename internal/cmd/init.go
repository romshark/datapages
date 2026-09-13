package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/generator/skeleton"
)

// newInitCmd takes two versions: version goes into the scaffolded CI workflow,
// modVersion into the new go.mod. See [pinDatapages].
func newInitCmd(stderr io.Writer, version, modVersion string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Args:  cobra.NoArgs,
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
	prometheus := cmd.Flags().Bool("prometheus", true,
		"Enable Prometheus metrics generation")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		// Use accessible mode for non-terminal input (tests, piped input).
		// When stdin is a real terminal, pass nil so huh uses its TUI.
		var in io.Reader
		if _, ok := c.InOrStdin().(*os.File); !ok {
			in = c.InOrStdin()
		}
		return runInit(c.Context(), in, c.OutOrStdout(), stderr, *nonInteractive,
			*name, *module, *prometheus, version, modVersion)
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

func runInit(
	ctx context.Context, in io.Reader, out, stderr io.Writer, nonInteractive bool,
	dir, module string, prometheus bool, version, modVersion string,
) error {
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

	ciWorkflow, err := skeleton.CIWorkflow(version)
	if err != nil {
		return err
	}

	// Step 7: Write the remaining project files if missing.
	for _, f := range []struct {
		rel     string
		content string
	}{
		{"compose.yaml", skeleton.ComposeYAML},
		{"Makefile", skeleton.Makefile},
		{".vscode/extensions.json", skeleton.VSCodeExtensions},
		{".github/workflows/ci.yml", ciWorkflow},
	} {
		if _, err := writeIfMissing(
			projectDir, f.rel, []byte(f.content), out,
		); err != nil {
			return err
		}
	}

	// Step 8: Require datapages at the version this binary was built from,
	// before go mod tidy is left to choose one.
	if err := pinDatapages(projectDir, modVersion, out, stderr); err != nil {
		return err
	}

	// Step 9: Run templ generate to produce the _templ.go files from the .templ sources.
	// It runs "go run templ@version", which reads the sources and nothing of the module,
	// hence it needs no tidy module of its own.
	//
	// Ahead of go mod tidy: the templ import arrives with those files, and a  tidy that
	// runs first records no requirement for it.
	// The parser then fails on a package go.mod does not carry.
	if err := templGenerate(projectDir); err != nil {
		return err
	}

	// Step 10: Run go mod tidy to resolve app package dependencies
	// (e.g. templ) so the parser can type-check before code generation.
	if err := goModTidy(projectDir); err != nil {
		return err
	}

	// Step 11: Check the version tidy resolved, before the parser reads it.
	if err := checkDatapagesRoot(projectDir); err != nil {
		return err
	}

	// Step 12: Run code generation so all imports exist for the final tidy.
	conf, _, err := config.Load(projectDir)
	if err != nil {
		return err
	}
	if err := runGen(projectDir, conf, prometheus, stderr, ""); err != nil {
		return err
	}

	// Step 13: Run go mod tidy again to resolve generated code dependencies.
	if err := goModTidy(projectDir); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Project initialized successfully.")

	if !nonInteractive {
		runNow := true
		if err := runField(
			huh.NewConfirm().
				Title("Run the app now?").
				Value(&runNow),
			in, out,
		); err != nil {
			return err
		}
		if runNow {
			if err := os.Chdir(projectDir); err != nil {
				return fmt.Errorf("changing to project directory: %w", err)
			}
			c := exec.CommandContext(ctx, "make", "dev")
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		}
	}
	return nil
}

// resolveGitDir prompts for or defaults the directory name for a new git repo.
// If dir is non-empty, it is used directly without prompting.
func resolveGitDir(
	in io.Reader, out io.Writer, nonInteractive bool, dir string,
) (string, error) {
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

// runIn runs a command in dir, reporting its combined output on failure.
// label names the command in that error.
func runIn(dir, label, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		return execErr(label, err, out)
	}
	return nil
}

// execErr reports why a command failed. The output is empty when the program
// is missing or cannot start, and err is then the only account of it.
func execErr(label string, err error, out []byte) error {
	if s := strings.TrimSpace(string(out)); s != "" {
		return fmt.Errorf("%s: %w: %s", label, err, s)
	}
	return fmt.Errorf("%s: %w", label, err)
}

func gitInit(dir string) error { return runIn(dir, "git init", "git", "init") }

func goModInit(dir, modulePath string) error {
	return runIn(dir, "go mod init", "go", "mod", "init", modulePath)
}

func templGenerate(dir string) error {
	return runIn(dir, "templ generate",
		"go", "run", skeleton.TemplCmd, "generate", "./app/")
}

func goModTidy(dir string) error { return runIn(dir, "go mod tidy", "go", "mod", "tidy") }

func writeDefaultConfigIfMissing(projectDir string, w io.Writer) (bool, error) {
	for _, name := range []string{"datapages.yml", "datapages.yaml"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err == nil {
			return false, nil
		}
	}
	if err := config.WriteDefault(projectDir); err != nil {
		return false, err
	}
	_, _ = fmt.Fprintln(w, "Created datapages.yaml")
	return true, nil
}

// writeIfMissing writes content to rel under projectDir unless rel exists.
func writeIfMissing(
	projectDir, rel string, content []byte, w io.Writer,
) (bool, error) {
	path := filepath.Join(projectDir, filepath.FromSlash(rel))
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if dir := filepath.Dir(path); dir != projectDir {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("creating %s: %w", filepath.Dir(rel), err)
		}
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", rel, err)
	}
	_, _ = fmt.Fprintf(w, "Created %s\n", rel)
	return true, nil
}

func writeAppGoIfMissing(projectDir string, w io.Writer) (bool, error) {
	src, err := skeleton.AppGo()
	if err != nil {
		return false, err
	}
	wrote, err := writeIfMissing(
		projectDir, "app/app.go", src, w,
	)
	if err != nil || !wrote {
		return false, err
	}
	_, err = writeIfMissing(
		projectDir, "app/app.templ", []byte(skeleton.AppTempl), w,
	)
	return true, err
}

func writeEnvIfMissing(projectDir string, w io.Writer) (bool, error) {
	if _, err := os.Stat(filepath.Join(projectDir, ".env")); err == nil {
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
	return writeIfMissing(projectDir, ".env", []byte(content), w)
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
	for line := range strings.SplitSeq(string(content), "\n") {
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

// pinDatapages writes the datapages requirement into the project's go.mod
// before the first tidy runs.
//
// Without it tidy picks whatever the proxy calls latest. The generated code
// imports runtime/httpserve, runtime/auth and the rest, which have to come
// from the same tree as the generator that wrote it.
//
// A version the proxy resolves becomes a require. Otherwise the checkout the
// binary was compiled from becomes a replace, see [datapagesCheckout]. With
// neither, init warns and leaves the choice to tidy.
//
// A go.mod that already requires datapages keeps it. init runs in an existing
// module too, and the version there is the user's to choose.
func pinDatapages(projectDir, version string, out, stderr io.Writer) error {
	gomodPath := filepath.Join(projectDir, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return fmt.Errorf("reading go.mod: %w", err)
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return fmt.Errorf("parsing go.mod: %w", err)
	}
	for _, req := range f.Require {
		if req.Mod.Path == datapagesModulePath {
			return nil
		}
	}

	// A require the proxy cannot resolve breaks every import in the module,
	// which is worse than the version tidy would have picked. A binary stamped
	// with a tag that was never pushed carries such a require.
	reason := "this build carries no version"
	if version != "" {
		reason = ""
		if err := moduleVersionExists(projectDir, version); err != nil {
			reason = err.Error()
		}
	}

	var done string
	switch dir := ""; reason {
	case "":
		if err := f.AddRequire(datapagesModulePath, version); err != nil {
			return fmt.Errorf(
				"adding the %s requirement: %w", datapagesModulePath, err,
			)
		}
		done = fmt.Sprintf("Required %s %s", datapagesModulePath, version)

	default:
		dir = datapagesCheckout()
		if dir == "" {
			_, _ = fmt.Fprintf(stderr,
				"warning: go.mod does not require the %s this CLI was built "+
					"from: %s\n"+
					"  go mod tidy picks a version instead, and the generated "+
					"code is compiled against that one.\n"+
					"  Add a replace for your checkout, or install a release: "+
					"go install %s/cmd/datapages@latest\n",
				datapagesModulePath, reason, datapagesModulePath)
			return nil
		}
		if err := f.AddReplace(datapagesModulePath, "", dir, ""); err != nil {
			return fmt.Errorf(
				"adding the %s replace: %w", datapagesModulePath, err,
			)
		}
		done = fmt.Sprintf("Replaced %s => %s", datapagesModulePath, dir)
		_, _ = fmt.Fprintf(stderr,
			"warning: %s, hence go.mod names the checkout this CLI was built "+
				"from.\n"+
				"  That path exists on this machine only. Install a release to "+
				"require a version instead:\n"+
				"  go install %s/cmd/datapages@latest\n",
			reason, datapagesModulePath)
	}

	f.Cleanup()
	b, err := f.Format()
	if err != nil {
		return fmt.Errorf("formatting go.mod: %w", err)
	}
	if err := os.WriteFile(gomodPath, b, 0o644); err != nil {
		return fmt.Errorf("writing go.mod: %w", err)
	}
	_, _ = fmt.Fprintln(out, done)
	return nil
}

// datapagesCheckout returns the directory of the datapages module this binary
// was compiled from, empty when there is none to find.
//
// The compiler records the source path of every file it builds, which runtime.Caller
// reads back. A release build erases it with -trimpath and carries a version instead.
// A binary moved to another machine records a path that does not exist there.
func datapagesCheckout() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return ""
	}
	for dir := filepath.Dir(file); ; {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			f, err := modfile.ParseLax("go.mod", data, nil)
			if err != nil || f.Module == nil ||
				f.Module.Mod.Path != datapagesModulePath {
				// Another module: this file is no part of a datapages checkout.
				return ""
			}
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// moduleVersionExists reports whether the proxy can resolve the datapages
// module at version. It asks with "go list -m", which reports the answer in
// its output rather than its exit status.
// A caller may have set GOFLAGS=-e, which suppresses the exit status.
func moduleVersionExists(projectDir, version string) error {
	out, err := goListValue(projectDir,
		"{{if .Error}}{{.Error.Err}}{{end}}",
		"-m", datapagesModulePath+"@"+version)
	if err != nil {
		return err
	}
	if out != "" {
		return errors.New(out)
	}
	return nil
}

// checkDatapagesRoot reports a resolved datapages version whose
// root package cannot be imported.
//
// Every release up to v0.9.4 carries package main at the module root:
// the CLI lived there before it moved to cmd/datapages.
// The app package imports the root package, hence the parser,
// the generator and the build all fail with errors that name the user's
// own app package and never the version that cannot be imported.
func checkDatapagesRoot(projectDir string) error {
	name, err := goListValue(projectDir, "{{.Name}}", datapagesModulePath)
	if err != nil || name != "main" {
		// A load error is one the parser reports with more context.
		return nil
	}
	version, err := goListValue(projectDir, "{{.Version}}", "-m", datapagesModulePath)
	if err != nil {
		version = "the resolved version"
	}
	return fmt.Errorf(
		"%s %s has no importable root package: it is a command\n"+
			"  The app package imports %s, which that version does not provide.\n"+
			"  Require a version that does, or add a replace for a local checkout:\n"+
			"    go mod edit -require=%s@<version>\n"+
			"    go mod edit -replace=%s=<path to your checkout>",
		datapagesModulePath, version,
		datapagesModulePath, datapagesModulePath, datapagesModulePath,
	)
}

// goListValue runs "go list" with the given format and arguments in dir and
// returns the first line of its output. The -e keeps a package that does not
// load from failing the command, which is the case the caller asks about.
func goListValue(dir, format string, args ...string) (string, error) {
	argv := append([]string{"list", "-e", "-f", format}, args...)
	cmd := exec.Command("go", argv...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list: %w", err)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line, nil
}
