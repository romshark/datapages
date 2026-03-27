package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/datapages/example/todolist/datapagesgen/httperr"
	"github.com/romshark/datapages/example/todolist/list"
)

//go:embed static/*
var StaticFS embed.FS

// EventTodoUpdated is "todo.updated"
type EventTodoUpdated struct{}

// tabState holds per-tab server-side state managed via stream hooks.
type tabState struct {
	list.ViewParameters
	ItemID string // todo ID being viewed; only set for PageItem streams
}

type App struct {
	hmacKey [32]byte

	list               *list.List
	lockTabs           sync.RWMutex
	streamIDToTabState map[uint64]*tabState
}

func NewApp(hmacKey [32]byte, list *list.List) *App {
	return &App{
		hmacKey:            hmacKey,
		list:               list,
		streamIDToTabState: make(map[uint64]*tabState),
	}
}

func (*App) Head(r *http.Request) templ.Component { return head() }

// PUTEdit is /{id}
//
// This action is shared across all pages.
func (a *App) PUTEdit(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
	query struct {
		Toggle bool `query:"toggle"`
	},
	signals struct {
		TabID       string `json:"tab_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Done        bool   `json:"done"`
		Due         string `json:"due"`
	},
	dispatch func(EventTodoUpdated) error,
) error {
	if _, err := a.verifyTabID(signals.TabID); err != nil {
		return fmt.Errorf("%w: %w", httperr.BadRequest, err)
	}
	if query.Toggle {
		if !a.list.ToggleItem(path.ID) {
			return fmt.Errorf("%w: todo not found", httperr.NotFound)
		}
		return dispatch(EventTodoUpdated{})
	}
	title := strings.TrimSpace(signals.Title)
	if title == "" {
		return fmt.Errorf("%w: title is required", httperr.BadRequest)
	}
	var dueAt time.Time
	if signals.Due != "" {
		var err error
		dueAt, err = time.Parse("2006-01-02T15:04", signals.Due)
		if err != nil {
			return fmt.Errorf("%w: invalid due date", httperr.BadRequest)
		}
	}
	if !a.list.UpdateItem(
		path.ID, title, strings.TrimSpace(signals.Description),
		signals.Done, dueAt,
	) {
		return fmt.Errorf("%w: todo not found", httperr.NotFound)
	}
	return dispatch(EventTodoUpdated{})
}

// signStreamID produces an HMAC-signed tab identifier from a streamID.
func (a *App) signStreamID(streamID uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], streamID)
	mac := hmac.New(sha256.New, a.hmacKey[:])
	mac.Write(buf[:])
	return base64.RawURLEncoding.EncodeToString(buf[:]) +
		"~" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var errInvalidTabID = errors.New("invalid tab ID")

// verifyTabID verifies the HMAC signature and extracts the streamID.
func (a *App) verifyTabID(tabID string) (uint64, error) {
	parts := strings.SplitN(tabID, "~", 2)
	if len(parts) != 2 {
		return 0, errInvalidTabID
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(raw) != 8 {
		return 0, errInvalidTabID
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errInvalidTabID
	}
	mac := hmac.New(sha256.New, a.hmacKey[:])
	mac.Write(raw)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return 0, errInvalidTabID
	}
	return binary.BigEndian.Uint64(raw), nil
}

func (a *App) streamState(streamID uint64) *tabState {
	a.lockTabs.RLock()
	defer a.lockTabs.RUnlock()
	ts := a.streamIDToTabState[streamID]
	if ts == nil {
		return nil
	}
	cp := *ts
	return &cp
}

func (a *App) patchTabID(streamID uint64, sse *datastar.ServerSentEventGenerator) error {
	return sse.MarshalAndPatchSignals(struct {
		TabID string `json:"tab_id"`
	}{TabID: a.signStreamID(streamID)})
}
