package main

import (
	"os"
	"os/exec"
	"path/filepath"
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

func output(args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	return string(out), err
}
