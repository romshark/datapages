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

/* ErrAppDocumentMustReturnDocument */

func (*App) Document(r *http.Request) (datapages.Document, error) {
	return datapages.Document{}, nil
}
