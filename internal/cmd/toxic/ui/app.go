// Package ui implements the Toxiproxy settings page for datapages watch.
// README.md contains its development instructions.
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

// init gives [neo.Icon] the embedded SVGs. After a theme change, <neo-icon>
// fetches the same files from its default /static/icons/ path.
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
// The settings or endpoints changed. Every tab renders again.
type EventChanged struct{}

// EventViewChanged is "toxiproxy.view"
//
// The view of 1 tab changed.
type EventViewChanged struct {
	Tab datapages.SubjectStateID `json:"tab"`
}

// PageIndex is /
//
// Shows global toxics or 1 endpoint's overrides. It filters endpoints by q and
// kinds. Actions update settings or tab state, then dispatch a render event.
type PageIndex struct{ App *App }

// StateIndex is what 1 tab shows.
type StateIndex struct {
	// Endpoint names the selected scope. Empty selects the global toxics.
	Endpoint string
	// Search and Kinds filter the endpoint list. See [filter].
	Search, Kinds string

	// Notice is a save, reload or reset result shown once as a toast.
	// Err is a rejected value shown in its slot until the next tab command.
	Notice notice
	Err    slotError

	NewPreset presetDraft

	// Units stores each unit of a byte or rate field selected in this tab.
	// [defaultUnit] picks the unit of a missing entry.
	Units map[fieldRef]valueUnit
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

// StreamOpen restores the URL-backed tab state and renders changes missed
// while the stream was closed. It replaces an endpoint pattern with its handler.
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

// render patches the tab, reflects its view in the URL and appends its notice
// once to the toaster outside the morphed regions.
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
// Shows the selected scope without reloading or clearing the search field.
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
// Sets the slot to on, off or, for an endpoint, inherit.
// Turning a toxic on again restores its previous values.
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
// Sets 1 slot field and turns the toxic on. A byte or rate field uses the unit
// query parameter, or B or B/s when absent. A rejected value appears in the slot row.
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
// Changes how this tab displays a byte or rate field without changing its value.
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
	var units []valueUnit
	if i >= 0 {
		units = unitsOf(toxic.Types[ref.slot.Type].Fields[i].Unit)
	}
	if units == nil {
		return fmt.Errorf("%w: %s has no field %q with units",
			datapages.ErrBadRequest, ref.slot.Name(), ref.key)
	}
	u, ok := lookupUnit(units, signals.Values.Unit)
	if !ok {
		return fmt.Errorf("%w: %w %q",
			datapages.ErrBadRequest, errUnknownUnit, signals.Values.Unit)
	}
	s := state.Values
	s.clearOutcome()
	if s.Units == nil {
		s.Units = map[fieldRef]valueUnit{}
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

// POSTResetDataCaps is /reset-data-caps
//
// Clears per-client data-cap usage.
func (p PageIndex) POSTResetDataCaps(
	_ *http.Request,
	state datapages.State[StateIndex],
	stateID string,
	viewChanged datapages.Dispatcher[EventViewChanged],
) error {
	p.App.Front.ResetDataCaps()
	s := state.Values
	s.Notice = notice{
		title: "Data caps reset", text: "Restored each browser's full volume.",
	}
	s.Err = slotError{}
	return viewChanged.Dispatch(EventViewChanged{Tab: datapages.SubjectStateID(stateID)})
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
		return notice{title: "Could not save config", text: err.Error(), failed: true}
	}
	file := p.App.Front.Info().ConfigFile
	return notice{title: "Config saved", text: "Saved presets to " + file + "."}
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
		title: "Config reloaded",
		text:  "Loaded settings from " + file + ".",
	}
	if err := p.App.Front.Reload(); err != nil {
		s.Notice = notice{
			title: "Could not reload config", text: err.Error(), failed: true,
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
// Applies the selected preset.
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

// NewPresetSignals contains the new-preset fields. Save reports that the save
// button opened the dialog to name otherwise unnamed toxics.
type NewPresetSignals struct {
	Name string `json:"name"`
	From string `json:"from"`
	Save bool   `json:"save"`
}

// POSTPresetDraft is /preset-draft
//
// Stores the new-preset fields and renders the tab.
// A Datapages preset fills an empty name. An existing name disables Create.
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
// Creates and applies a preset. It copies the selected app or Datapages preset,
// or the current toxics when no source is selected.
// With Save set, it also writes the presets to the configuration file.
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
// Deletes the selected preset. No throttling replaces an active preset.
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
