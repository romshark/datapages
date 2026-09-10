// Package action is a stub so the IDE can resolve action references
// in app.templ without errors. The parser only matches action calls in templ
// expressions; it does not compile this package.
package action

// PageProfile holds the actions of PageProfile.
var PageProfile pageProfile

type pageProfile struct {
	Save pageProfile_Save
}

type pageProfile_Save struct{}

func (pageProfile_Save) POST() string { return "" }

// PageSettings holds the actions of PageSettings.
var PageSettings pageSettings

type pageSettings struct {
	Update pageSettings_Update
}

type pageSettings_Update struct{}

func (pageSettings_Update) POST() string { return "" }

// App holds the actions of App.
var App app

type app struct {
	Global app_Global
}

type app_Global struct{}

func (app_Global) POST() string { return "" }
