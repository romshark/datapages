package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/todolist/app/datapagesgen/href"
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
		return nil, datapages.Redirect{URL: href.PageIndex(href.QueryPageIndex{})}, nil
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

// PUTEdit is /item/{id}/edit
//
// It passes stateID in the event, which keeps the tab that sent the edit
// from patching its own fields. See [PageItem.OnTodoUpdated].
func (p PageItem) PUTEdit(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Done        bool   `json:"done"`
		Due         string `json:"due"`
	}],
	// stateID requires the state parameter.
	_ datapages.State[StateItem],
	stateID string,
	todoUpdated datapages.Dispatcher[EventTodoUpdated],
) error {
	title := strings.TrimSpace(signals.Values.Title)
	if title == "" {
		return fmt.Errorf("%w: title is required", datapages.ErrBadRequest)
	}
	var dueAt time.Time
	if signals.Values.Due != "" {
		var err error
		dueAt, err = time.Parse("2006-01-02T15:04", signals.Values.Due)
		if err != nil {
			return fmt.Errorf("%w: invalid due date", datapages.ErrBadRequest)
		}
	}
	if !p.App.list.UpdateItem(
		path.Values.ID, title, strings.TrimSpace(signals.Values.Description),
		signals.Values.Done, dueAt,
	) {
		return fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
	}
	return todoUpdated.Dispatch(EventTodoUpdated{Origin: stateID})
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
	return datapages.Redirect{URL: href.PageIndex(href.QueryPageIndex{})}, nil
}

func (p PageItem) OnTodoUpdated(
	event EventTodoUpdated,
	sse datapages.SSE,
	state datapages.State[StateItem],
	stateID string,
) error {
	if state.Values.ItemID == "" {
		return nil
	}
	todo, ok := p.App.list.GetItem(state.Values.ItemID)
	if !ok {
		return sse.Redirect(href.PageIndex(href.QueryPageIndex{}))
	}
	if event.Origin == stateID {
		// The fields of the tab that sent the edit already hold it, and may
		// hold input typed while it was in flight. The signals in a full
		// patch would replace that input with the older value.
		return sse.PatchElement(itemTitle(todo))
	}
	return sse.PatchElement(pageItem(todo))
}
