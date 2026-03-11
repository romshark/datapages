package cmd

import (
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newLintCmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Args:  cobra.NoArgs,
		Short: "Validate the application model",
		Long: `Parse the application model from the app package and report any errors
without generating code. Useful for CI checks and editor integration.

Requires a datapages.yaml config file. Run "datapages init" to create one first.`,
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
				return errNoConfig
			}

			_, err = parseApp(filepath.Join(moduleDir, config.App), config, stderr)
			return err
		},
	}
}
