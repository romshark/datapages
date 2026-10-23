// Package app reflects a query field that decodes from text but cannot
// write itself back as text.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// Day is a date that only parses.
type Day struct{ Value string }

func (d *Day) UnmarshalText(text []byte) error {
	d.Value = string(text)
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	query datapages.Query[struct {
		Day Day `query:"day" reflectsignal:"day"` /* ErrQueryReflectSignalNotText */
	}],
	signals datapages.Signals[struct {
		Day string `json:"day"`
	}],
) (body datapages.Component, err error) {
	_, _ = query, signals
	return body, err
}

// PageSearch is /search
//
// Without a Signals parameter, Day is rejected all the same. The page seeds
// the signal with the field's text, and Day cannot write itself as text.
type PageSearch struct{ App *App }

func (PageSearch) GET(
	r *http.Request,
	query datapages.Query[struct {
		Day Day `query:"day" reflectsignal:"day"` /* ErrQueryReflectSignalNotText */
	}],
) (body datapages.Component, err error) {
	_ = query
	return body, err
}
