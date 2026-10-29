// Package ui is the settings page of the Toxiproxy front of datapages watch,
// a Datapages application. README.md explains how to work on it.
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

// init makes [neo.Icon] inline the SVGs of static/icons. After a theme change
// <neo-icon> fetches them from /static/icons/, its default base.
func init() {
	icons, err := fs.Sub(StaticFS, "static/icons")
	if err != nil {
		panic(err)
	}
	neo.SetIconFS(icons)
}

// App is the settings page of 1 Toxiproxy front.
type App struct {
	Front *toxic.Front
}

func (a *App) Head(_ *http.Request) datapages.Head { return head(a.Front.Info()) }

// scope returns the pattern of the scope that the endpoint query parameter key names.
func (a *App) scope(key string) string { return scopePattern(key, a.Front.Endpoints()) }

// EventChanged is "toxiproxy.changed"
//
// The settings or the endpoints of the app changed. Every tab renders.
type EventChanged struct{}

// EventViewChanged is "toxiproxy.view"
//
// The view of 1 tab changed.
type EventViewChanged struct {
	Tab datapages.SubjectStateID `json:"tab"`
}

// PageIndex is /
//
// Shows the global toxics, or the overrides of the endpoint query parameter,
// beside the endpoints that match q and kinds. Each action changes the
// settings or the tab state and dispatches an event that renders the page.
type PageIndex struct{ App *App }

// StateIndex is what 1 tab shows.
type StateIndex struct {
	// Endpoint is the endpoint query parameter, which names the scope,
	// empty for the global toxics. See [scopePattern].
	Endpoint string
	// Search and Kinds filter the endpoint list. See [filter].
	Search, Kinds string

	// Notice is the outcome of a save or a reload, shown once as a toast.
	// Err is a value Toxiproxy would refuse, shown in the row of its slot
	// until the next command of the tab.
	Notice notice
	Err    slotError

	NewPreset presetDraft

	// Units holds the unit the tab picked for a byte field.
	// A field without one shows the unit [defaultByteUnit] picks.
	Units map[fieldRef]byteUnit
}

func (s *StateIndex) clearOutcome() { s.Notice, s.Err = notice{}, slotError{} }

// ViewSignals put the view of a tab in its URL as the query parameters of [PageIndex].
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
) (body datapages.Component) {
	q := query.Values
	s := StateIndex{Endpoint: q.Endpoint, Search: q.Search, Kinds: q.Kinds}
	return app(p.App.view(s))
}

