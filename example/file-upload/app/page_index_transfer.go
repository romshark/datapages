package app

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/file-upload/store"
	"github.com/romshark/datapages/example/file-upload/throttle"
)

const (
	// maxChunkBody stops a client that does not follow the protocol from
	// streaming without end. The uploader sends a megabyte per chunk.
	maxChunkBody = 4 << 20

	progressInterval = 250 * time.Millisecond
)

// PUTChunk is /upload/{id}
//
// The request body is the chunk and offset says where it belongs. It's the
// one handler the uploader calls with fetch instead of a Datastar action,
// since a File is sliced and streamed by the browser and never passes through
// a signal. Its status codes are the protocol the uploader reacts to:
//
//	403  the file is paused or complete, stop sending
//	404  the file is gone, forget it
//	409  the offset leaves a hole, the transfer has to be resumed
func (p PageIndex) PUTChunk(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	query datapages.Query[struct {
		Offset int64 `query:"offset"`
	}],
	filesChanged datapages.Dispatcher[EventFilesChanged],
) error {
	// Reading the body slowly fills the socket buffers and makes the browser slow down.
	// The ticker reports while the bytes arrive, which moves the progress bar
	// during a slow chunk instead of jumping when the request ends.
	body := throttle.Reader(
		r.Context(), io.LimitReader(r.Body, maxChunkBody), p.App.upload,
	)
	_, err := p.App.files.Append(
		path.Values.ID, query.Values.Offset,
		&tickReader{r: body, every: progressInterval, fn: func() {
			_ = filesChanged.Dispatch(EventFilesChanged{})
		}},
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)
	case errors.Is(err, store.ErrNotUploading):
		return fmt.Errorf("%w: %w", datapages.ErrForbidden, err)
	case errors.Is(err, store.ErrGap):
		return fmt.Errorf("%w: %w", datapages.ErrConflict, err)
	case err != nil:
		return err
	}
	return filesChanged.Dispatch(EventFilesChanged{})
}

// POSTPause is /pause/{id}
//
// Pausing is the visitor pressing Pause and nothing else: a transfer that
// fails in the browser changes no state on the server. Only the tab sending
// the bytes may pause them, since no other tab can go on with the transfer.
func (p PageIndex) POSTPause(
	r *http.Request,
	state datapages.State[StateIndex], // stateID needs it, the handler doesn't
	stateID string,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	filesChanged datapages.Dispatcher[EventFilesChanged],
) error {
	if err := p.ownedBy(path.Values.ID, stateID); err != nil {
		return err
	}
	_, err := p.App.files.SetStatus(path.Values.ID, store.StatusPaused)
	switch {
	case errors.Is(err, store.ErrNotUploading):
		// The file completed between the last chunk and this request.
		return nil
	case errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)
	case err != nil:
		return err
	}
	return filesChanged.Dispatch(EventFilesChanged{})
}

// POSTResume is /resume/{id}
//
// A tab that does not hold the bytes takes the transfer over instead,
// see [PageIndex.POSTReattach]. The uploader starts sending only once the store
// expects bytes again, hence the command travels back on this response.
func (p PageIndex) POSTResume(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateIndex], // stateID needs it, the handler does not
	stateID string,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	filesChanged datapages.Dispatcher[EventFilesChanged],
) error {
	if err := p.ownedBy(path.Values.ID, stateID); err != nil {
		return err
	}
	return p.resume(sse, path.Values.ID, filesChanged)
}

// POSTReattach is /reattach
//
// It checks that the file the visitor picked is the one the store is missing bytes of,
// then hands the transfer to this tab and resumes it.
//
// Name and size are all a browser reveals before the bytes are read.
// An application that must not append the wrong bytes would keep a checksum of
// what arrived and have the client prove it.
func (p PageIndex) POSTReattach(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateIndex], // stateID needs it, the handler doesn't
	stateID string,
	signals datapages.Signals[struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Size int64  `json:"size"`
	}],
	filesChanged datapages.Dispatcher[EventFilesChanged],
) error {
	f, err := p.App.files.Get(signals.Values.ID)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)
	} else if err != nil {
		return err
	}
	if f.OriginalName != signals.Values.Name || f.Size != signals.Values.Size {
		return fmt.Errorf(
			"%w: %q of %d bytes is not %q of %d bytes",
			datapages.ErrBadRequest,
			signals.Values.Name, signals.Values.Size, f.OriginalName, f.Size,
		)
	}
	if _, err := p.App.files.SetOwner(f.ID, stateID); err != nil {
		return err
	}
	return p.resume(sse, f.ID, filesChanged)
}

// ownedBy fails unless the tab named by stateID holds
// the bytes of the file id refers to.
func (p PageIndex) ownedBy(id, stateID string) error {
	f, err := p.App.files.Get(id)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)
	} else if err != nil {
		return err
	}
	if f.Owner != stateID {
		return fmt.Errorf(
			"%w: this tab does not hold the bytes of that file",
			datapages.ErrForbidden,
		)
	}
	return nil
}

// resume marks the file as expecting bytes and tells the calling tab to send them.
func (p PageIndex) resume(
	sse datapages.SSE, id string, filesChanged datapages.Dispatcher[EventFilesChanged],
) error {
	f, err := p.App.files.SetStatus(id, store.StatusUploading)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)
	case errors.Is(err, store.ErrNotUploading):
		return fmt.Errorf("%w: %w", datapages.ErrConflict, err)
	case err != nil:
		return err
	}
	if err := filesChanged.Dispatch(EventFilesChanged{}); err != nil {
		return err
	}
	script, err := callScript("dpUpload.go", newUploadJob(f, p.App.ChunkSize()))
	if err != nil {
		return err
	}
	return sse.ExecuteScript(script)
}
