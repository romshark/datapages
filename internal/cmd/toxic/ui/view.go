package ui

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/romshark/morpheus/neo"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/cmd/toxic"
	"github.com/romshark/datapages/internal/cmd/toxic/ui/datapagesgen/action"
)

const (
	modeOn      = "on"
	modeOff     = "off"
	modeInherit = "inherit" // endpoint only, the global toxic applies
)

var (
	errUnknownField = errors.New("unknown field")
	errUnknownUnit  = errors.New("unknown unit")
)

// setField parses and sets 1 field or the toxicity percentage.
// Byte and rate fields use unit, or B and B/s when empty.
// Choice fields use their configuration name.
func setField(x *toxic.Toxic, t toxic.Type, key, value, unit string) error {
	if key == "toxicity" {
		pct, err := parseInt(value)
		if err != nil || pct < 0 || pct > 100 {
			return errors.New(
				"requests affected must be a whole percentage from 0 to 100",
			)
		}
		x.Toxicity = float32(pct) / 100
		return nil
	}
	fields, i := toxic.Types[t].Fields, fieldIndex(t, key)
	if i < 0 {
		return fmt.Errorf("%w %q of %s", errUnknownField, key, toxic.Types[t].Name)
	}
	f := fields[i]
	if f.Unit == toxic.UnitChoice {
		c := slices.Index(f.Choices, value)
		if c < 0 {
			return fmt.Errorf("%s must be %s, not %q",
				strings.ToLower(f.Label), strings.Join(f.Choices, " or "), value)
		}
		x.Values[i] = int64(c)
		return nil
	}
	parse := parseInt
	if units := unitsOf(f.Unit); units != nil {
		u, ok := lookupUnit(units, unit)
		if !ok {
			return fmt.Errorf("%w %q", errUnknownUnit, unit)
		}
		parse = u.parse
	}
	v, err := parse(value)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.ToLower(f.Label), err)
	}
	x.Values[i] = v
	return nil
}

// parseInt parses a field value. An empty field reads as 0,
// the value a cleared field shows after the next render.
func parseInt(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a whole number", s)
	}
	return v, nil
}

// choiceLabel capitalizes a [toxic.UnitChoice] configuration value for the UI.
func choiceLabel(c string) string { return strings.ToUpper(c[:1]) + c[1:] }

// fieldIndex returns the index of the field of t with the key, or -1.
func fieldIndex(t toxic.Type, key string) int {
	return slices.IndexFunc(toxic.Types[t].Fields, func(f toxic.Field) bool {
		return f.Key == key
	})
}

// valueUnit is a unit a byte or rate field shows its value in. size is the
// bytes, or bytes per second, of 1 of it.
type valueUnit struct {
	name string
	size int64
}

// byteUnits and rateUnits list the units of the unit select of a byte and a rate
// field in their order. The first is the unit the field holds its value in.
var (
	byteUnits = []valueUnit{
		{"B", 1},
		{"KB", 1e3},
		{"MB", 1e6},
		{"GB", 1e9},
		{"KiB", 1 << 10},
		{"MiB", 1 << 20},
		{"GiB", 1 << 30},
	}
	rateUnits = []valueUnit{
		{"B/s", 1},
		{"KB/s", 1e3},
		{"MB/s", 1e6},
		{"GB/s", 1e9},
		{"KiB/s", 1 << 10},
		{"MiB/s", 1 << 20},
		{"GiB/s", 1 << 30},
		{"kbit/s", 125},
		{"Mbit/s", 125e3},
		{"Gbit/s", 125e6},
	}
)

// unitsOf returns the units a field of unit u shows its value in, nil for none.
func unitsOf(u toxic.Unit) []valueUnit {
	switch u {
	case toxic.UnitBytes:
		return byteUnits
	case toxic.UnitBps:
		return rateUnits
	}
	return nil
}

// lookupUnit returns the unit of units with name, the first for an empty name.
func lookupUnit(units []valueUnit, name string) (valueUnit, bool) {
	if name == "" {
		return units[0], true
	}
	for _, u := range units {
		if u.name == name {
			return u, true
		}
	}
	return valueUnit{}, false
}

