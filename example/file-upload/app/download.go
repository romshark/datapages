package app

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/romshark/datapages/example/file-upload/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/file-upload/store"
	"github.com/romshark/datapages/example/file-upload/throttle"
)

// DownloadPrefix is where a completed upload is served. It sits below the
// asset prefix so that the templates can build the link with href.Asset,
// which the linter checks, instead of writing a URL by hand.
const DownloadPrefix = assets.URLPrefix + "files/"

// Downloads serves the blob of a completed file. It's middleware because a
// page renders a component and cannot write bytes, and not an asset file
// system because a handler is where the response headers, the download limit and,
// in an application that has visitors, the authorization check belong.
//
// This example authenticates nobody: every uploaded file is readable by
// whoever knows its 128 bit identifier.
func Downloads(a *App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := strings.CutPrefix(r.URL.Path, DownloadPrefix)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed),
					http.StatusMethodNotAllowed)
				return
			}
			serveBlob(w, r, a, id)
		})
	}
}

func serveBlob(w http.ResponseWriter, r *http.Request, a *App, id string) {
	// The identifier never reaches a path: the store looks it up and answers
	// with the blob it minted the name of.
	f, err := a.files.Get(id)
	if err != nil {
		httpErr(w, err)
		return
	}
	blob, err := a.files.Open(id)
	if err != nil {
		httpErr(w, err)
		return
	}
	defer func() { _ = blob.Close() }()
	info, err := blob.Stat()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}

	// The type is what the browser reported on upload: the blob is named after
	// the identifier and carries no extension to sniff.
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType(
		"attachment", map[string]string{"filename": f.Name},
	))
	// The bytes under one identifier never change,
	// and a deleted file takes its identifier with it.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	// ServeContent writes what it reads,
	// which is why pacing the reader paces the response.
	// It also rules out sendfile, which would hand the file to the
	// kernel and leave nothing to pace.
	http.ServeContent(w, r, "", info.ModTime(),
		throttle.ReadSeeker(r.Context(), blob, a.download))
}

// httpErr answers a download that has nothing to serve. An incomplete file is
// a 404 rather than a 409: the URL starts working once the bytes are there.
func httpErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrIncomplete) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	http.Error(w, http.StatusText(http.StatusInternalServerError),
		http.StatusInternalServerError)
}
