package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The watch command writes 1 record per module with the PID,
// module directory, and development server host separated by tabs.
// watchLockTTL must match the writer's timeout.
//
// Modification times provide the same liveness check on Linux, macOS,
// and Windows without process lookup or signals.
const watchLockTTL = 10 * time.Second

func watchLockDir() string {
	if dir := os.Getenv("DATAPAGES_WATCH_LOCK_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "datapages-watch")
}

// CheckWatch fails when a datapages watch runs on a module of this repository.
//
// Templ generation deletes the temporary template files used by watch mode.
func CheckWatch() error { return checkNoWatch("templ generate") }

// checkNoWatch reports the active watches that conflict with the operation.
//
// templ generate deletes the watch mode files of the templates under the
// directory it walks, and no others: each file name is a hash of the path of
// its template. A watch conflicts when its module and the repository overlap.
// A watch of another repository does not.
func checkNoWatch(what string) error {
	dir := watchLockDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		// A directory that is missing or unreadable is no evidence of a watch.
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	var running []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".lock" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if time.Since(info.ModTime()) > watchLockTTL {
			// SIGKILL can leave a stale lock behind.
			_ = os.Remove(path)
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// A record without a module directory may be of any module.
		if module := watchLockModule(content); module != "" &&
			!within(root, module) && !within(module, root) {
			continue
		}
		running = append(running, describeWatchLock(content))
	}
	if len(running) == 0 {
		return nil
	}
	return fmt.Errorf("cannot run %s while datapages watch is active:\n"+
		"  %s\n"+
		"stop each listed watch; stale locks expire after %s in %s",
		what, strings.Join(running, "\n  "), watchLockTTL, dir)
}

// watchLockModule returns the module directory of a lock record,
// "" when the record has none.
func watchLockModule(content []byte) string {
	fields := strings.Split(strings.TrimSpace(string(content)), "\t")
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// within reports whether path is dir or lies under it.
// Symbolic links are resolved first, which matches 2 spellings of 1 directory,
// such as /var/... and /private/var/... on macOS.
func within(dir, path string) bool {
	rel, err := filepath.Rel(resolve(dir), resolve(path))
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve returns path with its symbolic links resolved,
// or path unchanged when they cannot be.
func resolve(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return filepath.Clean(path)
}

func describeWatchLock(content []byte) string {
	fields := strings.Split(strings.TrimSpace(string(content)), "\t")
	for len(fields) < 3 {
		fields = append(fields, "?")
	}
	return fmt.Sprintf("%s (pid %s, %s)", fields[1], fields[0], fields[2])
}
