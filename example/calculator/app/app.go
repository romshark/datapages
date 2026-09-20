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
	errInvalidNum   = errors.New("invalid num signal")
	errInvalidBtn   = errors.New("invalid btn parameter")
	errInvalidInput = errors.New("invalid input signal")
	numRe           = regexp.MustCompile(`^-?\d*\.?\d+$`)
)

func (*App) Head(_ *http.Request) datapages.Head { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return pageCalculator("", false), nil
}

// POSTInput is /input/{$}
func (PageIndex) POSTInput(
	r *http.Request,
	sse datapages.SSE,
	query datapages.Query[struct {
		Btn   int  `query:"btn"`
		Paste bool `query:"paste"`
	}],
	signals datapages.Signals[struct {
		Input string `json:"input"`
		Fresh bool   `json:"fresh"`
		Num   string `json:"num"`
	}],
) error {
	if !calc.ValidInput(signals.Values.Input) {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidInput)
	}
	if query.Values.Paste {
		if !numRe.MatchString(signals.Values.Num) {
			return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidNum)
		}
		input := signals.Values.Input
		if signals.Values.Fresh {
			input = ""
		}
		return sse.PatchElement(pageCalculator(input+signals.Values.Num, false))
	}
	btn := calc.CalcButton(query.Values.Btn)
	if !calc.ValidButton(btn) {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidBtn)
	}
	input, fresh := calc.Press(signals.Values.Input, signals.Values.Fresh, btn)
	return sse.PatchElement(pageCalculator(input, fresh))
}
