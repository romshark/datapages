package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/romshark/datapages/generator"
	datapagesparser "github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/errsuggest"
	"github.com/romshark/datapages/parser/model"
	"github.com/spf13/cobra"
)

func newGenCmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "gen",
		Short: "Generate the server and helper packages",
		Long: `Parse the application model from the app package and generate:
  - Server implementation with request handling, middleware, and sessions
  - Type-safe URL helpers (href package)
  - Type-safe action helpers (action package)
  - Server entry point (cmd package, created only if missing)

If no datapages.yaml config file exists, a default one is created.

The generated package is always written, even when the app package contains
errors, so that IDEs can resolve the import while you fix the errors.
Errors are always reported to stderr and the exit code is non-zero whenever
parsing fails.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			moduleDir, err := findModuleDir()
			if err != nil {
				return err
			}
			config, found, err := loadConfig(moduleDir)
			if err != nil {
				return err
			}
			if !found {
				if err := writeDefaultConfig(moduleDir, true); err != nil {
					return err
				}
			}
			return runGen(moduleDir, config, stderr)
		},
	}
}

func runGen(moduleDir string, config config, stderr io.Writer) error {
	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}

	cmdDir := filepath.Join(moduleDir, config.Cmd)
	cmdExists, err := checkCmdPackage(cmdDir)
	if err != nil {
		return err
	}

	app, parseErr := parseApp(filepath.Join(moduleDir, config.App), stderr)

	// Always generate the package; when app is nil, stub files are written so
	// that IDEs can resolve the import while errors are fixed.
	genDir := filepath.Join(moduleDir, config.Gen.Package)
	genPkgName := filepath.Base(genDir)
	prometheus := app != nil && config.Gen.Prometheus != nil && *config.Gen.Prometheus
	if err := generator.Generate(
		genDir, genPkgName, app, 0o644, generator.Options{Prometheus: prometheus},
	); err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	if app != nil && !cmdExists {
		appImport := modulePath + "/" + config.App
		genImport := modulePath + "/" + config.Gen.Package
		hasSession := app.Session != nil
		if err := generator.GenerateCmd(
			cmdDir, appImport, genImport, genPkgName, prometheus, hasSession, 0o644,
		); err != nil {
			return fmt.Errorf("generating cmd: %w", err)
		}
	}

	return parseErr
}

func parseApp(appDir string, stderr io.Writer) (*model.App, error) {
	app, errs := datapagesparser.Parse(appDir)

	// Also check .templ files for hardcoded app-internal URLs.
	templErrs := datapagesparser.CheckTemplFiles(appDir)

	totalErrs := errs.Len() + templErrs.Len()
	if totalErrs > 0 {
		for _, err := range errs.All() {
			_, _ = fmt.Fprintln(stderr, err)
			if hint := errsuggest.Suggest(err); hint != "" {
				_, _ = fmt.Fprintln(stderr, "")
				_, _ = fmt.Fprintln(stderr, hint)
			}
		}
		for _, err := range templErrs.All() {
			_, _ = fmt.Fprintln(stderr, err)
			if hint := errsuggest.Suggest(err); hint != "" {
				_, _ = fmt.Fprintln(stderr, "")
				_, _ = fmt.Fprintln(stderr, hint)
			}
		}
		// Return the partial model alongside the error: callers may still
		// generate code from whatever was successfully parsed.
		return app, fmt.Errorf("parsing app package: %d error(s)", totalErrs)
	}
	return app, nil
}
