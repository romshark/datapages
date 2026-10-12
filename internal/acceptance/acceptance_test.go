// Runs every acceptance case.
//
// A case is a module under this directory: an application, the code generated from it,
// and the tests that send it requests over HTTP. The generated code is committed.
//
// Per case the runner regenerates it into a temporary directory and compares
// the result with what is committed, which makes the code under test the code
// the generator writes today. It then runs the case's own tests.
//
// A case runs "go test" in its own module, which the -coverprofile of this package
// doesn't measure. Instead, each case measures its generated packages and the
// datapages packages they import, and the runner merges the profiles of all cases.
// It prints the coverage of the generated code, which -cover.min can require.
// -cover.out writes the merged profile, which CI uploads to Coveralls and
// "mage coverage" reads.

package acceptance_test

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/cover"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/serverscan"
)

var (
	flagCoverOut = flag.String("cover.out", "",
		"write the merged coverage profile of the generated code "+
			"and the datapages packages it imports to this file")
	flagCoverMin = flag.Float64("cover.min", 0,
		"fail if coverage of the generated code is below this percentage")
)

// caseOptions is the optional acceptance.json of a case.
type caseOptions struct {
	// NoRace turns the race detector off for cases where it costs more than it can find.
	NoRace bool `json:"no_race"`
}

// TestAcceptance runs every case module: it regenerates the case,
// diffs the result against the committed output and runs the case's own tests over HTTP.
// 1 subtest per directory holding a go.mod.
func TestAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a module per case")
	}

	for _, name := range caseNames(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runCase(t, name)
		})
	}
}

// caseNames lists the case modules: every directory here holding a go.mod.
func caseNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Name(), "go.mod")); err != nil {
			continue // No go.mod, no case. This is client or contract.
		}
		names = append(names, e.Name())
	}
	require.NotEmpty(t, names, "no acceptance cases")
	return names
}

func runCase(t *testing.T, name string) {
	t.Helper()
	opts := readCaseOptions(t, name)

	apps := requireGeneratedIsCurrent(t, name)

	profile := filepath.Join(t.TempDir(), "cover.out")
	args := []string{
		"test", "-count=1",
		"-coverpkg=" + coverPkgs(apps), "-coverprofile=" + profile, "./...",
	}
	if !opts.NoRace {
		args = append(args, "-race")
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = name
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", strings.TrimSpace(string(out)))

	coverage.add(t, name, profile)
}

func readCaseOptions(t *testing.T, name string) caseOptions {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(name, "acceptance.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return caseOptions{}
	}
	require.NoError(t, err)
	var opts caseOptions
	require.NoError(t, json.Unmarshal(b, &opts), "parsing acceptance.json")
	return opts
}

// requireGeneratedIsCurrent regenerates the case beside its committed code and
// compares the 2. What the suite runs is then what the generator writes.
func requireGeneratedIsCurrent(t *testing.T, name string) []serverscan.App {
	t.Helper()
	dst := t.TempDir()
	apps := generateInto(t, name, dst)
	for _, app := range apps {
		compareTrees(t, filepath.Join(dst, app.GenDir), filepath.Join(name, app.GenDir))
	}
	return apps
}

// generateInto parses the app packages of the case and generates them into dst,
// the way "datapages gen" does, and returns the apps the scan found.
//
// How many applications a case builds is not fixed. Most build 1; multiapp builds 2.
// Each is generated on its own, from its own app package.
func generateInto(t *testing.T, name, dst string) []serverscan.App {
	t.Helper()

	modPath := modulePath(name)
	scan, err := serverscan.Scan(name, modPath)
	require.NoError(t, err)
	require.False(t, scan.Fallback, "%s holds no datapages.NewServer call", name)
	require.NotEmpty(t, scan.Apps)

	for _, app := range scan.Apps {
		m, errs := parser.Parse(filepath.Join(name, app.Dir))
		for _, err := range errs.All() {
			t.Errorf("parser: %v", err)
		}
		require.Zero(t, errs.Len())
		require.NotNil(t, m, "parser returned nil model")
		require.NoError(t, serverscan.CheckSessionData(app, m.Session != nil))

		require.NoError(t, generator.Generate(
			filepath.Join(dst, app.GenDir), serverscan.GenSubdir,
			m, 0o644, generator.Options{
				Prometheus:      app.Prometheus,
				AssetsURLPrefix: m.Assets.URLPrefix,
				AssetsDir:       m.Assets.Dir,
				AppDir:          app.Dir,
				GenImport:       app.GenImport,
			},
		))
	}
	return scan.Apps
}

// coverPkgs is the -coverpkg list of a case: the generated package of
// every app it builds, which is the code the suite reports coverage of,
// and [importedPkgs].
func coverPkgs(apps []serverscan.App) string {
	var pkgs []string
	for _, a := range apps {
		pkgs = append(pkgs, "./"+filepath.ToSlash(a.GenDir)+"/...")
	}
	return strings.Join(append(pkgs, importedPkgs...), ",")
}

// importedPkgs are the datapages packages that generated and application code import,
// which the cases run over HTTP. -coverpkg matches only the ones a case imports.
var importedPkgs = []string{
	"github.com/romshark/datapages",
	"github.com/romshark/datapages/modules/...",
	"github.com/romshark/datapages/runtime/...",
}

// modulePath is the import path of a case module.
func modulePath(name string) string {
	return "github.com/romshark/datapages/internal/acceptance/" + name
}

// compareTrees fails when the 2 directories differ in file names or content.
func compareTrees(t *testing.T, got, want string) {
	t.Helper()
	gotFiles := readTree(t, got)
	wantFiles := readTree(t, want)

	for name, content := range gotFiles {
		committed, ok := wantFiles[name]
		if !ok {
			t.Errorf("%s is generated and not committed; run: mage genDatapages", name)
			continue
		}
		if committed != content {
			t.Errorf("%s differs from generator output; run: mage genDatapages", name)
		}
	}
	for name := range wantFiles {
		if _, ok := gotFiles[name]; !ok {
			t.Errorf("%s is committed and no longer generated; run: mage genDatapages",
				name)
		}
	}
}

// readTree reads every file of a directory tree, keyed by its path within it.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := os.DirFS(dir)
	files := map[string]string{}
	err := fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(tree, p)
		if err != nil {
			return err
		}
		files[p] = string(b)
		return nil
	})
	require.NoError(t, err, "reading %s", dir)
	return files
}

