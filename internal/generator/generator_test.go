package generator_test

import (
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser"
)

// examples lists the example applications whose generated code is committed,
// with the options their datapages.yaml carries.
//
// Their datapagesgen directories are read by people learning what the generator produces,
// and they are the only generated code in this repository that a reader ever sees.
// Committed output that no longer matches the generator misleads every one of
// those readers.
var examples = map[string]struct {
	prometheus      bool
	assetsURLPrefix string
	assetsDir       string
}{
	"calculator": {assetsURLPrefix: "/static/", assetsDir: "static"},
	"classifieds": {
		prometheus: true, assetsURLPrefix: "/static/", assetsDir: "static",
	},
	"counter":        {},
	"fancy-counter":  {},
	"sqlitesessions": {},
	"tailwindcss":    {assetsURLPrefix: "/static/", assetsDir: "static"},
	"todolist":       {assetsURLPrefix: "/static/", assetsDir: "static"},
	"webcomponents":  {assetsURLPrefix: "/static/", assetsDir: "static"},
}

// TestExamplesAreUpToDate regenerates each example and
// compares the result with what is committed.
//
// This is the one test that reads generated code as text, and it reads it for
// a reason text is the right medium for: the committed files are the artifact.
// What the generated code does is covered by the acceptance suites.
func TestExamplesAreUpToDate(t *testing.T) {
	for name, opts := range examples {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join("..", "..", "example", name)
			app, errs := parser.Parse(filepath.Join(dir, "app"))
			require.Zero(t, errs.Len(), "unexpected parser errors:\n%s", listErrors(errs))
			require.NotNil(t, app, "parser returned nil model")

			modPath := modulePathOf(t, dir)
			got := t.TempDir()
			require.NoError(t, generator.Generate(
				got, "datapagesgen", app, 0o644, generator.Options{
					Prometheus:      opts.prometheus,
					AssetsURLPrefix: opts.assetsURLPrefix,
					AssetsDir:       opts.assetsDir,
					AppDir:          "app",
					GenImport:       modPath + "/datapagesgen",
				},
			))

			compareTrees(t, got, filepath.Join(dir, "datapagesgen"))
		})
	}
}

func listErrors(errs parser.Errors) string {
	var b strings.Builder
	for _, err := range errs.All() {
		b.WriteString("  ")
		b.WriteString(err.Error())
		b.WriteByte('\n')
	}
	return b.String()
}

// modulePathOf reads the module path out of a go.mod.
func modulePathOf(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)
	line, _, ok := strings.Cut(strings.TrimPrefix(string(b), "module "), "\n")
	require.True(t, ok, "no module path in %s/go.mod", dir)
	return strings.TrimSpace(line)
}

// compareTrees compares every generated file with its committed counterpart,
// in both directions: a file the generator no longer writes is drift too.
func compareTrees(t *testing.T, gotDir, wantDir string) {
	t.Helper()

	generated := goFilesOf(t, gotDir)
	committed := goFilesOf(t, wantDir)

	for rel := range generated {
		if _, ok := committed[rel]; !ok {
			t.Errorf("%s is generated but not committed; run: mage genDatapages", rel)
		}
	}
	for rel := range committed {
		if _, ok := generated[rel]; !ok {
			t.Errorf("%s is committed but no longer generated; run: mage genDatapages", rel)
			continue
		}
		if normalize(generated[rel]) != normalize(committed[rel]) {
			t.Errorf("%s differs from the committed output; run: mage genDatapages", rel)
		}
	}
}

// goFilesOf reads every generated Go file of a tree,
// keyed by its path relative to the tree.
func goFilesOf(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = b
		return nil
	})
	require.NoError(t, err)
	return files
}

// normalize formats source so that a difference in
// whitespace alone is not reported as drift.
func normalize(src []byte) string {
	if out, err := format.Source(src); err == nil {
		return string(out)
	}
	return string(src)
}
