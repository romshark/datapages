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

// GETFile is /files/{id}
//
// Serves the blob of a completed file, paced by the download limit.
//
// This example authenticates nobody: every uploaded file is readable by
// whoever knows its 128 bit identifier.
func (a *App) GETFile(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (datapages.File, error) {
	// The identifier never reaches a path: the store looks it up and answers
	// with the blob it minted the name of.
	f, err := a.files.Get(path.Values.ID)
	if err != nil {
		return datapages.File{}, downloadErr(err)
	}
	blob, err := a.files.Open(path.Values.ID)
	if err != nil {
		return datapages.File{}, downloadErr(err)
	}
	info, err := blob.Stat()
	if err != nil {
		return datapages.File{}, errors.Join(err, blob.Close())
	}
	return datapages.File{
		// The type is what the browser reported on upload: the blob is named
		// after the identifier and carries no extension to sniff.
		Type: f.ContentType,
		// ServeContent writes what it reads, which is why pacing the reader
		// paces the response. It also rules out sendfile, which would hand the
		// file to the kernel and leave nothing to pace.
		// The paced reader has no Close. The blob's is what closes it.
		Body: struct {
			io.ReadSeeker
			io.Closer
		}{throttle.ReadSeeker(r.Context(), blob, a.download), blob},
		ModTime: info.ModTime(),
		// The bytes under one identifier never change,
		// and a deleted file takes its identifier with it.
		Cache: datapages.FileCache{
			Private: true, MaxAge: 365 * 24 * time.Hour, Immutable: true,
		},
		// Download keeps an uploaded HTML or SVG file, whose type the
		// uploading browser chose, from running script on this origin.
		Disposition: datapages.FileDisposition{Download: true, Filename: f.Name},
	}, nil
}

// downloadErr reports a file that has nothing to serve as a 404. An incomplete
// file is a 404 rather than a 409: the URL starts working once the bytes are there.
func downloadErr(err error) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrIncomplete) {
		return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)
	}
	return err
}
