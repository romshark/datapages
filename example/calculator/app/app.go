package app

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/calculator/app/calc"
)

type App struct{}

func NewApp() *App { return &App{} }

var (
	errInvalidNum = errors.New("invalid num parameter")
	errInvalidBtn = errors.New("invalid btn parameter")
	numRe         = regexp.MustCompile(`^-?\d*\.?\d+$`)
)

func (*App) Head(_ *http.Request) datapages.Head { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (
	body datapages.Component,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	return pageCalculator("", false), true, nil
}

// POSTInput is /input/{$}
func (PageIndex) POSTInput(
	r *http.Request,
	sse datapages.SSE,
	query datapages.Query[struct {
		Btn int    `query:"btn"`
		Num string `query:"num"`
	}],
	signals datapages.Signals[struct {
		Input string `json:"input"`
		Fresh bool   `json:"fresh"`
	}],
) error {
	if query.Values.Num != "" {
		if !numRe.MatchString(query.Values.Num) {
			return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidNum)
		}
		input := signals.Values.Input
		if signals.Values.Fresh {
			input = ""
		}
		return sse.PatchElement(pageCalculator(input+query.Values.Num, false))
	}
	btn := calc.CalcButton(query.Values.Btn)
	if !calc.ValidButton(btn) {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidBtn)
	}
	input, fresh := calc.Press(signals.Values.Input, signals.Values.Fresh, btn)
	return sse.PatchElement(pageCalculator(input, fresh))
}
