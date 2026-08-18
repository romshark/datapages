// Package app declares two actions of one page at one URL.
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

// POSTSame is /same/{$}
func (PageIndex) POSTSame(r *http.Request) error { return nil }

// POSTOther is /same/{$}
func (PageIndex) POSTOther(r *http.Request) error { return nil }
