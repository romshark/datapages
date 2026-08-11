package generator_test

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

	"github.com/romshark/datapages/generator"
	"github.com/romshark/datapages/parser"
)

// acceptanceModule is the module path every case module uses. The case's own
// test files import themselves by it. It must stay the same for all cases.
// Coverage profiles are keyed by case name at merge time instead.
const acceptanceModule = "dpacceptance"

var (
	flagCoverOut = flag.String("cover.out", "",
		"write the merged coverage profile of the generated code to this file")
	flagCoverMin = flag.Float64("cover.min", 0,
		"fail if coverage of the generated code is below this percentage")
	flagKeep = flag.String("keep", "",
		"keep each generated case module under this directory")
)

// TestAcceptance generates an application, builds it and runs its own tests
// against the running server.
//
// What matters about a generator is what its output does. Reading the output
// as text checks one version of the code instead of the behaviour every
// version has to provide. The cases here send requests and read responses.
//
// Each case is a directory under testdata/acceptance holding an app package
// and its own tests. Both are copied into a throwaway module where datapages
// is replaced by the working tree. The case's tests run with coverage over the
// generated packages. The suite can then report how much of the generated code
// it executes.
func TestAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a module per case")
	}
	repoRoot, err := filepath.Abs("..")
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join("testdata", "acceptance"))
	require.NoError(t, err)
	require.NotEmpty(t, entries, "no acceptance cases")

	for _, e := range entries {
		// Directories starting with "_" hold material for the cases rather
		// than cases. Go tooling ignores them for the same reason.
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			t.Parallel()
			runAcceptanceCase(t, e.Name(), repoRoot)
		})
	}
}

// acceptanceOptions is the optional options.json of a case. Its fields are the
// generator options a datapages.yaml controls. A case can therefore cover the
// options that change what is generated, not only what a default app emits.
type acceptanceOptions struct {
	Prometheus      bool   `json:"prometheus"`
	AssetsURLPrefix string `json:"assets_url_prefix"`
	AssetsDir       string `json:"assets_dir"`
	AppDir          string `json:"app_dir"`
	GenPkg          string `json:"gen_pkg"`
	// Cmd generates the cmd/server entry point into the module, which puts
	// the skeleton through the compiler instead of through a substring check.
	Cmd bool `json:"cmd"`
	// NoRace turns the race detector off for cases where it costs more than
	// it can find.
	NoRace bool `json:"no_race"`
	// KnownBug records a generator bug this case reproduces. The case must
	// fail, and the failure must still contain this substring. Fixing the
	// generator turns the case red until the entry is removed.
	KnownBug string `json:"known_bug"`
	// KnownBugReason says what is wrong, for whoever reads the failure.
	KnownBugReason string `json:"known_bug_reason"`
}

