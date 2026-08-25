package cmd

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// Run executes the datapages CLI with the given arguments.
// It returns the exit code.
func Run(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	version, commit, buildDate string,
) int {
	root := &cobra.Command{
		Use:   "datapages",
		Short: "Datapages code generator and dev server",
		Long: `Datapages is a framework for building multi-page web applications in Go.

It parses your application model, generates routing, handler wiring,
and type-safe href/action helpers, and provides a live-reloading dev server.`,
	}
	root.SetContext(ctx)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args[1:])
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newGenCmd(stderr, version),
		newInitCmd(stderr, version),
		newLintCmd(stderr, version),
		newVersionCmd(stdout, version, commit, buildDate),
		newWatchCmd(stderr, version),
	)

	if err := root.Execute(); err != nil {
		label := color.New(color.FgRed, color.Bold)
		if wantColorFor(stderr) {
			label.EnableColor()
		} else {
			label.DisableColor()
		}
		_, _ = fmt.Fprintln(stderr, label.Sprint("error:"), err)
		return 1
	}
	return 0
}

// findModuleDir walks up from the current working directory
// looking for a go.mod file. Returns the directory containing go.mod.
func findModuleDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New(
				"not inside a Go module (no go.mod found in any parent directory)",
			)
		}
		dir = parent
	}
}

// findGitDir walks up from dir looking for a .git directory or file.
// Returns the directory containing .git, or empty string if not found.
func findGitDir(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// readModulePath reads go.mod from moduleDir and returns the module path.
func readModulePath(moduleDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("reading go.mod: %w", err)
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return "", fmt.Errorf("parsing go.mod: %w", err)
	}
	if f.Module == nil {
		return "", errors.New("go.mod has no module directive")
	}
	return f.Module.Mod.Path, nil
}

// checkGoModVersion returns an error if go.mod requires a newer version of
// datapages than the running binary. It is a no-op for dev builds (empty version) or
// when the dependency is missing or up to date.
func checkGoModVersion(moduleDir, version string) error {
	if version == "" {
		return nil
	}
	gomodPath := filepath.Join(moduleDir, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return fmt.Errorf("reading go.mod: %w", err)
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return fmt.Errorf("parsing go.mod: %w", err)
	}
	running := "v" + version
	if !semver.IsValid(running) {
		return nil
	}
	for _, req := range f.Require {
		if req.Mod.Path != datapagesModulePath {
			continue
		}
		if semver.Compare(running, req.Mod.Version) < 0 {
			return fmt.Errorf(
				"go.mod requires %s %s but you are running %s\n"+
					"  run: go install %s@%s",
				datapagesModulePath, req.Mod.Version, running,
				datapagesModulePath, req.Mod.Version,
			)
		}
		return nil
	}
	return nil
}

// upgradeGoMod updates the datapages require in go.mod to match the running
// version when the running version is strictly newer. Returns an error when
// go.mod requires a newer version than the running binary (the user should upgrade).
// It is a no-op for dev builds (empty version).
func upgradeGoMod(moduleDir, version string) error {
	if err := checkGoModVersion(moduleDir, version); err != nil {
		return err
	}
	if version == "" {
		return nil
	}
	gomodPath := filepath.Join(moduleDir, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return fmt.Errorf("reading go.mod: %w", err)
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return fmt.Errorf("parsing go.mod: %w", err)
	}
	running := "v" + version
	for _, req := range f.Require {
		if req.Mod.Path != datapagesModulePath {
			continue
		}
		if semver.Compare(running, req.Mod.Version) <= 0 {
			return nil // already up to date
		}
		if err := f.AddRequire(req.Mod.Path, running); err != nil {
			return fmt.Errorf("updating go.mod: %w", err)
		}
		f.Cleanup()
		out, err := f.Format()
		if err != nil {
			return fmt.Errorf("formatting go.mod: %w", err)
		}
		if err := os.WriteFile(gomodPath, out, 0o644); err != nil {
			return fmt.Errorf("writing go.mod: %w", err)
		}
		return nil
	}
	return nil
}

// checkCmdPackage checks the package at dir. Returns true if the directory exists.
// Returns an error if it exists but contains a non-main package.
func checkCmdPackage(dir string) (exists bool, _ error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading cmd directory: %w", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".go" {
			continue
		}
		f, err := parser.ParseFile(
			fset, filepath.Join(dir, e.Name()), nil, parser.PackageClauseOnly,
		)
		if err != nil {
			continue
		}
		if f.Name.Name != "main" {
			return true, fmt.Errorf(
				"cmd package at %s is %q, expected \"main\"", dir, f.Name.Name,
			)
		}
		return true, nil
	}
	// Directory exists but has no Go files — treat as non-existent.
	return false, nil
}
