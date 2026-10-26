// Package ui provides the settings page of the Toxiproxy front of
// datapages watch, a Datapages application. README.md explains how to work on it.
package ui

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/romshark/morpheus/neo"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/cmd/toxic"
)

// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS

// init has neo.Icon inline the icons of static/icons. <neo-icon> loads them
// from /static/icons/, its default, after a change of theme.
func init() {
	icons, err := fs.Sub(StaticFS, "static/icons")
	if err != nil {
		panic(err)
	}
	neo.SetIconFS(icons)
}

// App is the settings page of 1 Toxiproxy front.
type App struct {
	// Front holds the settings the page shows and changes.
	Front *toxic.Front
}

func (a *App) Head(_ *http.Request) datapages.Head { return head(a.Front.Info()) }

// EventChanged is "toxiproxy.changed"
//
// The settings or the endpoints of the app changed, which every tab shows.
type EventChanged struct{}

// EventViewChanged is "toxiproxy.view"
//
// What 1 tab shows changed.
type EventViewChanged struct {
	Tab datapages.SubjectStateID `json:"tab"`
}

// PageIndex is /
//
// Shows the global toxics, or the overrides of the endpoint with the pattern
// in the endpoint query parameter, beside the endpoints that match the search
// in q and the kinds in kinds. Its actions are commands: they change the
// settings or the state of the tab and dispatch an event, for which the page
// renders again.
type PageIndex struct{ App *App }

// StateIndex is what 1 tab shows.
type StateIndex struct {
	// Endpoint is the pattern of the scope, empty for the global toxics.
	Endpoint string
	// Search and Kinds filter the endpoint list. Kinds lists the kinds
	// separated by commas, empty for every kind.
	Search, Kinds string

	// Notice is the outcome of a save or a reload, which the next render shows
	// once, as a toast. Err is a value Toxiproxy would refuse, which the row of
	// its slot shows until the next command of the tab.
	Notice notice
	Err    slotError

	// NewPreset is what the dialog of a new preset holds.
	NewPreset presetDraft
}

// clearOutcome clears what the last command of the tab left to show.
func (s *StateIndex) clearOutcome() { s.Notice, s.Err = notice{}, slotError{} }

// ViewSignals put what a tab shows in its URL, as the query parameters of
// PageIndex they reflect.
type ViewSignals struct {
	Endpoint string `json:"endpoint"`
	Search   string `json:"search"`
	Kinds    string `json:"kinds"`
}

func (p PageIndex) GET(
	_ *http.Request,
	query datapages.Query[struct {
		Endpoint string `query:"endpoint" reflectsignal:"endpoint"`
		Search   string `query:"q" reflectsignal:"search"`
		Kinds    string `query:"kinds" reflectsignal:"kinds"`
	}],
) (body datapages.Component, err error) {
	q := query.Values
	s := StateIndex{Endpoint: q.Endpoint, Search: q.Search, Kinds: q.Kinds}
	return app(p.App.view(s)), nil
}

// StreamOpen starts the state of the tab from the URL, which the signals
// reflect, and renders the tab, which catches up on what it missed without a
// stream.
func (p PageIndex) StreamOpen(
	_ *http.Request,
	sse datapages.SSE,
	state datapages.State[StateIndex],
	signals datapages.Signals[ViewSignals],
) error {
	v := signals.Values
	*state.Values = StateIndex{
		Endpoint: v.Endpoint,
		Search:   v.Search,
		Kinds:    canonicalKinds(v.Kinds),
	}
	return p.render(sse, state.Values)
}

func (p PageIndex) OnChanged(
	_ EventChanged, sse datapages.SSE, state datapages.State[StateIndex],
) error {
	return p.render(sse, state.Values)
}

func (p PageIndex) OnViewChanged(
	_ EventViewChanged, sse datapages.SSE, state datapages.State[StateIndex],
) error {
	return p.render(sse, state.Values)
}

// render shows the tab what s selects, and puts it in the URL. It shows the
// notice of s once, as a toast, in the toaster, which no render morphs.
func (p PageIndex) render(sse datapages.SSE, s *StateIndex) error {
	err := sse.PatchSignals(ViewSignals{
		Endpoint: s.Endpoint,
		Search:   s.Search,
		Kinds:    s.Kinds,
	})
	if err != nil {
		return err
	}
	if err := sse.PatchElement(regions(p.App.view(*s))); err != nil {
		return err
	}
	if s.Notice.title == "" {
		return nil
	}
	n := s.Notice
	s.Notice = notice{}
	return sse.PatchElementAt(toast(n), "#tp-toaster", datapages.PatchModeAppend)
}

