package cmd

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func newVersionCmd(stdout io.Writer, version, commit, buildDate string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long: `Print the datapages CLI version. Use --full to include commit hash,
build date, Go version, and module dependencies.`,
	}
	full := cmd.Flags().Bool("full", false, "Print full build information")
	cmd.Run = func(c *cobra.Command, args []string) {
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
	return cmd
}
