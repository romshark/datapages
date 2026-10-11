package app

import (
	"embed"
	"encoding/json"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/store"
)

// BodySizeLimit bounds the body of an action. Pasting a document of
// [store.MaxLength] code units exceeds the 1 MiB default, and an image of
// [images.MaxSize] bytes arrives base64-encoded, at 4/3 of its size.
const BodySizeLimit = 8 << 20

// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS

// EventDocChanged is "doc.changed"
//
// It reports a change to the content, title, collaborators or a cursor of
// 1 document. Pages with the document open read the changes from the store.
type EventDocChanged struct {
	Doc datapages.Subject `json:"doc" signal:"docId"`
}

// EventListChanged is "list.changed"
//
// It reports that a document was created, renamed, edited or deleted.
type EventListChanged struct{}

// StateDoc is what 1 tab of [PageDoc] got last, to tell what to send next.
type StateDoc struct {
	DocID string
	// Client identifies the editor of the tab and authorizes its changes.
	// Never sent to another tab.
	Client string

	Rev               int
	Presence, Cursors int
	Title             string

	// Stopped reports that the editor is read-only.
	Stopped bool
}

type App struct {
	docs   *store.Store
	images *images.Store
}

func NewApp(docs *store.Store, imgs *images.Store) *App {
	return &App{docs: docs, images: imgs}
}

func (*App) Head(r *http.Request) datapages.Head { return head() }

func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElement(errorToast("Something went wrong. Try again."))
}

// jsStr encodes s as a JavaScript string for a Datastar attribute.
func jsStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
