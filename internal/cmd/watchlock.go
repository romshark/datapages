package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	watchLockHeartbeat = 2 * time.Second

	// watchLockTTL allows five missed heartbeats before readers treat a lock as stale.
	// SIGKILL prevents the watch from removing its lock.
	watchLockTTL = 10 * time.Second

	// envWatchLockDir isolates tests and parallel CI jobs.
	envWatchLockDir = "DATAPAGES_WATCH_LOCK_DIR"
)

func watchLockDir() string {
	if dir := os.Getenv(envWatchLockDir); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "datapages-watch")
}

// acquireWatchLock creates and refreshes a module's watch record until release is called.
//
// Generators reject records newer than [watchLockTTL] because templ generation
// deletes the temporary files used by watch mode. Modification times avoid
// platform-specific process checks and PID reuse.
// Stale records expire when a watch cannot remove its file.
func acquireWatchLock(moduleDir, host string) (release func(), err error) {
	dir := watchLockDir()
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return func() {}, fmt.Errorf("creating watch lock directory: %w", err)
	}
	path := filepath.Join(dir, watchLockName(moduleDir))
	line := fmt.Appendf(nil, "%d\t%s\t%s\n", os.Getpid(), moduleDir, host)
	if err := os.WriteFile(path, line, 0o666); err != nil {
		return func() {}, fmt.Errorf("writing watch lock: %w", err)
	}

	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		t := time.NewTicker(watchLockHeartbeat)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				// Rewriting rather than touching also restores the file after
				// a second watch on the same module removed it on its way out.
				_ = os.WriteFile(path, line, 0o666)
			}
		}
	}()

	return func() {
		close(stop)
		// Stop the heartbeat before removing the file so it cannot recreate it.
		<-stopped
		_ = os.Remove(path)
	}, nil
}

// watchLockName hashes the cleaned absolute path so
// equivalent module paths share a portable file name.
func watchLockName(moduleDir string) string {
	if abs, err := filepath.Abs(moduleDir); err == nil {
		moduleDir = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(moduleDir)))
	return hex.EncodeToString(sum[:8]) + ".lock"
}

// watchLockHeld reports whether a lock file was refreshed within [watchLockTTL].
func watchLockHeld() bool {
	entries, err := os.ReadDir(watchLockDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".lock" {
			continue
		}
		info, err := e.Info()
		if err == nil && time.Since(info.ModTime()) <= watchLockTTL {
			return true
		}
	}
	return false
}