// StreamOpen sets the tab state from the signals, which mirror the URL,
// and renders the tab to catch up on changes made while it had no stream.
// An endpoint the URL names by its pattern is renamed after its handler.
func (p PageIndex) StreamOpen(
	_ *http.Request,
	sse datapages.SSE,
	state datapages.State[StateIndex],
	signals datapages.Signals[ViewSignals],
) error {
	v, eps := signals.Values, p.App.Front.Endpoints()
	*state.Values = StateIndex{
		Endpoint: scopeKey(scopePattern(v.Endpoint, eps), eps),
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

// render patches the tab with the view of s and puts it in the URL.
// It appends the notice of s once to the toaster, which no render morphs.
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
// Shows the scope of the endpoint query parameter without a page load,
// which keeps the text of the search field.
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
// Filters the endpoint list by the search and kinds signals.
// The kinds signal is "none" for no kind.
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
// Sets the slot to the mode signal, which is on, off or, for an endpoint,
// inherit. A toxic turned on again keeps its earlier values.
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
	scope := p.App.scope(s.Endpoint)
	_ = p.App.Front.Update(func(st *toxic.Settings) error {
		setSlot(st, scope, slot, mode, current(*st, scope, slot))
		return nil
	})
	return changed.Dispatch(EventChanged{})
}

// POSTValue is /value
//
// Sets the field of the slot, a key such as "jitter" or "toxicity", to the
// value signal and turns the toxic on. A byte field reads the value in the unit
// query parameter, B when absent. A value Toxiproxy would refuse shows in
// the row of the slot.
func (p PageIndex) POSTValue(
	_ *http.Request,
	query datapages.Query[struct {
		Slot  toxic.Slot `query:"slot"`
		Field string     `query:"field"`
		Unit  string     `query:"unit"`
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
	scope := p.App.scope(s.Endpoint)
	err := p.App.Front.Update(func(st *toxic.Settings) error {
		x := current(*st, scope, slot)
		err := setField(&x, slot.Type, query.Values.Field, signals.Values.Value,
			query.Values.Unit)
		if err != nil {
			return err
		}
		if err := x.Validate(slot.Type); err != nil {
			return err
		}
		setSlot(st, scope, slot, modeOn, x)
		return nil
	})
	if errors.Is(err, errUnknownField) || errors.Is(err, errUnknownUnit) {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	s.clearOutcome()
	if err != nil {
		s.Err = slotError{slot: slot, msg: err.Error()}
		return viewChanged.Dispatch(
			EventViewChanged{Tab: datapages.SubjectStateID(stateID)},
		)
	}
	return changed.Dispatch(EventChanged{})
}

// POSTUnit is /unit
//
// Shows the byte field of the slot in the unit signal, in this tab only.
// The value of the field stays.
func (p PageIndex) POSTUnit(
	_ *http.Request,
	query datapages.Query[struct {
		Slot  toxic.Slot `query:"slot"`
		Field string     `query:"field"`
	}],
	signals datapages.Signals[struct {
		Unit string `json:"unit"`
	}],
	state datapages.State[StateIndex],
	stateID string,
	viewChanged datapages.Dispatcher[EventViewChanged],
) error {
	ref := fieldRef{query.Values.Slot, query.Values.Field}
	i := fieldIndex(ref.slot.Type, ref.key)
	if i < 0 || toxic.Types[ref.slot.Type].Fields[i].Unit != toxic.UnitBytes {
		return fmt.Errorf("%w: %s has no byte field %q",
			datapages.ErrBadRequest, ref.slot.Name(), ref.key)
	}
	u, ok := lookupByteUnit(signals.Values.Unit)
	if !ok {
		return fmt.Errorf("%w: %w %q",
			datapages.ErrBadRequest, errUnknownUnit, signals.Values.Unit)
	}
	s := state.Values
	s.clearOutcome()
	if s.Units == nil {
		s.Units = map[fieldRef]byteUnit{}
	}
	s.Units[ref] = u
	return viewChanged.Dispatch(EventViewChanged{Tab: datapages.SubjectStateID(stateID)})
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
	scope := p.App.scope(s.Endpoint)
	_ = p.App.Front.Update(func(st *toxic.Settings) error {
		delete(st.Endpoints, scope)
		return nil
	})
	return changed.Dispatch(EventChanged{})
}

// POSTSave is /save
//
// Writes the presets into the configuration file.
func (p PageIndex) POSTSave(
	_ *http.Request,
	state datapages.State[StateIndex],
	changed datapages.Dispatcher[EventChanged],
) error {
	s := state.Values
	s.Notice, s.Err = p.save(), slotError{}
	return changed.Dispatch(EventChanged{})
}

// save writes the presets into the configuration file and returns the notice.
func (p PageIndex) save() notice {
	if err := p.App.Front.Save(); err != nil {
		return notice{title: "Saving config failed", text: err.Error(), failed: true}
	}
	file := p.App.Front.Info().ConfigFile
	return notice{title: "Saved config", text: "Wrote the presets to " + file + "."}
}

// POSTReload is /reload
//
// Replaces the settings with those of the configuration file.
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

// PresetSignals name the preset to apply or delete.
type PresetSignals struct {
	Preset string `json:"preset"`
}

// POSTApplyPreset is /apply-preset
//
// Replaces the toxics in effect with those of the preset signal.
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

// NewPresetSignals are the name field and the copy combobox of the new preset
// dialog. Save is set when the save button opened it for toxics no preset holds.
type NewPresetSignals struct {
	Name string `json:"name"`
	From string `json:"from"`
	Save bool   `json:"save"`
}

// POSTPresetDraft is /preset-draft
//
// Stores the fields of the new preset dialog and renders the tab.
// Picking a Datapages preset fills in an empty name. A taken name disables Create.
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
// Adds a preset named by the name signal and applies it. It copies the preset
// of the app or of Datapages the from signal names, or the current toxics for
// an empty from. A refused name shows in the dialog. Success closes it and,
// with the save signal, writes the presets into the configuration file.
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
		return viewChanged.Dispatch(
			EventViewChanged{Tab: datapages.SubjectStateID(stateID)},
		)
	}
	s.NewPreset = presetDraft{}
	if err := sse.ExecuteScript(presetCreated); err != nil {
		return err
	}
	if v.Save {
		s.Notice = p.save()
	}
	return changed.Dispatch(EventChanged{})
}

// POSTDeletePreset is /delete-preset
//
// Removes the preset the preset signal names. When it is in effect,
// No throttling replaces it.
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