// defaultUnit returns the unit of units that shows v in the fewest characters
// with up to 3 decimals, the larger of 2 that tie, as in 1.5 KiB for 1536 and
// 8.1 Mbit/s for 1012500 B/s. It is the unit of a field the tab picked none for.
func defaultUnit(units []valueUnit, v int64) valueUnit {
	best, bestLen := units[0], len(units[0].format(v))
	for _, u := range units[1:] {
		// v%u.size is below 2^30, which keeps the product in range.
		if v < u.size || v%u.size*1000%u.size != 0 {
			continue
		}
		if n := len(u.format(v)); n < bestLen || n == bestLen && u.size > best.size {
			best, bestLen = u, n
		}
	}
	return best
}

// format returns v in u, as in "1.5" for 1536 bytes in KiB.
func (u valueUnit) format(v int64) string {
	if u.size == 1 {
		return strconv.FormatInt(v, 10)
	}
	return strconv.FormatFloat(float64(v)/float64(u.size), 'f', -1, 64)
}

// parse returns the number s in u in the unit of the first of its units.
// That unit takes a whole number, the others a fraction too, rounded to the
// nearest whole. An empty s reads as 0.
func (u valueUnit) parse(s string) (int64, error) {
	if u.size == 1 {
		return parseInt(s)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	b := math.Round(f * float64(u.size))
	if math.Abs(b) >= math.MaxInt64 {
		return 0, fmt.Errorf("%s %s is too large", s, u.name)
	}
	return int64(b), nil
}

// step returns the step attribute of a number input in u.
func (u valueUnit) step() string {
	if u.size == 1 {
		return "1"
	}
	return "any"
}

// inputmode returns the virtual keyboard of a number input in u.
func (u valueUnit) inputmode() neo.TextInputInputmode {
	if u.size == 1 {
		return neo.TextInputInputmodeNumeric
	}
	return neo.TextInputInputmodeDecimal
}

// fieldRef names 1 field of a slot.
type fieldRef struct {
	slot toxic.Slot
	key  string
}

// unit returns the unit of units the field ref shows v in.
func (s StateIndex) unit(ref fieldRef, units []valueUnit, v int64) valueUnit {
	if u, ok := s.Units[ref]; ok {
		return u
	}
	return defaultUnit(units, v)
}

// current returns the toxic slot turns on with in scope, which is its own,
// else the global one, else [toxic.NewToxic].
func current(s toxic.Settings, scope string, slot toxic.Slot) toxic.Toxic {
	x, ok := s.Endpoints[scope][slot]
	if !ok || scope == "" {
		x, ok = s.Global[slot]
	}
	if !ok {
		x = toxic.NewToxic(slot.Type)
	}
	x.Disabled = false
	return x
}

// setSlot sets slot in scope, an endpoint pattern or empty for the global scope, to mode.
// A toxic turned off keeps its values.
func setSlot(
	s *toxic.Settings, scope string, slot toxic.Slot, mode string, x toxic.Toxic,
) {
	set := s.Global
	if scope != "" {
		set = s.Endpoints[scope]
		if set == nil {
			set = toxic.Set{}
			s.Endpoints[scope] = set
		}
	}
	switch mode {
	case modeOn:
		set[slot] = x
	case modeOff:
		prev, ok := set[slot]
		if !ok {
			prev, ok = s.Global[slot]
		}
		if !ok {
			prev = toxic.NewToxic(slot.Type)
		}
		prev.Disabled = true
		set[slot] = prev
	case modeInherit:
		if scope == "" {
			return
		}
		delete(set, slot)
		if len(set) == 0 {
			delete(s.Endpoints, scope)
		}
	}
}

type notice struct {
	title, text string
	failed      bool
}

// toastOpts shows a notice for 4s and a failure until the user closes it.
func toastOpts(n notice) neo.ToastOpts {
	if n.failed {
		return neo.ToastOpts{Variant: neo.Set(neo.ToastVariantError)}
	}
	return neo.ToastOpts{
		Variant:  neo.Set(neo.ToastVariantSuccess),
		Duration: neo.Set(4000),
	}
}

// slotError is a rejected input, none when msg is empty.
type slotError struct {
	slot toxic.Slot
	msg  string
}

// pageData is what the settings page shows.
type pageData struct {
	info toxic.Info

	// scope is the selected endpoint pattern, empty for the global toxics.
	scope string
	// scopeEndpoint has an empty pattern when the app has no endpoint scope.
	scopeEndpoint toxic.Endpoint
	// scopeKind is derived from the pattern for an unknown endpoint.
	scopeKind string
	// scopeOverrides counts the slots scope sets.
	scopeOverrides int

	filter filter
	// endpoints and unknown are the entries filter shows.
	endpoints []navItem
	// unknown holds the overrides of patterns the app has no endpoint for.
	unknown []navItem
	// hidden counts the endpoints filter hides.
	hidden int

	presets []presetItem
	// activePreset is the preset in effect, empty for toxics no preset holds.
	activePreset string
	// copyOptions are the presets the new preset dialog copies from.
	copyOptions []copyOption
	newPreset   newPresetView

	types []typeView
	// saved reports whether the configuration file holds the settings.
	saved bool
}

type navItem struct {
	pattern  string
	name     string
	kind     string
	selected bool
	// overrides counts the slots the endpoint sets.
	overrides int
}

// key returns the endpoint query parameter that selects e. See [scopePattern].
func (e navItem) key() string { return cmp.Or(e.name, e.pattern) }

// scopePattern returns the pattern of the scope that the endpoint query
// parameter key names. key is the handler of an endpoint in eps, as in
// PageIndex.GET, else a pattern. A handler name needs no escaping in the URL,
// where the "/", "{" and "$" of a pattern do.
func scopePattern(key string, eps []toxic.Endpoint) string {
	for _, e := range eps {
		if e.Name() == key {
			return e.Pattern
		}
	}
	return key
}

// scopeKey returns the endpoint query parameter of the scope pattern:
// the handler of its endpoint in eps, else pattern.
func scopeKey(pattern string, eps []toxic.Endpoint) string {
	for _, e := range eps {
		if e.Pattern == pattern {
			return e.Name()
		}
	}
	return pattern
}

type typeView struct {
	typ  toxic.Type
	info toxic.TypeInfo
	rows []rowView
}

type rowView struct {
	slot toxic.Slot
	mode string
	// toxic is the toxic the fields show.
	toxic toxic.Toxic
	// global is the global toxic of the slot, which an endpoint inherits.
	global   toxic.Toxic
	isGlobal bool
	err      string
	// units holds the unit of each byte and rate field of toxic.
	units [toxic.MaxFields]valueUnit
}

// view returns the page data of a tab in state tab.
func (a *App) view(tab StateIndex) pageData {
	s := a.Front.Settings()
	eps := a.Front.Endpoints()
	scope := scopePattern(tab.Endpoint, eps)
	f := filter{search: tab.Search, kinds: tab.Kinds}
	d := pageData{
		info:           a.Front.Info(),
		scope:          scope,
		scopeOverrides: len(s.Endpoints[scope]),
		filter:         f,
	}
	patterns := make([]string, len(eps))
	for i, e := range eps {
		patterns[i] = e.Pattern
		if e.Pattern == scope {
			d.scopeEndpoint, d.scopeKind = e, e.Kind()
		}
		if !f.shows(e.Kind(), e.Pattern+" "+e.Name()) {
			d.hidden++
			continue
		}
		d.endpoints = append(d.endpoints, navItem{
			pattern:   e.Pattern,
			name:      e.Name(),
			kind:      e.Kind(),
			selected:  e.Pattern == scope,
			overrides: len(s.Endpoints[e.Pattern]),
		})
	}
	if scope != "" && d.scopeKind == "" {
		d.scopeKind = toxic.PatternKind(scope)
	}
	for _, p := range slices.Sorted(maps.Keys(s.Endpoints)) {
		if slices.Contains(patterns, p) || len(s.Endpoints[p]) == 0 {
			continue
		}
		if !f.shows(toxic.PatternKind(p), p) {
			d.hidden++
			continue
		}
		d.unknown = append(d.unknown, navItem{
			pattern:   p,
			selected:  p == scope,
			overrides: len(s.Endpoints[p]),
		})
	}

	for _, p := range s.AllPresets() {
		d.presets = append(d.presets, presetItem{
			name:    p.Name,
			icon:    presetIcon(p.Name),
			builtin: toxic.IsBuiltinPreset(p.Name),
		})
	}
	d.activePreset = s.Active
	d.copyOptions = copyOptions(s, d.info.AppName)
	d.newPreset = newPreset(s, tab.NewPreset)

	override := s.Endpoints[scope]
	for t, info := range toxic.Types {
		tv := typeView{typ: toxic.Type(t), info: info}
		for _, st := range toxic.Streams {
			slot := toxic.Slot{Type: toxic.Type(t), Stream: st}
			g, hasGlobal := s.Global[slot]
			if !hasGlobal {
				g = toxic.NewToxic(slot.Type)
				g.Disabled = true
			}
			r := rowView{slot: slot, global: g, isGlobal: scope == ""}
			switch x, ok := override[slot]; {
			case scope == "":
				r.toxic, r.mode = g, modeOn
				if g.Disabled {
					r.mode = modeOff
				}
			case ok:
				r.toxic, r.mode = x, modeOn
				if x.Disabled {
					r.mode = modeOff
				}
			default:
				r.toxic, r.mode = g, modeInherit
			}
			if tab.Err.slot == slot {
				r.err = tab.Err.msg
			}
			for i, f := range info.Fields {
				if units := unitsOf(f.Unit); units != nil {
					r.units[i] = tab.unit(fieldRef{slot, f.Key}, units, r.toxic.Values[i])
				}
			}
			tv.rows = append(tv.rows, r)
		}
		d.types = append(d.types, tv)
	}

	d.saved = a.Front.Saved()
	return d
}

// kinds lists the endpoint kinds in the order of their filter toggles.
var kinds = [...]string{
	toxic.KindGET, toxic.KindStream, toxic.KindAction, toxic.KindFile,
}

// kindInfo describes an endpoint kind in the endpoint list.
type kindInfo struct {
	// label and title belong to the filter toggle of the kind.
	label, title string
	// name labels 1 endpoint of the kind.
	// icon is a file name in static/icons without .svg.
	name, icon string
}

var kindInfos = map[string]kindInfo{
	toxic.KindGET: {
		label: "GET", title: "The GET handlers of pages",
		name: "Page", icon: "app-window",
	},
	toxic.KindStream: {
		label: "Streams", title: "The SSE streams of pages",
		name: "Stream", icon: "radio",
	},
	toxic.KindAction: {
		label: "Actions", title: "Actions other than GET actions",
		name: "Action", icon: "zap",
	},
	toxic.KindFile: {
		label: "Files", title: "GET actions, which return files, and the assets",
		name: "File", icon: "file",
	},
}

// filter selects the endpoints whose text matches search and whose kind is in kinds,
// a list separated by "-", which unlike "," needs no escaping in the URL.
// Empty kinds is every kind.
type filter struct {
	search, kinds string
}

// shows reports whether f shows an endpoint of kind with the search text.
func (f filter) shows(kind, text string) bool {
	return f.showsKind(kind) && matches(f.search, text)
}

func (f filter) showsKind(kind string) bool {
	return f.kinds == "" || slices.Contains(strings.Split(f.kinds, "-"), kind)
}

// allKinds reports whether f shows every kind.
func (f filter) allKinds() bool {
	return !slices.ContainsFunc(kinds[:], func(k string) bool { return !f.showsKind(k) })
}

// canonicalKinds returns kinds, or empty when kinds lists every kind,
// which leaves kinds out of the URL.
func canonicalKinds(kinds string) string {
	if (filter{kinds: kinds}).allKinds() {
		return ""
	}
	return kinds
}

// pressedKinds returns the toggle group value of the kinds f shows.
func (f filter) pressedKinds() string {
	var shown []string
	for _, k := range kinds {
		if f.showsKind(k) {
			shown = append(shown, k)
		}
	}
	return strings.Join(shown, ",")
}

// matches reports whether each term of query occurs in text as a subsequence,
// ignoring case. "msg send" matches "POST /messages/sendmessage/{$}".
func matches(query, text string) bool {
	text = strings.ToLower(text)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		rest := text
		for _, c := range term {
			i := strings.IndexRune(rest, c)
			if i < 0 {
				return false
			}
			rest = rest[i+utf8.RuneLen(c):]
		}
	}
	return true
}

