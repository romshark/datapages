package app

import (
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/todolist/list"
)

// ViewSignals are the search, filter and sort controls of [PageIndex],
// reflected in the URL query. It converts to [list.ViewParameters].
type ViewSignals struct {
	Search string `json:"search" query:"q" reflectsignal:"search"`
	Filter string `json:"filter" query:"filter" reflectsignal:"filter"`
	Sort   string `json:"sort" query:"sort" reflectsignal:"sort"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request,
	query datapages.Query[ViewSignals],
) (body datapages.Component, err error) {
	vp := list.ViewParameters(query.Values)
	// An empty value leaves the selects without a selected option.
	// The other handlers pass empty values to [list.List.GetItems] as is.
	if vp.Filter == "" {
		vp.Filter = "all"
	}
	if vp.Sort == "" {
		vp.Sort = "created"
	}
	return pageIndex(p.App.list.GetItems(vp), vp), nil
}

func (p PageIndex) StreamOpen(
	r *http.Request,
	state datapages.State[StateIndex],
	signals datapages.Signals[ViewSignals],
) error {
	state.Values.ViewParameters = list.ViewParameters(signals.Values)
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
	title, desc, dueAt, err := parseTodoInput(
		signals.Values.NewTitle, signals.Values.NewDesc, signals.Values.NewDue,
	)
	if err != nil {
		return err
	}
	p.App.list.AddItem(title, desc, dueAt)
	return todoUpdated.Dispatch(EventTodoUpdated{})
}

// POSTFilter is /filter
func (p PageIndex) POSTFilter(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateIndex],
	signals datapages.Signals[ViewSignals],
) error {
	state.Values.ViewParameters = list.ViewParameters(signals.Values)
	todos := p.App.list.GetItems(state.Values.ViewParameters)
	return sse.PatchElement(todoList(todos))
}

func (p PageIndex) OnTodoUpdated(
	event EventTodoUpdated,
	sse datapages.SSE,
	state datapages.State[StateIndex],
) error {
	todos := p.App.list.GetItems(state.Values.ViewParameters)
	return sse.PatchElement(todoList(todos))
}
