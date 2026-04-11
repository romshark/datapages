package cmd

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// wantColorFor reports whether w should receive ANSI color escapes.
// It honors NO_COLOR (https://no-color.org) and FORCE_COLOR /
// CLICOLOR_FORCE (https://bixense.com/clicolors) so that parent
// processes like templier can opt into color output over a pipe.
func wantColorFor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") == "1" {
		return true
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