// fieldInfo returns the field's help, or nil.
func fieldInfo(t toxic.Type, f toxic.Field) templ.Component {
	switch f.Key {
	case "jitter":
		return jitterInfo()
	case "per":
		return perInfo(t)
	case "rate":
		if t == toxic.DataCap {
			return dataCapRateInfo()
		}
	}
	return nil
}

// setValue returns the action expression that posts the value of the field key
// of slot, a number in unit for a byte or rate field and in no unit, "", for the others.
func setValue(slot toxic.Slot, key, unit string) string {
	return action.PageIndex.Value.POST(action.PageIndex.Value.POSTQuery(slot, key, unit),
		action.WithBefore("$value = el.value"))
}

// setUnit returns the action expression that posts the unit picked for the byte
// field key of slot.
func setUnit(slot toxic.Slot, key string) string {
	return action.PageIndex.Unit.POST(action.PageIndex.Unit.POSTQuery(slot, key),
		action.WithBefore("$unit = evt.detail.value"))
}

// setToxicity returns the action expression that posts the value of the toxicity
// slider of slot. The value signal is a string, the value of the event a number.
func setToxicity(slot toxic.Slot) string {
	return action.PageIndex.Value.POST(action.PageIndex.Value.POSTQuery(slot, "toxicity", ""),
		action.WithBefore("$value = String(evt.detail.value)"))
}

