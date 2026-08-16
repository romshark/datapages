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

func (*App) Head(r *http.Request) datapages.Component { return head() }

// PUTEdit is /{id}
//
// This action is shared across pages. It does not take state because
// editing a todo mutates global list state, not per-tab state.
func (a *App) PUTEdit(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
	query struct {
		Toggle bool `query:"toggle"`
	},
	signals struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Done        bool   `json:"done"`
		Due         string `json:"due"`
	},
	dispatch func(EventTodoUpdated) error,
) error {
	if query.Toggle {
		if !a.list.ToggleItem(path.ID) {
			return fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
		}
		return dispatch(EventTodoUpdated{})
	}
	title := strings.TrimSpace(signals.Title)
	if title == "" {
		return fmt.Errorf("%w: title is required", datapages.ErrBadRequest)
	}
	var dueAt time.Time
	if signals.Due != "" {
		var err error
		dueAt, err = time.Parse("2006-01-02T15:04", signals.Due)
		if err != nil {
			return fmt.Errorf("%w: invalid due date", datapages.ErrBadRequest)
		}
	}
	if !a.list.UpdateItem(
		path.ID, title, strings.TrimSpace(signals.Description),
		signals.Done, dueAt,
	) {
		return fmt.Errorf("%w: todo not found", datapages.ErrNotFound)
	}
	return dispatch(EventTodoUpdated{})
}
