//nolint:all
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

/* ErrSignatureActionDocumentAttrsWithoutBody: the document has no response to travel in */

// POSTLang is /lang
func (PageIndex) POSTLang(r *http.Request) (docAttrs datapages.DocumentAttrs, err error) {
	return docAttrs, err
}

/* ErrSignatureDuplicateOutput */

// POSTTwice is /twice
func (PageIndex) POSTTwice(r *http.Request) (
	body datapages.Component,
	a datapages.DocumentAttrs,
	b datapages.DocumentAttrs,
	err error,
) {
	return body, a, b, err
}
