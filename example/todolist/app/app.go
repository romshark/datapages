package app

import (
	"embed"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/todolist/list"
)

// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS

// EventTodoUpdated is "todo.updated"
type EventTodoUpdated struct{}

// StateIndex is the per-tab state held by PageIndex.
//
// The datapages generator allocates one *StateIndex from a sync.Pool
// per SSE stream (i.e. per browser tab), zero-resets it before use,
// and returns it to the pool after StreamClose + the configured grace
// period. All stateful handlers touching the same instance are
// serialized by the generator under a per-instance mutex.
type StateIndex struct {
	list.ViewParameters
}

// StateItem is the per-tab state held by PageItem. It tracks which
// todo the current tab is viewing so the OnTodoUpdated handler can
// re-render only the affected item.
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

// PUTEdit is /{id}
//
// This action is shared across pages. It does not take state because
// editing a todo mutates global list state, not per-tab state.
func (a *App) PUTEdit(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	query datapages.Query[struct {
		Toggle bool `query:"toggle"`
	}],
	signals datapages.Signals[struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Done        bool   `json:"done"`
		Due         string `json:"due"`
	}],
	todoUpdated datapages.Dispatcher[EventTodoUpdated],
) error {
	if query.Values.Toggle {
		if !a.list.ToggleItem(path.Values.ID) {
			return fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
		}
		return todoUpdated.Dispatch(EventTodoUpdated{})
	}
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
	if !a.list.UpdateItem(
		path.Values.ID, title, strings.TrimSpace(signals.Values.Description),
		signals.Values.Done, dueAt,
	) {
		return fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
	}
	return todoUpdated.Dispatch(EventTodoUpdated{})
}
