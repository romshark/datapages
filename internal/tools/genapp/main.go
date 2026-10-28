// Command genapp writes the datapagesgen package of an app package of the
// datapages module, as "datapages gen" does in an application module.
// "datapages gen" refuses this module because its runtime packages import
// each other and its tests call datapages.NewServer.
//
// Usage, from the module root:
//
//	go run ./internal/tools/genapp internal/cmd/toxic/ui
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/serverscan"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genapp <app package directory>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(appDir string) error {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("reading go.mod of the module root: %w", err)
	}
	modulePath := modfile.ModulePath(data)

	m, errs := parser.Parse(appDir)
	if errs.Len() > 0 {
		for _, err := range errs.All() {
			fmt.Fprintln(os.Stderr, err)
		}
		return fmt.Errorf("parsing %s: %d error(s)", appDir, errs.Len())
	}
	dir := filepath.ToSlash(filepath.Clean(appDir))
	// Prometheus stays off, matching the datapages.DisablePrometheus the
	// settings page, the only app of this module, passes to NewServer.
	return generator.Generate(
		filepath.Join(appDir, serverscan.GenSubdir), serverscan.GenSubdir, m, 0o644,
		generator.Options{
			AssetsURLPrefix: m.Assets.URLPrefix,
			AssetsDir:       m.Assets.Dir,
			AppDir:          dir,
			GenImport:       modulePath + "/" + dir + "/" + serverscan.GenSubdir,
		},
	)
}
