// Build targets for this repository.
package main

// Mage generates the main this package lacks: every exported function is a
// target, and "mage -l" lists them with the first line of their doc comment.
// A plain program would need that main, argument parsing and a table of
// targets, with every new target written once and registered once.
//
// Nothing here imports mage. The targets are plain Go on the standard library.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	toolGofumpt     = "mvdan.cc/gofumpt@latest"
	toolGCI         = "github.com/daixiang0/gci@latest"
	toolGolangCI    = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
	toolGovulncheck = "golang.org/x/vuln/cmd/govulncheck@latest"
	toolTempl       = "github.com/a-h/templ/cmd/templ@v0.3.1020"
	toolMinify      = "github.com/tdewolff/minify/v2/cmd/minify@latest"
)

// submoduleRoots lists directories containing sub-modules (their own go.mod)
// that mage commands need to operate on.
var submoduleRoots = []string{
	"example",
	acceptanceRoot,
	"internal/templatingbench",
	"internal/parser/testdata",
	"internal/parser/internal/templcheck/testdata",
}

// Build verifies the datapages CLI and all example binaries compile.
func Build() error {
	if err := BuildCLI(); err != nil {
		return err
	}
	return BuildExamples()
}

// BuildCLI verifies the datapages CLI compiles.
func BuildCLI() error {
	fmt.Println("==> go build .")
	return run("go", "build", "-o", os.DevNull, "./cmd/datapages")
}

// BuildExamples verifies all example binaries compile.
func BuildExamples() error {
	return forEachModule("example", func(dir string) error {
		fmt.Println("==> go build ./... in", dir)
		return runIn(dir, "go", "build", "./...")
	})
}

// Test runs lint then go test with coverage.
func Test() error {
	if err := Lint(); err != nil {
		return err
	}
	return run("go", "test", "./...", "-cover")
}

