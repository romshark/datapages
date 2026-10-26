package ui

import (
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

	"github.com/romshark/datapages/internal/cmd/toxic"
	"github.com/romshark/datapages/internal/cmd/toxic/ui/datapagesgen/action"
)

// Modes of a slot on the settings page.
const (
	modeOn      = "on"
	modeOff     = "off"
	modeInherit = "inherit" // endpoint only, the global toxic applies
)

// errUnknownField is a field key that no field of the toxic type has.
var errUnknownField = errors.New("unknown field")

// setField sets the field of x with the key to the whole number in value.
// The key is that of a field of type t, as in "jitter", or "toxicity" for the
// percentage of requests affected.
func setField(x *toxic.Toxic, t toxic.Type, key, value string) error {
	if key == "toxicity" {
		pct, err := parseInt(value)
		if err != nil || pct < 0 || pct > 100 {
			return errors.New(
				"requests affected must be a whole percentage from 0 to 100")
		}
		x.Toxicity = float32(pct) / 100
		return nil
	}
	fields := toxic.Types[t].Fields
	i := slices.IndexFunc(fields, func(f toxic.Field) bool { return f.Key == key })
	if i < 0 {
		return fmt.Errorf("%w %q of %s", errUnknownField, key, toxic.Types[t].Name)
	}
	v, err := parseInt(value)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.ToLower(fields[i].Label), err)
	}
	x.Values[i] = v
	return nil
}

// parseInt parses a field value. An empty field reads as 0,
// which is the value a cleared field shows after the page updates.
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

// current returns the toxic a slot of scope turns on with: its own,
// else the global one, else that of [toxic.NewToxic].
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

// setSlot applies the mode of 1 slot of scope, an endpoint pattern or empty
// for the global scope. A toxic turned off keeps its values for when it is
// turned on again.
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

// notice is the title and the text of a toast, failed for one that reports
// an error.
type notice struct {
	title, text string
	failed      bool
}

// toastOpts shows a notice for 4 seconds, and a failure until the user closes it.
func toastOpts(n notice) neo.ToastOpts {
	if n.failed {
		return neo.ToastOpts{Variant: neo.Set(neo.ToastVariantError)}
	}
	return neo.ToastOpts{
		Variant:  neo.Set(neo.ToastVariantSuccess),
		Duration: neo.Set(4000),
	}
}

// slotError is an input the settings page rejected, none when msg is empty.
type slotError struct {
	slot toxic.Slot
	msg  string
}

// pageData is what the settings page shows.
type pageData struct {
	info toxic.Info

	// scope is the selected endpoint pattern, empty for the global toxics.
	scope string
	// scopeEndpoint is the endpoint of scope. Its pattern is empty when scope
	// names none of the app.
	scopeEndpoint toxic.Endpoint
	// scopeKind is the kind of scope, as its pattern implies for one that
	// names none of the app.
	scopeKind string
	// scopeOverrides counts the slots scope sets.
	scopeOverrides int

	filter filter
	// endpoints and unknown list the endpoints filter shows, unknown those
	// with overrides that name no endpoint of the app.
	endpoints, unknown []navItem
	// hidden counts the endpoints filter hides.
	hidden int

	// presets are the presets the picker lists, and activePreset is the name
	// of the one the global toxics match, empty for none.
	presets      []presetItem
	activePreset string
	// copyOptions are the presets the dialog of a new preset copies from,
	// and newPreset is what the dialog shows.
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

type typeView struct {
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
}

// view returns what the page shows a tab with the state tab.
func (a *App) view(tab StateIndex) pageData {
	s := a.Front.Settings()
	eps := a.Front.Endpoints()
	scope, f := tab.Endpoint, filter{search: tab.Search, kinds: tab.Kinds}
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
			name: p.Name, builtin: toxic.IsBuiltinPreset(p.Name),
		})
	}
	d.activePreset = s.ActivePreset()
	d.copyOptions = copyOptions(s, d.info.AppName)
	d.newPreset = newPreset(s, tab.NewPreset)

	override := s.Endpoints[scope]
	for t, info := range toxic.Types {
		tv := typeView{info: info}
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
			tv.rows = append(tv.rows, r)
		}
		d.types = append(d.types, tv)
	}

	d.saved = a.Front.Saved()
	return d
}

// kinds lists the kinds of endpoints in the order of the toggles that filter
// the endpoint list by them.
var kinds = [...]string{toxic.KindGET, toxic.KindStream, toxic.KindAction, toxic.KindFile}

