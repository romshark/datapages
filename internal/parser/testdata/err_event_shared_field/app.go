// Package app declares four events with a field of one struct type,
// whose own field has no json tag. Each event reports that field.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// Shared lacks the json tag of Note.
type Shared struct {
	Note string
}

// EventA is "a"
type EventA struct {
	Shared Shared `json:"shared"`
}

// EventB is "b"
type EventB struct {
	Shared Shared `json:"shared"`
}

// EventC is "c"
type EventC struct {
	Shared Shared `json:"shared"`
}

// EventD is "d"
type EventD struct {
	Shared Shared `json:"shared"`
}
