package httpserve

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"

	"github.com/romshark/datapages"
)

// IsDatastarRequest reports whether h carries the header the Datastar client sends.
func IsDatastarRequest(h http.Header) bool {
	return h.Get("Datastar-Request") == "true"
}

// Redirect writes the redirect to w and reports whether it wrote one.
// A Datastar request cannot follow an HTTP redirect, which is why one
// navigates client-side instead.
func Redirect(
	w http.ResponseWriter, r *http.Request, redirect datapages.Redirect,
) (exit bool) {
	if redirect.URL == "" {
		return false
	}

	if IsDatastarRequest(r.Header) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = fmt.Fprintf(w, "window.location = %q;", redirect.URL)
		return true
	}

	status := redirect.Status
	switch status {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		// OK
	default:
		status = http.StatusFound
	}

	http.Redirect(w, r, redirect.URL, status)
	return true
}

// DevNoCache stops the browser from caching what next serves.
func DevNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// WriteReloadOnVisibility writes the body attribute that reloads a page
// the browser shows again after the server restarted.
//
// Like every attribute writer [Core.WriteHTML] calls,
// it opens with the space that separates it from what stands before it.
func WriteReloadOnVisibility(w io.Writer) {
	_, _ = io.WriteString(w,
		` data-on:visibilitychange__window="`+
			`if (!document.hidden) window.location.reload()"`)
}

// AssetsFileSystem is what the static files of an application are served from.
//
// dir is the subdirectory of the embed.FS the application declared and devDir
// the path the same files live at in the source tree. An empty dir means the
// application declares no assets, which makes datapages.WithAssets an error.
//
// In dev mode (datapages.IsDevMode) the files are read from devDir on disk so
// that a change reloads without recompilation.
//
// Directory listing is decided by [NewCore], which sees the file system of
// datapages.WithAssetsFS as well.
func AssetsFileSystem(
	cfg datapages.ServerConfig, devDir, dir string,
) (http.FileSystem, error) {
	if cfg.AssetsFS != nil {
		return cfg.AssetsFS, nil
	}
	if cfg.AssetsEmbed == nil {
		return nil, nil
	}
	if dir == "" {
		return nil, errors.New(
			"datapages.WithAssets: the app package declares no assets",
		)
	}
	if datapages.IsDevMode() {
		return http.Dir(devDir), nil
	}
	sub, err := fs.Sub(*cfg.AssetsEmbed, dir)
	if err != nil {
		return nil, fmt.Errorf("datapages.WithAssets: %w", err)
	}
	return http.FS(sub), nil
}

// notBrowsableFS returns fs.ErrNotExist for a directory that holds no index.html,
// which [http.FileServer] turns into 404.
// Without it a GET on the assets URL prefix lists the whole tree.
type notBrowsableFS struct{ fsys http.FileSystem }

func (f notBrowsableFS) Open(name string) (http.File, error) {
	file, err := f.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !stat.IsDir() {
		return file, nil
	}
	// A directory holding an index.html stays open: [http.FileServer] serves
	// that file through a second Open and never lists such a directory.
	index, err := f.fsys.Open(path.Join(name, "index.html"))
	if err != nil {
		_ = file.Close()
		return nil, fs.ErrNotExist
	}
	_ = index.Close()
	return file, nil
}

// WriteErrStatus writes the HTTP error response err maps to.
// The datapages error sentinels select the status, anything else is a 500.
func WriteErrStatus(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, datapages.ErrBadRequest):
		code = http.StatusBadRequest
	case errors.Is(err, datapages.ErrForbidden):
		code = http.StatusForbidden
	case errors.Is(err, datapages.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, datapages.ErrConflict):
		code = http.StatusConflict
	}
	http.Error(w, http.StatusText(code), code)
}
