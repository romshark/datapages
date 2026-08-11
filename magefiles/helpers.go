package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// forEachModule finds go.mod files under root and calls fn for each directory.
func forEachModule(root string, fn func(dir string) error) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "vendor" {
			return filepath.SkipDir
		}
		if d.Name() == "go.mod" {
			return fn(filepath.Dir(path))
		}
		return nil
	})
}

func goRun(pkg string, args ...string) error {
	return run(append([]string{"go", "run", pkg}, args...)...)
}

func run(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runIn(dir string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// hasTemplFiles reports whether dir or any subdirectory contains a .templ file.
func hasTemplFiles(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".templ" {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// coverProfilePercent reports the share of covered statements in a Go coverage
// profile.
//
// The same block is listed once per test binary that reported it. Blocks are
// merged by location the way "go tool cover" merges them. Counts add up and
// statements are counted once.
func coverProfilePercent(profile string) (float64, error) {
	b, err := os.ReadFile(profile)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", profile, err)
	}

	type block struct {
		numStmt int
		count   int
	}
	blocks := map[string]block{}
	for line := range strings.Lines(string(b)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, fmt.Errorf("malformed profile line %q", line)
		}
		numStmt, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, fmt.Errorf("malformed statement count in %q: %w", line, err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return 0, fmt.Errorf("malformed hit count in %q: %w", line, err)
		}
		prev := blocks[fields[0]]
		blocks[fields[0]] = block{numStmt: numStmt, count: prev.count + count}
	}

	var total, covered int
	for _, bl := range blocks {
		total += bl.numStmt
		if bl.count > 0 {
			covered += bl.numStmt
		}
	}
	if total == 0 {
		return 0, nil
	}
	return float64(covered) / float64(total) * 100, nil
}

func output(args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	return string(out), err
}
