//nolint:all
package app

import (
	"net/http"

	"datapagestest/fixture/err_templ_subpackage/template"
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
	return template.ProfilePage(), nil
}

// POSTSave is /profile/save
func (PageProfile) POSTSave(r *http.Request) error { return nil }

// PageSettings is /settings
type PageSettings struct{ App *App }

func (PageSettings) GET(r *http.Request) (body templ.Component, err error) {
	return template.SettingsPage(), nil
}