// statusOption is 1 option of the status combobox,
// as in 503 labeled "503 Service Unavailable".
type statusOption struct {
	value, label string
}

// statusOptions lists the statuses in the range of f that [net/http.StatusText] names,
// plus current, which datapages.yaml may set to an unnamed one.
func statusOptions(f toxic.Field, current int64) []statusOption {
	var opts []statusOption
	for code := f.Min; code <= f.Max; code++ {
		text := http.StatusText(int(code))
		if text == "" && code != current {
			continue
		}
		v := strconv.FormatInt(code, 10)
		opts = append(opts,
			statusOption{value: v, label: strings.TrimSpace(v + " " + text)})
	}
	return opts
}

// withAttrs returns a copy of a with b merged in.
func withAttrs(a, b templ.Attributes) templ.Attributes {
	m := maps.Clone(a)
	maps.Copy(m, b)
	return m
}

// formDisabled returns the Disabled option of a form-associated Morpheus control,
// such as neo-combobox and neo-textinput. [neo.Set] renders false as
// disabled="false", which disables such a control because the browser reads the
// attribute by presence.
func formDisabled(disabled bool) neo.Attr[bool] {
	if disabled {
		return neo.Set(true)
	}
	return neo.Attr[bool]{}
}

func method(pattern string) string {
	m, _, _ := strings.Cut(pattern, " ")
	return m
}

