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
	datapagesparser "github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/parser/errsuggest"
	"github.com/romshark/datapages/internal/parser/model"
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

Requires a datapages.yaml config file. Run "datapages init" to create one.

This command does not run "templ generate". You must run it yourself
before "datapages gen" if you have created or modified .templ files.

A failed run never replaces generated code that already exists. It keeps
what the last successful run produced. A package that was never generated is
written as stubs. IDEs can then resolve the import while you fix the errors.
Errors go to stderr. The exit code is non-zero whenever parsing fails.`,
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
			return runGen(moduleDir, conf, stderr, version)
		},
	}
}

func runGen(moduleDir string, cfg config.Config, stderr io.Writer, version string) error {
	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}

	if err := upgradeGoMod(moduleDir, version); err != nil {
		return err
	}

	cmdDir := filepath.Join(moduleDir, cfg.Cmd)
	cmdExists, err := checkCmdPackage(cmdDir)
	if err != nil {
		return err
	}

	genDir := filepath.Join(moduleDir, cfg.Gen.Package)
	genPkgName := filepath.Base(genDir)
	genImport := modulePath + "/" + cfg.Gen.Package
	prometheus := cfg.Gen.Prometheus != nil && *cfg.Gen.Prometheus
	var assetsURLPrefix, assetsDir string
	if cfg.Assets != nil {
		assetsURLPrefix = cfg.Assets.URLPrefix
		// Derive embed.FS subdirectory from the on-disk path by stripping
		// the app package prefix: "./app/static" -> "static".
		cleaned := filepath.Clean(cfg.Assets.Dir)
		assetsDir = strings.TrimPrefix(cleaned, cfg.App+string(filepath.Separator))
	}

	app, parseErr := parseApp(filepath.Join(moduleDir, cfg.App), stderr)
	if parseErr != nil {
		// Existing generated code is left alone. The parser returns a partial
		// model for a rejected package. Code generated from it describes an
		// application the user did not write. It would replace working code
		// with code that does not build and hide the errors reported above.
		//
		// A package that was never generated is different. There is nothing
		// to lose and the app package imports it.
		// Stubs make the import resolve while the errors are fixed.
		if err := writeStubsIfAbsent(
			genDir, genPkgName, assetsURLPrefix != "",
		); err != nil {
			return err
		}
		return parseErr
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

	if !cmdExists {
		appImport := modulePath + "/" + cfg.App
		if err := generator.GenerateCmd(
			cmdDir, appImport, genImport, genPkgName, prometheus, app, 0o644,
		); err != nil {
			return fmt.Errorf("generating cmd: %w", err)
		}
	}

	// Run go mod tidy after generation so that go.sum stays in sync,
	// especially after upgradeGoMod bumps the datapages version.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = moduleDir
	if out, err := tidy.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %s", out)
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
func writeStubsIfAbsent(genDir, genPkgName string, hasAssets bool) error {
	if _, err := os.Stat(filepath.Join(genDir, "app_gen.go")); err == nil {
		return nil // generated before, keep it
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", genDir, err)
	}
	if err := generator.Generate(
		genDir, genPkgName, nil, 0o644,
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
	// The partial model is returned for callers that only inspect it.
	// Generating from it is not safe. It describes an application the user did not write.
	return app, fmt.Errorf("parsing app package: %s",
		count.Sprintf("%d error(s)", errs.Len()))
}
