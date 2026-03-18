package main

import (
	"context"
	"fmt"
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

// RunServer starts NATS via docker compose, then builds and runs the server.
// Press Ctrl+C to stop both the server and NATS.
func RunServer() error {
	if err := Gen(); err != nil {
		return err
	}

	fmt.Println("==> docker compose up -d")
	if err := run("docker", "compose", "up", "-d"); err != nil {
		return fmt.Errorf("starting docker compose: %w", err)
	}
	defer func() {
		fmt.Println("==> docker compose down")
		_ = run("docker", "compose", "down")
	}()

	fmt.Println("==> go build ./cmd/server")
	if err := run("go", "build", "-o", "server", "./cmd/server"); err != nil {
		return err
	}
	defer os.Remove("server")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if os.Getenv("HMAC_SECRET_KEY") == "" {
		os.Setenv("HMAC_SECRET_KEY", "dev-secret")
	}

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
