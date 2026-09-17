// Package app declares a signal whose name no attribute name and no JavaScript
// identifier can carry, a second one called "-", which encoding/json reads as
// "leave this field out", and a third whose name carries a period.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTQuoted is /quoted
func (PageIndex) POSTQuoted(
	r *http.Request,
	signals datapages.Signals[struct {
		Term string `json:"a\"b"` /* ErrSignalsFieldNameInvalid */
	}],
) error {
	_ = signals
	return nil
}

// POSTNested is /nested
func (PageIndex) POSTNested(
	r *http.Request,
	signals datapages.Signals[struct {
		Form struct {
			Term string `json:"a\"b"` /* ErrSignalsFieldNameInvalid */
		} `json:"form"`
	}],
) error {
	_ = signals
	return nil
}

// POSTDotted is /dotted
//
// A period stands between the steps of a signal path. A signals struct writes a
// path by nesting a struct, which leaves this one key carrying a period that
// no client sends.
func (PageIndex) POSTDotted(
	r *http.Request,
	signals datapages.Signals[struct {
		Term string `json:"foo.bar"` /* ErrSignalsFieldNameInvalid */
	}],
) error {
	_ = signals
	return nil
}

// POSTExcluded is /excluded
func (PageIndex) POSTExcluded(
	r *http.Request,
	signals datapages.Signals[struct {
		Term string `json:"-"` /* ErrSignalsFieldNameInvalid */
	}],
) error {
	_ = signals
	return nil
}
