package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/docs/app/datapagesgen/href"
	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/store"
)

// PageDoc is /doc/{id}
type PageDoc struct{ App *App }

func (p PageDoc) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (
	body datapages.Component,
	head datapages.Head,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	client := store.NewClient()
	doc, err := p.App.docs.Get(path.Values.ID)
	var v store.View
	if err == nil {
		v, err = p.App.docs.View(doc.ID, client, doc.Rev)
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, false, datapages.ErrNotFound
	}
	if err != nil {
		return nil, nil, false, err
	}
	// The tab joins when its stream opens, after everyone already there.
	me := store.Identity(client)
	me.You = true
	// A hidden tab keeps its stream open. Reloading it when shown again would
	// lose the undo history and the cursor position.
	return pageDoc(
		doc, client, append(v.Collaborators, me),
	), headDoc(doc.Title), true, nil
}

func (p PageDoc) StreamOpen(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateDoc],
	signals datapages.Signals[struct {
		DocID  string `json:"docId"`
		Client string `json:"client"`
		Rev    int    `json:"rev"`
	}],
	docChanged datapages.Dispatcher[EventDocChanged],
) error {
	st := state.Values
	*st = StateDoc{
		DocID:  signals.Values.DocID,
		Client: signals.Values.Client,
		Rev:    signals.Values.Rev,
	}
	switch err := p.App.docs.Join(st.DocID, st.Client); {
	case errors.Is(err, store.ErrNotFound):
		return sse.Redirect(href.PageIndex())
	case errors.Is(err, store.ErrInvalid):
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	case err != nil:
		return err
	}
	changed := EventDocChanged{Doc: datapages.Subject(st.DocID)}
	// [PageDoc.StreamClose] does not run if [PageDoc.StreamOpen] fails or panics.
	// Call Leave here instead.
	opened := false
	defer func() {
		if !opened {
			_ = p.App.docs.Leave(st.DocID, st.Client)
			_ = docChanged.Dispatch(changed)
		}
	}()
	if err := docChanged.Dispatch(changed); err != nil {
		return err
	}
	if err := p.sync(sse, st); err != nil {
		return err
	}
	opened = true
	return nil
}

func (p PageDoc) StreamClose(
	r *http.Request,
	state datapages.State[StateDoc],
	docChanged datapages.Dispatcher[EventDocChanged],
) error {
	err := p.App.docs.Leave(state.Values.DocID, state.Values.Client)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return docChanged.Dispatch(EventDocChanged{
		Doc: datapages.Subject(state.Values.DocID),
	})
}

func (p PageDoc) OnDocChanged(
	event EventDocChanged,
	sse datapages.SSE,
	state datapages.State[StateDoc],
) error {
	return p.sync(sse, state.Values)
}

// sync sends a tab what changed since the last sync: the changes after its revision,
// the cursors, the collaborators and the title. It reads all of it from the store,
// and the next event delivers what a dropped one missed.
func (p PageDoc) sync(sse datapages.SSE, st *StateDoc) error {
	if st.Stopped {
		return nil
	}
	v, err := p.App.docs.View(st.DocID, st.Client, st.Rev)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return sse.Redirect(href.PageIndex())
	case errors.Is(err, store.ErrStale):
		st.Stopped = true
		return stopEditor(sse, "This tab fell too far behind the document.")
	case err != nil:
		return err
	}
	if len(v.Changes) > 0 || v.Cursors != st.Cursors {
		script, err := editorCall("receive", newUpdate(v, st.Client))
		if err != nil {
			return err
		}
		if err := sse.ExecuteScript(script); err != nil {
			return err
		}
		st.Rev, st.Cursors = v.Rev, v.Cursors
	}
	if v.Presence != st.Presence {
		if err := sse.PatchElement(collaboratorList(v.Collaborators)); err != nil {
			return err
		}
		st.Presence = v.Presence
	}
	if v.Title != st.Title {
		if err := sse.PatchElement(titleInput(st.DocID, v.Title)); err != nil {
			return err
		}
		if err := sse.PatchElement(headTitle(v.Title)); err != nil {
			return err
		}
		// The editor stores the title with the document in the browser.
		if err := sse.PatchSignals(map[string]string{"_title": v.Title}); err != nil {
			return err
		}
		st.Title = v.Title
	}
	return nil
}

