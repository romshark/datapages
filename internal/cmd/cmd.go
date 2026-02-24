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

	"github.com/romshark/yamagiconf"
	"github.com/spf13/cobra"

	"golang.org/x/mod/modfile"
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
		newGenCmd(stderr),
		newInitCmd(stderr),
		newLintCmd(stderr),
		newVersionCmd(stdout, version, commit, buildDate),
		newWatchCmd(stderr),
	)

	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
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

// loadConfig reads datapages.yml or datapages.yaml from moduleDir.
// If neither file exists, default values are returned and found is false.
func loadConfig(moduleDir string) (c config, found bool, _ error) {
	for _, name := range []string{"datapages.yml", "datapages.yaml"} {
		p := filepath.Join(moduleDir, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := yamagiconf.LoadFile(
			p, &c, yamagiconf.WithOptionalPresence(),
		); err != nil {
			return config{}, false, fmt.Errorf("loading %s: %w", name, err)
		}
		found = true
		break
	}
	if c.App == "" {
		c.App = "app"
	}
	if c.Gen.Package == "" {
		c.Gen.Package = "datapagesgen"
	}
	if c.Cmd == "" {
		c.Cmd = "cmd/server"
	}
	if c.Gen.Prometheus == nil {
		v := true
		c.Gen.Prometheus = &v
	}
	return c, found, nil
}

func defaultConfigYAML(prometheus bool) string {
	return fmt.Sprintf(`app: app
gen:
  package: datapagesgen
  prometheus: %t
cmd: cmd/server
`, prometheus)
}

func writeDefaultConfig(moduleDir string, prometheus bool) error {
	p := filepath.Join(moduleDir, "datapages.yaml")
	return os.WriteFile(p, []byte(defaultConfigYAML(prometheus)), 0o644)
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

// checkCmdPackage checks the package at dir. Returns true if the directory
// exists. Returns an error if it exists but contains a non-main package.
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
