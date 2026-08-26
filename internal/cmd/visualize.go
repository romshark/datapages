package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/romshark/datapages/internal/graph"
	"github.com/romshark/datapages/internal/serverscan"
)

func newVisualizeCmd(stdout, stderr io.Writer, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "visualize",
		Args:  cobra.NoArgs,
		Short: "Render the application model as a graph",
		Long: `Parse the application model and render it as a graph of pages, their
handlers and the events they dispatch and subscribe to.

The DOT source is written to stdout by default and needs no external tool.
HTML and SVG require Graphviz ("dot") in PATH:

  datapages visualize -o app.html   # page with a tree view next to the graph
  datapages visualize -o app.svg
  datapages visualize | dot -Tsvg > app.svg

The HTML page is self-contained and loads nothing from the network. Clicking a
page, a handler or an event in its tree highlights that item in the graph along
with everything it connects to, and its details name the file and the
line:column the app package declares it at.

The format follows the output file extension and can be set with --format.

The layout runs top to bottom: abstract pages on top, pages below them, events
in the last rank. Every dispatch points down, every subscription up. --rankdir
LR lays the same graph out left to right.

One graph shows one application. A module that builds more than one needs
--app to say which, naming the app package: "datapages visualize --app frontend".

An abstract page gets a node of its own holding the handlers it declares, and a
dashed edge to every page that embeds it. A page node holds only what the page
type itself declares.

Redirect targets and links between pages are not drawn, they are not part of
the application model.`,
	}
	app := cmd.Flags().String("app", "",
		"App package to render, required when the module builds more than one")
	output := cmd.Flags().StringP("output", "o", "",
		"File to write to, defaults to stdout")
	format := cmd.Flags().String("format", "",
		`Output format, "dot", "svg" or "html", `+
			`defaults to the output file extension`)
	rankDir := cmd.Flags().String("rankdir", "TB",
		`Layout direction, "TB" (top to bottom) or "LR" (left to right)`)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runVisualize(
			c.Context(), *app, *output, *format, *rankDir, stdout, stderr, version,
		)
	}
	return cmd
}

func runVisualize(
	ctx context.Context,
	appName, output, format, rankDir string,
	stdout, stderr io.Writer,
	version string,
) error {
	format, err := resolveGraphFormat(format, output)
	if err != nil {
		return err
	}
	switch rankDir {
	case "TB", "LR":
	default:
		return fmt.Errorf("unsupported rankdir %q, use \"TB\" or \"LR\"", rankDir)
	}

	moduleDir, err := findModuleDir()
	if err != nil {
		return err
	}
	modulePath, err := readModulePath(moduleDir)
	if err != nil {
		return err
	}
	scan, err := serverscan.Scan(moduleDir, modulePath)
	if err != nil {
		return err
	}
	if err := checkGoModVersion(moduleDir, version); err != nil {
		return err
	}
	app, err := selectApp(scan, appName)
	if err != nil {
		return err
	}

	m, err := parseApp(filepath.Join(moduleDir, app.Dir), stderr)
	if err != nil {
		return err
	}

	var dot bytes.Buffer
	if err := graph.WriteDOT(&dot, m, app.Name, graph.Options{
		RankDir:   rankDir,
		HideLabel: format == "html",
	}); err != nil {
		return fmt.Errorf("rendering graph: %w", err)
	}

	out := dot.Bytes()
	switch format {
	case "svg":
		if out, err = renderSVG(ctx, dot.Bytes()); err != nil {
			return err
		}
	case "html":
		svg, err := renderSVG(ctx, dot.Bytes())
		if err != nil {
			return err
		}
		var page bytes.Buffer
		if err := graph.WriteHTML(&page, m, app.Name, svg); err != nil {
			return err
		}
		out = page.Bytes()
	}

	if output == "" {
		_, err = stdout.Write(out)
		return err
	}
	if err := os.WriteFile(output, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", output, err)
	}
	_, _ = fmt.Fprintf(stderr, "wrote %s\n", output)
	return nil
}

// resolveGraphFormat picks the output format. An explicit flag wins, otherwise
// the output file extension decides and stdout defaults to DOT.
func resolveGraphFormat(format, output string) (string, error) {
	switch format {
	case "dot", "svg", "html":
		return format, nil
	case "":
	default:
		return "", fmt.Errorf(
			"unsupported format %q, use \"dot\", \"svg\" or \"html\"", format)
	}
	switch strings.ToLower(filepath.Ext(output)) {
	case ".svg":
		return "svg", nil
	case ".html", ".htm":
		return "html", nil
	}
	return "dot", nil
}

// renderSVG pipes the DOT source through Graphviz.
func renderSVG(ctx context.Context, dot []byte) ([]byte, error) {
	if _, err := exec.LookPath("dot"); err != nil {
		return nil, errors.New(
			`HTML and SVG output require Graphviz but "dot" was not found in PATH.` +
				"\ninstall it (macOS: brew install graphviz, " +
				"Debian/Ubuntu: apt install graphviz)" +
				"\nor write the DOT source instead: datapages visualize --format dot",
		)
	}
	c := exec.CommandContext(ctx, "dot", "-Tsvg")
	c.Stdin = bytes.NewReader(dot)
	var svg, errBuf bytes.Buffer
	c.Stdout = &svg
	c.Stderr = &errBuf
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("dot -Tsvg: %w: %s",
			err, strings.TrimSpace(errBuf.String()))
	}
	return svg.Bytes(), nil
}
