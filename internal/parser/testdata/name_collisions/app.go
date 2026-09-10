//nolint:all

// Package app carries every pair of names that concatenate to one identifier
// when spelled as one. Each pair is valid Go with distinct routes.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

// GET takes the session, which is how the parser finds
// the Session type EventDirect needs.
func (PageIndex) GET(
	r *http.Request, session Session,
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

// A page name ending in a verb against another page's action:
// PageAPOSTB.GET and PageA.POSTBGET concatenate to handlePageAPOSTBGET.

// PageAPOSTB is /apostb
type PageAPOSTB struct{ App *App }

func (PageAPOSTB) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageA is /a
type PageA struct{ App *App }

func (PageA) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTBGET is /a/bget
func (PageA) POSTBGET(r *http.Request) error { return nil }

// The same against a stream handler: PageCPOSTD's stream and
// PageC.POSTDGETStream concatenate to handlePageCPOSTDGETStream.

// PageCPOSTD is /cpostd
type PageCPOSTD struct{ App *App }

func (PageCPOSTD) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageCPOSTD) OnTick(_ EventTick, _ datapages.SSE) error { return nil }

// PageC is /c
type PageC struct{ App *App }

func (PageC) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTDGETStream is /c/dgetstream
func (PageC) POSTDGETStream(r *http.Request) error { return nil }

// The same against an anonymous stream handler, which a page mixing a public
// and a user-addressed event is served:
// PageIPOSTJ's anonymous stream and PageI.POSTJGETStreamAnon concatenate to
// handlePageIPOSTJGETStreamAnon.

// PageIPOSTJ is /ipostj
type PageIPOSTJ struct{ App *App }

func (PageIPOSTJ) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageIPOSTJ) OnTick(_ EventTick, _ datapages.SSE) error { return nil }

func (PageIPOSTJ) OnDirect(_ EventDirect, _ datapages.SSE) error { return nil }

// PageI is /i
type PageI struct{ App *App }

func (PageI) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTJGETStreamAnon is /i/jgetstreamanon
func (PageI) POSTJGETStreamAnon(r *http.Request) error { return nil }

// EventDirect is "direct"
type EventDirect struct {
	To datapages.SubjectUser `json:"to"`

	Text string `json:"text"`
}

// A page suffix against an action name:
// PageE.POSTFG and PageEF.POSTG concatenate to POSTPageEFG.

// PageE is /e
type PageE struct{ App *App }

func (PageE) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTFG is /e/fg
func (PageE) POSTFG(r *http.Request) error { return nil }

// PageEF is /ef
type PageEF struct{ App *App }

func (PageEF) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTG is /ef/g
func (PageEF) POSTG(r *http.Request) error { return nil }

// The same where both take a query, whose argument type concatenates to one
// QueryPOSTPageUserSettingsSave:
// PageUser.POSTSettingsSave and PageUserSettings.POSTSave.

// PageUser is /user
type PageUser struct{ App *App }

func (PageUser) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTSettingsSave is /user/settings-save
func (PageUser) POSTSettingsSave(
	r *http.Request,
	query datapages.Query[struct {
		Force bool `query:"force"`
	}],
) error {
	_ = query
	return nil
}

// PageUserSettings is /user/settings
type PageUserSettings struct{ App *App }

func (PageUserSettings) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTSave is /user/settings/save
func (PageUserSettings) POSTSave(
	r *http.Request,
	query datapages.Query[struct {
		Force bool `query:"force"`
	}],
) error {
	_ = query
	return nil
}

// An app action against a page action:
// App.POSTHI and a PageAppH.POSTI concatenate to POSTAppHI.

// POSTHI is /hi
func (a *App) POSTHI(r *http.Request) error { return nil }

// The prefix constant of one event against the subject constant of another:
// EventTick's prefix and EventPrefTick's subject concatenate to EvSubjPrefTick.

// EventTick is "tick"
type EventTick struct {
	Room datapages.Subject `json:"room"`

	Text string `json:"text"`
}

// EventPrefTick is "preftick"
type EventPrefTick struct {
	Text string `json:"text"`
}

// EventPrefixTick is "prefixtick"
//
// Named after the prefix EventTick's constants carry, which is the pair the
// two constant prefixes must keep apart.
type EventPrefixTick struct {
	Text string `json:"text"`
}

func (PageIndex) OnPrefTick(_ EventPrefTick, _ datapages.SSE) error { return nil }

func (PageIndex) OnPrefixTick(_ EventPrefixTick, _ datapages.SSE) error { return nil }
