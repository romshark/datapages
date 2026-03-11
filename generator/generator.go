package generator

import (
	"fmt"
	"os"
	"path/filepath"

	goimports "golang.org/x/tools/imports"

	"github.com/romshark/datapages/parser/model"
)

// Options configures code generation.
type Options struct {
	// Prometheus enables generation of Prometheus metrics instrumentation.
	Prometheus bool
	// AssetsURLPrefix is the URL path prefix for serving static asset files.
	// When non-empty, the generator emits an assets subpackage with a URLPrefix
	// constant and enables WithAssets. When empty, asset serving is disabled.
	AssetsURLPrefix string
	// AssetsDir is the subdirectory within the app package containing static files
	// (e.g. "static"). Used as the embed.FS subdirectory and for dev-mode disk serving.
	AssetsDir string
	// AppDir is the path to the app source package relative to the module root
	// (e.g. "app"). Used to compute the dev-mode disk path for assets.
	AppDir string
	// GenImport is the full import path of the generated root package
	// (e.g. "github.com/example/myapp/datapagesgen").
	// Required for generating subpackage imports.
	GenImport string
}

// Generate generates the complete generated Datapages package with subpackages to
// destination directory dstDir. pkgName is the Go package name for the generated
// root package (e.g. "datapagesgen"). When m is nil, minimal stub files containing
// only the package declaration are written so that IDEs can resolve the import.
func Generate(
	dstDir string, pkgName string, m *model.App, perm os.FileMode, opts Options,
) error {
	if m == nil {
		return generateStubs(dstDir, pkgName, perm, opts.AssetsURLPrefix != "")
	}

	w := writerPool.Get().(*Writer)
	defer writerPool.Put(w)

	var err error

	assetsDir := filepath.Join(dstDir, "assets")

	// Generate assets/assets_gen.go first so goimports can resolve the import.
	// When assets are disabled, remove any previously generated assets directory.
	if opts.AssetsURLPrefix != "" {
		w.Reset()
		w.assetsURLPrefix = opts.AssetsURLPrefix
		w.assetsDir = opts.AssetsDir
		w.appDir = opts.AppDir
		w.WritePkgAssets()
		assetsGenPath := filepath.Join(assetsDir, "assets_gen.go")
		w.Buf, err = goimports.Process(assetsGenPath, w.Buf, nil)
		if err != nil {
			return fmt.Errorf("formatting assets/assets_gen.go: %w", err)
		}
		if err := os.MkdirAll(assetsDir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", assetsDir, err)
		}
		if err := os.WriteFile(assetsGenPath, w.Buf, perm); err != nil {
			return fmt.Errorf("writing assets/assets_gen.go: %w", err)
		}
	} else {
		_ = os.RemoveAll(assetsDir)
	}

	// Generate app_gen.go

	w.Reset()
	w.prometheus = opts.Prometheus
	w.assetsURLPrefix = opts.AssetsURLPrefix
	w.assetsDir = opts.AssetsDir
	w.appDir = opts.AppDir
	w.genImport = opts.GenImport
	w.WriteApp(pkgName, m)
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
func generateStubs(dstDir, pkgName string, perm os.FileMode, hasAssets bool) error {
	stubs := []struct{ dir, name, file string }{
		{dstDir, pkgName, "app_gen.go"},
		{filepath.Join(dstDir, "action"), "action", "action_gen.go"},
		{filepath.Join(dstDir, "href"), "href", "href_gen.go"},
	}
	assetsDir := filepath.Join(dstDir, "assets")
	if hasAssets {
		stubs = append(stubs,
			struct{ dir, name, file string }{assetsDir, "assets", "assets_gen.go"})
	} else {
		_ = os.RemoveAll(assetsDir)
	}
	for _, pkg := range stubs {
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
