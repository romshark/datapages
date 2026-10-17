// Package loadcache repeats a go/packages load without running the go command
// or reading unchanged dependencies from source.
//
// [Cache.Store] records the package graph, load inputs and export data for
// GOROOT and module-cache packages. [Cache.Load] validates the inputs and rebuilds
// source packages. [Result.Leaked] detects positions whose export data omitted a column.
package loadcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/packages"
)

// Mode is the go/packages mode the cache repeats. NeedTypesSizes adds no go
// command work and records the sizes used by the type checker.
const Mode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedDeps |
	packages.NeedTypes |
	packages.NeedTypesInfo |
	packages.NeedTypesSizes |
	packages.NeedSyntax |
	packages.NeedModule

// ParseFunc is the ParseFile hook of a [packages.Config].
type ParseFunc = func(
	fset *token.FileSet, filename string, src []byte,
) (*ast.File, error)

const format = "v1"

// Cache is the load cache of 1 app package directory.
type Cache struct {
	dir   string // absolute app package directory
	base  string
	debug bool
}

// Open returns the cache for dir. DATAPAGES_LOADCACHE may disable it or name
// its directory, which defaults to datapages/load in [os.UserCacheDir].
// A GOPACKAGESDRIVER also disables it.
func Open(dir string) *Cache {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	// go/packages runs a driver named by GOPACKAGESDRIVER or found on PATH
	// in place of go list, whose inputs the cache does not know.
	if d, ok := os.LookupEnv("GOPACKAGESDRIVER"); ok && d != "off" {
		return nil
	} else if !ok {
		if _, err := exec.LookPath("gopackagesdriver"); err == nil {
			return nil
		}
	}
	base := os.Getenv("DATAPAGES_LOADCACHE")
	switch {
	case base == "off":
		return nil
	case base == "":
		ucd, err := os.UserCacheDir()
		if err != nil {
			return nil
		}
		base = filepath.Join(ucd, "datapages", "load")
	case !filepath.IsAbs(base):
		return nil
	}
	return &Cache{
		dir:   abs,
		base:  filepath.Join(base, format),
		debug: os.Getenv("DATAPAGES_LOADCACHE_DEBUG") != "",
	}
}

type meta struct {
	Dir     string
	Stamp   []entry
	Roots   []string // GOROOT and GOMODCACHE with a trailing separator
	Arch    string   // types.SizesFor("gc", Arch) is what go/packages used
	Warn    string   // what go/packages appends to the errors of a package
	Modules []packages.Module
	Pkgs    []pkgMeta // a package after the ones it imports, the app last
}

type pkgMeta struct {
	ID, Name, PkgPath, Dir string

	GoFiles, CompiledGoFiles, OtherFiles, IgnoredFiles []string

	Imports  map[string]int // import path as written -> index in meta.Pkgs
	Module   int            // index in meta.Modules, -1 without a module
	IllTyped bool

	// Source packages are parsed and type-checked on every load.
	// The others come from export data.
	Source bool

	// Export is the key of the export data of a package that a source package
	// imports. The others are only reached through export data.
	Export string
}

// Result describes a load from the cache.
type Result struct {
	// exportDirs maps the directory of every package read from export data,
	// with a trailing separator, to the package ID.
	exportDirs map[string]string
	// exportFiles does the same for files outside that directory, such as the
	// generated files of cgo.
	exportFiles map[string]string
}

func (r *Result) addExport(id, dir string, files ...[]string) {
	dir += string(filepath.Separator)
	r.exportDirs[dir] = id
	for _, fs := range files {
		for _, f := range fs {
			if !strings.HasPrefix(f, dir) {
				r.exportFiles[f] = id
			}
		}
	}
}

