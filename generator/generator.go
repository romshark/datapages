package generator

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"

	"github.com/romshark/datapages/parser/model"
)

// Generate generates the complete generated Datapages package with subpackages to
// destination directory dstDir. pkgName is the Go package name for the generated
// root package (e.g. "datapagesgen").
func Generate(dstDir string, pkgName string, m *model.App, perm os.FileMode) error {
	w := writerPool.Get().(*Writer)
	defer writerPool.Put(w)

	// Generate app_gen.go

	w.Reset()
	w.WriteApp(pkgName, m)
	var err error
	w.Buf, err = format.Source(w.Buf)
	if err != nil {
		return fmt.Errorf("formatting app_gen.go: %w", err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dstDir, err)
	}
	if err := os.WriteFile(
		filepath.Join(dstDir, "app_gen.go"), w.Buf, perm,
	); err != nil {
		return fmt.Errorf("writing app_gen.go: %w", err)
	}

	// Generate action/action_gen.go
	w.Reset()
	w.WritePkgAction(m)
	w.Buf, err = format.Source(w.Buf)
	if err != nil {
		return fmt.Errorf("formatting action/action_gen.go: %w", err)
	}
	actionDir := filepath.Join(dstDir, "action")
	if err := os.MkdirAll(actionDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", actionDir, err)
	}
	if err := os.WriteFile(
		filepath.Join(actionDir, "action_gen.go"), w.Buf, perm,
	); err != nil {
		return fmt.Errorf("writing action/action_gen.go: %w", err)
	}

	// Generate href/href_gen.go
	w.Reset()
	w.WritePkgHref(m)
	w.Buf, err = format.Source(w.Buf)
	if err != nil {
		return fmt.Errorf("formatting href/href_gen.go: %w", err)
	}
	hrefDir := filepath.Join(dstDir, "href")
	if err := os.MkdirAll(hrefDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", hrefDir, err)
	}
	if err := os.WriteFile(
		filepath.Join(hrefDir, "href_gen.go"), w.Buf, perm,
	); err != nil {
		return fmt.Errorf("writing href/href_gen.go: %w", err)
	}

	return nil
}