// POSTShow is /show
//
// Shows the scope of the endpoint query parameter in place, which keeps the
// search that loading the page would leave behind in the search field.
func (p PageIndex) POSTShow(
	_ *http.Request,
	query datapages.Query[struct {
		Endpoint string `query:"endpoint"`
	}],
	state datapages.State[StateIndex],
	stateID string,
	viewChanged datapages.Dispatcher[EventViewChanged],
) error {
	state.Values.clearOutcome()
	state.Values.Endpoint = query.Values.Endpoint
	return viewChanged.Dispatch(EventViewChanged{Tab: datapages.SubjectStateID(stateID)})
}

// POSTFilter is /filter
//
// Filters the endpoint list by the search and kinds signals. The kinds signal
// is "none" for no kind.
func (p PageIndex) POSTFilter(
	_ *http.Request,
	signals datapages.Signals[struct {
		Search string `json:"search"`
		Kinds  string `json:"kinds"`
	}],
	state datapages.State[StateIndex],
	stateID string,
	viewChanged datapages.Dispatcher[EventViewChanged],
) error {
	s := state.Values
	s.clearOutcome()
	s.Search, s.Kinds = signals.Values.Search, canonicalKinds(signals.Values.Kinds)
	return viewChanged.Dispatch(EventViewChanged{Tab: datapages.SubjectStateID(stateID)})
}

// POSTMode is /mode
//
// Turns the toxic of the slot on or off, or for an endpoint to inherit the
// global one, as the mode signal says. A toxic turned on comes back with the
// values it had.
func (p PageIndex) POSTMode(
	_ *http.Request,
	query datapages.Query[struct {
		Slot toxic.Slot `query:"slot"`
	}],
	signals datapages.Signals[struct {
		Mode string `json:"mode"`
	}],
	state datapages.State[StateIndex],
	changed datapages.Dispatcher[EventChanged],
) error {
	mode := signals.Values.Mode
	if mode != modeOn && mode != modeOff && mode != modeInherit {
		return fmt.Errorf("%w: unknown mode %q", datapages.ErrBadRequest, mode)
	}
	s, slot := state.Values, query.Values.Slot
	s.clearOutcome()
	_ = p.App.Front.Update(func(st *toxic.Settings) error {
		setSlot(st, s.Endpoint, slot, mode, current(*st, s.Endpoint, slot))
		return nil
	})
	return changed.Dispatch(EventChanged{})
}

