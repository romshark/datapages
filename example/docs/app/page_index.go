package app

import (
	"errors"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/docs/app/datapagesgen/href"
	"github.com/romshark/datapages/example/docs/store"
	"github.com/romshark/datapages/example/docs/templates"
)

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (
	body datapages.Component, head datapages.Head, err error,
) {
	return pageIndex(p.App.docs.List()), headIndex(), nil
}

// POSTCreate is /create/{template}
func (p PageIndex) POSTCreate(
	r *http.Request,
	path datapages.Path[struct {
		Template string `path:"template"`
	}],
	listChanged datapages.Dispatcher[EventListChanged],
) (datapages.Redirect, error) {
	tmpl, ok := templates.Get(path.Values.Template)
	if !ok {
		return datapages.Redirect{}, datapages.ErrNotFound
	}
	id, err := p.App.docs.Create(tmpl.Title, tmpl.Content)
	if err != nil {
		return datapages.Redirect{}, err
	}
	if err := listChanged.Dispatch(EventListChanged{}); err != nil {
		return datapages.Redirect{}, err
	}
	return datapages.Redirect{URL: href.PageDoc(id)}, nil
}

// DELETEDoc is /delete/{id}
func (p PageIndex) DELETEDoc(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	listChanged datapages.Dispatcher[EventListChanged],
	docChanged datapages.Dispatcher[EventDocChanged],
) error {
	err := p.App.docs.Delete(path.Values.ID)
	if errors.Is(err, store.ErrNotFound) {
		return datapages.ErrNotFound
	}
	// Tabs with the document open find it gone and leave.
	return errors.Join(
		err,
		listChanged.Dispatch(EventListChanged{}),
		docChanged.Dispatch(EventDocChanged{Doc: datapages.Subject(path.Values.ID)}),
	)
}

func (p PageIndex) OnListChanged(event EventListChanged, sse datapages.SSE) error {
	return sse.PatchElement(docList(p.App.docs.List()))
}
