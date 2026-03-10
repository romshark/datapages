package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	toolGofumpt     = "mvdan.cc/gofumpt@latest"
	toolGCI         = "github.com/daixiang0/gci@latest"
	toolGolangCI    = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
	toolGovulncheck = "golang.org/x/vuln/cmd/govulncheck@latest"
	toolTempl       = "github.com/a-h/templ/cmd/templ@v0.3.1001"
)

// Test runs lint then go test with coverage.
func Test() error {
	if err := Lint(); err != nil {
		return err
	}
	return run("go", "test", "./...", "-cover")
}

// Fmt formats Go source files with gofumpt and gci.
func Fmt() error {
	if err := goRun(toolGofumpt, "-w", "."); err != nil {
		return err
	}
	return goRun(toolGCI, "write",
		"--skip-generated",
		"-s", "standard",
		"-s", "default",
		"-s", "prefix(github.com/romshark/datapages)", ".")
}

// Lint runs formatting checks, module tidiness checks,
// datapages lint, and golangci-lint.
func Lint() error {
	if err := CheckFmt(); err != nil {
		return err
	}
	if err := CheckMod(); err != nil {
		return err
	}
	if err := LintDatapages(); err != nil {
		return err
	}
	if err := goRun(toolGolangCI, "run", "./..."); err != nil {
		return err
	}
	return forEachModule("example", func(dir string) error {
		fmt.Println("==> golangci-lint in", dir)
		return runIn(dir, "go", "run", toolGolangCI, "run", "./...")
	})
}

// CheckFmt verifies that all Go files are properly formatted.
func CheckFmt() error {
	out, err := output("go", "run", toolGofumpt, "-l", ".")
	if err != nil {
		return err
	}
	gciOut, err := output("go", "run", toolGCI, "list",
		"--skip-generated",
		"-s", "standard",
		"-s", "default",
		"-s", "prefix(github.com/romshark/datapages)", ".")
	if err != nil {
		return err
	}
	out += gciOut
	if out != "" {
		return fmt.Errorf("files not formatted (run mage fmt):\n%s", out)
	}
	return nil
}

// CheckMod verifies all go.mod/go.sum files in the repo are tidy.
func CheckMod() error {
	return forEachModule(".", func(dir string) error {
		modPath := filepath.Join(dir, "go.mod")
		sumPath := filepath.Join(dir, "go.sum")

		modOrig, err := os.ReadFile(modPath)
		if err != nil {
			return err
		}
		sumOrig, _ := os.ReadFile(sumPath)

		if err := runIn(dir, "go", "mod", "tidy"); err != nil {
			return err
		}

		modAfter, err := os.ReadFile(modPath)
		if err != nil {
			return err
		}
		sumAfter, _ := os.ReadFile(sumPath)

		if !bytes.Equal(modOrig, modAfter) || !bytes.Equal(sumOrig, sumAfter) {
			// Restore originals.
			_ = os.WriteFile(modPath, modOrig, 0o644)
			_ = os.WriteFile(sumPath, sumOrig, 0o644)
			return fmt.Errorf("go.mod not tidy in %s", dir)
		}
		return nil
	})
}

// LintDatapages builds the datapages CLI from source
// and runs "datapages lint" on each example.
func LintDatapages() error {
	tmp, err := os.MkdirTemp("", "datapages-lint-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "datapages")
	if err := run("go", "build", "-o", bin, "."); err != nil {
		return err
	}
	return forEachModule("example", func(dir string) error {
		fmt.Println("==> datapages lint in", dir)
		return runIn(dir, bin, "lint")
	})
}

// Vulncheck runs govulncheck on the root module and all examples.
func Vulncheck() error {
	if err := goRun(toolGovulncheck, "./..."); err != nil {
		return err
	}
	return forEachModule("example", func(dir string) error {
		fmt.Println("==> govulncheck in", dir)
		return runIn(dir, "go", "run", toolGovulncheck, "./...")
	})
}

// ModUpdate updates dependencies for all modules.
func ModUpdate() error {
	if err := run("go", "get", "-u", "-t", "./..."); err != nil {
		return err
	}
	for _, root := range []string{"example", "parser/testdata"} {
		if err := forEachModule(root, func(dir string) error {
			fmt.Println("==> go get -u -t ./... in", dir)
			return runIn(dir, "go", "get", "-u", "-t", "./...")
		}); err != nil {
			return err
		}
	}
	return nil
}

// ModTidy tidies all modules in the repo.
func ModTidy() error {
	if err := run("go", "mod", "tidy"); err != nil {
		return err
	}
	for _, root := range []string{"example", "parser/testdata"} {
		if err := forEachModule(root, func(dir string) error {
			fmt.Println("==> go mod tidy in", dir)
			return runIn(dir, "go", "mod", "tidy")
		}); err != nil {
			return err
		}
	}
	return nil
}

// GenTempl generates templ templates for examples and parser testdata.
func GenTempl() error {
	for _, root := range []string{"example", "parser/testdata"} {
		if err := forEachModule(root, func(dir string) error {
			fmt.Println("==> templ generate in", dir)
			return runIn(dir, "go", "run", toolTempl, "generate")
		}); err != nil {
			return err
		}
	}
	return nil
}

// GenDocs generates documentation pages.
func GenDocs() error {
	version, err := output("git", "describe", "--tags", "--abbrev=0")
	if err != nil {
		version = "latest"
	}
	version = strings.TrimSpace(version)
	if err := goRun(toolTempl, "generate", "-path", "./docs-src"); err != nil {
		return err
	}
	return run("go", "run", "./scripts/render-pages", "-version", version)
}

// All runs test, vulncheck, fmt, mod-tidy, gen-templ, and gen-docs.
func All() error {
	if err := Test(); err != nil {
		return err
	}
	if err := Vulncheck(); err != nil {
		return err
	}
	if err := Fmt(); err != nil {
		return err
	}
	if err := ModTidy(); err != nil {
		return err
	}
	if err := GenTempl(); err != nil {
		return err
	}
	return GenDocs()
}
