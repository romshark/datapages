package cmd

import (
	"errors"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/cmd/toxic"
	"github.com/romshark/datapages/internal/serverscan"
	"github.com/romshark/datapages/internal/subject"
)

func newLintCmd(stderr io.Writer, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Args:  cobra.NoArgs,
		Short: "Validate the application model",
		Long: `Parse the application model from the app package and report any errors
without generating code. Useful for CI checks and editor integration.

The app package is read from the type arguments of the datapages.NewServer
call, defaulting to ./app when the module holds none.

Unless watch.toxiproxy.disabled is set in datapages.yaml, lint also reports
an endpoint override in watch.toxiproxy that names no route of any app.`,
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
			conf, _, err := config.Load(moduleDir)
			if err != nil {
				return err
			}
			// Watch mode runs the proxy without the section too.
			tp := &config.WatchToxiproxy{}
			if conf.Watch != nil && conf.Watch.Toxiproxy != nil {
				tp = conf.Watch.Toxiproxy
			}
			if tp.Disabled {
				tp = nil
			}

			// Every app is linted, even when an earlier one failed:
			// 1 report per run beats 1 run per app.
			var errs []error
			var events []subject.AppEvent
			var endpoints []toxic.Endpoint
			allParsed := true
			for _, a := range scan.Apps {
				if err := serverscan.CheckAppDir(moduleDir, a); err != nil {
					errs = append(errs, err)
					allParsed = false
					continue
				}
				m, err := parseApp(filepath.Join(moduleDir, a.Dir), stderr)
				events = append(events, appEvents(a.Dir, m)...)
				if tp != nil {
					// The endpoints of every app, 1 of which watch mode runs.
					endpoints = append(endpoints, toxic.Endpoints(m)...)
				}
				if err != nil {
					errs = append(errs, err)
					allParsed = false
					continue
				}
				if err := serverscan.CheckSessionData(
					a, m.Session != nil,
				); err != nil {
					errs = append(errs, err)
				}
			}
			if err := subject.CheckAcrossApps(events); err != nil {
				errs = append(errs, err)
			}
			// The partial model of an app with errors lacks routes,
			// which would make overrides of them read as unknown.
			if tp != nil && allParsed {
				err := checkEndpointKeys(tp, endpoints)
				if err != nil {
					errs = append(errs, err)
				}
			}
			return errors.Join(errs...)
		},
	}
}
