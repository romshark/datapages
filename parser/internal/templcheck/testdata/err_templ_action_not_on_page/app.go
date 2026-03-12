//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return indexPage(), nil
}

// PageProfile is /profile
type PageProfile struct{ App *App }

func (PageProfile) GET(r *http.Request) (body templ.Component, err error) {
	return profilePage(), nil
}

// POSTSave is /profile/save
func (PageProfile) POSTSave(r *http.Request) error { return nil }

// PageSettings is /settings
type PageSettings struct{ App *App }

func (PageSettings) GET(r *http.Request) (body templ.Component, err error) {
	return settingsPage(), nil
}

// POSTUpdate is /settings/update
func (PageSettings) POSTUpdate(r *http.Request) error { return nil }

// POSTAppGlobal is /global
func (*App) POSTGlobal(r *http.Request) error { return nil }
