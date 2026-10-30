package app

import _ "embed"

// Icons are inlined so currentColor follows surrounding text.
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