// kindInfo describes a kind of endpoints to the endpoint list.
type kindInfo struct {
	// label and title are those of the toggle of the kind.
	label, title string
	// name is that of 1 endpoint of the kind, and icon the file in
	// static/icons, without .svg, that shows it in front of its handler.
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

// filter selects the endpoints of the list: those whose search text matches
// search, of a kind in kinds, a comma-separated list. Empty kinds is every kind.
type filter struct {
	search, kinds string
}

// shows reports whether f shows an endpoint of the kind with the search text.
func (f filter) shows(kind, text string) bool {
	return f.showsKind(kind) && matches(f.search, text)
}

func (f filter) showsKind(kind string) bool {
	return f.kinds == "" || slices.Contains(strings.Split(f.kinds, ","), kind)
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

// pressedKinds is the value of the toggle group of the kinds f shows.
func (f filter) pressedKinds() string {
	var shown []string
	for _, k := range kinds {
		if f.showsKind(k) {
			shown = append(shown, k)
		}
	}
	return strings.Join(shown, ",")
}

// matches reports whether each space-separated term of query occurs in text
// as a subsequence, ignoring case. "msg send" matches
// "POST /messages/sendmessage/{$}".
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

// fieldInfo returns the info button of the field f, nil for a field without one.
func fieldInfo(f toxic.Field) templ.Component {
	if f.Key == "jitter" {
		return jitterInfo()
	}
	return nil
}

// setValue is the action expression that posts the value of the field with the
// key of the toxic of slot, or "toxicity".
func setValue(slot toxic.Slot, key string) string {
	return action.PageIndex.Value.POST(action.PageIndex.Value.POSTQuery(slot, key),
		action.WithBefore("$value = el.value"))
}

// statusOption is 1 option of the status combobox, as in 503 with the label
// "503 Service Unavailable".
type statusOption struct {
	value, label string
}

// statusOptions lists the statuses f takes that [net/http.StatusText] names,
// and current, which datapages.yaml may set to one it does not name.
func statusOptions(f toxic.Field, current int64) []statusOption {
	var opts []statusOption
	for code := f.Min; code <= f.Max; code++ {
		text := http.StatusText(int(code))
		if text == "" && code != current {
			continue
		}
		v := strconv.FormatInt(code, 10)
		opts = append(opts, statusOption{value: v, label: strings.TrimSpace(v + " " + text)})
	}
	return opts
}

// withAttrs returns the attributes of a and b, those of b winning.
func withAttrs(a, b templ.Attributes) templ.Attributes {
	m := maps.Clone(a)
	maps.Copy(m, b)
	return m
}

func method(pattern string) string {
	m, _, _ := strings.Cut(pattern, " ")
	return m
}

func path(pattern string) string {
	_, p, _ := strings.Cut(pattern, " ")
	return strings.TrimSpace(p)
}

// pathPart is a piece of the path of a route pattern, with the class that
// colors it, empty for literal text.
type pathPart struct {
	text, class string
}

// pathParts splits the path of a route pattern into literal text, wildcards
// such as {slug} or {rest...}, and the end anchor {$}.
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

// streamIcon is the icon in static/icons of the direction of the data of s.
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

// summary describes x of the type info in 1 line, as in "Latency 300 ms,
// Jitter 0 ms, 100% of requests".
func summary(info toxic.TypeInfo, x toxic.Toxic) string {
	if x.Disabled {
		return "off"
	}
	parts := make([]string, 0, len(info.Fields)+1)
	for i, f := range info.Fields {
		p := f.Label + " " + strconv.FormatInt(x.Values[i], 10)
		if u := f.Unit.String(); u != "" {
			p += " " + u
		}
		parts = append(parts, p)
	}
	parts = append(parts, strconv.Itoa(toxicityPercent(x))+"% of requests")
	return strings.Join(parts, ", ")
}

// newPresetOption is the value of the option of the preset picker that opens
// the dialog of a new preset. A preset name has no space at either end, which
// keeps this value apart from every name.
const newPresetOption = " new"

// pickerOpts are the options of the preset picker, which selects the preset
// active, none for an empty one.
func pickerOpts(active string) neo.ComboboxOpts {
	o := neo.ComboboxOpts{
		Placeholder:       neo.Set("Custom"),
		SearchPlaceholder: neo.Set("Search presets"),
		AriaLabel:         neo.Set("Preset"),
		PopoverFitContent: neo.Set(true),
	}
	if active != "" {
		o.Value = neo.Set(active)
	}
	return o
}

// pickPreset is the action expression of the preset picker, whose selected
// preset is active, empty for none. It applies the preset picked. For the
// option of a new preset it selects active again and opens the dialog.
func pickPreset(active string) string {
	reselect := "null"
	if active != "" {
		reselect = jsString(active)
	}
	return action.PageIndex.ApplyPreset.POST(action.WithBefore(
		"if (evt.detail.value === " + jsString(newPresetOption) + ") { " +
			"el.value = " + reselect + "; " +
			openDialog("tp-preset-new") + "; " +
			// Clears why the last attempt to create a preset failed.
			postPresetDraft() + "; return } " +
			"$preset = evt.detail.value"))
}

// askDeletePreset is the action expression of the button that deletes the
// preset with the name in its option of the picker. It keeps the click from
// picking the preset, closes the picker and opens the dialog that confirms.
func askDeletePreset(name string) string {
	return "evt.stopPropagation(); $preset = " + jsString(name) + "; " +
		"el.closest('neo-combobox').hide({restoreFocus: true}); " +
		openDialog("tp-preset-delete")
}

// openDialog returns a statement that opens the neo-dialog with the id in the
// next frame. By then the picker closed and took the focus back, which the
// dialog returns to it when it closes.
func openDialog(id string) string {
	return "requestAnimationFrame(() => document.getElementById('" + id + "').show())"
}

// readPresetFields sets the name and from signals to the name field and the
// copy combobox of the dialog of a new preset.
const readPresetFields = "$name = document.getElementById('tp-preset-name').value; " +
	"$from = document.getElementById('tp-preset-from').value ?? ''"

// createPreset is the action expression that creates the preset of the
// dialog. before runs first and may return to post nothing.
func createPreset(before string) string {
	return action.PageIndex.CreatePreset.POST(action.WithBefore(before + readPresetFields))
}

// postPresetDraft is the action expression that posts the fields of the
// dialog of a new preset as they are.
func postPresetDraft() string {
	return action.PageIndex.PresetDraft.POST(action.WithBefore(readPresetFields))
}

// copyOpts are the options of the copy combobox, which selects from,
// the global toxics for an empty one.
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

// The prefixes of the value of a preset in the copy combobox of the dialog of
// a new preset, which keep a preset of the app and one of Datapages with the
// same name apart. The empty value copies the global toxics.
const (
	appPresetPrefix       = "app:"
	datapagesPresetPrefix = "datapages:"
)

// presetItem is 1 preset of the picker. A built-in preset has no button that
// deletes it.
type presetItem struct {
	name    string
	builtin bool
}

// copyOption is 1 preset of the copy combobox, with the source of the preset,
// as in "Datapages preset".
type copyOption struct {
	value, name, source string
}

// copyOptions lists the presets of s, of the app with the name, then those of
// Datapages. The built-in presets, No throttling without toxics and Offline,
// are no start for another preset.
func copyOptions(s toxic.Settings, app string) []copyOption {
	var o []copyOption
	for _, p := range s.Presets {
		o = append(o, copyOption{
			value: appPresetPrefix + p.Name, name: p.Name, source: app + " preset",
		})
	}
	for _, p := range toxic.DefaultPresets() {
		o = append(o, copyOption{
			value: datapagesPresetPrefix + p.Name, name: p.Name, source: "Datapages preset",
		})
	}
	return o
}

// copySource returns the global toxics and the overrides the value from of
// the copy combobox names: those of s for an empty from, else those of a
// preset of s or of Datapages.
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

// presetDraft is what the dialog of a new preset holds: the name, the value of
// the copy combobox, and why the last attempt to create the preset failed.
type presetDraft struct {
	name, from, err string
	// autoName is the name the dialog filled in for a preset of Datapages,
	// which follows the copy combobox until the user changes the name.
	autoName string
}

// update takes the name and the copy value of the dialog. The copy value of
// a preset of Datapages fills in its name where the name is empty or one the
// dialog filled in, and any other empties such a name.
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

// newPresetView is what the dialog of a new preset shows.
type newPresetView struct {
	name, from, err string
	// taken reports whether a preset has the name, which disables creating it.
	taken bool
}

// newPreset returns what the dialog shows for the draft d with the presets of
// s: why the last attempt failed, else that a preset has the name.
func newPreset(s toxic.Settings, d presetDraft) newPresetView {
	_, err := s.Preset(strings.TrimSpace(d.name))
	v := newPresetView{name: d.name, from: d.from, err: d.err, taken: err == nil}
	if v.err == "" && v.taken {
		v.err = createPresetError(toxic.ErrPresetExists)
	}
	return v
}

// presetCreated closes the dialog of a new preset and empties its fields,
// whose text and choice a morph keeps.
const presetCreated = "document.getElementById('tp-preset-new').hide(); " +
	"document.getElementById('tp-preset-name').value = ''; " +
	"document.getElementById('tp-preset-from').value = null"

// createPresetError is the message the dialog of a new preset shows for err.
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

// jsString returns s as a JavaScript string literal for an action expression,
// which templ escapes for the attribute.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
