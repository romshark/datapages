package cmd

import (
	"errors"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/romshark/datapages/internal/serverscan"
)

func newLintCmd(stderr io.Writer, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Args:  cobra.NoArgs,
		Short: "Validate the application model",
		Long: `Parse the application model from the app package and report any errors
without generating code. Useful for CI checks and editor integration.

The app package is read from the type arguments of the datapages.NewServer
call, defaulting to ./app when the module holds none.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			moduleDir, err := findModuleDir()
			if err != nil {
				return err
			}
			modulePath, err := readModulePath(moduleDir)
			if err != nil {
				return err
			}
			scan, err := serverscan.Scan(moduleDir, modulePath)
			if err != nil {
				return err
			}

			if err := checkGoModVersion(moduleDir, version); err != nil {
				return err
			}

			// Every app is linted, even when an earlier one failed:
			// one report per run beats one run per app.
			var errs []error
			for _, a := range scan.Apps {
				m, err := parseApp(filepath.Join(moduleDir, a.Dir), stderr)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				if err := serverscan.CheckSessionData(
					a, m.Session != nil,
				); err != nil {
					errs = append(errs, err)
				}
			}
			return errors.Join(errs...)
		},
	}
}
