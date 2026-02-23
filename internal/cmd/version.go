package cmd

import (
	"flag"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

func runVersion(args []string, stdout io.Writer, version, commit, buildDate string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	full := fs.Bool("full", false, "print full build information")
	_ = fs.Parse(args)

	if !*full {
		_, _ = fmt.Fprintln(stdout, version)
		return
	}

	_, _ = fmt.Fprintf(stdout, "datapages %s\n", version)
	_, _ = fmt.Fprintf(stdout, "  commit: %s\n", commit)
	_, _ = fmt.Fprintf(stdout, "  built:  %s\n", buildDate)
	_, _ = fmt.Fprintf(stdout, "  go:     %s\n", runtime.Version())
	_, _ = fmt.Fprintf(stdout, "  os:     %s/%s\n", runtime.GOOS, runtime.GOARCH)

	if info, ok := debug.ReadBuildInfo(); ok {
		_, _ = fmt.Fprintln(stdout, "\ndependencies:")
		for _, dep := range info.Deps {
			if dep.Replace != nil {
				_, _ = fmt.Fprintf(stdout, "  %s %s => %s %s\n",
					dep.Path, dep.Version, dep.Replace.Path, dep.Replace.Version)
			} else {
				_, _ = fmt.Fprintf(stdout, "  %s %s\n", dep.Path, dep.Version)
			}
		}
	}
}
