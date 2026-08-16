package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/romshark/datapages/internal/generator/skeleton"
	"github.com/romshark/datapages/internal/parser/model"
)

// GenerateCmd generates a default cmd/server main.go at dstDir.
// appImportPath and genImportPath are the full Go import paths of the app
// and generated packages. genPkgName is the Go package name of the generated
// package (e.g. "datapagesgen").
func GenerateCmd(
	dstDir string,
	appImportPath, genImportPath, genPkgName string,
	prometheus bool, m *model.App,
	perm os.FileMode,
) error {
	// The session manager is generic over the session Data type,
	// not over the datapages.Session instantiation the app names.
	var sessionData string
	if m != nil && m.Session != nil {
		sessionData = renderType(m.Session.Data)
	}
	src, err := skeleton.MainGo(
		appImportPath, genImportPath, genPkgName, prometheus, sessionData,
	)
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
