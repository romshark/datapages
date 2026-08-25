// Runs every acceptance case.
//
// A case is a module under this directory: an application, the code generated from it,
// and the tests that send it requests over HTTP. The generated code is committed.
//
// Per case the runner regenerates it into a temporary directory and compares
// the result with what is committed, which makes the code under test the code
// the generator writes today. It then runs the case's own tests, with coverage
// over its generated packages, and reports how much of that code the suite executes.

package acceptance_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/serverscan"
)

var (
	flagCoverOut = flag.String("cover.out", "",
		"write the merged coverage profile of the generated code to this file")
	flagCoverMin = flag.Float64("cover.min", 0,
		"fail if coverage of the generated code is below this percentage")
)

// caseOptions is the optional acceptance.json of a case.
type caseOptions struct {
	// NoRace turns the race detector off for cases where it costs more than it can find.
	NoRace bool `json:"no_race"`
}

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
	if os.IsNotExist(err) {
		return caseOptions{}
	}
	require.NoError(t, err)
	var opts caseOptions
	require.NoError(t, json.Unmarshal(b, &opts), "parsing acceptance.json")
	return opts
}

// requireGeneratedIsCurrent regenerates the case beside its committed code and
// compares the two. What the suite runs is then what the generator writes.
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
// How many applications a case builds is not fixed. Most build one; multiapp builds two.
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
		require.NoError(t, serverscan.CheckSessionOption(app, m.Session != nil))

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
// every app it builds, which is the code the suite reports coverage of.
func coverPkgs(apps []serverscan.App) string {
	pkgs := make([]string, len(apps))
	for i, a := range apps {
		pkgs[i] = "./" + filepath.ToSlash(a.GenDir) + "/..."
	}
	return strings.Join(pkgs, ",")
}

// modulePath is the import path of a case module.
func modulePath(name string) string {
	return "github.com/romshark/datapages/internal/acceptance/" + name
}

// compareTrees fails when the two directories differ in file names or content.
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
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	require.NoError(t, err, "reading %s", dir)
	return files
}

// coverage accumulates one coverage profile per acceptance case.
var coverage coverageSet

type coverageSet struct {
	mu    sync.Mutex
	cases map[string][]coverageBlock
}

// coverageBlock is one block of a Go coverage profile:
// a source range, the number of statements in it, and how often it ran.
type coverageBlock struct {
	Location string // "<case>/<file>:startLine.col,endLine.col"
	NumStmt  int
	Count    int
}

func (c *coverageSet) add(t *testing.T, name, profile string) {
	t.Helper()
	blocks, err := parseCoverProfile(profile, name)
	require.NoError(t, err, "reading coverage profile of case %s", name)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cases == nil {
		c.cases = map[string][]coverageBlock{}
	}
	c.cases[name] = blocks
}

// parseCoverProfile reads a profile written by "go test -coverprofile" and
// keys every block by the case it came from.
//
// "go test ./..." runs one test binary per package and each of them reports
// every block of every package named by -coverpkg. The same block therefore
// appears once per binary. Blocks are merged by location the way
// "go tool cover" merges them. Counts add up and statements are counted once.
func parseCoverProfile(profile, caseName string) ([]coverageBlock, error) {
	b, err := os.ReadFile(profile)
	if err != nil {
		return nil, err
	}
	var blocks []coverageBlock
	index := map[string]int{}
	for line := range strings.Lines(string(b)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// <file>:<start>,<end> <numStmt> <count>
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed profile line %q", line)
		}
		numStmt, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("malformed statement count in %q: %w", line, err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("malformed hit count in %q: %w", line, err)
		}
		location := path.Join(caseName, fields[0])
		if i, ok := index[location]; ok {
			blocks[i].Count += count
			continue
		}
		index[location] = len(blocks)
		blocks = append(blocks, coverageBlock{
			Location: location,
			NumStmt:  numStmt,
			Count:    count,
		})
	}
	return blocks, nil
}

// report prints coverage of the generated code per case and in total,
// writes the merged profile when asked for one,
// and reports whether the total clears the floor.
func (c *coverageSet) report(w *strings.Builder) (total float64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cases) == 0 {
		return 0, true
	}

	names := make([]string, 0, len(c.cases))
	for name := range c.cases {
		names = append(names, name)
	}
	sort.Strings(names)

	width := 0
	for _, name := range names {
		if len(name) > width {
			width = len(name)
		}
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

// writeMerged writes every case's blocks as one profile. The file names carry
// the case as their first path element, which keeps the blocks distinct.
func (c *coverageSet) writeMerged(dst string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var b strings.Builder
	b.WriteString("mode: set\n")
	names := make([]string, 0, len(c.cases))
	for name := range c.cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, block := range c.cases[name] {
			fmt.Fprintf(&b, "%s %d %d\n", block.Location, block.NumStmt, block.Count)
		}
	}
	return os.WriteFile(dst, []byte(b.String()), 0o644)
}

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
