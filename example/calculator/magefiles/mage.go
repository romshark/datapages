package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// MaestroTest builds and starts the calculator server,
// runs Maestro flows against it, then stops the server.
func MaestroTest() error {
	fmt.Println("==> go build ./cmd/server")
	if err := run("go", "build", "-o", "server", "./cmd/server"); err != nil {
		return err
	}
	defer os.Remove("server")

	fmt.Println("==> starting server on localhost:8080")
	cmd := exec.Command("./server", "-host", "localhost:8080")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Wait for server readiness.
	for range 50 {
		resp, err := http.Get("http://localhost:8080")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("==> maestro test .maestro/")
	return run("maestro", "test", ".maestro/")
}

// BuildDesktop builds the Wails v2 desktop app.
// On macOS it produces Calculator.app, on other platforms a plain binary.
func BuildDesktop() error {
	if runtime.GOOS == "darwin" {
		return buildMacOSApp("Calculator.app", "calculator", "./cmd/desktop",
			"cmd/desktop/Info.plist", "-tags", "desktop,production")
	}
	fmt.Println("==> go build ./cmd/desktop")
	return run("go", "build", "-tags", "desktop,production",
		"-o", "calculator-desktop", "./cmd/desktop")
}

// RunDesktop builds and runs the Wails v2 desktop app.
func RunDesktop() error {
	if err := BuildDesktop(); err != nil {
		return err
	}
	defer cleanupBuild("Calculator.app", "calculator-desktop")
	return runApp("Calculator.app", "calculator-desktop")
}

func runApp(macOSApp, binary string) error {
	if runtime.GOOS == "darwin" {
		fmt.Printf("==> open %s\n", macOSApp)
		bin := filepath.Join(macOSApp, "Contents", "MacOS", "calculator")
		cmd := exec.Command(bin)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	fmt.Printf("==> ./%s\n", binary)
	cmd := exec.Command("./" + binary)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cleanupBuild(macOSApp, binary string) {
	_ = os.RemoveAll(macOSApp)
	_ = os.Remove(binary)
}

func buildMacOSApp(
	appName, binName, pkg, plistPath string, extraFlags ...string,
) error {
	macOS := filepath.Join(appName, "Contents", "MacOS")

	_ = os.RemoveAll(appName)
	if err := os.MkdirAll(macOS, 0o755); err != nil {
		return err
	}

	args := []string{"build"}
	args = append(args, extraFlags...)
	args = append(args, "-o", filepath.Join(macOS, binName), pkg)
	fmt.Printf("==> go %s\n", args)
	if err := run(append([]string{"go"}, args...)...); err != nil {
		return err
	}

	fmt.Println("==> copying Info.plist")
	plist, err := os.ReadFile(plistPath)
	if err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(appName, "Contents", "Info.plist"), plist, 0o644,
	)
}

func run(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
