//go:build mage

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

// TestUIWorkflows builds and starts the calculator server,
// runs Maestro flows against it, then stops the server.
func TestUIWorkflows() error {
	fmt.Println("==> go build ./cmd/server")
	if err := run("go", "build", "-o", "server", "./cmd/server"); err != nil {
		return err
	}
	defer func() {
		if err := os.Remove("server"); err != nil {
			log.Printf("ERR: removing server executable: %v", err)
		}
	}()

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
	ready := false
	for range 50 {
		resp, err := http.Get("http://localhost:8080")
		if err == nil {
			_ = resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		return fmt.Errorf("server did not become ready within 5s")
	}

	fmt.Println("==> maestro test .maestro/")
	return run("maestro", "test", ".maestro/")
}

// BuildDesktop builds the desktop app.
func BuildDesktop() error {
	if err := Gen(); err != nil {
		return err
	}
	fmt.Println("==> go build -o calculator .")
	return run("go", "build", "-o", "calculator", ".")
}

// RunDesktop runs the desktop app.
func RunDesktop() error {
	if err := Gen(); err != nil {
		return err
	}
	fmt.Println("==> go run .")
	return run("go", "run", ".")
}

// RunServer builds and runs the server. Press Ctrl+C to stop it.
func RunServer() error {
	if err := Gen(); err != nil {
		return err
	}

	fmt.Println("==> go build ./cmd/server")
	if err := run("go", "build", "-o", "server", "./cmd/server"); err != nil {
		return err
	}
	defer func() {
		if err := os.Remove("server"); err != nil {
			log.Printf("ERR: removing server executable: %v", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Println("==> starting server on localhost:8080")
	cmd := exec.CommandContext(ctx, "./server", "-host", "localhost:8080")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil && ctx.Err() != nil {
		return nil // interrupted by Ctrl+C
	} else if err != nil {
		return fmt.Errorf("running server: %w", err)
	}
	return nil
}

// Dev runs datapages watch. Press Ctrl+C to stop.
func Dev() error {
	fmt.Println("==> datapages watch")
	return run("datapages", "watch")
}

func Gen() error {
	fmt.Println("==> templ generate")
	if err := run("templ", "generate"); err != nil {
		return err
	}
	fmt.Println("==> datapages gen")
	return run("datapages", "gen")
}

func run(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
