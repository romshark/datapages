package app

import (
	"fmt"
	"net/http"

	"github.com/romshark/datapages"
)

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (p PageItem) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, redirect datapages.Redirect, err error) {
	todo, ok := p.App.list.GetItem(path.Values.ID)
	if !ok {
		return nil, datapages.Redirect{URL: "/"}, nil
	}
	return pageItem(todo), redirect, nil
}

func (p PageItem) StreamOpen(
	r *http.Request,
	state datapages.State[StateItem],
	signals datapages.Signals[struct {
		ItemID string `json:"itemId"`
	}],
) error {
	state.Values.ItemID = signals.Values.ItemID
	return nil
}

// DELETEItem is /item/{id}/
func (p PageItem) DELETEItem(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	todoUpdated datapages.Dispatcher[EventTodoUpdated],
) (redirect datapages.Redirect, err error) {
	if !p.App.list.DeleteItem(path.Values.ID) {
		return redirect, fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
	}
	if err := todoUpdated.Dispatch(EventTodoUpdated{}); err != nil {
		return redirect, err
	}
	return datapages.Redirect{URL: "/"}, nil
}

func (p PageItem) OnTodoUpdated(
	event EventTodoUpdated,
	sse datapages.SSE,
	state datapages.State[StateItem],
) error {
	if state.Values.ItemID == "" {
		return nil
	}
	todo, ok := p.App.list.GetItem(state.Values.ItemID)
	if !ok {
		return sse.Redirect("/")
	}
	return sse.PatchElement(pageItem(todo))
}
