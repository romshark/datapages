package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	docspage "github.com/romshark/datapages/docs-src"
)

func main() {
	var version, out string
	flag.StringVar(&version, "version", "", "Version string to render")
	flag.StringVar(&out, "out", "docs/index.html", "Output path")
	flag.Parse()

	if version == "" {
		fmt.Fprintln(os.Stderr, "missing required -version")
		os.Exit(2)
	}

	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create output file: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	if err := docspage.Index(version).Render(context.Background(), f); err != nil {
		fmt.Fprintf(os.Stderr, "render docs page: %v\n", err)
		os.Exit(1)
	}
}
