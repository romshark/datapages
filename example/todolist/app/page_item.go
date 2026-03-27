package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/romshark/datapages/example/todolist/datapagesgen/httperr"
	"github.com/starfederation/datastar-go/datastar"
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
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		ItemID string `json:"itemId"`
	},
) error {
	p.App.lockTabs.Lock()
	p.App.streamIDToTabState[streamID] = &tabState{
		ItemID: signals.ItemID,
	}
	p.App.lockTabs.Unlock()
	return p.App.patchTabID(streamID, sse)
}

func (p PageItem) StreamClose(r *http.Request, streamID uint64) {
	p.App.lockTabs.Lock()
	delete(p.App.streamIDToTabState, streamID)
	p.App.lockTabs.Unlock()
}

// DELETEItem is /item/{id}/
func (p PageItem) DELETEItem(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
	signals struct {
		TabID string `json:"tab_id"`
	},
	dispatch func(EventTodoUpdated) error,
) (redirect string, err error) {
	if _, err := p.App.verifyTabID(signals.TabID); err != nil {
		return "", fmt.Errorf("%w: %w", httperr.BadRequest, err)
	}
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
	streamID uint64,
) error {
	ts := p.App.streamState(streamID)
	if ts == nil || ts.ItemID == "" {
		return nil
	}
	todo, ok := p.App.list.GetItem(ts.ItemID)
	if !ok {
		return sse.Redirect("/")
	}
	return sse.PatchElementTempl(pageItem(todo))
}
