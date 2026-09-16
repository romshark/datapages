package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type StateA struct{}

type StateB struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTA is /a
func (PageIndex) POSTA(
	r *http.Request,
	state datapages.State[StateA],
) error {
	return nil
}

// POSTB is /b
func (PageIndex) POSTB(
	r *http.Request,
	state datapages.State[StateB], // conflicts with StateA on the same page
) error {
	return nil
}
