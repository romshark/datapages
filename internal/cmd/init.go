package cmd

import (
	"cmp"
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
that would normally be prompted for, pass it via --module.

Outside an initialized project, init asks whether to create a
subdirectory for the project, and its name. --name answers without the
prompt, and -n uses the current directory. The subdirectory gets a Go
module of its own, even inside another one. Otherwise, inside a Go
module, init runs in the directory of its go.mod. If not inside a Go
module, init asks for the module path, suggesting one from the git
remote or the directory name, and initializes one.

If not inside a git repository, init asks whether to create one, except
in a Go module that has a datapages.yaml. Pass --no-git to answer no
without the prompt, or --no-git=false to create one. -n creates one.

Code generation is run, and finally go mod tidy resolves all
dependencies. Init never deletes any files.

Init writes these only when they are missing and never touches them
again: datapages.yaml, app/app.go, app/app.templ, cmd/server/main.go, .env,
compose.yaml, Makefile, .vscode/extensions.json, .vscode/settings.json for
hidden generated code and .github/workflows/ci.yml. It also adds .env, and
the hidden generated code, to .gitignore.

When init writes app/app.go, it asks how to set up Web Components, for
JavaScript that would be impractical in Datastar attributes. Pass
--web-components to answer without the prompt:

  rocket      Datastar Rocket components in app/static/components, loaded
              with the Rocket bundle of the Datastar release Datapages uses.
  typescript  TypeScript in web/src, bundled by esbuild into
              app/static/components.js. Needs Node.js: init runs npm install
              and the first build, and datapages watch rebuilds on changes.
  javascript  JavaScript modules in app/static/components, served as they are.
  none        No Web Components, the default with -n.

Each setup comes with an example component on the welcome page.

When init writes the project files, it asks whether git keeps the generated
code, datapagesgen/ and *_templ.go. Pass --generated-code to answer without
the prompt:

  committed  Checked into git, the default with -n. A clone builds as it
             is, and so does go install of the module. Diffs and merges
             include the generated files, and CI fails when they are stale.
  hidden     Listed in .gitignore and hidden in VS Code. Diffs and merges
             hold only source, and nothing can go stale. A clone builds only
             after templ generate and datapages gen, which make build and CI
             run, and go install of the module fails. datapages gen fails on
             a clone while the app uses the href, action or assets packages,
             because the app does not compile without them.

It also asks for the CI system to write a workflow for. Pass --ci to answer
without the prompt:

  github-actions  .github/workflows/ci.yml, the default with -n.
  none            No CI workflow.

Init rewrites these on every run: AGENTS.md, CLAUDE.md, GEMINI.md,
.github/copilot-instructions.md, .cursor/rules/datapages.mdc and the
skills under .agents/skills and .claude/skills. If you edit these files
then init will create <name>.bak backup files before it replaces them.
An AGENTS.md that does not end with the datapages stamp is your own and
stays untouched, and so do skills with other names.
Init asks whether to write these files, and -n writes them. Pass
--no-ai-skills to skip them without the prompt, or --no-ai-skills=false
to write them.`,
	}
	nonInteractive := cmd.Flags().BoolP("non-interactive", "n", false,
		"Disable interactive prompts (requires --module when applicable)")
	name := cmd.Flags().String("name", "",
		"Directory to create and initialize the project in")
	noGit := cmd.Flags().Bool("no-git", false,
		"Skip creating a git repository")
	module := cmd.Flags().String("module", "",
		"Go module path")
	prometheus := cmd.Flags().Bool("prometheus", scaffoldPrometheus,
		"Enable Prometheus metrics in the generated entry point")
	noAISkills := cmd.Flags().Bool("no-ai-skills", false,
		"Skip agent instructions and skills")
	ci := cmd.Flags().String("ci", "",
		"CI system to write a workflow for: github-actions or none")
	webComponents := cmd.Flags().String("web-components", "",
		"Web Component setup: rocket, typescript, javascript or none")
	generatedCode := cmd.Flags().String("generated-code", "",
		"Whether git keeps the generated code: committed or hidden")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		var git, aiSkills *bool
		if c.Flags().Changed("no-git") {
			git = new(!*noGit)
		}
		if c.Flags().Changed("no-ai-skills") {
			aiSkills = new(!*noAISkills)
		}
		// Non-file input uses huh's line-based accessible mode.
		// Leave in nil for Cobra's *os.File stdin so huh selects the TUI.
		var in io.Reader
		if _, ok := c.InOrStdin().(*os.File); !ok {
			in = c.InOrStdin()
		}
		return runInit(c.Context(), in, c.OutOrStdout(), stderr, initOptions{
			nonInteractive: *nonInteractive,
			dir:            *name,
			module:         *module,
			webComponents:  *webComponents,
			generatedCode:  *generatedCode,
			ci:             *ci,
			prometheus:     *prometheus,
			git:            git,
			aiSkills:       aiSkills,
			version:        version,
			modVersion:     modVersion,
		})
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

// initOptions holds what the flags of init set. An empty webComponents,
// generatedCode or ci and a nil git or aiSkills leave the choice to its
// prompt, or to the default with nonInteractive.
type initOptions struct {
	nonInteractive bool

	// dir is the directory --name creates and init runs in.
	dir    string
	module string

	webComponents, generatedCode, ci string

	git, aiSkills *bool

	prometheus bool

	// version and modVersion are as [newInitCmd] describes them.
	version, modVersion string
}

func runInit(
	ctx context.Context, in io.Reader, out, stderr io.Writer, opts initOptions,
) error {
	if opts.webComponents != "" {
		if _, err := skeleton.ParseWebComponents(opts.webComponents); err != nil {
			return err
		}
	}
	if opts.generatedCode != "" {
		if _, err := skeleton.ParseGeneratedCode(opts.generatedCode); err != nil {
			return err
		}
	}
	if opts.ci != "" {
		if _, err := skeleton.ParseCI(opts.ci); err != nil {
			return err
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	if in != nil {
		in = oneByteReader{in}
	}

	var created bool
	moduleRoot := moduleDirOf(cwd)
	initialized := moduleRoot != "" && config.Exists(moduleRoot)
	dir := opts.dir
	if dir == "" && !initialized && !opts.nonInteractive {
		dir, err = resolveSubdir(in, out, cwd, moduleRoot)
		if err != nil {
			return err
		}
	}

	projectDir := cwd
	if dir != "" {
		projectDir = filepath.Join(cwd, dir)
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			return fmt.Errorf("creating project directory: %w", err)
		}
		// A new directory is a new project, which go mod init below
		// takes out of an enclosing module.
		moduleRoot = ""
		if fileExists(filepath.Join(projectDir, "go.mod")) {
			moduleRoot = projectDir
		}
		initialized = moduleRoot != "" && config.Exists(moduleRoot)
	} else if moduleRoot != "" && moduleRoot != cwd {
		// The enclosing module is the one gen and lint use from here.
		// go mod init below its go.mod would take the directory out of it.
		projectDir = moduleRoot
		_, _ = fmt.Fprintf(out, "Using the Go module in %s\n", moduleRoot)
	}

	// An initialized project needs no git repository. gen recommends init -n
	// to refresh the agent instructions of one that has none.
	if !initialized && findGitDir(projectDir) == "" {
		git, err := resolveGit(in, out, opts.nonInteractive, opts.git)
		if err != nil {
			return err
		}
		if git {
			if err := gitInit(projectDir); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "Initialized git repository in %s\n", projectDir)
			created = true
		}
	}

	goModPath := filepath.Join(projectDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		modulePath, err := resolveModulePath(
			in, out, projectDir, opts.nonInteractive, opts.module,
		)
		if err != nil {
			return err
		}
		if err := goModInit(projectDir, modulePath); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Initialized Go module %s\n", modulePath)
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

	// Web Components come with the app package init writes. An app of the
	// project's own has none of the files the scaffold builds on, and init
	// rewrites app.templ once the assets package exists.
	components := skeleton.WebComponentsNone
	if scan.Fallback &&
		!fileExists(filepath.Join(projectDir, "app", "app.go")) &&
		!fileExists(filepath.Join(projectDir, "app", "app.templ")) {
		components, err = resolveWebComponents(
			in, out, opts.nonInteractive, opts.webComponents,
		)
		if err != nil {
			return err
		}
		// Fail before init writes anything that a second run would not repeat.
		if components == skeleton.WebComponentsTypeScript {
			if err := requireNode(); err != nil {
				return err
			}
		}
	} else if opts.webComponents != "" {
		_, _ = fmt.Fprintln(out,
			"Ignored --web-components: init writes no app package here.")
	}

	// These choices shape the files init writes only when it sets created.
	// The condition holds when 1 of the writes below will set it.
	// Asked after them, an aborted prompt leaves a project that
	// a second run reports as initialized and never writes those files for.
	generated := skeleton.GeneratedCodeCommitted
	ci := skeleton.CIGitHubActions
	if created || !config.Exists(projectDir) ||
		(scan.Fallback && !fileExists(filepath.Join(projectDir, "app", "app.go"))) {
		generated, err = resolveGeneratedCode(
			in, out, opts.nonInteractive, opts.generatedCode,
		)
		if err != nil {
			return err
		}
		ci, err = resolveCI(in, out, opts.nonInteractive, opts.ci)
		if err != nil {
			return err
		}
	} else {
		if opts.generatedCode != "" {
			_, _ = fmt.Fprintln(out,
				"Ignored --generated-code: the project is already initialized.")
		}
		if opts.ci != "" {
			_, _ = fmt.Fprintln(out,
				"Ignored --ci: the project is already initialized.")
		}
	}

	aiSkills, err := resolveAISkills(in, out, opts.nonInteractive, opts.aiSkills)
	if err != nil {
		return err
	}

	if wrote, err := writeDefaultConfigIfMissing(
		projectDir, out, components,
	); err != nil {
		return err
	} else if wrote {
		created = true
	}

	if scan.Fallback {
		if wrote, err := writeAppGoIfMissing(projectDir, out, components); err != nil {
			return err
		} else if wrote {
			created = true
		}
	}

	// Refresh agent instructions even when init creates no project files.
	if aiSkills {
		if err := writeAgentDocs(projectDir, out, opts.version); err != nil {
			return err
		}
	}

	ignore := []string{".env"}
	if components == skeleton.WebComponentsTypeScript {
		ignore = append(ignore, "web/node_modules/")
	}
	if generated == skeleton.GeneratedCodeHidden {
		ignore = append(ignore, skeleton.GeneratedCodeIgnore...)
	}
	if err := gitignore(projectDir, ignore...); err != nil {
		return err
	}

	if !created {
		_, _ = fmt.Fprintln(out, "Project already initialized.")
		return nil
	}

	if _, err := writeEnvIfMissing(projectDir, out); err != nil {
		return err
	}

	makefile, err := skeleton.Makefile(components, generated)
	if err != nil {
		return err
	}

	for _, f := range []struct {
		rel     string
		content string
	}{
		{"compose.yaml", skeleton.ComposeYAML},
		{"Makefile", makefile},
		{".vscode/extensions.json", skeleton.VSCodeExtensions},
	} {
		if _, err := writeIfMissing(
			projectDir, f.rel, []byte(f.content), 0o644, out,
		); err != nil {
			return err
		}
	}
	if ci == skeleton.CIGitHubActions {
		// CI must install the version [pinDatapages] writes to go.mod.
		// Source builds use a pseudo-version instead of a release.
		ciWorkflow, err := skeleton.CIWorkflow(opts.modVersion, generated)
		if err != nil {
			return err
		}
		if _, err := writeIfMissing(projectDir, ".github/workflows/ci.yml",
			[]byte(ciWorkflow), 0o644, out); err != nil {
			return err
		}
	}
	if generated == skeleton.GeneratedCodeHidden {
		if _, err := writeIfMissing(projectDir, ".vscode/settings.json",
			[]byte(skeleton.VSCodeSettingsHidden), 0o644, out); err != nil {
			return err
		}
	}

	// Pin Datapages before go mod tidy can select another version.
	if err := pinDatapages(ctx, projectDir, opts.modVersion, out, stderr); err != nil {
		return err
	}

	// The app package embeds the bundle, which must exist before it compiles.
	if components == skeleton.WebComponentsTypeScript {
		if err := bundleTypeScript(projectDir, out); err != nil {
			return err
		}
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
	sc := scaffold{prometheus: opts.prometheus}
	if components == skeleton.WebComponentsRocket {
		sc.datastarJS = skeleton.RocketJS
	}
	if err := runGen(projectDir, conf, sc, stderr, ""); err != nil {
		return err
	}

	// The head refers to the components through the assets package, which exists now.
	if components != skeleton.WebComponentsNone {
		templ, err := skeleton.AppTempl(components, scan.Apps[0].GenImport+"/assets")
		if err != nil {
			return err
		}
		err = os.WriteFile(filepath.Join(projectDir, "app", "app.templ"), templ, 0o644)
		if err != nil {
			return fmt.Errorf("writing app/app.templ: %w", err)
		}
		if err := templGenerate(projectDir); err != nil {
			return err
		}
	}

	// Record dependencies imported only by generated code.
	if err := goModTidy(projectDir); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Project initialized successfully.")

	if !opts.nonInteractive {
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

// resolveSubdir asks whether to create the project in a new subdirectory
// of cwd and returns its name, or an empty string for cwd itself.
// It suggests a subdirectory for a directory with files of its own outside
// a Go module, such as a home directory, and none for a directory made for
// the project or an existing module.
func resolveSubdir(in io.Reader, out io.Writer, cwd, moduleRoot string) (string, error) {
	sub := moduleRoot == "" && hasVisibleEntries(cwd)
	if err := runField(
		huh.NewConfirm().
			Title("Create a subdirectory for the project?").
			Value(&sub),
		in, out,
	); err != nil {
		return "", err
	}
	if !sub {
		return "", nil
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
	// Accessible mode returns an empty name on EOF.
	if name == "" {
		return "", fmt.Errorf("directory name is required")
	}
	return name, nil
}

// hasVisibleEntries reports whether dir holds an entry other than dotfiles
// such as .git and .DS_Store.
func hasVisibleEntries(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(entries, func(e os.DirEntry) bool {
		return !strings.HasPrefix(e.Name(), ".")
	})
}

// resolveGit reports whether to create a git repository.
// A non-nil flag holds the inverse of --no-git. Without it, it asks,
// or creates one in non-interactive mode.
func resolveGit(
	in io.Reader, out io.Writer, nonInteractive bool, flag *bool,
) (bool, error) {
	if flag != nil {
		return *flag, nil
	}
	if nonInteractive {
		return true, nil
	}
	choice := true
	err := runField(
		huh.NewConfirm().
			Title("Not inside a git repository. Create one?").
			Value(&choice),
		in, out,
	)
	return choice, err
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

	modulePath := defaultPath
	if err := runField(
		huh.NewInput().
			Title("Module path").
			Value(&modulePath),
		in, out,
	); err != nil {
		return "", err
	}
	// An empty answer takes the suggestion. Accessible mode shows no
	// prefilled value and validates the raw answer, so a validator
	// rejecting an empty one would keep asking.
	return cmp.Or(strings.TrimSpace(modulePath), defaultPath), nil
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

// writeDefaultConfigIfMissing writes the default datapages.yaml.
// For TypeScript components it adds the watcher that bundles them.
func writeDefaultConfigIfMissing(
	projectDir string, w io.Writer, components skeleton.WebComponents,
) (bool, error) {
	for _, name := range []string{"datapages.yml", "datapages.yaml"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err == nil {
			return false, nil
		}
	}
	content := config.DefaultYAML
	if components == skeleton.WebComponentsTypeScript {
		content += skeleton.TypeScriptWatchYAML
	}
	err := os.WriteFile(
		filepath.Join(projectDir, "datapages.yaml"),
		[]byte(content),
		0o644,
	)
	if err != nil {
		return false, err
	}
	_, _ = fmt.Fprintln(w, "Created datapages.yaml")
	return true, nil
}

// resolveWebComponents returns the setup the flag names.
// Without the flag it asks, or picks none in non-interactive mode.
func resolveWebComponents(
	in io.Reader, out io.Writer, nonInteractive bool, flag string,
) (skeleton.WebComponents, error) {
	if flag != "" {
		return skeleton.ParseWebComponents(flag)
	}
	if nonInteractive {
		return skeleton.WebComponentsNone, nil
	}
	labels := map[skeleton.WebComponents]string{
		skeleton.WebComponentsRocket:     "Datastar Rocket",
		skeleton.WebComponentsTypeScript: "TypeScript, bundled by esbuild (needs Node.js)",
		skeleton.WebComponentsJavaScript: "JavaScript modules",
		skeleton.WebComponentsNone:       "None",
	}
	options := make(
		[]huh.Option[skeleton.WebComponents], len(skeleton.WebComponentsChoices),
	)
	for i, c := range skeleton.WebComponentsChoices {
		options[i] = huh.NewOption(labels[c], c)
	}
	choice := skeleton.WebComponentsNone
	err := runField(
		huh.NewSelect[skeleton.WebComponents]().
			Title("Set up Web Components?").
			Description("For JavaScript that would be impractical in "+
				"Datastar attributes, such as a map or a rich text editor.").
			Options(options...).
			Value(&choice),
		in, out,
	)
	return choice, err
}

// resolveGeneratedCode returns the choice the flag names.
// Without the flag it asks, or picks committed in non-interactive mode.
func resolveGeneratedCode(
	in io.Reader, out io.Writer, nonInteractive bool, flag string,
) (skeleton.GeneratedCode, error) {
	if flag != "" {
		return skeleton.ParseGeneratedCode(flag)
	}
	if nonInteractive {
		return skeleton.GeneratedCodeCommitted, nil
	}
	choice := skeleton.GeneratedCodeCommitted
	err := runField(
		huh.NewSelect[skeleton.GeneratedCode]().
			Title("Commit generated files?").
			Description("datapagesgen/ and *_templ.go are generated.\n").
			// huh options have no description field. The 2nd line of a label
			// serves as one. Trailing newlines separate the options.
			Options(
				huh.NewOption("Commit to Git\n"+
					"Clones build immediately. "+
					"CI checks that generated files are current.\n",
					skeleton.GeneratedCodeCommitted),
				huh.NewOption("Ignore and hide\n"+
					"Keeps generated files out of diffs. "+
					"Clones must generate before building,\n"+
					"which can fail when the app imports generated packages.",
					skeleton.GeneratedCodeHidden),
			).
			Value(&choice),
		in, out,
	)
	return choice, err
}

// resolveCI returns the CI system the flag names.
// Without the flag it asks, or picks GitHub Actions in non-interactive mode.
func resolveCI(
	in io.Reader, out io.Writer, nonInteractive bool, flag string,
) (skeleton.CI, error) {
	if flag != "" {
		return skeleton.ParseCI(flag)
	}
	if nonInteractive {
		return skeleton.CIGitHubActions, nil
	}
	choice := skeleton.CIGitHubActions
	err := runField(
		huh.NewSelect[skeleton.CI]().
			Title("Set up CI?").
			Description(
				"Runs the tests and builds the server on every push and pull request.",
			).
			Options(
				huh.NewOption("GitHub Actions", skeleton.CIGitHubActions),
				huh.NewOption("None", skeleton.CINone),
			).
			Value(&choice),
		in, out,
	)
	return choice, err
}

// resolveAISkills reports whether to write the agent instructions and skills.
// A non-nil flag holds the inverse of --no-ai-skills. Without it, it asks,
// or writes them in non-interactive mode.
func resolveAISkills(
	in io.Reader, out io.Writer, nonInteractive bool, flag *bool,
) (bool, error) {
	if flag != nil {
		return *flag, nil
	}
	if nonInteractive {
		return true, nil
	}
	choice := true
	err := runField(
		huh.NewConfirm().
			Title("Write AI agent instructions and skills?").
			Description("AGENTS.md, CLAUDE.md, GEMINI.md, Copilot and Cursor rules "+
				"and the Datapages and Datastar skills.\n"+
				"Init rewrites them on every run.").
			Value(&choice),
		in, out,
	)
	return choice, err
}

// requireNode reports a missing npm or node, which TypeScript components
// need to install esbuild and to bundle.
func requireNode() error {
	for _, name := range []string{"npm", "node"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("TypeScript Web Components need %s: install Node.js, "+
				"or pass --web-components javascript", name)
		}
	}
	return nil
}

// bundleTypeScript installs the npm packages of web/ and writes the first bundle.
func bundleTypeScript(projectDir string, w io.Writer) error {
	web := filepath.Join(projectDir, "web")
	if err := runIn(web, "npm install", "npm", "install"); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "Installed the npm packages in web")
	if err := runIn(web, "node build.mjs", "node", "build.mjs"); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "Bundled the Web Components into app/static/components.js")
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
	var components *agentdocs.Components
	if c := detectWebComponents(projectDir); c != skeleton.WebComponentsNone {
		components = &agentdocs.Components{
			Dir: skeleton.ComponentsDir(c), About: skeleton.ComponentsAbout(c),
		}
	}
	res, err := agentdocs.Write(projectDir, agentdocs.Project{
		Cmd:        cfg.Cmd,
		Cmds:       cmds,
		Apps:       apps,
		Components: components,
		Version:    version,
	}, 0o644)
	if err != nil {
		return fmt.Errorf("writing agent instructions: %w", err)
	}

	// Group skill writes into 1 line to keep init output short.
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
	for _, rel := range res.Kept {
		_, _ = fmt.Fprintf(w,
			"Left %s alone: it has no datapages stamp. "+
				"Delete it to have init write it.\n",
			rel)
	}
	return nil
}

// detectWebComponents returns the setup of the Web Components found where
// init scaffolds them, or none. A module records no choice: AGENTS.md is
// rewritten on every run, from what the module holds then.
func detectWebComponents(projectDir string) skeleton.WebComponents {
	if fileExists(filepath.Join(projectDir, "web", "src", "index.ts")) {
		return skeleton.WebComponentsTypeScript
	}
	dir := filepath.Join(projectDir, "app", "static", "components")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return skeleton.WebComponentsNone
	}
	found := skeleton.WebComponentsNone
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".js" {
			continue
		}
		found = skeleton.WebComponentsJavaScript
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && strings.Contains(string(b), "datastar-rocket") {
			return skeleton.WebComponentsRocket
		}
	}
	return found
}

func writeIfMissing(
	projectDir, rel string, content []byte, perm os.FileMode, w io.Writer,
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
	if err := os.WriteFile(path, content, perm); err != nil {
		return false, fmt.Errorf("writing %s: %w", rel, err)
	}
	_, _ = fmt.Fprintf(w, "Created %s\n", rel)
	return true, nil
}

// writeAppGoIfMissing writes the app package and the files of the Web
// Component setup. Its head leaves out the components until the generated
// assets package exists: see [skeleton.AppTempl].
func writeAppGoIfMissing(
	projectDir string, w io.Writer, components skeleton.WebComponents,
) (bool, error) {
	src, err := skeleton.AppGo(components)
	if err != nil {
		return false, err
	}
	wrote, err := writeIfMissing(
		projectDir, "app/app.go", src, 0o644, w,
	)
	if err != nil || !wrote {
		return false, err
	}
	templ, err := skeleton.AppTempl(components, "")
	if err != nil {
		return true, err
	}
	if _, err := writeIfMissing(
		projectDir, "app/app.templ", templ, 0o644, w,
	); err != nil {
		return true, err
	}
	files, err := skeleton.WebComponentFiles(components)
	if err != nil {
		return true, err
	}
	for _, f := range files {
		if _, err := writeIfMissing(
			projectDir, f.Path, f.Content, 0o644, w,
		); err != nil {
			return true, err
		}
	}
	return true, nil
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
	// Other accounts on the machine must not read the session encryption key.
	return writeIfMissing(projectDir, ".env", []byte(content), 0o600, w)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// gitignore appends the entries .gitignore lacks without changing the others.
func gitignore(projectDir string, entries ...string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}
	lines := strings.Split(string(content), "\n")
	var missing strings.Builder
	for _, e := range entries {
		if !slices.ContainsFunc(lines, func(line string) bool {
			return strings.TrimSpace(line) == e
		}) {
			missing.WriteString(e)
			missing.WriteByte('\n')
		}
	}
	if missing.Len() == 0 {
		return nil
	}
	f, err := os.OpenFile(gitignorePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	entry := missing.String()
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
