package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageUser is /user
type PageUser struct{ App *App }

func (PageUser) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTSettingsSave is /user/settings-save
func (PageUser) POSTSettingsSave(_ *http.Request) error { return nil }

// PageUserSettings is /user/settings
type PageUserSettings struct{ App *App }

func (PageUserSettings) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTSave is /user/settings/save
func (PageUserSettings) POSTSave(_ *http.Request) error /* ErrGeneratedNameConflict */ {
	return nil
}