func runAcceptanceCase(t *testing.T, name, repoRoot string) {
	t.Helper()

	src := filepath.Join("testdata", "acceptance", name)
	opts := readAcceptanceOptions(t, src)

	mod := t.TempDir()
	if *flagKeep != "" {
		mod = filepath.Join(*flagKeep, name)
		require.NoError(t, os.RemoveAll(mod))
		require.NoError(t, os.MkdirAll(mod, 0o755))
	}

	writeAcceptanceModule(t, mod, src, repoRoot)

	// Cases that reproduce a bug are expected to fail. The shared suite would
	// only add noise to a failure that is already understood.
	if opts.KnownBug == "" {
		writeContractSuite(t, mod)
	}

	appDir := opts.AppDir
	if appDir == "" {
		appDir = "app"
	}
	genPkg := opts.GenPkg
	if genPkg == "" {
		genPkg = "datapagesgen"
	}

	// An app package may import the generated httperr sentinels. It cannot do
	// that before they exist. Their content does not depend on the model.
	// They are written before the package is read.
	writeHTTPErrPkg(t, filepath.Join(mod, genPkg))

	app, errs := parser.Parse(filepath.Join(mod, appDir))
	for _, err := range errs.All() {
		t.Errorf("parser: %v", err)
	}
	require.Zero(t, errs.Len())
	require.NotNil(t, app, "parser returned nil model")

	genImport := acceptanceModule + "/" + genPkg
	require.NoError(t, generator.Generate(
		filepath.Join(mod, genPkg), genPkg, app, 0o644,
		generator.Options{
			Prometheus:      opts.Prometheus,
			AssetsURLPrefix: opts.AssetsURLPrefix,
			AssetsDir:       opts.AssetsDir,
			AppDir:          appDir,
			GenImport:       genImport,
		},
	))

	if opts.Cmd {
		require.NoError(t, generator.GenerateCmd(
			filepath.Join(mod, "cmd", "server"),
			acceptanceModule+"/"+appDir, genImport, genPkg,
			opts.Prometheus, app.Session != nil, 0o644,
		))
	}

	profile := filepath.Join(mod, "cover.out")
	args := []string{
		"test", "-count=1",
		"-coverpkg=./" + genPkg + "/...", "-coverprofile=" + profile, "./...",
	}
	if !opts.NoRace {
		args = append(args, "-race")
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()

	if opts.KnownBug != "" {
		require.Error(t, err,
			"%s passes now; remove known_bug from its options.json", name)
		require.Contains(t, string(out), opts.KnownBug,
			"%s fails differently than recorded:\n%s",
			name, strings.TrimSpace(string(out)))
		t.Logf("known generator bug: %s", opts.KnownBugReason)
		return
	}
	require.NoError(t, err, "%s", strings.TrimSpace(string(out)))

	coverage.add(t, name, profile)
}

// writeContractSuite copies the shared conformance suite into a case module.
//
// It holds the assertions that apply to every generated server regardless of
// the application. The server is generated per model, which means the same
// assertion runs against a different piece of code in each case.
func writeContractSuite(t *testing.T, mod string) {
	t.Helper()
	const name = "contract_test.go"
	b, err := os.ReadFile(filepath.Join("testdata", "acceptance", "_contract", name))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mod, name), b, 0o644))
}

// writeHTTPErrPkg writes the httperr subpackage of a generated package.
//
// SPECIFICATION.md tells an action handler to return the sentinels this
// package holds. The app package therefore imports generated code. The package
// is the same for every model. Writing it before the app package is parsed
// costs nothing and lets a case use the sentinels the way an application
// does.
func writeHTTPErrPkg(t *testing.T, genDir string) {
	t.Helper()
	var w generator.Writer
	w.WritePkgHTTPErr()
	dir := filepath.Join(genDir, "httperr")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t,
		os.WriteFile(filepath.Join(dir, "httperr_gen.go"), w.Buf, 0o644))
}

func readAcceptanceOptions(t *testing.T, src string) acceptanceOptions {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(src, "options.json"))
	if os.IsNotExist(err) {
		return acceptanceOptions{}
	}
	require.NoError(t, err)
	var opts acceptanceOptions
	require.NoError(t, json.Unmarshal(b, &opts), "parsing options.json")
	return opts
}

// writeAcceptanceModule copies a case into its own module.
func writeAcceptanceModule(t *testing.T, mod, src, repoRoot string) {
	t.Helper()

	require.NoError(t, os.CopyFS(mod, os.DirFS(src)))
	require.NoError(t, os.RemoveAll(filepath.Join(mod, "options.json")))

	writeModuleFiles(t, mod, acceptanceModule, repoRoot)
}

// --- coverage of the generated code ----------------------------------------

// coverage accumulates one coverage profile per acceptance case.
//
// Every case module carries the same module path. The profiles therefore
// collide on file name while describing different generated code. Merging keys
// each block by the case it came from.
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
// prefixes every file name with the case, which keeps blocks of different
// cases apart once they share one merged profile.
//
// "go test ./..." runs one test binary per package and each of them reports
// every block of every package named by -coverpkg. The same block therefore
// appears once per binary. Blocks are merged by location the way "go tool
// cover" merges them. Counts add up and statements are counted once.
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
	fmt.Fprintf(w, "\ncoverage of generated code (acceptance suites):\n")
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
