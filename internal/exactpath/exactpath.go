// Package exactpath reports whether a path spells its elements as their
// directories list them. NTFS and APFS also open other spellings.
package exactpath

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
)

// Clean returns p cleaned and without its leading slash, the form [Listings.Exact] takes.
// It allocates only for a p that is not clean.
func Clean(p string) string {
	rel := strings.TrimPrefix(p, "/")
	if path.Clean(rel) == rel && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, "/") && !strings.HasPrefix(rel, "../") {
		return rel
	}
	return path.Clean("/" + rel)[1:]
}

// Listings caches the names in the directories of a file system.
type Listings struct {
	fsys http.FileSystem
	// dirs maps an exact directory path to its names, nil when the directory
	// cannot be listed. Exact keys bound it by the directories of fsys.
	dirs sync.Map
}

// New returns the listings of fsys, each read on first use.
func New(fsys http.FileSystem) *Listings { return &Listings{fsys: fsys} }

// Exact reports whether each element of p, a clean path relative to the root,
// appears in the listing of its directory. An element in a directory that
// cannot be listed counts as exact. The empty path is the root.
func (l *Listings) Exact(p string) bool {
	for start := 0; start < len(p); {
		end := strings.IndexByte(p[start:], '/')
		if end < 0 {
			end = len(p)
		} else {
			end += start
		}
		names := l.names(p[:max(start-1, 0)])
		if names == nil {
			return true
		}
		if _, ok := names[p[start:end]]; !ok {
			return false
		}
		start = end + 1
	}
	return true
}

// names returns the names in dir, nil when dir cannot be listed.
func (l *Listings) names(dir string) map[string]struct{} {
	if v, ok := l.dirs.Load(dir); ok {
		return v.(map[string]struct{})
	}
	v, _ := l.dirs.LoadOrStore(dir, l.list(dir))
	return v.(map[string]struct{})
}

func (l *Listings) list(dir string) map[string]struct{} {
	f, err := l.fsys.Open("/" + dir)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	// ReadDir skips the stat of each entry that Readdir takes.
	if d, ok := f.(fs.ReadDirFile); ok {
		return nameSet(d.ReadDir(-1))
	}
	return nameSet(f.Readdir(-1))
}

func nameSet[E interface{ Name() string }](entries []E, err error) map[string]struct{} {
	if err != nil {
		return nil
	}
	names := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		names[e.Name()] = struct{}{}
	}
	return names
}