// POSTSubmit is /doc/{id}/submit
//
// It applies a change the editor made at revision rev. The stream confirms it by
// sending it back among the changes of others, in the order the server applied them.
func (p PageDoc) POSTSubmit(
	r *http.Request,
	sse datapages.SSE,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Client string          `json:"client"`
		Seq    int             `json:"seq"`
		Rev    int             `json:"rev"`
		Delta  json.RawMessage `json:"delta"`
	}],
	docChanged datapages.Dispatcher[EventDocChanged],
	listChanged datapages.Dispatcher[EventListChanged],
) error {
	sub := store.Submission{
		Client: signals.Values.Client,
		Seq:    signals.Values.Seq,
		Rev:    signals.Values.Rev,
	}
	err := json.Unmarshal(signals.Values.Delta, &sub.Delta)
	if err != nil {
		err = fmt.Errorf("%w: %w", store.ErrInvalid, err)
	} else {
		err = p.App.docs.Submit(path.Values.ID, sub)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		return sse.Redirect(href.PageIndex())
	case errors.Is(err, store.ErrStale):
		return stopEditor(sse, "This tab fell too far behind the document.")
	case errors.Is(err, store.ErrInvalid):
		return stopEditor(sse, "The server could not apply a change from this tab.")
	case err != nil:
		return err
	}
	return errors.Join(
		docChanged.Dispatch(EventDocChanged{Doc: datapages.Subject(path.Values.ID)}),
		listChanged.Dispatch(EventListChanged{}),
	)
}

// POSTRestore is /doc/{id}/restore
//
// It applies the changes a closed tab kept in the browser and the server
// never confirmed. The streams deliver them to every tab as changes of another editor.
// Changes that no longer apply go to a new document with the content the closed
// tab showed. Either way the editor then deletes what the closed tab kept.
func (p PageDoc) POSTRestore(
	r *http.Request,
	sse datapages.SSE,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Client  string          `json:"client"`
		Seq     int             `json:"seq"`
		Rev     int             `json:"rev"`
		Sent    json.RawMessage `json:"sent"`
		Buffer  json.RawMessage `json:"buffer"`
		Title   string          `json:"title"`
		Content json.RawMessage `json:"content"`
	}],
	docChanged datapages.Dispatcher[EventDocChanged],
	listChanged datapages.Dispatcher[EventListChanged],
) error {
	s := signals.Values
	pending := store.Pending{Client: s.Client, Seq: s.Seq, Rev: s.Rev}
	err := errors.Join(
		json.Unmarshal(s.Sent, &pending.Sent),
		json.Unmarshal(s.Buffer, &pending.Buffer),
	)
	if err != nil {
		err = fmt.Errorf("%w: %w", store.ErrInvalid, err)
	} else {
		err = p.App.docs.Restore(path.Values.ID, pending)
	}
	switch {
	case err == nil:
		err = errors.Join(
			docChanged.Dispatch(EventDocChanged{Doc: datapages.Subject(path.Values.ID)}),
			listChanged.Dispatch(EventListChanged{}),
		)
	case errors.Is(err, store.ErrNotFound),
		errors.Is(err, store.ErrStale),
		errors.Is(err, store.ErrInvalid):
		err = p.saveCopy(sse, s.Title, s.Content, listChanged)
	}
	if err != nil {
		return err
	}
	script, err := editorCall("restored", s.Client)
	if err != nil {
		return err
	}
	return sse.ExecuteScript(script)
}

// saveCopy creates a document with content, which a closed tab showed with
// changes the document no longer takes, and links it in a toast.
func (p PageDoc) saveCopy(
	sse datapages.SSE, title string, content json.RawMessage,
	listChanged datapages.Dispatcher[EventListChanged],
) error {
	var doc delta.Delta
	if err := json.Unmarshal(content, &doc); err != nil {
		return sse.PatchElement(errorToast("Changes kept in this browser could not be saved."))
	}
	id, err := p.App.docs.Create(title+" (offline copy)", doc)
	if errors.Is(err, store.ErrInvalid) {
		return sse.PatchElement(errorToast("Changes kept in this browser could not be saved."))
	}
	if err != nil {
		return err
	}
	if err := listChanged.Dispatch(EventListChanged{}); err != nil {
		return err
	}
	return sse.PatchElement(copyToast(id))
}