func path(pattern string) string {
	_, p, _ := strings.Cut(pattern, " ")
	return strings.TrimSpace(p)
}

// pathPart is a piece of a route path with its CSS class, empty for literal text.
type pathPart struct {
	text, class string
}

// pathParts splits a route path into literal text, wildcards such as {slug}
// or {rest...}, and the end anchor {$}.
func pathParts(path string) []pathPart {
	var parts []pathPart
	for path != "" {
		i := strings.IndexByte(path, '{')
		j := strings.IndexByte(path[max(i, 0):], '}')
		if i < 0 || j < 0 {
			return append(parts, pathPart{text: path})
		}
		if i > 0 {
			parts = append(parts, pathPart{text: path[:i]})
		}
		w := path[i : i+j+1]
		class := "tp-route-wild"
		if w == "{$}" {
			class = "tp-route-end"
		}
		parts = append(parts, pathPart{text: w, class: class})
		path = path[i+j+1:]
	}
	return parts
}

func badgeVariant(unknown bool) neo.BadgeVariant {
	if unknown {
		return neo.BadgeVariantDanger
	}
	return neo.BadgeVariantPrimary
}

func streamIcon(s toxic.Stream) string {
	if s == toxic.Upstream {
		return "arrow-up"
	}
	return "arrow-down"
}

