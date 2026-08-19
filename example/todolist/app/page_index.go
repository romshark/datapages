package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/todolist/list"
)

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request,
	query datapages.Query[struct {
		Search string `query:"q" reflectsignal:"search"`
		Filter string `query:"filter" reflectsignal:"filter"`
		Sort   string `query:"sort" reflectsignal:"sort"`
	}],
) (body datapages.Component, err error) {
	filter := query.Values.Filter
	if filter == "" {
		filter = "all"
	}
	sortMode := query.Values.Sort
	if sortMode == "" {
		sortMode = "created"
	}
	vp := list.ViewParameters{
		Search: query.Values.Search,
		Filter: filter,
		Sort:   sortMode,
	}
	todos := p.App.list.GetItems(vp)
	return pageIndex(todos, query.Values.Search, filter, sortMode), nil
}

func (p PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Search string `json:"search"`
		Filter string `json:"filter"`
		Sort   string `json:"sort"`
	}],
) error {
	filter := signals.Values.Filter
	if filter == "" {
		filter = "all"
	}
	sortMode := signals.Values.Sort
	if sortMode == "" {
		sortMode = "created"
	}
	p.App.lockTabs.Lock()
	p.App.streamIDToTabState[streamID] = &tabState{
		Search: signals.Values.Search,
		Filter: filter,
		Sort:   sortMode,
	}
	p.App.lockTabs.Unlock()
	return p.App.patchTabID(streamID, sse)
}

func (p PageIndex) StreamClose(r *http.Request, streamID datapages.StreamID) {
	p.App.lockTabs.Lock()
	delete(p.App.streamIDToTabState, streamID)
	p.App.lockTabs.Unlock()
}

// POSTCreate is /
func (p PageIndex) POSTCreate(
	r *http.Request,
	signals datapages.Signals[struct {
		TabID    string `json:"tab_id"`
		NewTitle string `json:"newTitle"`
		NewDesc  string `json:"newDesc"`
		NewDue   string `json:"newDue"`
	}],
	todoUpdated datapages.Dispatcher[EventTodoUpdated],
) error {
	if _, err := p.App.verifyTabID(signals.Values.TabID); err != nil {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	title := strings.TrimSpace(signals.Values.NewTitle)
	if title == "" {
		return fmt.Errorf("%w: title is required", datapages.ErrBadRequest)
	}
	var dueAt time.Time
	if signals.Values.NewDue != "" {
		var err error
		dueAt, err = time.Parse("2006-01-02T15:04", signals.Values.NewDue)
		if err != nil {
			return fmt.Errorf("%w: invalid due date", datapages.ErrBadRequest)
		}
	}
	p.App.list.AddItem(title, strings.TrimSpace(signals.Values.NewDesc), dueAt)
	return todoUpdated.Dispatch(EventTodoUpdated{})
}

// POSTFilter is /filter
func (p PageIndex) POSTFilter(
	r *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		TabID  string `json:"tab_id"`
		Search string `json:"search"`
		Filter string `json:"filter"`
		Sort   string `json:"sort"`
	}],
) error {
	streamID, err := p.App.verifyTabID(signals.Values.TabID)
	if err != nil {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}

	// Update server-side tab state so event handlers use current filters.
	p.App.lockTabs.Lock()
	if ts := p.App.streamIDToTabState[streamID]; ts != nil {
		ts.Search = signals.Values.Search
		ts.Filter = signals.Values.Filter
		ts.Sort = signals.Values.Sort
	}
	p.App.lockTabs.Unlock()

	vp := list.ViewParameters{
		Search: signals.Values.Search,
		Filter: signals.Values.Filter,
		Sort:   signals.Values.Sort,
	}
	todos := p.App.list.GetItems(vp)
	return sse.PatchElement(todoList(todos))
}

func (p PageIndex) OnTodoUpdated(
	event EventTodoUpdated,
	sse datapages.SSE,
	streamID datapages.StreamID,
) error {
	s := p.App.streamState(streamID)
	todos := p.App.list.GetItems(s.ViewParameters)
	return sse.PatchElement(todoList(todos))
}