// Leaked returns export-data package IDs referenced by files or msgs.
// Their positions lack columns.
// A //line directive can name a file in the package directory.
func (r *Result) Leaked(files, msgs []string) []string {
	if r == nil {
		return nil
	}
	ids := map[string]bool{}
	for _, f := range files {
		if id, ok := r.exportFiles[f]; ok {
			ids[id] = true
		} else if id, ok := r.exportDirs[filepath.Dir(f)+string(filepath.Separator)]; ok {
			ids[id] = true
		}
	}
	for _, msg := range msgs {
		if !strings.Contains(msg, string(filepath.Separator)) {
			continue
		}
		for _, m := range []map[string]string{r.exportDirs, r.exportFiles} {
			for f, id := range m {
				if strings.Contains(msg, f) {
					ids[id] = true
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(ids))
}

func (c *Cache) metaPath() string {
	sum := sha256.Sum256([]byte(c.dir))
	return filepath.Join(c.base, "meta", hex.EncodeToString(sum[:16]))
}

func (c *Cache) exportPath(key string) string {
	return filepath.Join(c.base, "export", key[:2], key)
}

func (c *Cache) logf(format string, args ...any) {
	if c.debug {
		fmt.Fprintf(os.Stderr, "loadcache: "+format+"\n", args...)
	}
}

// Load returns the cached app package, or nil after a miss.
// parse builds the cold load's ParseFile hook for the cached roots.
func (c *Cache) Load(parse func(roots []string) ParseFunc) (*packages.Package, *Result) {
	if c == nil {
		return nil, nil
	}
	start := time.Now()
	pkg, res, err := c.load(parse)
	if err != nil {
		c.logf("miss after %v: %v", time.Since(start), err)
		return nil, nil
	}
	c.logf("hit in %v", time.Since(start))
	return pkg, res
}

func (c *Cache) load(parse func(roots []string) ParseFunc) (*packages.Package, *Result, error) {
	start := time.Now()
	m, size, err := c.readMeta()
	if err != nil {
		return nil, nil, err
	}
	decoded := time.Now()
	for _, e := range m.Stamp {
		v, err := stampValue(e.Key)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", e.Key, err)
		}
		if v != e.Value {
			return nil, nil, fmt.Errorf("%s changed", e.Key)
		}
	}
	stamped := time.Now()
	pkgs, res, st, err := c.build(m, parse, nil)
	if err != nil {
		return nil, nil, err
	}
	c.logf("meta %d bytes read in %v, stamp of %d inputs in %v, "+
		"%d packages from source in %v, %d from export data in %v,"+
		" %d reached through them",
		size, decoded.Sub(start), len(m.Stamp), stamped.Sub(decoded),
		st.nsrc, st.src, st.nexp, st.exp, len(m.Pkgs)-st.nsrc-st.nexp)
	return pkgs[len(pkgs)-1], res, nil
}

func (c *Cache) readMeta() (*meta, int, error) {
	data, err := os.ReadFile(c.metaPath())
	if err != nil {
		return nil, 0, err
	}
	var m meta
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&m); err != nil {
		return nil, 0, err
	}
	if m.Dir != c.dir {
		return nil, 0, errors.New("cache entry of another directory")
	}
	return &m, len(data), nil
}

type buildStats struct {
	nsrc, nexp int
	src, exp   time.Duration
}

// build reconstructs m.Pkgs in dependency order and type-checks source packages.
func (c *Cache) build(
	m *meta, parse func(roots []string) ParseFunc, source map[string]bool,
) ([]*packages.Package, *Result, buildStats, error) {
	var st buildStats
	sizes := types.SizesFor("gc", m.Arch)
	if sizes == nil {
		return nil, nil, st, fmt.Errorf("unknown architecture %q", m.Arch)
	}
	parseFile := parse(m.Roots)
	if parseFile == nil {
		return nil, nil, st, errors.New("no parser")
	}

	fset := token.NewFileSet()
	pkgs := make([]*packages.Package, len(m.Pkgs))
	imports := make(map[string]*types.Package, len(m.Pkgs))
	res := &Result{exportDirs: map[string]string{}, exportFiles: map[string]string{}}
	for i, pm := range m.Pkgs {
		p := &packages.Package{
			ID:              pm.ID,
			Name:            pm.Name,
			PkgPath:         pm.PkgPath,
			Dir:             pm.Dir,
			GoFiles:         pm.GoFiles,
			CompiledGoFiles: pm.CompiledGoFiles,
			OtherFiles:      pm.OtherFiles,
			IgnoredFiles:    pm.IgnoredFiles,
			Imports:         make(map[string]*packages.Package, len(pm.Imports)),
			Fset:            fset,
			IllTyped:        pm.IllTyped,
			TypesSizes:      sizes,
		}
		if pm.Module >= 0 {
			mod := m.Modules[pm.Module]
			p.Module = &mod
		}
		if pm.PkgPath == "unsafe" {
			p.Types = types.Unsafe
			p.Syntax = []*ast.File{}
			p.TypesInfo = newTypesInfo()
		} else {
			p.Types = types.NewPackage(pm.PkgPath, pm.Name)
		}
		imports[pm.PkgPath] = p.Types
		if !pm.Source && !source[pm.ID] {
			res.addExport(pm.ID, pm.Dir, pm.GoFiles, pm.CompiledGoFiles)
		}
		pkgs[i] = p
	}
	for i, pm := range m.Pkgs {
		for path, j := range pm.Imports {
			pkgs[i].Imports[path] = pkgs[j]
		}
	}

	// Dependencies come first. A source package is therefore checked before
	// any export data that refers to it is read, which then reuses its objects.
	for i, pm := range m.Pkgs {
		p := pkgs[i]
		t := time.Now()
		switch {
		case pm.PkgPath == "unsafe":
		case pm.Source || source[pm.ID]:
			st.nsrc++
			if err := checkSource(p, parseFile, sizes, m.Warn); err != nil {
				return nil, nil, st, err
			}
			st.src += time.Since(t)
		case pm.Export != "":
			st.nexp++
			data, err := os.ReadFile(c.exportPath(pm.Export))
			if err != nil {
				return nil, nil, st, err
			}
			if _, err := gcexportdata.Read(
				bytes.NewReader(data), fset, imports, pm.PkgPath,
			); err != nil {
				return nil, nil, st, fmt.Errorf("export data of %s: %w", pm.PkgPath, err)
			}
			st.exp += time.Since(t)
		}
	}
	return pkgs, res, st, nil
}

// Store caches a cold load rooted at root. roots are GOROOT and GOMODCACHE.
// keep and source select packages to rebuild from source.
//
// A load the cache cannot repeat exactly is not stored. Store reports why.
func (c *Cache) Store(
	root *packages.Package, roots []string,
	keep func(*packages.Package) bool, source []string,
) {
	if c == nil {
		return
	}
	start := time.Now()
	if err := c.store(root, roots, keep, source); err != nil {
		c.logf("not stored: %v", err)
		return
	}
	c.logf("stored in %v", time.Since(start))
}

func (c *Cache) store(
	root *packages.Package, roots []string,
	keep func(*packages.Package) bool, source []string,
) error {
	if len(roots) != 2 {
		return errors.New("GOROOT or GOMODCACHE unknown")
	}
	if strings.Contains(os.Getenv("GOFLAGS"), "-modfile") ||
		strings.Contains(os.Getenv("GOFLAGS"), "-overlay") {
		return errors.New("GOFLAGS sets -modfile or -overlay")
	}
	underRoots := func(path string) bool {
		return slices.ContainsFunc(roots, func(r string) bool {
			return strings.HasPrefix(path, r)
		})
	}

	var order []*packages.Package
	index := map[*packages.Package]int{}
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if _, ok := index[p]; ok {
			return
		}
		index[p] = -1
		for _, path := range slices.Sorted(maps.Keys(p.Imports)) {
			visit(p.Imports[path])
		}
		index[p] = len(order)
		order = append(order, p)
	}
	visit(root)

	arch := sizesArch(root.TypesSizes)
	if arch == "" {
		return errors.New("type sizes of no gc architecture")
	}
	warn, err := versionWarning(roots[0])
	if err != nil {
		return err
	}

	extra := map[string]bool{}
	for _, id := range source {
		extra[id] = true
	}
	m := meta{Dir: c.dir, Roots: roots, Arch: arch, Warn: warn}
	type modKey struct{ path, version, dir string }
	modIndex := map[modKey]int{}
	isSource := make([]bool, len(order))
	for i, p := range order {
		if p.Dir == "" {
			return fmt.Errorf("%s has no directory", p.ID)
		}
		isSource[i] = p == root || !underRoots(p.Dir+string(filepath.Separator)) ||
			extra[p.ID] || (p.PkgPath != "unsafe" && keep(p))
		if isSource[i] && !slices.Equal(p.CompiledGoFiles, p.GoFiles) {
			// go list generates the compiled files of a cgo package.
			return fmt.Errorf("%s compiles generated files", p.ID)
		}
		if err := checkErrors(p, isSource[i], warn); err != nil {
			return err
		}
		pm := pkgMeta{
			ID:              p.ID,
			Name:            p.Name,
			PkgPath:         p.PkgPath,
			Dir:             p.Dir,
			GoFiles:         p.GoFiles,
			CompiledGoFiles: p.CompiledGoFiles,
			OtherFiles:      p.OtherFiles,
			IgnoredFiles:    p.IgnoredFiles,
			Imports:         make(map[string]int, len(p.Imports)),
			Module:          -1,
			IllTyped:        p.IllTyped,
			Source:          isSource[i],
		}
		for path, imp := range p.Imports {
			pm.Imports[path] = index[imp]
		}
		if p.Module != nil {
			k := modKey{p.Module.Path, p.Module.Version, p.Module.Dir}
			mi, ok := modIndex[k]
			if !ok {
				mi = len(m.Modules)
				modIndex[k] = mi
				m.Modules = append(m.Modules, *p.Module)
			}
			pm.Module = mi
		}
		m.Pkgs = append(m.Pkgs, pm)
	}

	// A package from export data that imports a source package outside the
	// roots holds types of a version of it that may since have changed.
	for i, p := range order {
		if isSource[i] {
			continue
		}
		for _, imp := range p.Imports {
			if isSource[index[imp]] && !underRoots(imp.Dir+string(filepath.Separator)) {
				return fmt.Errorf("%s from export data imports %s, which may change",
					p.ID, imp.ID)
			}
		}
	}

	stampEntries, err := c.stampKeys(root, order, roots, underRoots)
	if err != nil {
		return err
	}
	for _, key := range stampEntries {
		v, err := stampValue(key)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		m.Stamp = append(m.Stamp, entry{Key: key, Value: v})
	}

	keys := make([]string, len(order))
	base := sha256.New()
	for _, e := range m.Stamp {
		if e.Key == "exe" || e.Key == "env" || e.Key == "go" {
			_, _ = fmt.Fprintf(base, "%s=%s\n", e.Key, e.Value)
		}
	}
	_, _ = fmt.Fprintf(base, "roots=%q arch=%s\n", roots, arch)
	baseSum := base.Sum(nil)
	// The key of a package covers the keys of its imports, whose types its
	// export data holds. A package outside the roots gets a key too, which no
	// export data uses: none imports it.
	for i, p := range order {
		h := sha256.New()
		h.Write(baseSum)
		_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n%s\n", p.ID, p.PkgPath, p.Name, p.Dir)
		if p.Module != nil {
			_, _ = fmt.Fprintf(h, "%s@%s\n", p.Module.Path, p.Module.Version)
		}
		for _, f := range p.CompiledGoFiles {
			_, _ = fmt.Fprintf(h, "%s\n", f)
		}
		for _, path := range slices.Sorted(maps.Keys(p.Imports)) {
			_, _ = fmt.Fprintf(h, "%s=%s\n", path, keys[index[p.Imports[path]]])
		}
		keys[i] = hex.EncodeToString(h.Sum(nil))
	}
	for i, p := range order {
		if !isSource[i] {
			continue
		}
		for _, imp := range p.Imports {
			j := index[imp]
			if isSource[j] || imp.PkgPath == "unsafe" || m.Pkgs[j].Export != "" {
				continue
			}
			if err := c.writeExport(keys[j], imp); err != nil {
				return err
			}
			m.Pkgs[j].Export = keys[j]
		}
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&m); err != nil {
		return err
	}
	return writeFileAtomic(c.metaPath(), buf.Bytes())
}

