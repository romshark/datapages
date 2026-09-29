package app

import _ "embed"

// The icons of the tabs and the toolbar draw in the color of the text around them. The
// page inlines them: an img would draw them in black.
var (
	//go:embed icons/charts.svg
	iconCharts string

	//go:embed icons/map.svg
	iconMap string

	//go:embed icons/portfolio.svg
	iconPortfolio string

	//go:embed icons/sync.svg
	iconSync string

	//go:embed icons/rows.svg
	iconRows string

	//go:embed icons/grid.svg
	iconGrid string
)
