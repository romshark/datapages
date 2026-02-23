package main

import (
	"os"

	"github.com/romshark/datapages/internal/cmd"
)

// Set by goreleaser via -ldflags -X.
var version, commit, date string

func main() {
	c := cmd.Run(os.Args, os.Environ(), os.Stdout, os.Stderr, version, commit, date)
	os.Exit(c)
}
