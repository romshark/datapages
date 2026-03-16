package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages/example/calculator/app/calc"
)

type App struct{}

func (*App) Head(_ *http.Request) templ.Component { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return pageCalculator(""), nil
}

// POSTCalculate is /calculate/{$}
func (PageIndex) POSTCalculate(
	r *http.Request,
	signals struct {
		Expr string `json:"expr"`
	},
) (body templ.Component, err error) {
	return pageCalculator(calc.Evaluate(signals.Expr)), nil
}

// jsStr wraps s in single quotes for use as a JavaScript string literal.
func jsStr(s string) string {
	return "'" + s + "'"
}