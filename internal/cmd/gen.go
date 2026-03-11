package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/internal/cmd/config"
	datapagesparser "github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/errsuggest"
	"github.com/romshark/datapages/parser/model"
)

func newGenCmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "gen",
		Args:  cobra.NoArgs,
		Short: "Generate the server and helper packages",
		Long: `Parse the application model from the app package and generate:
  - Server implementation with request handling, middleware, and sessions
  - Type-safe URL helpers (href package)
  - Type-safe action helpers (action package)
  - Server entry point (cmd package, created only if missing)

Requires a datapages.yaml config file. Run "datapages init" to create one.

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
			conf, found, err := config.Load(moduleDir)
			if err != nil {
				return err
			}
			if !found {
				return config.ErrNoConfig
			}
			return runGen(moduleDir, conf, stderr)
		},
	}
}

func runGen(moduleDir string, cfg config.Config, stderr io.Writer) error {
	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}

	cmdDir := filepath.Join(moduleDir, cfg.Cmd)
	cmdExists, err := checkCmdPackage(cmdDir)
	if err != nil {
		return err
	}

	app, parseErr := parseApp(filepath.Join(moduleDir, cfg.App), stderr)

	// Always generate the package; when app is nil, stub files are written so
	// that IDEs can resolve the import while errors are fixed.
	genDir := filepath.Join(moduleDir, cfg.Gen.Package)
	genPkgName := filepath.Base(genDir)
	genImport := modulePath + "/" + cfg.Gen.Package
	prometheus := app != nil && cfg.Gen.Prometheus != nil && *cfg.Gen.Prometheus
	var assetsURLPrefix, assetsDir string
	if cfg.Assets != nil {
		assetsURLPrefix = cfg.Assets.URLPrefix
		// Derive embed.FS subdirectory from the on-disk path by stripping
		// the app package prefix: "./app/static" → "static".
		cleaned := filepath.Clean(cfg.Assets.Dir)
		assetsDir = strings.TrimPrefix(cleaned, cfg.App+string(filepath.Separator))
	}
	if err := generator.Generate(
		genDir, genPkgName, app, 0o644, generator.Options{
			Prometheus:      prometheus,
			AssetsURLPrefix: assetsURLPrefix,
			AssetsDir:       assetsDir,
			AppDir:          cfg.App,
			GenImport:       genImport,
		},
	); err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	if app != nil && !cmdExists {
		appImport := modulePath + "/" + cfg.App
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
	if errs.Len() > 0 {
		for _, err := range errs.All() {
			_, _ = fmt.Fprintln(stderr, err)
			if hint := errsuggest.Suggest(err); hint != "" {
				_, _ = fmt.Fprintln(stderr, "")
				_, _ = fmt.Fprintln(stderr, hint)
			}
		}
		// Return the partial model alongside the error: callers may still
		// generate code from whatever was successfully parsed.
		return app, fmt.Errorf("parsing app package: %d error(s)", errs.Len())
	}
	return app, nil
}
