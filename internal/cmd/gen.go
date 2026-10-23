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
	"github.com/romshark/datapages/internal/parser"
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

The app package is read from the type arguments of the datapages.NewServer
call, and the code is generated into datapagesgen directly below it.
A module without a call is generated for ./app into ./app/datapagesgen and
gets a cmd/server/main.go written for it.

Prometheus is read from the Metrics type argument of the same call.
Assets are read from the embed.FS variable of the app package whose
doc comment gives the URL path they are served at.

This command does not run "templ generate". Generate the app model first,
then run "templ generate" after changing .templ files. If generated Templ
references a helper that does not exist yet, remove the reference,
regenerate Templ, run "datapages gen", then restore it and regenerate Templ.

Everything under datapagesgen/ is rewritten on each run. An app with errors
keeps the code an earlier run generated for it. The other apps are generated.
A package that was never generated is written as stubs. IDEs can then resolve
the import while you fix the errors.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			moduleDir, err := findModuleDir()
			if err != nil {
				return err
			}
			conf, _, err := config.Load(moduleDir)
			if err != nil {
				return err
			}
			return runGen(moduleDir, conf, scaffold{prometheus: scaffoldPrometheus}, stderr, version)
		},
	}
}

// scaffoldPrometheus decides the Metrics type argument of the
// cmd/server/main.go written for a module without a NewServer call.
// gen, watch and init write that call, init's --prometheus overrides it,
// and every later run reads the argument back out of the call.
const scaffoldPrometheus = true

// scaffold is what the main.go written for a module that holds no NewServer
// call yet is set up with. That run is the only one with no call to read the
// options from.
type scaffold struct {
	prometheus bool

	// datastarJS is the URL of the Datastar bundle, empty for the default.
	datastarJS string
}

// runGen generates every app of the module.
func runGen(
	moduleDir string, cfg config.Config, sc scaffold,
	stderr io.Writer, version string,
) error {
	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}

	// An older CLI may misread a newer module,
	// which makes its version the first thing to check.
	if err := checkGoModVersion(moduleDir, version); err != nil {
		return err
	}

	scan, err := serverscan.Scan(moduleDir, modulePath)
	if err != nil {
		return err
	}

	// A missing app package leaves nothing to generate.
	// Checking it before the go.mod upgrade keeps the run from writing anything.
	var missing []error
	for _, app := range scan.Apps {
		if err := serverscan.CheckAppDir(moduleDir, app); err != nil {
			missing = append(missing, err)
		}
	}
	if len(missing) > 0 {
		return errors.Join(missing...)
	}

	if err := upgradeGoMod(moduleDir, version); err != nil {
		return err
	}

	// Every app is parsed and checked before any is generated, which keeps a
	// failed check from replacing its code. The apps that pass are generated
	// even when another one failed. 1 broken model must not leave the rest of
	// the module without code.
	apps := make([]checkedApp, len(scan.Apps))
	var events []subject.AppEvent
	for i, app := range scan.Apps {
		app.Prometheus = app.Prometheus || (scan.Fallback && sc.prometheus)
		m, err := parseApp(filepath.Join(moduleDir, app.Dir), stderr)
		if err == nil {
			err = serverscan.CheckSessionData(app, m.Session != nil)
		}
		apps[i] = checkedApp{app: app, model: m, err: err}
		events = append(events, appEvents(app.Dir, m)...)
	}
	conflicts := subject.Conflicts(events)
	inConflict := make(map[string]bool, 2*len(conflicts))
	for _, c := range conflicts {
		inConflict[c.First.App], inConflict[c.Event.App] = true, true
	}

	var errs []error
	for _, a := range apps {
		if a.err != nil {
			errs = append(errs, a.err)
		}
		failed := a.err != nil || inConflict[a.app.Dir]
		if err := genApp(
			moduleDir, cfg, scan.Fallback, a, failed, sc.datastarJS,
		); err != nil {
			errs = append(errs, err)
		}
	}
	for _, c := range conflicts {
		errs = append(errs, c)
	}

	// Run go mod tidy after generation so that go.sum stays in sync,
	// especially after upgradeGoMod bumps the datapages version.
	// It reads the imports of the generated code, which is why its failure
	// cannot keep that code from being written.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = moduleDir
	if out, err := tidy.CombinedOutput(); err != nil {
		errs = append(errs, execErr("go mod tidy", err, out))
	}
	stale, err := agentdocs.SkillsDiffer(moduleDir, version)
	if err != nil {
		errs = append(errs, fmt.Errorf("checking agent instructions: %w", err))
	} else if stale {
		_, _ = fmt.Fprintln(stderr,
			"Agent instructions differ from this CLI; run `datapages init -n` to update them (edits are backed up).")
	}
	return errors.Join(errs...)
}

