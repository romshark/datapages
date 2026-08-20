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
	state *StateIndex,
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
	state.ViewParameters = list.ViewParameters{
		Search: signals.Values.Search,
		Filter: filter,
		Sort:   sortMode,
	}
	return nil
}

// POSTCreate is /
func (p PageIndex) POSTCreate(
	r *http.Request,
	signals datapages.Signals[struct {
		NewTitle string `json:"newTitle"`
		NewDesc  string `json:"newDesc"`
		NewDue   string `json:"newDue"`
	}],
	todoUpdated datapages.Dispatcher[EventTodoUpdated],
) error {
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
	state *StateIndex,
	signals datapages.Signals[struct {
		Search string `json:"search"`
		Filter string `json:"filter"`
		Sort   string `json:"sort"`
	}],
) error {
	state.Search = signals.Values.Search
	state.Filter = signals.Values.Filter
	state.Sort = signals.Values.Sort

	todos := p.App.list.GetItems(state.ViewParameters)
	return sse.PatchElement(todoList(todos))
}

func (p PageIndex) OnTodoUpdated(
	event EventTodoUpdated,
	sse datapages.SSE,
	state *StateIndex,
) error {
	todos := p.App.list.GetItems(state.ViewParameters)
	return sse.PatchElement(todoList(todos))
}
