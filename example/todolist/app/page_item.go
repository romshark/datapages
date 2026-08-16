package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (p PageItem) GET(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
) (body templ.Component, redirect datapages.Redirect, err error) {
	todo, ok := p.App.list.GetItem(path.ID)
	if !ok {
		return nil, datapages.Redirect{URL: "/"}, nil
	}
	return pageItem(todo), redirect, nil
}

func (p PageItem) StreamOpen(
	r *http.Request,
	streamID uint64,
	sse datapages.SSE,
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
) (redirect datapages.Redirect, err error) {
	if _, err := p.App.verifyTabID(signals.TabID); err != nil {
		return redirect, fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	if !p.App.list.DeleteItem(path.ID) {
		return redirect, fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
	}
	if err := dispatch(EventTodoUpdated{}); err != nil {
		return redirect, err
	}
	return datapages.Redirect{URL: "/"}, nil
}

func (p PageItem) OnTodoUpdated(
	event EventTodoUpdated,
	sse datapages.SSE,
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
	return sse.PatchElement(pageItem(todo))
}