func streamLabel(s toxic.Stream) string {
	if s == toxic.Upstream {
		return "Request (upstream)"
	}
	return "Response (downstream)"
}

func toxicityPercent(x toxic.Toxic) int {
	return int(math.Round(float64(x.Toxicity) * 100))
}

// newPresetOption is the picker value that opens the new preset dialog.
// [toxic.Settings.CreatePreset] trims names, which keeps them apart from it.
const newPresetOption = " new"

// pickerOpts returns the preset picker options with active selected, none if empty.
func pickerOpts(active string) neo.ComboboxOpts {
	o := neo.ComboboxOpts{
		Placeholder:       neo.Set("Unsaved"),
		SearchPlaceholder: neo.Set("Search presets"),
		AriaLabel:         neo.Set("Preset"),
		PopoverFitContent: neo.Set(true),
	}
	if active != "" {
		o.Value = neo.Set(active)
	}
	return o
}

// pickPreset returns the action expression of the preset picker,
// which applies the picked preset. For [newPresetOption] it reselects active,
// empty for none, and opens the new preset dialog. With toxics no preset holds,
// it reselects none and asks before it discards them.
func pickPreset(active string) string {
	reselect := "null"
	if active != "" {
		reselect = jsString(active)
	}
	before := "if (evt.detail.value === " + jsString(newPresetOption) + ") { " +
		"el.value = " + reselect + "; $save = false; " +
		openDialog("tp-preset-new") + "; " +
		// Clears the error of the last create attempt.
		postPresetDraft() + "; return } " +
		"$preset = evt.detail.value"
	if active == "" {
		before += "; el.value = null; " + openDialog("tp-discard") + "; return"
	}
	return action.PageIndex.ApplyPreset.POST(action.WithBefore(before))
}

// saveClick returns the action expression of the save button. For toxics no
// preset holds, active is empty, and it opens the new preset dialog to name them first.
func saveClick(active string) string {
	if active != "" {
		return action.PageIndex.Save.POST()
	}
	return "$save = true; document.getElementById('tp-preset-from').value = null; " +
		openDialog("tp-preset-new") + "; " + postPresetDraft()
}

// askDeletePreset returns the action expression of the delete button in the
// picker option of the preset. It keeps the click from picking the preset,
// closes the picker and opens the confirm dialog.
func askDeletePreset(name string) string {
	return "evt.stopPropagation(); $preset = " + jsString(name) + "; " +
		"el.closest('neo-combobox').hide({restoreFocus: true}); " +
		openDialog("tp-preset-delete")
}

// reloadClick returns the action expression of the reload button.
// With unsaved settings it opens the confirm dialog, else it reloads at once.
func reloadClick(saved bool) string {
	if saved {
		return action.PageIndex.Reload.POST()
	}
	return "document.getElementById('tp-reload').show()"
}

// openDialog returns a statement that opens the neo-dialog with the id in the
// next frame. By then the picker has closed and taken back the focus,
// which the dialog returns to it on close.
func openDialog(id string) string {
	return "requestAnimationFrame(() => document.getElementById('" + id + "').show())"
}

// readPresetFields sets the name and from signals from the new preset dialog.
const readPresetFields = "$name = document.getElementById('tp-preset-name').value; " +
	"$from = document.getElementById('tp-preset-from').value ?? ''"

// createPreset returns the action expression that creates the preset of the dialog.
// before runs first and may return to skip the request.
func createPreset(before string) string {
	return action.PageIndex.CreatePreset.POST(
		action.WithBefore(before + readPresetFields),
	)
}

// postPresetDraft returns the action expression that
// posts the fields of the new preset dialog.
func postPresetDraft() string {
	return action.PageIndex.PresetDraft.POST(action.WithBefore(readPresetFields))
}

// copyOpts returns the copy combobox options with from selected.
func copyOpts(from string) neo.ComboboxOpts {
	o := neo.ComboboxOpts{
		Size:              neo.Set("sm"),
		AriaLabel:         neo.Set("Copy from"),
		SearchPlaceholder: neo.Set("Search presets"),
	}
	if from != "" {
		o.Value = neo.Set(from)
	}
	return o
}