// stampKeys returns the executable, environment and go-list inputs.
func (c *Cache) stampKeys(
	root *packages.Package, order []*packages.Package,
	roots []string, underRoots func(string) bool,
) ([]string, error) {
	keys := []string{"exe", "env", "go"}
	if f := goEnvFile(); f != "" {
		keys = append(keys, "file:"+f)
	}
	keys = append(keys, "file:"+filepath.Join(roots[0], "go.env"))
	// The go command takes the module and the workspace from the closest
	// directory above holding a go.mod and a go.work.
	for d := c.dir; ; d = filepath.Dir(d) {
		for _, name := range []string{"go.mod", "go.work", "go.work.sum"} {
			keys = append(keys, "file:"+filepath.Join(d, name))
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	mods := map[string]bool{}
	for _, p := range order {
		if p.Module != nil && p.Module.Dir != "" && !mods[p.Module.Dir] {
			mods[p.Module.Dir] = true
			if underRoots(p.Module.Dir + string(filepath.Separator)) {
				keys = append(keys, "isdir:"+p.Module.Dir)
			} else {
				keys = append(keys,
					"file:"+p.Module.GoMod,
					"file:"+filepath.Join(p.Module.Dir, "go.sum"),
					"file:"+filepath.Join(p.Module.Dir, "vendor", "modules.txt"))
			}
		}
		if !underRoots(p.Dir + string(filepath.Separator)) {
			keys = append(keys, "dir:"+p.Dir)
		}
	}
	if root.Module == nil {
		return nil, errors.New("app package outside a module")
	}
	return keys, nil
}

// checkErrors rejects errors the cached load cannot reproduce. Source packages
// may have type errors. Export packages may have only soft, bodiless errors.
func checkErrors(p *packages.Package, source bool, warn string) error {
	types := 0
	for _, e := range p.Errors {
		switch {
		case e.Kind == packages.TypeError || e.Kind == packages.ParseError && source:
			types++
		case e.Kind == packages.UnknownError && e.Msg == warn && warn != "":
		default:
			return fmt.Errorf("%s: %s error: %s", p.ID, kindName(e.Kind), e.Msg)
		}
	}
	if source {
		return nil
	}
	if types != len(p.TypeErrors) {
		return fmt.Errorf("%s: %d errors from export data", p.ID, types)
	}
	for _, e := range p.TypeErrors {
		if !e.Soft {
			return fmt.Errorf("%s: type error: %s", p.ID, e.Msg)
		}
	}
	return nil
}

func kindName(k packages.ErrorKind) string {
	switch k {
	case packages.ListError:
		return "list"
	case packages.ParseError:
		return "parse"
	case packages.TypeError:
		return "type"
	}
	return "unknown"
}

func (c *Cache) writeExport(key string, p *packages.Package) error {
	path := c.exportPath(key)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := gcexportdata.Write(&buf, p.Fset, p.Types); err != nil {
		return fmt.Errorf("export data of %s: %w", p.PkgPath, err)
	}
	return writeFileAtomic(path, buf.Bytes())
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), path)
}

