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
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/generator/agentdocs"
	"github.com/romshark/datapages/internal/generator/skeleton"
	"github.com/romshark/datapages/internal/serverscan"
)

// newInitCmd uses version for agent docs and modVersion for go.mod and CI.
// [pinDatapages] defines how modVersion is applied.
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
a Go module, a new one is initialized. Code generation is run, and
finally go mod tidy resolves all dependencies. Init never deletes any files.

Init writes these only when they are missing and never touches them
again: datapages.yaml, app/app.go, app/app.templ, cmd/server/main.go, .env,
compose.yaml, Makefile, .vscode/extensions.json and
.github/workflows/ci.yml. It also adds .env to .gitignore.

Init rewrites these on every run: AGENTS.md, CLAUDE.md, GEMINI.md,
.github/copilot-instructions.md, .cursor/rules/datapages.mdc and the
skills under .agents/skills and .claude/skills. If you edit these files
then init will create <name>.bak backup files before it replaces them.
Skills with other names stay untouched.
Pass --no-ai-skills to skip all of these files.`,
	}
	nonInteractive := cmd.Flags().BoolP("non-interactive", "n", false,
		"Disable interactive prompts (requires --name/--module when applicable)")
	name := cmd.Flags().String("name", "",
		"Project name (used as directory name)")
	module := cmd.Flags().String("module", "",
		"Go module path")
	prometheus := cmd.Flags().Bool("prometheus", scaffoldPrometheus,
		"Enable Prometheus metrics in the generated entry point")
	noAISkills := cmd.Flags().Bool("no-ai-skills", false,
		"Skip agent instructions and skills")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		// Non-file input uses huh's line-based accessible mode.
		// Leave in nil for Cobra's *os.File stdin so huh selects the TUI.
		var in io.Reader
		if _, ok := c.InOrStdin().(*os.File); !ok {
			in = c.InOrStdin()
		}
		return runInit(c.Context(), in, c.OutOrStdout(), stderr, *nonInteractive,
			*name, *module, *prometheus, !*noAISkills, version, modVersion)
	}
	return cmd
}

// oneByteReader prevents one huh field's scanner from
// consuming input meant for the next field.
type oneByteReader struct{ r io.Reader }

func (o oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

// runField uses huh's line-based accessible mode when in is set.
// A nil reader selects the TUI.
func runField(f huh.Field, in io.Reader, out io.Writer) error {
	if in != nil {
		return f.RunAccessible(out, in)
	}
	return f.Run()
}

func runInit(
	ctx context.Context, in io.Reader, out, stderr io.Writer, nonInteractive bool,
	dir, module string, prometheus, aiSkills bool, version, modVersion string,
) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	if in != nil {
		in = oneByteReader{in}
	}

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

	if wrote, err := writeDefaultConfigIfMissing(projectDir, out); err != nil {
		return err
	} else if wrote {
		created = true
	}

	// [github.com/romshark/datapages.NewServer] calls identify
	// app packages before those packages exist on disk.
	modulePath, err := readModulePath(projectDir)
	if err != nil {
		return err
	}
	scan, err := serverscan.Scan(projectDir, modulePath)
	if err != nil {
		return err
	}
	if scan.Fallback {
		if wrote, err := writeAppGoIfMissing(projectDir, out); err != nil {
			return err
		} else if wrote {
			created = true
		}
	}

	// Refresh agent instructions even when init creates no project files.
	if aiSkills {
		if err := writeAgentDocs(projectDir, out, version); err != nil {
			return err
		}
	}

	if err := gitignoreEnv(projectDir); err != nil {
		return err
	}

	if !created {
		_, _ = fmt.Fprintln(out, "Project already initialized.")
		return nil
	}

	if _, err := writeEnvIfMissing(projectDir, out); err != nil {
		return err
	}

	// CI must install the version [pinDatapages] writes to go.mod.
	// Source builds use a pseudo-version instead of a release.
	ciWorkflow, err := skeleton.CIWorkflow(modVersion)
	if err != nil {
		return err
	}

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

	// Pin Datapages before go mod tidy can select another version.
	if err := pinDatapages(ctx, projectDir, modVersion, out, stderr); err != nil {
		return err
	}

	// Generate Templ before go mod tidy records dependencies.
	// The parser needs the templ requirement to type-check the app package.
	if err := templGenerate(projectDir); err != nil {
		return err
	}

	if err := goModTidy(projectDir); err != nil {
		return err
	}

	if err := checkDatapagesRoot(ctx, projectDir); err != nil {
		return err
	}

	conf, _, err := config.Load(projectDir)
	if err != nil {
		return err
	}
	if err := runGen(projectDir, conf, prometheus, stderr, ""); err != nil {
		return err
	}

	// Record dependencies imported only by generated code.
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

// resolveGitDir returns dir when set. Otherwise it prompts for a repository
// directory or requires --name in non-interactive mode.
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

// resolveModulePath returns module when set. Otherwise it prompts with a default
// based on the Git remote or directory name. Non-interactive mode requires --module.
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

// gitRemoteModulePath converts the origin URL to a Go module path.
// For a subdirectory, it appends the path relative to the repository root.
// It returns an empty string when origin cannot be read.
func gitRemoteModulePath(dir string) string {
	c := exec.Command("git", "remote", "get-url", "origin")
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return ""
	}
	modulePath := remoteURLToModulePath(strings.TrimSpace(string(out)))

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

// runIn executes name in dir and includes its
// combined output in an error prefixed with label.
func runIn(dir, label, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		return execErr(label, err, out)
	}
	return nil
}

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

func goModTidy(dir string) error {
	return runIn(dir, "go mod tidy", "go", "mod", "tidy")
}

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

// writeAgentDocs writes AI instructions from the module layout and server scan.
// It doesn't require generated packages.
func writeAgentDocs(projectDir string, w io.Writer, version string) error {
	modulePath, err := readModulePath(projectDir)
	if err != nil {
		return err
	}
	scan, err := serverscan.Scan(projectDir, modulePath)
	if err != nil {
		return err
	}
	cfg, _, err := config.Load(projectDir)
	if err != nil {
		return err
	}
	apps := make([]agentdocs.App, len(scan.Apps))
	var cmds []string
	for i, a := range scan.Apps {
		apps[i] = agentdocs.App{Dir: a.Dir, GenDir: a.GenDir}
		for _, c := range a.Calls {
			if c.Main {
				cmds = append(cmds, c.Dir)
			}
		}
	}
	res, err := agentdocs.Write(projectDir, agentdocs.Project{
		Cmd:     cfg.Cmd,
		Cmds:    cmds,
		Apps:    apps,
		Version: version,
	}, 0o644)
	if err != nil {
		return fmt.Errorf("writing agent instructions: %w", err)
	}

	// Group skill writes into one line to keep init output short.
	skillsDirs := []string{
		filepath.FromSlash(agentdocs.SkillsDir),
		filepath.FromSlash(agentdocs.AgentsSkillsDir),
	}
	var skillFiles int
	for _, rel := range res.Written {
		if slices.ContainsFunc(skillsDirs, func(d string) bool {
			return strings.HasPrefix(rel, d)
		}) {
			skillFiles++
			continue
		}
		_, _ = fmt.Fprintf(w, "Wrote %s\n", rel)
	}
	if skillFiles > 0 {
		_, _ = fmt.Fprintf(w, "Wrote %d skill files in %s\n",
			skillFiles, strings.Join(skillsDirs, " and "))
	}
	for _, b := range res.BackedUp {
		_, _ = fmt.Fprintf(w, "Kept the previous %s as %s\n", b.Path, b.To)
	}
	return nil
}

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
	sessKey, err := randomHex(16)
	if err != nil {
		return false, fmt.Errorf("generating session encryption key: %w", err)
	}
	// [github.com/romshark/datapages/modules/csrf.Tokens] derives
	// CSRF tokens from session tokens. No separate secret is needed.
	content := "NATS_URL=nats://localhost:4222\n" +
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

// gitignoreEnv appends .env without changing existing .gitignore entries.
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

// pinDatapages pins the runtime dependency to the generator's source before
// go mod tidy can select another version. It preserves an existing requirement.
//
// A resolvable version becomes a requirement.
// Without one, pinDatapages uses the path from [datapagesCheckout] as a replacement.
// Without either, init warns and lets go mod tidy select a version.
func pinDatapages(
	ctx context.Context, projectDir, version string, out, stderr io.Writer,
) error {
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

	// An unpublished build version would break every Datapages import.
	// Confirm that Go can resolve it before adding the requirement.
	reason := "this build carries no version"
	if version != "" {
		reason = ""
		if err := moduleVersionExists(ctx, projectDir, version); err != nil {
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

// datapagesCheckout returns the Datapages source tree used to build this binary.
// It returns an empty string when the recorded source path does not identify that module.
//
// runtime.Caller exposes the compiler-recorded file path. Local builds retain
// an absolute path. Release builds use -trimpath, and copied binaries may refer
// to a path absent on the new machine.
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
				// A different module boundary rules out a Datapages checkout.
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

// moduleVersionResolveTimeout bounds proxy and
// VCS lookups that have no deadline of their own.
const moduleVersionResolveTimeout = 30 * time.Second

// moduleVersionExists reports whether Go can resolve the Datapages module at version.
// It reads module errors from formatted output because GOFLAGS=-e can
// suppress the failure exit status.
func moduleVersionExists(ctx context.Context, projectDir, version string) error {
	query, cancel := context.WithTimeout(ctx, moduleVersionResolveTimeout)
	defer cancel()
	out, err := goListValue(query, projectDir,
		"{{if .Error}}{{.Error.Err}}{{end}}",
		"-m", datapagesModulePath+"@"+version)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case query.Err() != nil:
			return fmt.Errorf("resolving %s@%s took longer than %s",
				datapagesModulePath, version, moduleVersionResolveTimeout)
		}
		return err
	}
	if out != "" {
		return errors.New(out)
	}
	return nil
}

// checkDatapagesRoot reports Datapages versions whose module root is package main.
// Releases through v0.9.4 use this layout and cannot satisfy app imports.
// This check avoids parser and generator errors that name the app package
// instead of the incompatible Datapages version.
func checkDatapagesRoot(ctx context.Context, projectDir string) error {
	name, err := goListValue(ctx, projectDir, "{{.Name}}", datapagesModulePath)
	if err != nil || name != "main" {
		// Let the parser report load errors with application context.
		return nil
	}
	version, err := goListValue(
		ctx, projectDir, "{{.Version}}", "-m", datapagesModulePath,
	)
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

// goListValue returns the first formatted line from "go list -e". The -e flag
// exposes package load errors in template data instead of the command status.
func goListValue(
	ctx context.Context, dir, format string, args ...string,
) (string, error) {
	argv := append([]string{"list", "-e", "-f", format}, args...)
	cmd := exec.CommandContext(ctx, "go", argv...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list: %w", err)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line, nil
}
