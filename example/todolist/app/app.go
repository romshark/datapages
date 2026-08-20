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

	"github.com/romshark/datapages"
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
	streamIDToTabState map[datapages.StreamID]*tabState
}

func NewApp(hmacKey [32]byte, list *list.List) *App {
	return &App{
		hmacKey:            hmacKey,
		list:               list,
		streamIDToTabState: make(map[datapages.StreamID]*tabState),
	}
}

func (*App) Head(r *http.Request) datapages.Head { return head() }

// PUTEdit is /{id}
//
// This action is shared across all pages.
func (a *App) PUTEdit(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	query datapages.Query[struct {
		Toggle bool `query:"toggle"`
	}],
	signals datapages.Signals[struct {
		TabID       string `json:"tab_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Done        bool   `json:"done"`
		Due         string `json:"due"`
	}],
	todoUpdated datapages.Dispatcher[EventTodoUpdated],
) error {
	if _, err := a.verifyTabID(signals.Values.TabID); err != nil {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
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

// signStreamID produces an HMAC-signed tab identifier from a streamID.
func (a *App) signStreamID(streamID datapages.StreamID) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(streamID))
	mac := hmac.New(sha256.New, a.hmacKey[:])
	mac.Write(buf[:])
	return base64.RawURLEncoding.EncodeToString(buf[:]) +
		"~" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var errInvalidTabID = errors.New("invalid tab ID")

// verifyTabID verifies the HMAC signature and extracts the streamID.
func (a *App) verifyTabID(tabID string) (datapages.StreamID, error) {
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
	return datapages.StreamID(binary.BigEndian.Uint64(raw)), nil
}

func (a *App) streamState(streamID datapages.StreamID) *tabState {
	a.lockTabs.RLock()
	defer a.lockTabs.RUnlock()
	ts := a.streamIDToTabState[streamID]
	if ts == nil {
		return nil
	}
	cp := *ts
	return &cp
}

func (a *App) patchTabID(streamID datapages.StreamID, sse datapages.SSE) error {
	return sse.PatchSignals(struct {
		TabID string `json:"tab_id"`
	}{TabID: a.signStreamID(streamID)})
}
