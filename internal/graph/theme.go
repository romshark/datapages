package graph

import (
	"fmt"
	"strings"
)

// Graphviz bakes the colors into the SVG, so the page recolors it by
// selecting on the value baked in. These are the dark counterparts of the
// constants in graph.go, from the GitHub dark palette.
const (
	darkPage      = "#4493f8"
	darkAbstract  = "#a371f7"
	darkEvent     = "#3fb950"
	darkApp       = "#6e7681"
	darkDispatch  = "#db6d28"
	darkSubscribe = "#3fb950"
	darkEmbed     = "#6e7681"
	darkMuted     = "#8b949e"
	darkText      = "#e6edf3"
	darkCell      = "#161b22"

	// Light and dark values that have no counterpart in the drawing: the
	// cell borders, the fill of a selected handler row, and the ink on a
	// node header, which is white on the light palette and dark on the
	// lighter dark one.
	lineLight   = "#d0d7de"
	lineDark    = "#30363d"
	markLight   = "#fff8c5"
	markDark    = "#4d3800"
	headerLight = "#ffffff"
	headerDark  = "#0d1117"
)

// swatch is one color of the drawing: the CSS variable holding it, the value
// Graphviz baked in, and what each theme shows instead.
type swatch struct {
	name        string // CSS variable, without the leading dashes.
	baked       string // What Graphviz wrote, or "" for colors it never writes.
	light, dark string
	rules       []string // Selectors keyed on baked, %s is the variable.
}

func diagramSwatches() []swatch {
	nodeFill := func(baked, light, dark, name string) swatch {
		return swatch{
			name: name, baked: baked, light: light, dark: dark,
			rules: []string{"#canvas g.node polygon[fill=%q i] { fill: %s; }"},
		}
	}
	edge := func(baked, light, dark, name string) swatch {
		return swatch{
			name: name, baked: baked, light: light, dark: dark,
			rules: []string{
				// The line, then the arrowhead, which Graphviz fills and
				// strokes in the same color.
				"#canvas g.edge path[stroke=%[1]q i] { stroke: %[2]s; }",
				"#canvas g.edge polygon[fill=%[1]q i] { fill: %[2]s; }",
				"#canvas g.edge polygon[stroke=%[1]q i] { stroke: %[2]s; }",
			},
		}
	}

	return []swatch{
		{
			name: "dp-surface", baked: colorCell, light: colorCell, dark: darkCell,
			rules: []string{"#canvas g.node polygon[fill=%q i] { fill: %s; }"},
		},
		{
			name: "dp-ink", baked: colorText, light: colorText, dark: darkText,
			rules: []string{"#canvas g.node text[fill=%q i] { fill: %s; }"},
		},
		{
			name: "dp-muted", baked: colorMuted, light: colorMuted, dark: darkMuted,
			rules: []string{"#canvas g.node text[fill=%q i] { fill: %s; }"},
		},
		{
			// Graphviz draws every cell border in black.
			name: "dp-line", baked: "black", light: lineLight, dark: lineDark,
			rules: []string{"#canvas polygon[stroke=%q i] { stroke: %s; }"},
		},
		{
			name: "dp-header-ink", baked: headerLight,
			light: headerLight, dark: headerDark,
			rules: []string{"#canvas g.node text[fill=%q i] { fill: %s; }"},
		},
		nodeFill(colorPage, colorPage, darkPage, "dp-page"),
		nodeFill(colorAbstract, colorAbstract, darkAbstract, "dp-abstract"),
		nodeFill(colorEvent, colorEvent, darkEvent, "dp-event"),
		nodeFill(colorApp, colorApp, darkApp, "dp-app"),
		edge(colorDispatch, colorDispatch, darkDispatch, "dp-dispatch"),
		edge(colorSubscribe, colorSubscribe, darkSubscribe, "dp-subscribe"),
		edge(colorEmbed, colorEmbed, darkEmbed, "dp-embed"),
		{name: "dp-mark", light: markLight, dark: markDark},
	}
}

// diagramCSS renders the stylesheet that dresses the drawing in the theme.
// The selectors match on the color Graphviz baked in, so a node header keeps
// its white text while a white cell background turns dark: one is a <text>,
// the other a <polygon>.
func diagramCSS() string {
	swatches := diagramSwatches()

	var b strings.Builder
	vars := func(dark bool) string {
		var v strings.Builder
		for _, s := range swatches {
			value := s.light
			if dark {
				value = s.dark
			}
			fmt.Fprintf(&v, "  --%s: %s;\n", s.name, value)
		}
		return v.String()
	}

	fmt.Fprintf(&b, ":root {\n%s}\n", vars(false))
	fmt.Fprintf(&b, ":root.dark {\n%s}\n", vars(true))
	fmt.Fprintf(&b,
		"@media (prefers-color-scheme: dark) {\n:root:not(.light):not(.dark) {\n%s}\n}\n",
		vars(true))

	for _, s := range swatches {
		for _, rule := range s.rules {
			fmt.Fprintf(&b, rule+"\n", s.baked, "var(--"+s.name+")")
		}
	}
	return b.String()
}