// coverage accumulates the coverage profiles of the acceptance cases.
var coverage = coverageSet{
	cases: map[string][]cover.ProfileBlock{},
	all:   map[string]cover.ProfileBlock{},
}

type coverageSet struct {
	lock sync.Mutex
	// cases holds the blocks of the generated code of each case.
	cases map[string][]cover.ProfileBlock
	// all holds every block of every case by its location in the profile,
	// "<import path>/<file>:startLine.col,endLine.col". The cases share the
	// blocks of [importedPkgs].
	all map[string]cover.ProfileBlock
}

func (c *coverageSet) add(t *testing.T, name, profile string) {
	t.Helper()
	// Each test binary of the case reports every package -coverpkg names.
	// ParseProfiles merges the repeats of a block.
	profiles, err := cover.ParseProfiles(profile)
	require.NoError(t, err, "reading coverage profile of case %s", name)

	c.lock.Lock()
	defer c.lock.Unlock()
	var generated []cover.ProfileBlock
	for _, p := range profiles {
		// The generated packages are the only ones of -coverpkg in the case module.
		isGenerated := strings.HasPrefix(p.FileName, modulePath(name)+"/")
		for _, b := range p.Blocks {
			if isGenerated {
				generated = append(generated, b)
			}
			location := fmt.Sprintf("%s:%d.%d,%d.%d",
				p.FileName, b.StartLine, b.StartCol, b.EndLine, b.EndCol)
			b.Count += c.all[location].Count
			c.all[location] = b
		}
	}
	c.cases[name] = generated
}

// report prints coverage of the generated code per case and in total,
// and reports whether the total clears the floor.
func (c *coverageSet) report(w *strings.Builder) (total float64, ok bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if len(c.cases) == 0 {
		return 0, true
	}

	names := slices.Sorted(maps.Keys(c.cases))

	width := 0
	for _, name := range names {
		width = max(width, len(name))
	}

	var allStmt, allCovered int
	fmt.Fprintf(w, "\ncoverage of generated code (acceptance cases):\n")
	for _, name := range names {
		var stmt, covered int
		for _, b := range c.cases[name] {
			stmt += b.NumStmt
			if b.Count > 0 {
				covered += b.NumStmt
			}
		}
		allStmt += stmt
		allCovered += covered
		fmt.Fprintf(w, "  %-*s  %5.1f%%  (%d/%d statements)\n",
			width, name, percent(covered, stmt), covered, stmt)
	}
	fmt.Fprintf(w, "  %-*s  %5.1f%%  (%d/%d statements)\n",
		width, "TOTAL", percent(allCovered, allStmt), allCovered, allStmt)

	total = percent(allCovered, allStmt)
	return total, total >= *flagCoverMin
}

func percent(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total) * 100
}

// writeMerged writes the blocks of every case as 1 profile, sorted by location.
// The import path of each case module keeps its generated files apart.
// A block of [importedPkgs] appears once, with the counts of all cases added up.
func (c *coverageSet) writeMerged(dst string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	var b strings.Builder
	b.WriteString("mode: set\n")
	for _, location := range slices.Sorted(maps.Keys(c.all)) {
		block := c.all[location]
		fmt.Fprintf(&b, "%s %d %d\n", location, block.NumStmt, block.Count)
	}
	return os.WriteFile(dst, []byte(b.String()), 0o644)
}

// TestMain runs the cases and then reports the coverage they reached over the
// generated code, merged across every case. With -cover.out it also writes the
// merged profile of the generated code and [importedPkgs].
func TestMain(m *testing.M) {
	flag.Parse()
	code := m.Run()

	var out strings.Builder
	total, ok := coverage.report(&out)
	if out.Len() > 0 {
		fmt.Print(out.String())
	}
	if *flagCoverOut != "" {
		if err := coverage.writeMerged(*flagCoverOut); err != nil {
			fmt.Fprintf(os.Stderr, "writing merged coverage profile: %v\n", err)
			code = 1
		}
	}
	if !ok {
		fmt.Fprintf(os.Stderr,
			"FAIL: generated-code coverage %.1f%% is below -cover.min=%.1f%%\n",
			total, *flagCoverMin)
		code = 1
	}
	os.Exit(code)
}
