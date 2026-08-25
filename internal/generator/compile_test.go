package generator_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser"
)

// TestCompileFixtures compiles the code generated for every parser fixture
// that parses without errors.
//
// The acceptance suites cover what generated code does, but only for the model
// shapes their own apps have. The parser fixtures cover every shape the parser accepts,
// which is the input domain of the generator.
//
// A generator emits names as easily as it emits nonsense: an embed without its
// type argument, a slot that no line declares, a package qualifier that
// resolves to nothing. Each of those stops the user's build. None of them is
// visible to an assertion made on the output as text.
//
// The app package sits in a directory named "pages" while the package itself
// is named "app". The directory is whatever the datapages.NewServer call points
// at and the package name is whatever the source declares. The two need not match.
// A generator that assumes they match compiles only in the examples.
func TestCompileFixtures(t *testing.T) {
	if testing.Short() {
		t.Skip("builds one module per fixture")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	fixtures := parserFixtures(t)
	require.NotEmpty(t, fixtures)

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			src := filepath.Join("..", "parser", "testdata", fixture)
			mod := t.TempDir()

			modPath := writeCompileModule(t, mod, src, repoRoot)

			app, errs := parser.Parse(filepath.Join(mod, "pages"))
			for _, err := range errs.All() {
				t.Errorf("parser: %v", err)
			}
			require.Zero(t, errs.Len())
			require.NotNil(t, app, "parser returned nil model")

			require.NoError(t, generator.Generate(
				filepath.Join(mod, "datapagesgen"), "datapagesgen", app, 0o644,
				generator.Options{GenImport: modPath + "/datapagesgen"},
			))

			// The entry point is generated from the same model and is the one
			// file a user never edits before their first build.
			require.NoError(t, generator.GenerateCmd(
				filepath.Join(mod, "cmd", "server"),
				modPath+"/pages", modPath+"/datapagesgen", "datapagesgen",
				false, app, 0o644,
			))

			cmd := exec.Command("go", "build", "./...")
			cmd.Dir = mod
			cmd.Env = append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=mod")
			out, err := cmd.CombinedOutput()

			require.NoError(t, err, "generated code does not build:\n%s",
				strings.TrimSpace(string(out)))
		})
	}
}

// parserFixtures lists the fixtures the parser accepts. The err_ prefix marks
// the ones that are supposed to fail parsing, and those never reach a generator.
func parserFixtures(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "parser", "testdata"))
	require.NoError(t, err)

	var fixtures []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "err_") {
			continue
		}
		fixtures = append(fixtures, e.Name())
	}
	return fixtures
}

// writeCompileModule lays out a module around the fixture's app package and
// returns its module path. The fixture's own go.mod carries the versions the
// app package needs. What it lacks is what only generated code imports.
// datapages is replaced by the working tree, the code under test.
func writeCompileModule(t *testing.T, mod, src, repoRoot string) string {
	t.Helper()

	pages := filepath.Join(mod, "pages")
	require.NoError(t, os.MkdirAll(pages, 0o755))

	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		if name == "go.mod" || name == "go.sum" {
			continue
		}
		from := filepath.Join(src, name)
		if e.IsDir() {
			require.NoError(t,
				os.CopyFS(filepath.Join(pages, name), os.DirFS(from)))
			continue
		}
		b, err := os.ReadFile(from)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pages, name), b, 0o644))
	}

	gomod, err := os.ReadFile(filepath.Join(src, "go.mod"))
	require.NoError(t, err)
	modPath, _, ok := strings.Cut(strings.TrimPrefix(string(gomod), "module "), "\n")
	require.True(t, ok, "no module path in the fixture go.mod")
	modPath = strings.TrimSpace(modPath)

	// The fixture's own go.mod carries only what its app package imports.
	// Generated code imports more, and which of them depends on the model.
	// The versions come from the example that requires everything the generator can emit.
	writeModuleFiles(t, mod, modPath, repoRoot)
	return modPath
}

// writeModuleFiles writes the go.mod and go.sum of a throwaway module under
// the given module path. The dependency versions come from an example,
// which is what a user of this generator has.
func writeModuleFiles(t *testing.T, mod, modPath, repoRoot string) {
	t.Helper()

	example := filepath.Join(repoRoot, "example", "classifieds")

	sum, err := os.ReadFile(filepath.Join(example, "go.sum"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mod, "go.sum"), sum, 0o644))

	gomod, err := os.ReadFile(filepath.Join(example, "go.mod"))
	require.NoError(t, err)
	out := strings.Replace(string(gomod),
		"module github.com/romshark/datapages/example/classifieds",
		"module "+modPath, 1)
	out = strings.Replace(out,
		"replace github.com/romshark/datapages => ../../",
		"replace github.com/romshark/datapages => "+repoRoot, 1)
	require.NoError(t, os.WriteFile(filepath.Join(mod, "go.mod"), []byte(out), 0o644))
}
