package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The watch command writes one record per module with the PID,
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

// CheckWatch fails when a datapages watch is running.
//
// Templ generation deletes the temporary template files used by watch mode.
func CheckWatch() error { return checkNoWatch("templ generate") }

// checkNoWatch reports active watches and the operation that conflicts with them.
func checkNoWatch(what string) error {
	dir := watchLockDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		// A directory that is missing or unreadable is no evidence of a watch.
		return nil
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

func describeWatchLock(content []byte) string {
	fields := strings.Split(strings.TrimSpace(string(content)), "\t")
	for len(fields) < 3 {
		fields = append(fields, "?")
	}
	return fmt.Sprintf("%s (pid %s, %s)", fields[1], fields[0], fields[2])
}