// Prefixes of the copy combobox values. They keep apart a preset of the app
// and one of Datapages with the same name. The empty value copies the current toxics.
const (
	appPresetPrefix       = "app:"
	datapagesPresetPrefix = "datapages:"
)

// presetItem is 1 preset of the picker. A built-in preset has no delete button.
type presetItem struct {
	name, icon string
	builtin    bool
}

// presetIcon returns the icon of the preset with the name.
// The presets of the app share 1.
func presetIcon(name string) string {
	switch name {
	case config.PresetNoThrottling:
		return "gauge"
	case config.PresetUnreachable:
		return "server-off"
	}
	return "sliders-horizontal"
}

// copyOption is 1 preset of the copy combobox with a source such as "Datapages preset".
type copyOption struct {
	value, name, source string
}

// copyOptions lists the presets of s, of the app with the name, then those of Datapages.
// The built-in No throttling and Unreachable are not offered.
func copyOptions(s toxic.Settings, app string) []copyOption {
	var o []copyOption
	for _, p := range s.Presets {
		o = append(o, copyOption{
			value:  appPresetPrefix + p.Name,
			name:   p.Name,
			source: app + " preset",
		})
	}
	for _, p := range toxic.DefaultPresets() {
		o = append(o, copyOption{
			value:  datapagesPresetPrefix + p.Name,
			name:   p.Name,
			source: "Datapages preset",
		})
	}
	return o
}

// copySource returns the toxics and overrides that the copy combobox value from names.
// An empty from names those of s.
func copySource(s toxic.Settings, from string) (toxic.Preset, error) {
	if from == "" {
		return toxic.Preset{Toxics: s.Global, Endpoints: s.Endpoints}, nil
	}
	if name, ok := strings.CutPrefix(from, datapagesPresetPrefix); ok {
		return toxic.DefaultPreset(name)
	}
	if name, ok := strings.CutPrefix(from, appPresetPrefix); ok {
		return s.Preset(name)
	}
	return toxic.Preset{}, fmt.Errorf("%w %q", toxic.ErrNoPreset, from)
}

// presetDraft holds the fields of the new preset dialog and the error of the
// last create attempt.
type presetDraft struct {
	name, from, err string
	// autoName is the name filled in from a Datapages preset.
	// It follows the copy combobox until the user edits the name.
	autoName string
}

// update stores the dialog fields. A new Datapages copy value replaces an
// empty or filled-in name with its own name. Any other new value empties a
// filled-in name.
func (d *presetDraft) update(name, from string) {
	if from != d.from && (strings.TrimSpace(name) == "" || name == d.autoName) {
		name = ""
		if n, ok := strings.CutPrefix(from, datapagesPresetPrefix); ok {
			name = n
		}
		d.autoName = name
	}
	d.name, d.from, d.err = name, from, ""
}

// newPresetView is what the new preset dialog shows.
type newPresetView struct {
	name, from, err string
	// taken reports whether a preset has the name, which disables Create.
	taken bool
}

// newPreset returns the dialog view of d. Its error is that of the last attempt,
// else that a preset of s has the name.
func newPreset(s toxic.Settings, d presetDraft) newPresetView {
	_, err := s.Preset(strings.TrimSpace(d.name))
	v := newPresetView{name: d.name, from: d.from, err: d.err, taken: err == nil}
	if v.err == "" && v.taken {
		v.err = createPresetError(toxic.ErrPresetExists)
	}
	return v
}

// presetCreated closes the new preset dialog and empties its fields, which a morph keeps.
const presetCreated = "document.getElementById('tp-preset-new').hide(); " +
	"document.getElementById('tp-preset-name').value = ''; " +
	"document.getElementById('tp-preset-from').value = null"

// createPresetError returns the dialog message for err.
func createPresetError(err error) string {
	switch {
	case errors.Is(err, toxic.ErrPresetUnnamed):
		return "Type a name for the preset."
	case errors.Is(err, toxic.ErrPresetExists):
		return "A preset with this name exists."
	case errors.Is(err, toxic.ErrNoPreset):
		return "The preset to copy no longer exists."
	}
	return err.Error()
}

// orUnknown returns version, or "unknown" for a build that does not record it.
func orUnknown(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

// jsString returns s as a JavaScript string literal. templ escapes it for the attribute.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