// POSTCursor is /doc/{id}/cursor
//
// It records the cursor of the editor at revision rev.
func (p PageDoc) POSTCursor(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Client string `json:"client"`
		Seq    int    `json:"seq"`
		Rev    int    `json:"rev"`
		Index  int    `json:"index"`
		Length int    `json:"length"`
	}],
	docChanged datapages.Dispatcher[EventDocChanged],
) error {
	s := signals.Values
	moved, err := p.App.docs.MoveCursor(path.Values.ID, s.Client, s.Seq, s.Rev,
		store.Cursor{Index: s.Index, Length: s.Length})
	switch {
	case errors.Is(err, store.ErrNotFound):
		// The stream sends the tab to the document list.
		return nil
	case errors.Is(err, store.ErrInvalid):
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	case err != nil:
		return err
	case !moved:
		return nil
	}
	return docChanged.Dispatch(EventDocChanged{Doc: datapages.Subject(path.Values.ID)})
}

// POSTRename is /doc/{id}/rename
func (p PageDoc) POSTRename(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Title string `json:"title"`
	}],
	docChanged datapages.Dispatcher[EventDocChanged],
	listChanged datapages.Dispatcher[EventListChanged],
) error {
	err := p.App.docs.Rename(path.Values.ID, signals.Values.Title)
	if errors.Is(err, store.ErrNotFound) {
		return datapages.ErrNotFound
	}
	if err != nil {
		return err
	}
	return errors.Join(
		docChanged.Dispatch(EventDocChanged{Doc: datapages.Subject(path.Values.ID)}),
		listChanged.Dispatch(EventListChanged{}),
	)
}

// POSTImage is /doc/{id}/image
//
// It stores an image the editor uploads and passes its URL path to
// insertImage of the editor, which inserts the image as its own change.
// An empty URL path tells the editor that the server refused the file.
func (p PageDoc) POSTImage(
	r *http.Request,
	sse datapages.SSE,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Upload int    `json:"upload"`
		Data   []byte `json:"data"`
	}],
) error {
	if _, err := p.App.docs.Get(path.Values.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// The stream sends the tab to the document list.
			return nil
		}
		return err
	}
	src, err := p.App.images.Save(signals.Values.Data)
	switch {
	case errors.Is(err, images.ErrTooLarge):
		err = sse.PatchElement(errorToast(
			fmt.Sprintf("The image is larger than %d MB.", images.MaxSize>>20),
		))
	case errors.Is(err, images.ErrInvalid):
		err = sse.PatchElement(errorToast("The file is not a PNG, JPEG, GIF or WebP image."))
	}
	if err != nil {
		return err
	}
	script, err := editorCall("insertImage", imageUpload{signals.Values.Upload, src})
	if err != nil {
		return err
	}
	return sse.ExecuteScript(script)
}

// imageUpload tells the editor where the server stored an upload.
type imageUpload struct {
	Upload int    `json:"upload"`
	Src    string `json:"src"`
}

// update is what the editor receives: the changes it lacks
// and the cursors of the others after them.
type update struct {
	Changes []updateChange `json:"changes"`
	Cursors []updateCursor `json:"cursors"`
}

type updateChange struct {
	Rev   int         `json:"rev"`
	Delta delta.Delta `json:"delta"`
	// Ack is the sequence number the editor sent the change with,
	// 0 for changes of others.
	Ack int `json:"ack,omitempty"`
}

type updateCursor struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	Index  int    `json:"index"`
	Length int    `json:"length"`
}

func newUpdate(v store.View, client string) update {
	u := update{
		Changes: make([]updateChange, len(v.Changes)),
		Cursors: []updateCursor{},
	}
	for i, c := range v.Changes {
		u.Changes[i] = updateChange{Rev: c.Rev, Delta: c.Delta}
		if c.Client == client {
			u.Changes[i].Ack = c.Seq
		}
	}
	for _, c := range v.Collaborators {
		if c.You || c.Cursor == nil {
			continue
		}
		u.Cursors = append(u.Cursors, updateCursor{
			Name: c.Name, Color: c.Color,
			Index: c.Cursor.Index, Length: c.Cursor.Length,
		})
	}
	return u
}

// editorCall returns a script that calls method on the editor element.
// A script arriving before the bundle defines the element waits for it.
// Calls run in arrival order. [encoding/json] escapes "<" and ">", which keeps
// arg from closing the script element.
func editorCall(method string, arg any) (string, error) {
	b, err := json.Marshal(arg)
	if err != nil {
		return "", fmt.Errorf("encoding the argument of %s: %w", method, err)
	}
	return `customElements.whenDefined("docs-editor").then(() => ` +
		`document.getElementById("editor").` + method + `(` + string(b) + `))`, nil
}

// stopEditor makes the editor read-only and shows message with a reload link.
func stopEditor(sse datapages.SSE, message string) error {
	script, err := editorCall("stop", message)
	if err != nil {
		return err
	}
	return sse.ExecuteScript(script)
}
