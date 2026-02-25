package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/romshark/datapages/generator/skeleton"
)

// GenerateCmd generates a default cmd/server main.go at dstDir.
// appImportPath and genImportPath are the full Go import paths of the app
// and generated packages. genPkgName is the Go package name of the generated
// package (e.g. "datapagesgen").
func GenerateCmd(
	dstDir string,
	appImportPath, genImportPath, genPkgName string,
	prometheus bool,
	perm os.FileMode,
) error {
	src, err := skeleton.MainGo(appImportPath, genImportPath, genPkgName, prometheus)
	if err != nil {
		return fmt.Errorf("generating cmd/main.go: %w", err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dstDir, err)
	}
	if err := os.WriteFile(
		filepath.Join(dstDir, "main.go"), src, perm,
	); err != nil {
		return fmt.Errorf("writing cmd/main.go: %w", err)
	}
	return nil
}
