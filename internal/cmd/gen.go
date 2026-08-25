package cmd

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/generator"
	datapagesparser "github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/parser/errsuggest"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/serverscan"
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

This command does not run "templ generate". You must run it yourself
before "datapages gen" if you have created or modified .templ files.

The generated package is always written, even when the app package contains
errors, so that IDEs can resolve the import while you fix the errors.
Errors are always reported to stderr and the exit code is non-zero whenever
parsing fails.`,
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
	for _, app := range scan.Apps {
		app.Prometheus = app.Prometheus || (scan.Fallback && scaffoldProm)
		if err := genApp(moduleDir, cfg, scan, app, stderr); err != nil {
			errs = append(errs, err)
		}
	}

	// Run go mod tidy after generation so that go.sum stays in sync,
	// especially after upgradeGoMod bumps the datapages version.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = moduleDir
	if out, err := tidy.CombinedOutput(); err != nil {
		errs = append(errs, fmt.Errorf("go mod tidy: %s", out))
	}
	return errors.Join(errs...)
}

// genApp parses one app package and generates the code for it.
func genApp(
	moduleDir string, cfg config.Config,
	scan serverscan.Result, app serverscan.App, stderr io.Writer,
) error {
	m, parseErr := parseApp(filepath.Join(moduleDir, app.Dir), stderr)

	// Always generate the package; when m is nil, stub files are written so
	// that IDEs can resolve the import while errors are fixed.
	genDir := filepath.Join(moduleDir, app.GenDir)
	var assets model.Assets
	if m != nil {
		assets = m.Assets
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
		return fmt.Errorf("generating code: %w", err)
	}

	// A module without a NewServer call has no entry point yet.
	if m != nil && scan.Fallback {
		cmdDir := filepath.Join(moduleDir, cfg.Cmd)
		cmdExists, err := checkCmdPackage(cmdDir)
		if err != nil {
			return err
		}
		if !cmdExists {
			if err := generator.GenerateCmd(
				cmdDir, app.Import, app.GenImport, serverscan.GenSubdir,
				app.Prometheus, m, 0o644,
			); err != nil {
				return fmt.Errorf("generating cmd: %w", err)
			}
		}
	}

	if parseErr != nil {
		return parseErr
	}
	// The calls are checked against what the app package was parsed to
	// declare, which is why this runs last.
	return serverscan.CheckSessionData(app, m.Session != nil)
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
