package cmd

import (
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newLintCmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Short: "Validate the application model",
		Long: `Parse the application model from the app package and report any errors
without generating code. Useful for CI checks and editor integration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
		},
	}
}
