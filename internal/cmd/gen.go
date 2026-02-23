package cmd

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/romshark/datapages/generator"
	datapagesparser "github.com/romshark/datapages/parser"
	"github.com/romshark/datapages/parser/model"
)

func runGen(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	_ = fs.Parse(args)

	moduleDir, err := findModuleDir()
	if err != nil {
		return err
	}
	config, found, err := loadConfig(moduleDir)
	if err != nil {
		return err
	}
	if !found {
		if err := writeDefaultConfig(moduleDir); err != nil {
			return err
		}
	}

	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}

	cmdDir := filepath.Join(moduleDir, config.Cmd)
	cmdExists, err := checkCmdPackage(cmdDir)
	if err != nil {
		return err
	}

	app, err := parseApp(filepath.Join(moduleDir, config.App), stderr)
	if err != nil {
		return err
	}
	genDir := filepath.Join(moduleDir, config.Gen)
	genPkgName := filepath.Base(genDir)
	if err := generator.Generate(genDir, genPkgName, app, 0o644); err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	if !cmdExists {
		appImport := modulePath + "/" + config.App
		genImport := modulePath + "/" + config.Gen
		if err := generator.GenerateCmd(
			cmdDir, appImport, genImport, genPkgName, 0o644,
		); err != nil {
			return fmt.Errorf("generating cmd: %w", err)
		}
	}
	return nil
}

func parseApp(appDir string, stderr io.Writer) (*model.App, error) {
	app, errs := datapagesparser.Parse(appDir)
	if errs.Len() > 0 {
		for _, err := range errs.All() {
			_, _ = fmt.Fprintln(stderr, err)
		}
		return nil, fmt.Errorf("parsing app package: %d error(s)", errs.Len())
	}
	return app, nil
}
