package httpserve

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"

	"github.com/romshark/datapages"
)

// IsDatastarRequest reports whether r was issued by the Datastar client.
func IsDatastarRequest(r *http.Request) bool {
	return r.Header.Get("Datastar-Request") == "true"
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

	if IsDatastarRequest(r) {
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
func WriteReloadOnVisibility(w io.Writer) {
	_, _ = io.WriteString(w,
		`data-on:visibilitychange__window="`+
			`if (!document.hidden) window.location.reload()" `)
}

// AssetsFileSystem is what the static files of an application are served from.
//
// dir is the subdirectory of the embed.FS the application declared and devDir
// the path the same files live at in the source tree. An empty dir means the
// application declares no assets, which makes datapages.WithAssets an error.
//
// In dev mode (datapages.IsDevMode) the files are read from devDir on disk so
// that a change reloads without recompilation.
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
