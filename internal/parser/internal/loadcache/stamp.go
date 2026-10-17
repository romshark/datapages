package loadcache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	goparser "go/parser"
	"go/token"
	"hash"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// entry is 1 input of a load. [stampValue] computes Value from Key:
//
//	"exe"          the running executable
//	"env"          the environment the go command reads
//	"go"           the go command on PATH
//	"file:<path>"  the content of a file, "-" when it is missing
//	"dir:<path>"   the files of a package directory as go list reads them
//	"isdir:<path>" whether a directory exists
type entry struct {
	Key, Value string
}

func stampValue(key string) (string, error) {
	kind, arg, _ := strings.Cut(key, ":")
	switch kind {
	case "exe":
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		// The path reads as the program was started, ../datapages for one.
		if exe, err = filepath.EvalSymlinks(exe); err != nil {
			return "", err
		}
		return fileID(exe)
	case "env":
		return envValue(), nil
	case "go":
		bin, err := exec.LookPath("go")
		if err != nil {
			return "-", nil
		}
		if bin, err = filepath.EvalSymlinks(bin); err != nil {
			return "", err
		}
		return fileID(bin)
	case "file":
		data, err := os.ReadFile(arg)
		if errors.Is(err, fs.ErrNotExist) {
			return "-", nil
		}
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), nil
	case "dir":
		return dirValue(arg)
	case "isdir":
		if st, err := os.Stat(arg); err == nil && st.IsDir() {
			return "1", nil
		}
		return "-", nil
	}
	return "", fmt.Errorf("unknown stamp key %q", key)
}

// fileID identifies a replaced file by path, size and modification time.
func fileID(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return path + " " + strconv.FormatInt(st.Size(), 10) + " " +
		strconv.FormatInt(st.ModTime().UnixNano(), 10), nil
}

// envValue returns load-related environment variables and omits runtime tuning.
func envValue() string {
	var vars []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "GOGC", "GOMEMLIMIT", "GOMAXPROCS", "GOTRACEBACK", "GOPACKAGESDEBUG":
			continue
		case "CC", "CXX", "FC", "AR", "PKG_CONFIG",
			"PATH", "HOME", "XDG_CONFIG_HOME", "APPDATA":
		default:
			if !strings.HasPrefix(name, "GO") && !strings.HasPrefix(name, "CGO_") {
				continue
			}
		}
		vars = append(vars, kv)
	}
	slices.Sort(vars)
	return strings.Join(vars, "\x00")
}

// goEnvFile returns the file written by "go env -w".
func goEnvFile() string {
	if f, ok := os.LookupEnv("GOENV"); ok {
		return f
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "go", "env")
}

// goListExt holds the extensions of the files go/build assigns to a package.
// It ignores every other file.
var goListExt = map[string]bool{
	".go": true, ".c": true, ".cc": true, ".cxx": true, ".cpp": true, ".m": true,
	".h": true, ".hh": true, ".hpp": true, ".hxx": true, ".f": true, ".F": true,
	".for": true, ".f90": true, ".s": true, ".S": true, ".sx": true,
	".swig": true, ".swigcxx": true, ".syso": true,
}

// dirValue hashes what go list reads from a package directory.
// It covers file names, non-Go content and Go headers through the last import.
//
// A file importing "embed" is hashed whole along with the directory tree that
// determines its pattern matches.
func dirValue(dir string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	embeds := false
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if name == "go.mod" {
			// A go.mod moves the directory out of the module.
			_, _ = fmt.Fprintf(h, "go.mod\n")
			continue
		}
		ext := filepath.Ext(name)
		if strings.HasPrefix(name, ".") ||
			strings.HasPrefix(name, "_") ||
			!goListExt[ext] {
			continue
		}
		_, _ = fmt.Fprintf(h, "%s %v\n", name, e.Type())
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if ext != ".go" {
			writeSum(h, src)
			continue
		}
		header, embed, err := goHeader(path, src)
		if err != nil {
			return "", err
		}
		if embed {
			embeds = true
			writeSum(h, src)
			continue
		}
		writeSum(h, header)
	}
	if embeds {
		if err := writeTree(h, dir); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeSum(h hash.Hash, data []byte) {
	sum := sha256.Sum256(data)
	h.Write(sum[:])
}

// goHeader returns the part read by go list and whether it imports embed.
func goHeader(path string, src []byte) (header []byte, embed bool, err error) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, path, src,
		goparser.ImportsOnly|goparser.ParseComments|goparser.SkipObjectResolution)
	if err != nil {
		return nil, false, err
	}
	end := f.Name.End()
	for _, d := range f.Decls {
		end = max(end, d.End())
	}
	for _, imp := range f.Imports {
		if imp.Path.Value == `"embed"` {
			embed = true
		}
	}
	return src[:fset.File(f.Package).Offset(end)], embed, nil
}

// writeTree hashes //go:embed inputs without crossing module boundaries.
func writeTree(h hash.Hash, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		_, _ = fmt.Fprintf(h, "%s %v\n", filepath.ToSlash(rel), d.Type())
		if d.IsDir() {
			if _, err := os.Lstat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		return nil
	})
}