// Coverage reports two numbers. The first is how much of the generator itself
// the test suite runs. The second is how much of the code the generator writes
// the acceptance cases run.
//
// The second number is collected inside the case modules under
// internal/acceptance, where the generated packages live.
func Coverage() error {
	dir, err := os.MkdirTemp("", "datapages-coverage")
	if err != nil {
		return fmt.Errorf("creating a directory for the profiles: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	generatorProfile := filepath.Join(dir, "generator.out")
	generatedProfile := filepath.Join(dir, "generated.out")

	if err := run(
		"go", "test", "./internal/generator/...", "-count=1",
		"-coverprofile="+generatorProfile,
	); err != nil {
		return err
	}
	if err := run(
		"go", "test", "./"+acceptanceRoot+"/", "-count=1",
		"-args", "-cover.out="+generatedProfile,
	); err != nil {
		return err
	}

	generatorPct, err := coverProfilePercent(generatorProfile)
	if err != nil {
		return err
	}
	generatedPct, err := coverProfilePercent(generatedProfile)
	if err != nil {
		return err
	}

	fmt.Printf("\ngenerator (./internal/generator/...):  %.1f%% of statements\n",
		generatorPct)
	fmt.Printf("code it generates:           %.1f%% of statements "+
		"(run by the acceptance cases)\n", generatedPct)
	return nil
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

// CheckGen runs every generator and fails if a file changes.
// All generated code is committed.
// A change here means someone edited the source or the generator
// without running "mage gen".
//
// The working tree must be clean.
// Every change left afterwards is then one a generator made.
//
// The generators run one at a time and each reports the files it changed,
// which names the target to rerun and the place to look.
//
// Changed files are left in place: they belong in the commit.
// [CheckMod] restores instead, since a tidied go.mod is not always wanted.
func CheckGen() error {
	changed, err := gitChanged()
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		return fmt.Errorf("working tree is not clean, commit or stash first:\n%s",
			formatChanged(changed))
	}

	generators := []struct {
		target string
		run    func() error
	}{
		{"mage genTempl", GenTempl},
		{"mage genDatapages", GenDatapages},
		{"mage genDocs", GenDocs},
	}

	// A file one generator changed stays changed for the rest of the run.
	// Attributing it to the first generator that touched it keeps each report
	// to the files that generator is responsible for.
	attributed := make(map[string]bool)
	var report []string
	for _, g := range generators {
		if err := g.run(); err != nil {
			return err
		}
		if changed, err = gitChanged(); err != nil {
			return err
		}
		stale := make(map[string]string)
		for path, state := range changed {
			if !attributed[path] {
				attributed[path] = true
				stale[path] = state
			}
		}
		if len(stale) > 0 {
			report = append(report, g.target+":\n"+formatChanged(stale))
		}
	}
	if len(report) == 0 {
		return nil
	}
	return fmt.Errorf("generated code is stale, rerun and commit:\n%s",
		strings.Join(report, "\n"))
}

// gitChanged maps every changed path to its two-letter git status code.
// Ignored paths never appear, docs/index.html among them.
func gitChanged() (map[string]string, error) {
	out, err := output("git", "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	changed := make(map[string]string)
	for line := range strings.Lines(out) {
		// "XY path", where XY is the index and the worktree status.
		if line = strings.TrimRight(line, "\n"); len(line) < 4 {
			continue
		}
		changed[line[3:]] = line[:2]
	}
	return changed, nil
}

// formatChanged renders the paths the way "git status --porcelain" prints
// them, indented and sorted so that two runs read the same.
func formatChanged(changed map[string]string) string {
	lines := make([]string, 0, len(changed))
	for path, state := range changed {
		lines = append(lines, "  "+state+" "+path)
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n")
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
	if err := run("go", "build", "-o", bin, "./cmd/datapages"); err != nil {
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

// ModUpdate updates dependencies for all modules, then tidies them.
func ModUpdate() error {
	if err := run("go", "get", "-u", "-t", "./..."); err != nil {
		return err
	}
	for _, root := range submoduleRoots {
		if err := forEachModule(root, func(dir string) error {
			fmt.Println("==> go get -u -t ./... in", dir)
			return runIn(dir, "go", "get", "-u", "-t", "./...")
		}); err != nil {
			return err
		}
	}
	return ModTidy()
}

// ModTidy tidies all modules in the repo.
func ModTidy() error {
	if err := run("go", "mod", "tidy"); err != nil {
		return err
	}
	for _, root := range submoduleRoots {
		if err := forEachModule(root, func(dir string) error {
			fmt.Println("==> go mod tidy in", dir)
			return runIn(dir, "go", "mod", "tidy")
		}); err != nil {
			return err
		}
	}
	return nil
}

// GoFix runs go fix on the root module, examples, and parser testdata.
func GoFix() error {
	if err := GoFixCLI(); err != nil {
		return err
	}
	return GoFixExamples()
}

// GoFixCLI runs go fix on the root module.
func GoFixCLI() error {
	fmt.Println("==> go fix ./...")
	return run("go", "fix", "./...")
}

// goFixSkip lists modules go fix cannot process because they hold code that
// deliberately doesn't type check.
var goFixSkip = map[string]struct{}{
	filepath.Join("internal", "parser", "testdata", "err_typecheck"): {},
}

// GoFixExamples runs go fix on all example and parser testdata modules.
func GoFixExamples() error {
	for _, root := range submoduleRoots {
		if err := forEachModule(root, func(dir string) error {
			if _, ok := goFixSkip[filepath.Clean(dir)]; ok {
				fmt.Println("==> skipping go fix in", dir)
				return nil
			}
			fmt.Println("==> go fix ./... in", dir)
			return runIn(dir, "go", "fix", "./...")
		}); err != nil {
			return err
		}
	}
	return nil
}

// Gen runs all code generation (templ, datapages, docs).
func Gen() error {
	if err := GenTempl(); err != nil {
		return err
	}
	if err := GenDatapages(); err != nil {
		return err
	}
	return GenDocs()
}

// GenDatapages builds the datapages CLI from source and runs "datapages gen"
// on each example and each acceptance case.
//
// The generated code of both is committed. TestExamplesAreUpToDate and
// TestAcceptance regenerate and compare,
// and tell whoever reads the failure to run this target.
func GenDatapages() error {
	tmp, err := os.MkdirTemp("", "datapages-gen-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "datapages")
	if err := run("go", "build", "-o", bin, "./cmd/datapages"); err != nil {
		return err
	}
	for _, root := range []string{"example", acceptanceRoot} {
		if err := forEachModule(root, func(dir string) error {
			if skipGeneration(dir) {
				return nil
			}
			fmt.Println("==> datapages gen in", dir)
			return runIn(dir, bin, "gen")
		}); err != nil {
			return err
		}
	}
	return nil
}

// acceptanceRoot holds the acceptance cases, one module each.
const acceptanceRoot = "internal/acceptance"

// skipGeneration reports whether a module keeps no generated code.
//
// An acceptance case that records a build error is generated by its runner
// into a throwaway module and built there. Generating it here would leave a
// package that does not compile in the working tree,
// which is the very thing the case records.
func skipGeneration(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "acceptance.json"))
	return err == nil && strings.Contains(string(b), `"expect_build_error"`)
}

// GenTempl generates templ templates for examples and parser testdata.
func GenTempl() error {
	for _, root := range submoduleRoots {
		if err := forEachModule(root, func(dir string) error {
			if !hasTemplFiles(dir) {
				return nil
			}
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
	if err := goRun(toolTempl, "generate", "-path", "./internal/docs-src"); err != nil {
		return err
	}
	if err := run("go", "run", "./internal/tools/render-pages", "-version", version); err != nil {
		return err
	}
	fmt.Println("==> minify internal/docs-src/style.css -> docs/style.css")
	return goRun(toolMinify, "-o", "docs/style.css", "internal/docs-src/style.css")
}

// All runs test, vulncheck, fmt, mod-tidy, gen-templ, and gen-docs.
func All() error {
	if err := Fmt(); err != nil {
		return err
	}
	if err := ModTidy(); err != nil {
		return err
	}
	if err := GenTempl(); err != nil {
		return err
	}
	if err := GenDocs(); err != nil {
		return err
	}
	if err := Test(); err != nil {
		return err
	}
	return Vulncheck()
}
