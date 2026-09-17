package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/generator/agentdocs"
	datapagesparser "github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/parser/errsuggest"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/serverscan"
	"github.com/romshark/datapages/internal/subject"
)

func newGenCmd(stderr io.Writer, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "gen",
		Args:  cobra.NoArgs,
		Short: "Generate the server and helper packages",
		Long: `Parse the application model from the app package and generate:
  - Server implementation with request handling, middleware, and sessions
  - Type-safe URL helpers (href package)
  - Type-safe action helpers (action package)
  - Server entry point (cmd package, created only if missing)

The app package and the destination are read from the type arguments of the
datapages.NewServer call. A module without one is generated with the defaults
(./app and ./datapagesgen) and gets a cmd/server/main.go written for it.

Assets and Prometheus are read from the Config variable of the app package.

This command does not run "templ generate". Generate the app model first,
then run "templ generate" after changing .templ files. If generated Templ
references a helper that does not exist yet, remove the reference, regenerate
Templ, run "datapages gen", then restore it and regenerate Templ.

A failed run never replaces generated code that already exists. It keeps
what the last successful run produced. A package that was never generated is
written as stubs. IDEs can then resolve the import while you fix the errors.
Errors go to stderr. The exit code is non-zero whenever parsing fails.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			moduleDir, err := findModuleDir()
			if err != nil {
				return err
			}
			conf, _, err := config.Load(moduleDir)
			if err != nil {
				return err
			}
			return runGen(moduleDir, conf, false, stderr, version)
		},
	}
}

// runGen generates every app of the module. scaffoldProm asks for Prometheus
// in the main.go written for a module that holds no NewServer call yet, which
// is the only run with no call to read the option from.
func runGen(
	moduleDir string, cfg config.Config, scaffoldProm bool,
	stderr io.Writer, version string,
) error {
	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}

	if err := upgradeGoMod(moduleDir, version); err != nil {
		return err
	}

	scan, err := serverscan.Scan(moduleDir, modulePath)
	if err != nil {
		return err
	}

	// Every app is generated, even when another one failed to parse:
	// one broken model must not leave the rest of the module without code.
	var errs []error
	var events []subject.AppEvent
	for _, app := range scan.Apps {
		app.Prometheus = app.Prometheus || (scan.Fallback && scaffoldProm)
		m, err := genApp(moduleDir, cfg, scan, app, stderr)
		if err != nil {
			errs = append(errs, err)
		}
		events = append(events, appEvents(app.Dir, m)...)
	}
	if err := subject.CheckAcrossApps(events); err != nil {
		errs = append(errs, err)
	}

	// Run go mod tidy after generation so that go.sum stays in sync,
	// especially after upgradeGoMod bumps the datapages version.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = moduleDir
	if out, err := tidy.CombinedOutput(); err != nil {
		errs = append(errs, execErr("go mod tidy", err, out))
	}
	stale, err := agentdocs.SkillsDiffer(moduleDir)
	if err != nil {
		errs = append(errs, fmt.Errorf("checking agent instructions: %w", err))
	} else if stale {
		_, _ = fmt.Fprintln(stderr,
			"Agent instructions differ from this CLI; run `datapages init` to update them (edits are backed up).")
	}
	return errors.Join(errs...)
}

// appEvents is what [subject.CheckAcrossApps] takes for one application.
// A model that failed to parse contributes nothing.
func appEvents(appDir string, m *model.App) []subject.AppEvent {
	if m == nil {
		return nil
	}
	events := make([]subject.AppEvent, 0, len(m.Events))
	for _, e := range m.Events {
		events = append(events, subject.AppEvent{
			App:      appDir,
			TypeName: e.TypeName,
			Decl:     e.PkgPath + "." + e.TypeName,
			Claim: subject.Claim{
				Subject:   e.Subject,
				HasFields: len(e.SubjectFields) > 0,
			},
		})
	}
	return events
}

// genApp parses one app package and generates the code for it.
// It returns the parsed model, which is nil when the app package has no model
// to generate from, and partial when it has errors.
func genApp(
	moduleDir string, cfg config.Config,
	scan serverscan.Result, app serverscan.App, stderr io.Writer,
) (*model.App, error) {
	m, parseErr := parseApp(filepath.Join(moduleDir, app.Dir), stderr)

	genDir := filepath.Join(moduleDir, app.GenDir)
	var assets model.Assets
	if m != nil {
		assets = m.Assets
	}

	if parseErr != nil {
		// Existing generated code is left alone. The parser returns a partial
		// model for a rejected package. Code generated from it describes an
		// application the user did not write. It would replace working code
		// with code that does not build and hide the errors reported above.
		//
		// A package that was never generated is different. There is nothing
		// to lose and the app package imports it.
		// Stubs make the import resolve while the errors are fixed.
		if err := writeStubsIfAbsent(genDir, assets.URLPrefix != ""); err != nil {
			return m, err
		}
		return m, parseErr
	}

	if err := generator.Generate(
		genDir, serverscan.GenSubdir, m, 0o644, generator.Options{
			Prometheus:      app.Prometheus,
			AssetsURLPrefix: assets.URLPrefix,
			AssetsDir:       assets.Dir,
			AppDir:          app.Dir,
			GenImport:       app.GenImport,
		},
	); err != nil {
		return m, fmt.Errorf("generating code: %w", err)
	}

	// A module without a NewServer call has no entry point yet.
	if scan.Fallback {
		cmdDir := filepath.Join(moduleDir, cfg.Cmd)
		cmdExists, err := checkCmdPackage(cmdDir)
		if err != nil {
			return m, err
		}
		if !cmdExists {
			if err := generator.GenerateCmd(
				cmdDir, app.Import, app.GenImport, serverscan.GenSubdir,
				app.Prometheus, m, 0o644,
			); err != nil {
				return m, fmt.Errorf("generating cmd: %w", err)
			}
		}
	}

	// The calls are checked against what the app package was parsed to
	// declare, which is why this runs last.
	return m, serverscan.CheckSessionData(app, m.Session != nil)
}

// writeStubsIfAbsent writes package declaration stubs when
// nothing has been generated yet. It does nothing otherwise.
//
// It runs after the app package failed to parse. On a project that never generated,
// the app package imports a package that does not exist.
// The unresolved import then hides the reported errors.
// A stub holds no application code and cannot be wrong.
// On a project that generated before, the existing code is the better stub.
func writeStubsIfAbsent(genDir string, hasAssets bool) error {
	if _, err := os.Stat(filepath.Join(genDir, "app_gen.go")); err == nil {
		return nil // generated before, keep it
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", genDir, err)
	}
	if err := generator.Generate(
		genDir, serverscan.GenSubdir, nil, 0o644,
		generator.Options{AssetsURLPrefix: stubAssetsPrefix(hasAssets)},
	); err != nil {
		return fmt.Errorf("generating stubs: %w", err)
	}
	return nil
}

// stubAssetsPrefix reports the prefix that makes the stub writer include the
// assets package. Its value is not used in a stub, only its presence.
func stubAssetsPrefix(hasAssets bool) string {
	if hasAssets {
		return "/static/"
	}
	return ""
}

func parseApp(appDir string, stderr io.Writer) (*model.App, error) {
	app, errs := datapagesparser.Parse(appDir)
	if errs.Len() == 0 {
		return app, nil
	}
	loc := color.New(color.FgCyan)
	msg := color.New(color.FgRed, color.Bold)
	fix := color.New(color.FgGreen)
	fixLabel := color.New(color.FgGreen, color.Bold)
	count := color.New(color.FgRed, color.Bold)
	if wantColorFor(stderr) {
		loc.EnableColor()
		msg.EnableColor()
		fix.EnableColor()
		fixLabel.EnableColor()
		count.EnableColor()
	} else {
		loc.DisableColor()
		msg.DisableColor()
		fix.DisableColor()
		fixLabel.DisableColor()
		count.DisableColor()
	}
	for i := 0; i < errs.Len(); i++ {
		pos, innerErr := errs.Entry(i)
		if i > 0 {
			_, _ = fmt.Fprintln(stderr)
		}
		_, _ = fmt.Fprintf(
			stderr, "%s %s\n",
			loc.Sprintf("at %s:%d:%d:", pos.Filename, pos.Line, pos.Column),
			msg.Sprint(innerErr.Error()),
		)
		if hint := errsuggest.Suggest(innerErr); hint != "" {
			if label, rest, ok := strings.Cut(hint, " "); ok && label == "fix:" {
				_, _ = fmt.Fprintln(stderr, fixLabel.Sprint("fix:"), fix.Sprint(rest))
			} else {
				_, _ = fmt.Fprintln(stderr, fix.Sprint(hint))
			}
		}
	}
	_, _ = fmt.Fprintln(stderr)
	// Return the partial model alongside the error: callers may still
	// generate code from whatever was successfully parsed.
	return app, fmt.Errorf("parsing app package: %s",
		count.Sprintf("%d error(s)", errs.Len()))
}
