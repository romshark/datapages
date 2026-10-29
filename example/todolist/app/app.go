package app

import (
	"embed"
	"fmt"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/todolist/list"
)

// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS

// EventTodoUpdated is "todo.updated"
type EventTodoUpdated struct {
	// Origin is the stateID of the item page tab whose edit caused the update.
	// It is empty when another action changed the list.
	Origin string `json:"origin"`
}

// StateIndex is the per-tab state held by PageIndex.
//
// The datapages generator allocates 1 zeroed *StateIndex per SSE stream
// (i.e. per browser tab) and drops it when that stream closes.
// All stateful handlers touching the same instance are serialized by
// the generator under a per-instance mutex.
type StateIndex struct {
	list.ViewParameters
}

// StateItem is the per-tab state held by PageItem.
// It tracks which todo the current tab is viewing so the OnTodoUpdated handler
// can re-render only the affected item.
type StateItem struct {
	ItemID string
}

type App struct {
	list *list.List
}

func NewApp(l *list.List) *App {
	return &App{list: l}
}

func (*App) Head(r *http.Request) datapages.Head { return head() }

// PUTToggle is /toggle/{id}
//
// It does not take state because toggling a todo mutates global list state,
// not per-tab state.
func (a *App) PUTToggle(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	todoUpdated datapages.Dispatcher[EventTodoUpdated],
) error {
	if !a.list.ToggleItem(path.Values.ID) {
		return fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
	}
	return todoUpdated.Dispatch(EventTodoUpdated{})
}
