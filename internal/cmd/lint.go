package cmd

import (
	"flag"
	"io"
	"path/filepath"
)

func runLint(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	_ = fs.Parse(args)

	moduleDir, err := findModuleDir()
	if err != nil {
		return err
	}
	config, _, err := loadConfig(moduleDir)
	if err != nil {
		return err
	}

	_, err = parseApp(filepath.Join(moduleDir, config.App), stderr)
	return err
}