// appEvents is what [subject.CheckAcrossApps] takes for 1 application.
// A partial model, which the parser returns with its errors, contributes the
// events it holds. A nil model contributes nothing.
func appEvents(appDir string, m *model.App) []subject.AppEvent {
	if m == nil {
		return nil
	}
	events := make([]subject.AppEvent, 0, len(m.Events))
	for _, e := range m.Events {
		events = append(events, subject.AppEvent{
			App:       appDir,
			TypeName:  e.TypeName,
			Decl:      e.PkgPath + "." + e.TypeName,
			Subject:   e.Subject,
			HasFields: len(e.SubjectFields) > 0,
		})
	}
	return events
}

// checkedApp is 1 app package of the module, parsed and checked.
type checkedApp struct {
	app serverscan.App

	// model is nil when the app package has no model to generate from,
	// and partial when err holds its parse errors.
	model *model.App

	// err holds the parse errors, or the NewServer call naming the wrong
	// session type.
	err error
}

// genApp writes the code of 1 app, or only the stubs it lacks when it failed.
// datastarJS goes into the main.go written for a module that has none.
func genApp(
	moduleDir string, cfg config.Config, fallback bool,
	a checkedApp, failed bool, datastarJS string,
) error {
	genDir := filepath.Join(moduleDir, a.app.GenDir)
	var assets model.Assets
	if a.model != nil {
		assets = a.model.Assets
	}

	if failed {
		// Existing generated code is left alone. The parser returns a partial
		// model for a rejected package, which describes an application the user
		// didn't write. Code generated from it, or for a NewServer call naming
		// the wrong session type, would not build, and its compiler errors would
		// hide the errors gen reports. Code generated for 2 apps claiming
		// 1 subject builds, and each app receives the events of the other.
		//
		// A package that was never generated is different. There is nothing
		// to lose and the app package imports it.
		// Stubs make the import resolve while the errors are fixed.
		return writeStubsIfAbsent(genDir, assets.URLPrefix != "")
	}

	if err := generator.Generate(
		genDir, serverscan.GenSubdir, a.model, 0o644, generator.Options{
			Prometheus:      a.app.Prometheus,
			AssetsURLPrefix: assets.URLPrefix,
			AssetsDir:       assets.Dir,
			AppDir:          a.app.Dir,
			GenImport:       a.app.GenImport,
		},
	); err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	// A module without a NewServer call has no entry point yet.
	if fallback {
		cmdDir := filepath.Join(moduleDir, cfg.Cmd)
		cmdExists, err := checkCmdPackage(cmdDir)
		if err != nil {
			return err
		}
		if !cmdExists {
			if err := generator.GenerateCmd(
				cmdDir, a.app.Import, a.app.GenImport, serverscan.GenSubdir,
				a.app.Prometheus, datastarJS, a.model, 0o644,
			); err != nil {
				return fmt.Errorf("generating cmd: %w", err)
			}
		}
	}
	return nil
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
	app, errs := parser.Parse(appDir)
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
