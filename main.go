package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/romshark/datapages/internal/cmd"
)

// Set by goreleaser via -ldflags -X.
var version, commit, date string

func main() {
	// When built with "go install" the ldflags are not set.
	// Fall back to the build info embedded by the Go toolchain.
	if version == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			version = info.Main.Version
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					commit = s.Value
				case "vcs.time":
					date = s.Value
				}
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()
	c := cmd.Run(ctx, os.Args, os.Stdin, os.Stdout, os.Stderr, version, commit, date)
	os.Exit(c)
}
