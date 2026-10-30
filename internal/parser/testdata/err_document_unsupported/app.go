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

/* ErrAppDocumentUnsupportedInput */

func (*App) Document(r *http.Request, foo int) datapages.Document {
	return datapages.Document{}
}
