package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/romshark/datapages/parser/model"
	goimports "golang.org/x/tools/imports"
)

// Options configures code generation.
type Options struct {
	// Prometheus enables generation of Prometheus metrics instrumentation.
	Prometheus bool
}

// Generate generates the complete generated Datapages package with subpackages to
// destination directory dstDir. pkgName is the Go package name for the generated
// root package (e.g. "datapagesgen"). When m is nil, minimal stub files containing
// only the package declaration are written so that IDEs can resolve the import.
func Generate(
	dstDir string, pkgName string, m *model.App, perm os.FileMode, opts Options,
) error {
	if m == nil {
		return generateStubs(dstDir, pkgName, perm)
	}

	w := writerPool.Get().(*Writer)
	defer writerPool.Put(w)

	// Generate app_gen.go

	w.Reset()
	w.prometheus = opts.Prometheus
	w.WriteApp(pkgName, m)
	var err error
	appGenPath := filepath.Join(dstDir, "app_gen.go")
	w.Buf, err = goimports.Process(appGenPath, w.Buf, nil)
	if err != nil {
		return fmt.Errorf("formatting app_gen.go: %w", err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dstDir, err)
	}
	if err := os.WriteFile(appGenPath, w.Buf, perm); err != nil {
		return fmt.Errorf("writing app_gen.go: %w", err)
	}

	// Generate action/action_gen.go
	w.Reset()
	w.WritePkgAction(m)
	actionDir := filepath.Join(dstDir, "action")
	actionGenPath := filepath.Join(actionDir, "action_gen.go")
	w.Buf, err = goimports.Process(actionGenPath, w.Buf, nil)
	if err != nil {
		return fmt.Errorf("formatting action/action_gen.go: %w", err)
	}
	if err := os.MkdirAll(actionDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", actionDir, err)
	}
	if err := os.WriteFile(actionGenPath, w.Buf, perm); err != nil {
		return fmt.Errorf("writing action/action_gen.go: %w", err)
	}

	// Generate href/href_gen.go
	w.Reset()
	w.WritePkgHref(m)
	hrefDir := filepath.Join(dstDir, "href")
	hrefGenPath := filepath.Join(hrefDir, "href_gen.go")
	w.Buf, err = goimports.Process(hrefGenPath, w.Buf, nil)
	if err != nil {
		return fmt.Errorf("formatting href/href_gen.go: %w", err)
	}
	if err := os.MkdirAll(hrefDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", hrefDir, err)
	}
	if err := os.WriteFile(hrefGenPath, w.Buf, perm); err != nil {
		return fmt.Errorf("writing href/href_gen.go: %w", err)
	}

	return nil
}

// generateStubs writes minimal package declaration files for each generated
// package so that IDEs can resolve the import even when the app model is nil.
func generateStubs(dstDir, pkgName string, perm os.FileMode) error {
	for _, pkg := range []struct{ dir, name, file string }{
		{dstDir, pkgName, "app_gen.go"},
		{filepath.Join(dstDir, "action"), "action", "action_gen.go"},
		{filepath.Join(dstDir, "href"), "href", "href_gen.go"},
	} {
		if err := os.MkdirAll(pkg.dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", pkg.dir, err)
		}
		content := []byte("package " + pkg.name + "\n")
		p := filepath.Join(pkg.dir, pkg.file)
		if err := os.WriteFile(p, content, perm); err != nil {
			return fmt.Errorf("writing %s: %w", pkg.file, err)
		}
	}
	return nil
}
