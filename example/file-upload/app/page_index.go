package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/file-upload/store"
)

// PageIndex is /
type PageIndex struct{ App *App }

// GET keeps the stream open while the tab is hidden. Without it a tab that
// comes back reloads, which drops the File objects it was sending.
func (p PageIndex) GET(r *http.Request) (
	body datapages.Component,
	background datapages.EnableBackgroundStreaming,
	err error,
) {
	// GET cannot read stateID, and does not need to: a tab that just loaded
	// holds no File and is therefore sending nothing.
	return pageIndex(
		p.App.files.List(), "", originOf(r), p.App.files.Dir(), p.App.Limits(),
	), true, nil
}

// StreamOpen records where this tab reached the server, which is the only
// point an event handler can learn it: it has no request of its own.
func (p PageIndex) StreamOpen(
	r *http.Request, state datapages.State[StateIndex],
) error {
	state.Values.Origin = originOf(r)
	return nil
}

// originOf is the scheme and host a link back to this server has to name.
// A proxy is trusted for it, since the address it forwards is the one the
// visitor typed and what the server sees is the hop in front of it.
func originOf(r *http.Request) string {
	scheme := "http"
	switch {
	case r.Header.Get("X-Forwarded-Proto") != "":
		scheme = r.Header.Get("X-Forwarded-Proto")
	case r.TLS != nil:
		scheme = "https"
	}
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}
	return scheme + "://" + host
}

// DELETEFile is /files/{id}
func (p PageIndex) DELETEFile(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	filesChanged datapages.Dispatcher[EventFilesChanged],
) error {
	if err := p.App.files.Delete(path.Values.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)
		}
		return err
	}
	return filesChanged.Dispatch(EventFilesChanged{})
}

func (p PageIndex) OnFilesChanged(
	event EventFilesChanged,
	sse datapages.SSE,
	state datapages.State[StateIndex],
	stateID string,
) error {
	return sse.PatchElement(
		fileList(p.App.files.List(), stateID, state.Values.Origin),
	)
}

// StreamClose gives up what this tab was sending, so that another tab can take it over.
// The status stays as it is: a file the visitor did not pause is still one the server
// expects the rest of, only the sender is missing.
func (p PageIndex) StreamClose(
	r *http.Request,
	state datapages.State[StateIndex], // stateID needs it, the handler doesn't
	stateID string,
	filesChanged datapages.Dispatcher[EventFilesChanged],
) error {
	if !p.App.files.ReleaseOwner(stateID) {
		return nil
	}
	return filesChanged.Dispatch(EventFilesChanged{})
}
