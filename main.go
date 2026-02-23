package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/romshark/datapages/internal/cmd"
)

// Set by goreleaser via -ldflags -X.
var version, commit, date string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()
	c := cmd.Run(ctx, os.Args, os.Stdout, os.Stderr, version, commit, date)
	os.Exit(c)
}
