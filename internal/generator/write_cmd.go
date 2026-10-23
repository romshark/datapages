package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/romshark/datapages/internal/generator/skeleton"
	"github.com/romshark/datapages/internal/parser/model"
)

// GenerateCmd generates a default cmd/server main.go at dstDir.  appImportPath and
// genImportPath are the full Go import paths of the app and generated packages.
// genPkgName is the Go package name of the generated package (e.g. "datapagesgen").
// datastarJS is the URL of the Datastar bundle, empty for the default.
func GenerateCmd(
	dstDir string,
	appImportPath, genImportPath, genPkgName string,
	prometheus bool, datastarJS string, m *model.App,
	perm os.FileMode,
) error {
	appPkgName := "app"
	if m != nil && m.PkgName != "" {
		appPkgName = m.PkgName
	}

	// The session manager is generic over the session Data type, not over the
	// datapages.Session instantiation the app names. The type can be built
	// from packages main.go does not import yet, and from one whose declared
	// name main.go already binds, which is why it renders against a table.
	var sessionData string
	var extraImports []skeleton.Import
	if m != nil && m.Session != nil {
		appPkg, _ := skeleton.MainGoAppPkg(appPkgName, genPkgName)
		imports := newGenImports(
			collectSessionDataPkgs(m),
			skeleton.MainGoTaken(appPkgName, genPkgName),
			map[string]string{m.PkgPath: appPkg},
			false,
		)
		sessionData = renderTypeIn(imports.Qualifier(), m.Session.Data)
		for _, imp := range imports.Extra() {
			extraImports = append(extraImports, skeleton.Import(imp))
		}
	}
	var assetsVar string
	if m != nil {
		assetsVar = m.Assets.Var
	}
	src, err := skeleton.MainGo(skeleton.MainGoConfig{
		AppImport:    appImportPath,
		AppPkgName:   appPkgName,
		GenImport:    genImportPath,
		GenPkgName:   genPkgName,
		Prometheus:   prometheus,
		SessionData:  sessionData,
		ExtraImports: extraImports,
		AssetsVar:    assetsVar,
		DatastarJS:   datastarJS,
	})
	if err != nil {
		return fmt.Errorf("generating cmd/main.go: %w", err)
	}
	// gen leaves an existing cmd package alone,
	// which would keep a main.go cut off after its package clause.
	return writeFiles([]genFile{{filepath.Join(dstDir, "main.go"), src}}, perm)
}