// sizesArch returns the gc architecture whose sizes equal s.
func sizesArch(s types.Sizes) string {
	for _, arch := range []string{
		"386", "amd64", "amd64p32", "arm", "arm64", "loong64", "mips", "mipsle",
		"mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x", "sparc64", "wasm",
	} {
		if s != nil && types.SizesFor("gc", arch) == s {
			return arch
		}
	}
	return ""
}

// versionWarning returns the go/packages warning for a newer go command.
func versionWarning(goroot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(goroot, "VERSION"))
	if err != nil {
		return "", fmt.Errorf("version of the go command: %w", err)
	}
	var goVersion int
	if _, err := fmt.Sscanf(string(data), "go1.%d", &goVersion); err != nil {
		return "", fmt.Errorf("version of the go command: %q", data)
	}
	var runtimeVersion int
	if _, err := fmt.Sscanf(runtime.Version(), "go1.%d", &runtimeVersion); err != nil ||
		runtimeVersion >= goVersion {
		return "", nil
	}
	return fmt.Sprintf("This application uses version go1.%d of the source-processing"+
		" packages but runs version go1.%d of 'go list'. It may fail to process "+
		"source files that rely on newer language features. If so, rebuild "+
		"the application using a newer version of Go.", runtimeVersion, goVersion), nil
}
