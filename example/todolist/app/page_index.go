package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/datapages/example/todolist/datapagesgen/httperr"
	"github.com/romshark/datapages/example/todolist/list"
)

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request,
	query struct {
		Search string `query:"q" reflectsignal:"search"`
		Filter string `query:"filter" reflectsignal:"filter"`
		Sort   string `query:"sort" reflectsignal:"sort"`
	},
) (body templ.Component, err error) {
	filter := query.Filter
	if filter == "" {
		filter = "all"
	}
	sortMode := query.Sort
	if sortMode == "" {
		sortMode = "created"
	}
	vp := list.ViewParameters{
		Search: query.Search,
		Filter: filter,
		Sort:   sortMode,
	}
	todos := p.App.list.GetItems(vp)
	return pageIndex(todos, query.Search, filter, sortMode), nil
}

func (p PageIndex) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *StateIndex,
	signals struct {
		Search string `json:"search"`
		Filter string `json:"filter"`
		Sort   string `json:"sort"`
	},
) error {
	filter := signals.Filter
	if filter == "" {
		filter = "all"
	}
	sortMode := signals.Sort
	if sortMode == "" {
		sortMode = "created"
	}
	state.ViewParameters = list.ViewParameters{
		Search: signals.Search,
		Filter: filter,
		Sort:   sortMode,
	}
	return nil
}

// POSTCreate is /
func (p PageIndex) POSTCreate(
	r *http.Request,
	signals struct {
		NewTitle string `json:"newTitle"`
		NewDesc  string `json:"newDesc"`
		NewDue   string `json:"newDue"`
	},
	dispatch func(EventTodoUpdated) error,
) error {
	title := strings.TrimSpace(signals.NewTitle)
	if title == "" {
		return fmt.Errorf("%w: title is required", httperr.BadRequest)
	}
	var dueAt time.Time
	if signals.NewDue != "" {
		var err error
		dueAt, err = time.Parse("2006-01-02T15:04", signals.NewDue)
		if err != nil {
			return fmt.Errorf("%w: invalid due date", httperr.BadRequest)
		}
	}
	p.App.list.AddItem(title, strings.TrimSpace(signals.NewDesc), dueAt)
	return dispatch(EventTodoUpdated{})
}

// POSTFilter is /filter
func (p PageIndex) POSTFilter(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	state *StateIndex,
	signals struct {
		Search string `json:"search"`
		Filter string `json:"filter"`
		Sort   string `json:"sort"`
	},
) error {
	state.Search = signals.Search
	state.Filter = signals.Filter
	state.Sort = signals.Sort

	todos := p.App.list.GetItems(state.ViewParameters)
	return sse.PatchElementTempl(todoList(todos))
}

func (p PageIndex) OnTodoUpdated(
	event EventTodoUpdated,
	sse *datastar.ServerSentEventGenerator,
	state *StateIndex,
) error {
	todos := p.App.list.GetItems(state.ViewParameters)
	return sse.PatchElementTempl(todoList(todos))
}
