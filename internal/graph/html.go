package graph

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/romshark/datapages/internal/parser/model"
)

//go:embed page.html.tmpl
var pageSource string

// Morpheus is the web component kit the page is built from. The sources are
// vendored from the CDN and inlined, so the page needs no network.
//
//go:embed morpheus/morpheus.css
var morpheusCSS string

//go:embed morpheus/theme-default.css
var morpheusThemeCSS string

//go:embed morpheus/bundle.js
var morpheusJS string

// Icons are the Lucide icons the buttons carry, inlined for the same reason.
//
//go:embed icons/*.svg
var iconFS embed.FS

var pageTemplate = template.Must(template.New("page").Parse(pageSource))

// WriteHTML writes a self-contained page around svg, which must be what
// Graphviz laid out from the DOT source [WriteDOT] writes for the same model:
// the page addresses the SVG by the element ids that source carries.
//
// The page loads nothing from the network.
func WriteHTML(w io.Writer, m *model.App, name string, svg []byte) error {
	g := Build(m, name)

	// encoding/json escapes <, > and &, which keeps the payload inside the
	// script element whatever a route or a type name holds.
	data, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("encoding graph data: %w", err)
	}

	icons, err := loadIcons()
	if err != nil {
		return err
	}

	var b bytes.Buffer
	err = pageTemplate.Execute(&b, struct {
		Title       string
		SVG         string
		Data        string
		MorpheusCSS string
		ThemeCSS    string
		MorpheusJS  string
		Icons       map[string]string
		DiagramCSS  string
	}{
		Title:       g.Label,
		SVG:         svgElement(svg),
		Data:        string(data),
		MorpheusCSS: morpheusCSS,
		ThemeCSS:    morpheusThemeCSS,
		// A "</script" inside the bundle would end the element early.
		// v0.1.0 holds none; the escape keeps a later one from breaking out.
		MorpheusJS: strings.ReplaceAll(morpheusJS, "</script", `<\/script`),
		Icons:      icons,
		DiagramCSS: diagramCSS(),
	})
	if err != nil {
		return fmt.Errorf("rendering page: %w", err)
	}
	_, err = w.Write(b.Bytes())
	return err
}

// loadIcons reads the vendored icons, keyed by file name without the suffix.
func loadIcons() (map[string]string, error) {
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		return nil, fmt.Errorf("reading icons: %w", err)
	}
	icons := make(map[string]string, len(entries))
	for _, e := range entries {
		data, err := iconFS.ReadFile("icons/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading icon %s: %w", e.Name(), err)
		}
		// Kept whole, license comment included: the page is a copy of them.
		icons[strings.TrimSuffix(e.Name(), ".svg")] = strings.TrimSpace(string(data))
	}
	return icons, nil
}

// svgElement drops the XML prolog and the doctype Graphviz writes ahead of the
// root element, which an inline SVG must not carry.
func svgElement(svg []byte) string {
	if i := bytes.Index(svg, []byte("<svg")); i >= 0 {
		return string(svg[i:])
	}
	return string(svg)
}
