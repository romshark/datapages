package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/datapages/example/todolist/datapagesgen/httperr"
)

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (p PageItem) GET(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
) (body templ.Component, redirect string, err error) {
	todo, ok := p.App.list.GetItem(path.ID)
	if !ok {
		return nil, "/", nil
	}
	return pageItem(todo), "", nil
}

func (p PageItem) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *StateItem,
	signals struct {
		ItemID string `json:"itemId"`
	},
) error {
	state.ItemID = signals.ItemID
	return nil
}

// DELETEItem is /item/{id}/
func (p PageItem) DELETEItem(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
	dispatch func(EventTodoUpdated) error,
) (redirect string, err error) {
	if !p.App.list.DeleteItem(path.ID) {
		return "", fmt.Errorf("%w: todo not found", httperr.NotFound)
	}
	if err := dispatch(EventTodoUpdated{}); err != nil {
		return "", err
	}
	return "/", nil
}

func (p PageItem) OnTodoUpdated(
	event EventTodoUpdated,
	sse *datastar.ServerSentEventGenerator,
	state *StateItem,
) error {
	if state.ItemID == "" {
		return nil
	}
	todo, ok := p.App.list.GetItem(state.ItemID)
	if !ok {
		return sse.Redirect("/")
	}
	return sse.PatchElementTempl(pageItem(todo))
}