// POSTValue is /value
//
// Sets the field of the toxic of the slot to the value signal and turns the
// toxic on. The field is the key of a field of the type, as in "jitter",
// or "toxicity". A value Toxiproxy would refuse shows in the row of the slot.
func (p PageIndex) POSTValue(
	_ *http.Request,
	query datapages.Query[struct {
		Slot  toxic.Slot `query:"slot"`
		Field string     `query:"field"`
	}],
	signals datapages.Signals[struct {
		Value string `json:"value"`
	}],
	state datapages.State[StateIndex],
	stateID string,
	changed datapages.Dispatcher[EventChanged],
	viewChanged datapages.Dispatcher[EventViewChanged],
) error {
	s, slot := state.Values, query.Values.Slot
	err := p.App.Front.Update(func(st *toxic.Settings) error {
		x := current(*st, s.Endpoint, slot)
		err := setField(&x, slot.Type, query.Values.Field, signals.Values.Value)
		if err != nil {
			return err
		}
		if err := x.Validate(slot.Type); err != nil {
			return err
		}
		setSlot(st, s.Endpoint, slot, modeOn, x)
		return nil
	})
	if errors.Is(err, errUnknownField) {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	s.clearOutcome()
	if err != nil {
		s.Err = slotError{slot: slot, msg: err.Error()}
		return viewChanged.Dispatch(
			EventViewChanged{Tab: datapages.SubjectStateID(stateID)})
	}
	return changed.Dispatch(EventChanged{})
}

// POSTClear is /clear
//
// Removes the overrides of the endpoint the tab shows.
func (p PageIndex) POSTClear(
	_ *http.Request,
	state datapages.State[StateIndex],
	changed datapages.Dispatcher[EventChanged],
) error {
	s := state.Values
	s.clearOutcome()
	_ = p.App.Front.Update(func(st *toxic.Settings) error {
		delete(st.Endpoints, s.Endpoint)
		return nil
	})
	return changed.Dispatch(EventChanged{})
}

// POSTSave is /save
//
// Writes the settings into the configuration file.
func (p PageIndex) POSTSave(
	_ *http.Request,
	state datapages.State[StateIndex],
	changed datapages.Dispatcher[EventChanged],
) error {
	s := state.Values
	file := p.App.Front.Info().ConfigFile
	s.Notice = notice{
		title: "Saved config",
		text:  "Wrote the settings to " + file + ".",
	}
	if err := p.App.Front.Save(); err != nil {
		s.Notice = notice{title: "Saving config failed", text: err.Error(), failed: true}
	}
	s.Err = slotError{}
	return changed.Dispatch(EventChanged{})
}

// POSTReload is /reload
//
// Discards the changes made on the page for the settings of the
// configuration file.
func (p PageIndex) POSTReload(
	_ *http.Request,
	state datapages.State[StateIndex],
	changed datapages.Dispatcher[EventChanged],
) error {
	s := state.Values
	file := p.App.Front.Info().ConfigFile
	s.Notice = notice{
		title: "Reloaded config",
		text:  "Read the settings of " + file + ".",
	}
	if err := p.App.Front.Reload(); err != nil {
		s.Notice = notice{
			title: "Reloading config failed", text: err.Error(), failed: true,
		}
	}
	s.Err = slotError{}
	return changed.Dispatch(EventChanged{})
}

// PresetSignals name the preset an action of the presets acts on.
type PresetSignals struct {
	Preset string `json:"preset"`
}

// POSTApplyPreset is /apply-preset
//
// Makes the global toxics and the overrides of the endpoints those of the
// preset the preset signal names.
func (p PageIndex) POSTApplyPreset(
	_ *http.Request,
	signals datapages.Signals[PresetSignals],
	state datapages.State[StateIndex],
	changed datapages.Dispatcher[EventChanged],
) error {
	state.Values.clearOutcome()
	err := p.App.Front.Update(func(st *toxic.Settings) error {
		return st.ApplyPreset(signals.Values.Preset)
	})
	if err != nil {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	return changed.Dispatch(EventChanged{})
}

// NewPresetSignals are the name field and the value of the copy combobox of
// the dialog of a new preset.
type NewPresetSignals struct {
	Name string `json:"name"`
	From string `json:"from"`
}

// POSTPresetDraft is /preset-draft
//
// Takes the fields of the dialog of a new preset, which the tab shows. Picking
// a preset of Datapages fills in its name where the name is empty, and a name
// a preset has disables creating the preset.
func (p PageIndex) POSTPresetDraft(
	_ *http.Request,
	signals datapages.Signals[NewPresetSignals],
	state datapages.State[StateIndex],
	stateID string,
	viewChanged datapages.Dispatcher[EventViewChanged],
) error {
	state.Values.NewPreset.update(signals.Values.Name, signals.Values.From)
	return viewChanged.Dispatch(EventViewChanged{Tab: datapages.SubjectStateID(stateID)})
}

// POSTCreatePreset is /create-preset
//
// Adds a preset with the name signal and applies it. It copies the global
// toxics and the overrides of the preset of the app or of Datapages the from
// signal names, or the current ones for an empty one. It refuses a name a preset has, as any other name
// it refuses, in the dialog, which a created preset closes.
func (p PageIndex) POSTCreatePreset(
	_ *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[NewPresetSignals],
	state datapages.State[StateIndex],
	stateID string,
	changed datapages.Dispatcher[EventChanged],
	viewChanged datapages.Dispatcher[EventViewChanged],
) error {
	s, v := state.Values, signals.Values
	s.clearOutcome()
	err := p.App.Front.Update(func(st *toxic.Settings) error {
		src, err := copySource(*st, v.From)
		if err != nil {
			return err
		}
		return st.CreatePreset(v.Name, src.Toxics, src.Endpoints)
	})
	if err != nil {
		s.NewPreset.name, s.NewPreset.from = v.Name, v.From
		s.NewPreset.err = createPresetError(err)
		return viewChanged.Dispatch(EventViewChanged{Tab: datapages.SubjectStateID(stateID)})
	}
	s.NewPreset = presetDraft{}
	if err := sse.ExecuteScript(presetCreated); err != nil {
		return err
	}
	return changed.Dispatch(EventChanged{})
}

// POSTDeletePreset is /delete-preset
//
// Removes the preset the preset signal names. The toxics stay.
func (p PageIndex) POSTDeletePreset(
	_ *http.Request,
	signals datapages.Signals[PresetSignals],
	state datapages.State[StateIndex],
	changed datapages.Dispatcher[EventChanged],
) error {
	state.Values.clearOutcome()
	err := p.App.Front.Update(func(st *toxic.Settings) error {
		return st.DeletePreset(signals.Values.Preset)
	})
	if err != nil {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	return changed.Dispatch(EventChanged{})
}
