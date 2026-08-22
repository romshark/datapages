package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/romshark/datapages/internal/gotypes"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/routepattern"
	"github.com/romshark/datapages/internal/subject"
)

// eventVarName is the generated variable name of a decoded event.
func eventVarName(eventTypeName string) string {
	return strings.ToLower(eventTypeName[:1]) + eventTypeName[1:]
}

// dispatcherTypeName is the generated datapages.Dispatcher implementation
// of an event.
func dispatcherTypeName(eventTypeName string) string {
	return "dispatcher" + eventTypeName
}

// dispatchVarName is the generated variable name of a dispatcher.
// The prefix keeps the closures of different handlers apart inside one
// generated function; the event name keeps a handler's own closures apart.
//
//	("dispatch", "EventTodoUpdated") -> "dispatchTodoUpdated"
func dispatchVarName(prefix, eventTypeName string) string {
	return prefix + eventConstName(eventTypeName)
}

// eventConstName strips the "Event" prefix from an event type name.
// "EventMessagingSent" -> "MessagingSent"
func eventConstName(typeName string) string {
	return strings.TrimPrefix(typeName, "Event")
}

// evSubjConst returns the subscription subject constant name.
// "EventMessagingSent" -> "EvSubjMessagingSent"
func evSubjConst(e *model.Event) string {
	return "EvSubj" + eventConstName(e.TypeName)
}

// evUsesPrefixMatch reports whether the event's concrete subject is only
// known at runtime. Both subscription building and inbound matching must then
// go through the subject prefix instead of the wildcard constant.
// True for private (SubjectUser), signal-scoped and state-id-scoped events.
func evUsesPrefixMatch(e *model.Event) bool {
	// Any subject field makes the subscription a pattern: the constant carries
	// one "*" per field. A received subject carries the values instead,
	// so it is matched by the prefix in front of them rather than by equality.
	return e.HasSubjectFields()
}

// evSubjPrefConst returns the subject prefix constant name for events
// that use prefix-based subject matching.
// Returns "" for plain public events.
// "EventMessagingSent" -> "EvSubjPrefMessagingSent"
func evSubjPrefConst(e *model.Event) string {
	if !evUsesPrefixMatch(e) {
		return ""
	}
	return "EvSubjPref" + eventConstName(e.TypeName)
}

// evSubjValue returns the subscription subject constant value.
// For events with N subject fields: "messaging.sent.*.*" (N wildcards appended).
// For public events: "posts.archived"
func evSubjValue(e *model.Event) string {
	n := len(e.SubjectFields)
	if n == 0 {
		return e.Subject
	}
	var b strings.Builder
	b.WriteString(e.Subject)
	for range n {
		b.WriteString(".*")
	}
	return b.String()
}

// evSubjPrefValue returns the subject prefix value used to match private events.
// "messaging.sent."
func evSubjPrefValue(e *model.Event) string {
	return subject.Prefix(e.Subject)
}

// stripPagePrefix strips "Page" prefix from type name: "PageSettings" -> "Settings"
func stripPagePrefix(typeName string) string {
	return strings.TrimPrefix(typeName, "Page")
}

// pageHasStream returns true if the page has event handlers and needs a stream.
func pageHasStream(p *model.Page) bool {
	return len(p.EventHandlers) > 0 || p.StreamOpen != nil || p.StreamClose != nil
}

// pageStreamNeedsAuth returns true if handling the page's SSE stream requires
// loading session context.
func pageStreamNeedsAuth(
	p *model.Page, eventByName map[string]*model.Event,
) bool {
	if pageHasPrivateEvent(p, eventByName) {
		return true
	}
	for _, eh := range p.EventHandlers {
		if eh.InputSession != nil {
			return true
		}
	}
	for _, h := range []*model.Handler{p.StreamOpen, p.StreamClose} {
		if h == nil {
			continue
		}
		if h.InputSession != nil {
			return true
		}
	}
	return false
}

// pageHasPrivateEvent returns true if any event handler on the page
// handles a private event (has SubjectUser).
func pageHasPrivateEvent(p *model.Page, eventByName map[string]*model.Event) bool {
	for _, eh := range p.EventHandlers {
		if e, ok := eventByName[eh.EventTypeName]; ok && e.IsPrivate() {
			return true
		}
	}
	return false
}

// pageHasSignalScopedEvent returns true if any event handler on the page
// handles a signal-scoped event (has subject fields with signal tags).
func pageHasSignalScopedEvent(p *model.Page, eventByName map[string]*model.Event) bool {
	for _, eh := range p.EventHandlers {
		if e, ok := eventByName[eh.EventTypeName]; ok && e.IsSignalScoped() {
			return true
		}
	}
	return false
}

// pageHasStateIDScopedEvent returns true if any event handler on the page
// handles a state-id-scoped event (has a SubjectStateID field).
func pageHasStateIDScopedEvent(p *model.Page, eventByName map[string]*model.Event) bool {
	for _, eh := range p.EventHandlers {
		if e, ok := eventByName[eh.EventTypeName]; ok && e.IsStateIDScoped() {
			return true
		}
	}
	return false
}

// pageNeedsStateRouteKey returns true if the page's stream handler has to
// derive the tab's routing key, either to subscribe or to pass it on to a
// handler that takes stateID.
func pageNeedsStateRouteKey(p *model.Page, eventByName map[string]*model.Event) bool {
	if pageHasStateIDScopedEvent(p, eventByName) {
		return true
	}
	for _, h := range []*model.Handler{p.StreamOpen, p.StreamClose} {
		if h != nil && h.InputStateID != nil {
			return true
		}
	}
	for _, eh := range p.EventHandlers {
		if eh.InputStateID != nil {
			return true
		}
	}
	return false
}

// pageSignalSubjectFields returns the unique signal-scoped subject fields
// across all events handled by the page, in first-seen order.
func pageSignalSubjectFields(p *model.Page, eventByName map[string]*model.Event) []model.SubjectField {
	seen := make(map[string]bool)
	var fields []model.SubjectField
	for _, eh := range p.EventHandlers {
		e, ok := eventByName[eh.EventTypeName]
		if !ok {
			continue
		}
		for _, sf := range e.SubjectFields {
			if sf.SignalName != "" && !seen[sf.SignalName] {
				seen[sf.SignalName] = true
				fields = append(fields, sf)
			}
		}
	}
	return fields
}

// pageHasAnonStream returns true if a page needs an anonymous stream endpoint.
// This happens when the page has both public AND private events.
func pageHasAnonStream(p *model.Page, eventByName map[string]*model.Event) bool {
	hasPublic := false
	hasPrivate := false
	for _, eh := range p.EventHandlers {
		e, ok := eventByName[eh.EventTypeName]
		if !ok {
			continue
		}
		if e.IsPrivate() {
			hasPrivate = true
		} else {
			hasPublic = true
		}
	}
	return hasPublic && hasPrivate
}

// routeStreamPath returns the SSE stream path for a page route.
//   - "/settings/" -> "/settings/_$/"
//   - "/post/{slug}/" -> "/post/{slug}/_$/"
//   - "/" -> "/_$/"
func routeStreamPath(route string) string {
	return routepattern.WithTrailingSlash(route) + "_$/"
}

// renderType renders a Go type using types.TypeString,
// qualifying every package by the name it declares.
//
// That is what an unaliased import binds to, for the app package as much as
// for any other, and a package is free to declare a name its directory does not repeat.
func renderType(t model.Type) string {
	return types.TypeString(t.Resolved, func(p *types.Package) string {
		return p.Name()
	})
}

// renderAnonStructType renders an anonymous struct type, preserving struct tags.
// It renders from the resolved type rather than the source it was written as.
// The generated package is not the app package, which leaves a type the app
// names with nothing to resolve to unless it is written with its package.
func renderAnonStructType(t model.Type, fset *token.FileSet) string {
	st, ok := t.Resolved.Underlying().(*types.Struct)
	if !ok {
		// Not a struct. Fall back to the source expression.
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, t.TypeExpr); err != nil {
			return ""
		}
		return buf.String()
	}
	return renderStruct(st)
}

// renderStruct writes a struct type with every named type qualified by its package.
// Anonymous struct fields, which signals nest, recurse.
func renderStruct(st *types.Struct) string {
	var b strings.Builder
	b.WriteString("struct {\n")
	for i := range st.NumFields() {
		f := st.Field(i)
		b.WriteString(f.Name())
		b.WriteByte(' ')
		if nested, ok := f.Type().(*types.Struct); ok {
			b.WriteString(renderStruct(nested))
		} else {
			b.WriteString(types.TypeString(f.Type(), func(p *types.Package) string {
				return p.Name()
			}))
		}
		if tag := st.Tag(i); tag != "" {
			b.WriteString(" `")
			b.WriteString(tag)
			b.WriteByte('`')
		}
		b.WriteByte('\n')
	}
	b.WriteByte('}')
	return b.String()
}

// isNamedType returns true if the type is a named type (not anonymous struct).
func isNamedType(t model.Type) bool {
	_, ok := t.Resolved.(*types.Named)
	return ok
}

// structFieldInfo holds information about a single struct field.
type structFieldInfo struct {
	Name string
	Type types.Type
	Tag  string // raw struct tag
}

// structFields returns the fields of a struct type (works for both named and anonymous).
// Returns field name, type, and raw struct tag for each field.
// The returned slice is reused across calls; callers must consume it before
// calling structFields again.
func (w *Writer) structFields(t types.Type) []structFieldInfo {
	w.fields = w.fields[:0]
	if t == nil {
		return w.fields
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return w.fields
	}
	for i := range st.NumFields() {
		f := st.Field(i)
		w.fields = append(w.fields, structFieldInfo{
			Name: f.Name(),
			Type: f.Type(),
			Tag:  st.Tag(i),
		})
	}
	return w.fields
}

// appUsage tracks which optional helpers are referenced by the generated handler code.
// It is computed once from the model before any code is emitted, and used to
// conditionally emit helper functions/methods that would otherwise be dead code.
type appUsage struct {
	// hasSession: whether the app defines a Session type.
	hasSession bool
	// auth: func (s *Server) auth(...)
	auth bool
	// createSession: func (s *Server) createSession(...)
	createSession bool
	// closeSession: func (s *Server) closeSession(...)
	closeSession bool
	// httpRedirect: func httpRedirect(...)
	httpRedirect bool
	// stream: func (s *Server) handleStreamRequest(...)
	stream bool
	// streamAuth: whether any stream handler needs auth (page has private events).
	streamAuth bool
	// dsRequest: func (s *Server) checkIsDSReq(...)
	dsRequest bool
	// recoverError: httpErrIntern asks httpserve.IsDatastarRequest for an app that has
	// PageError500, RecoverError, or both. The two features are independent
	// and either one makes the helper's answer decide what the response is.
	recoverError bool
	// httpErrBad: whether the httpErrBad helper is needed.
	httpErrBad bool
	// errSentinels: whether any action returns an error, so the generated
	// fallback maps the datapages error sentinels to status codes.
	errSentinels bool
	// datapagesSSE: whether any handler takes a datapages.SSE param
	// (needs the datapages import and the generated sseWrapper).
	datapagesSSE bool
	// stateRuntime: whether any page (including via embedded abstract pages)
	// takes state *T; enables the per-page-instance state runtime.
	stateRuntime bool
	// signalSubjects: subject.IsToken is called by any page that builds
	// a subscription subject from a client-provided signal.
	signalSubjects bool
	// dispatchSubjects: subject.IsToken is called by any dispatch that
	// builds a publish subject from the subject fields of its event.
	dispatchSubjects bool
	// userSubjects: whether any event addresses a user, which makes the ID of
	// the session owner name a subject.
	userSubjects bool
	// privateStreams: func (s *Server) checkUserSubject(...), needed by any
	// page that subscribes to an event addressed to the session owner.
	privateStreams bool
}

// needsCheckIsDSReq returns true if the checkIsDSReq method must be emitted.
func (u appUsage) needsCheckIsDSReq() bool {
	return u.stream || u.dsRequest
}

// dispatchesSubjectFields reports whether any handler dispatches an event whose
// subject carries field values. Such a dispatch builds its subject at runtime
// and needs every value guarded: one that is not a single token produces a
// subject no subscription matches and the event reaches nobody.
func dispatchesSubjectFields(
	m *model.App, eventByName map[string]*model.Event,
) bool {
	dispatches := func(h *model.Handler) bool {
		if h == nil {
			return false
		}
		for _, d := range h.InputDispatches {
			ev := eventByName[d.EventTypeName]
			if ev != nil && ev.HasSubjectFields() {
				return true
			}
		}
		return false
	}
	if slices.ContainsFunc(m.Actions, dispatches) {
		return true
	}
	for _, p := range m.Pages {
		if p.GET != nil && dispatches(p.GET.Handler) {
			return true
		}
		if dispatches(p.StreamOpen) || dispatches(p.StreamClose) {
			return true
		}
		if slices.ContainsFunc(p.Actions, dispatches) {
			return true
		}
	}
	for _, p := range []*model.Page{m.PageError404, m.PageError500} {
		if p != nil && p.GET != nil && dispatches(p.GET.Handler) {
			return true
		}
	}
	return false
}

// computeAppUsage scans the model to determine which optional helpers are needed.
func computeAppUsage(m *model.App) appUsage {
	var u appUsage

	u.hasSession = m.Session != nil

	// A global Head that takes a session has every page handler read one,
	// whatever the handler itself asks for.
	if m.GlobalHeadGenerator != nil && m.GlobalHeadGenerator.InputSession {
		u.auth = true
	}

	if m.RecoverError != nil || m.PageError500 != nil {
		u.recoverError = true
	}
	if m.RecoverError != nil {
		// RecoverError always receives a datapages.SSE.
		u.datapagesSSE = true
	}

	checkHandler := func(h *model.Handler) {
		if h.InputSession != nil {
			u.auth = true
		}
		if needsCSRFOnly(h, m) {
			// The handler calls the session helper for its CSRF check.
			u.auth = true
		}
		if h.OutputNewSession != nil {
			u.createSession = true
		}
		if h.OutputCloseSession != nil {
			u.closeSession = true
		}
		if h.OutputRedirect != nil {
			u.httpRedirect = true
		}
		if h.InputSSE != nil || h.InputSignals != nil {
			u.dsRequest = true
		}
		if h.InputSSE != nil {
			u.datapagesSSE = true
		}
		if h.InputSignals != nil {
			u.httpErrBad = true
		}
		if h.InputQuery != nil && structHasNonStringField(h.InputQuery.Type.Resolved) {
			u.httpErrBad = true
		}
		if h.InputPath != nil && structHasNonStringField(h.InputPath.Type.Resolved) {
			u.httpErrBad = true
		}
	}

	// Build event map for subject field lookup.
	eventByName := make(map[string]*model.Event, len(m.Events))
	for _, e := range m.Events {
		eventByName[e.TypeName] = e
	}

	u.dispatchSubjects = dispatchesSubjectFields(m, eventByName)
	for _, e := range m.Events {
		if e.HasSubjectUser() {
			u.userSubjects = true
			break
		}
	}

	for _, h := range m.Actions {
		checkHandler(h)
		if h.OutputErr != nil {
			u.errSentinels = true
		}
	}
	for _, p := range m.Pages {
		if p.GET != nil {
			checkHandler(p.GET.Handler)
		}
		if pageHasStream(p) {
			u.stream = true
			// Event handlers and stream hooks receive a datapages.SSE.
			u.datapagesSSE = true
			if pageStreamNeedsAuth(p, eventByName) {
				u.streamAuth = true
				u.auth = true
			}
			if pageHasAnonStream(p, eventByName) {
				u.httpErrBad = true
			}
			if pageHasSignalScopedEvent(p, eventByName) {
				u.httpErrBad = true
				u.signalSubjects = true
			}
			if pageHasPrivateEvent(p, eventByName) {
				u.privateStreams = true
			}
			if p.StreamOpen != nil && p.StreamOpen.InputSignals != nil {
				u.httpErrBad = true
			}
		}
		for _, h := range p.Actions {
			checkHandler(h)
			if h.OutputErr != nil {
				u.errSentinels = true
			}
		}
		if p.State != nil {
			u.stateRuntime = true
		}
	}
	if m.PageError404 != nil && m.PageError404.GET != nil {
		checkHandler(m.PageError404.GET.Handler)
	}

	return u
}

// writerPool is a package-level pool for reusing Writer instances across Generate calls.
var writerPool = sync.Pool{
	New: func() any {
		return &Writer{
			Buf:      make([]byte, 0, 512*1024), // 512 KiB
			eventMap: make(map[string]*model.Event),
		}
	},
}

type Writer struct {
	Buf []byte
	// eventMap is built once per WriteApp, reused
	eventMap map[string]*model.Event
	// dispatchedEvents are the events a handler dispatches,
	// in the order they were first seen.
	dispatchedEvents []string
	// fields is a reusable scratch for structFields
	fields []structFieldInfo
	// prometheus defines whether to generate Prometheus metrics code
	prometheus bool
	// assetsURLPrefix is the URL path prefix for static files (empty = disabled)
	assetsURLPrefix string
	// assetsDir is the subdirectory within the app package for
	// static files (e.g. "static")
	assetsDir string
	// appDir is the app source package path relative to module root (e.g. "app")
	appDir string
	// genImport is the full import path of the generated root package
	genImport string
	// appPkgQual is the identifier that qualifies app types in generated code.
	appPkgQual string
	// usage is computed once per WriteApp
	usage appUsage
	// sessionType is the rendered session type of the application,
	// e.g. "datapages.Session[app.SessionData]". Empty if it has no session.
	sessionType string
	// newSessionType is the rendered type handlers return as newSession,
	// e.g. "datapages.NewSession[app.SessionData]".
	newSessionType string
	// makeSessionCall assembles a session from a session manager record.
	makeSessionCall string
	// recordType is the rendered sessions.Record instantiation.
	recordType string
	// sessionDataType is the rendered session Data type argument.
	sessionDataType string
}

// setSessionType renders the application's datapages.Session instantiation and
// the expressions derived from it.
func (w *Writer) setSessionType(m *model.App) {
	if m.Session == nil {
		w.sessionType, w.newSessionType = "", ""
		w.makeSessionCall, w.recordType = "", ""
		w.sessionDataType = ""
		return
	}
	data := renderType(m.Session.Data)
	w.sessionType = "datapages.Session[" + data + "]"
	w.newSessionType = "datapages.NewSession[" + data + "]"
	w.recordType = "sessions.Record[" + data + "]"
	w.sessionDataType = data
	w.makeSessionCall = "datapages.MakeSession(\n" +
		"\t\trec.UserID, token, rec.IssuedAt, rec.ExpiresAt, rec.Data,\n\t)"
}

func (w *Writer) Reset() {
	w.Buf = w.Buf[:0]
	clear(w.eventMap)
	w.dispatchedEvents = w.dispatchedEvents[:0]
	w.fields = w.fields[:0]
	w.usage = appUsage{}
}

// buildEventMap populates w.eventMap from the given events.
func (w *Writer) buildEventMap(events []*model.Event) {
	if w.eventMap == nil {
		w.eventMap = make(map[string]*model.Event, len(events))
	}
	for _, e := range events {
		w.eventMap[e.TypeName] = e
	}
}

// Raw appends a string to the buffer.
func (w *Writer) Raw(s string) { w.Buf = append(w.Buf, s...) }

// Rawf appends a formatted string to the buffer.
func (w *Writer) Rawf(format string, args ...any) {
	w.Buf = append(w.Buf, fmt.Sprintf(format, args...)...)
}

// Byte appends a single byte to the buffer.
func (w *Writer) Byte(b byte) { w.Buf = append(w.Buf, b) }

// Line writes an indented line.
// indent is the number of leading tabs.
func (w *Writer) Line(indent int, s string) {
	for range indent {
		w.Byte('\t')
	}
	w.Raw(s)
	w.Byte('\n')
}

// Linef writes an indented formatted line.
// indent is the number of leading tabs.
func (w *Writer) Linef(indent int, format string, args ...any) {
	for range indent {
		w.Byte('\t')
	}
	w.Rawf(format, args...)
	w.Byte('\n')
}

// writePageConstructor appends the page struct literal construction.
// e.g.: "app.PageSettings{\n\tApp: s.app,\n\tBase: app.Base{App: s.app},\n}"
func (w *Writer) writePageConstructor(p *model.Page, appPkg string) {
	w.Raw(appPkg)
	w.Byte('.')
	w.Raw(p.TypeName)
	w.Raw("{\n")
	w.Raw("\tApp: s.app,\n")
	for _, embed := range p.Embeds {
		w.writeEmbedInit(p, embed, appPkg, "\t", nil)
	}
	w.Byte('}')
}

// writeEmbedInit recursively appends an embed field initialization.
func (w *Writer) writeEmbedInit(
	p *model.Page, ap *model.AbstractPage, appPkg, indent string,
	parent types.Type,
) {
	w.Raw(indent)
	w.Raw(ap.TypeName)
	w.Raw(": ")
	expr, embedded := w.embedLiteralExpr(p, ap, appPkg, parent)
	w.Raw(expr)
	w.Raw("{\n")
	w.Raw(indent)
	w.Raw("\tApp: s.app,\n")
	for _, sub := range ap.Embeds {
		w.writeEmbedInit(p, sub, appPkg, indent+"\t", embedded)
	}
	w.Raw(indent)
	w.Raw("},\n")
}

// embedTypeExpr returns the type to use in the embed's composite literal.
// A generic abstract page must be instantiated with the type arguments
// written at the embed site, e.g. "app.Base[app.StateFoo]".
// embedLiteralExpr returns the composite literal prefix for an embed and
// whether the field is a pointer.
//
// Go lets a struct embed a pointer to another struct. The field type is then
// "*app.Base", which is not a type a composite literal can be written of: the
// literal is of the element type and its address is taken.
func (w *Writer) embedLiteralExpr(
	p *model.Page, ap *model.AbstractPage, appPkg string, parent types.Type,
) (expr string, embedded types.Type) {
	name := appPkg + "." + ap.TypeName
	var resolved types.Type
	switch {
	case parent != nil:
		// An embed of an embed: its type comes from the parent's, already
		// instantiated. The Base inside Mid[StateA] is Base[StateA].
		if t, ok := embedFieldType(parent, ap.TypeName); ok {
			resolved = t
		}
	default:
		if t, ok := p.EmbedTypes[ap.TypeName]; ok && t.Resolved != nil {
			resolved = t.Resolved
		}
	}
	if resolved != nil {
		name = renderType(model.Type{Resolved: resolved})
	}
	if rest, ok := strings.CutPrefix(name, "*"); ok {
		return "&" + rest, resolved
	}
	return name, resolved
}

// embedFieldType returns the type of the embedded field named name inside t.
// For a generic parent the field carries the instantiation the parent was
// given, which is what the composite literal has to name.
func embedFieldType(t types.Type, name string) (types.Type, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	for f := range st.Fields() {
		if f.Embedded() && f.Name() == name {
			return f.Type(), true
		}
	}
	return nil, false
}

// itoa converts a small non-negative integer to a string without allocation.
func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return fmt.Sprintf("%d", i)
}

// writeCommaSep writes items separated by ", ".
func (w *Writer) writeCommaSep(items []string) {
	for i, item := range items {
		if i > 0 {
			w.Raw(", ")
		}
		w.Raw(item)
	}
}

// writeCallExpr writes "receiver.method(args...)" to the buffer.
func (w *Writer) writeCallExpr(receiver, method string, args []string) {
	w.Raw(receiver)
	w.Byte('.')
	w.Raw(method)
	w.Byte('(')
	w.writeCommaSep(args)
	w.Byte(')')
}

// writeQuoted writes a Go double-quoted string literal to the buffer.
// Only safe for values that don't contain special characters (backslash, quote, newline).
func (w *Writer) writeQuoted(s string) {
	w.Byte('"')
	w.Raw(s)
	w.Byte('"')
}

// writeAnyCheck writes a boolean variable assignment that OR-combines
// zero-checks for the given fields. E.g.:
//
//	anyQuery := query.Foo != "" ||
//		query.Bar != 0
func (w *Writer) writeAnyCheck(varName string, fields []structFieldInfo) {
	if len(fields) == 0 {
		w.Raw("\t")
		w.Raw(varName)
		w.Raw(" := false\n")
		return
	}
	w.Raw("\t")
	w.Raw(varName)
	w.Raw(" := ")
	w.writeZeroCheck("query."+fields[0].Name, fields[0].Type)
	if len(fields) == 1 {
		w.Byte('\n')
		return
	}
	w.Raw(" ||\n")
	for i := 1; i < len(fields); i++ {
		w.Raw("\t\t")
		w.writeZeroCheck("query."+fields[i].Name, fields[i].Type)
		if i < len(fields)-1 {
			w.Raw(" ||\n")
		} else {
			w.Byte('\n')
		}
	}
}

// structHasNonStringField returns true if the resolved struct type
// has any field whose type is not string.
func structHasNonStringField(t types.Type) bool {
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for field := range st.Fields() {
		if !gotypes.IsString(field.Type()) {
			return true
		}
	}
	return false
}

// appPkgQualifier returns the identifier that qualifies app types in generated code.
// An unaliased import binds to the name the package declares,
// which is free to differ from its directory.
func appPkgQualifier(m *model.App) string {
	if m.PkgName != "" {
		return m.PkgName
	}
	return appPkgName(m.PkgPath)
}

// appPkgName returns the short package name from an import path.
// "github.com/romshark/datapages/example/classifieds/app" -> "app"
func appPkgName(pkgPath string) string {
	if i := strings.LastIndex(pkgPath, "/"); i >= 0 {
		return pkgPath[i+1:]
	}
	return pkgPath
}

// signalIdents returns unique exported identifiers for a page's signal-scoped
// subject fields, derived from their signal names. They name both the fields of
// the subjSignals struct the stream handler reads and the parameters of the
// page's evSubj function. The signal name is the key because that's what the
// fields are deduplicated by: two events can bind the same signal through
// differently named fields.
//
//	"instance_id" -> "InstanceID", "form.calc_id" -> "FormCalcID"
func signalIdents(fields []model.SubjectField) []string {
	idents := make([]string, len(fields))
	seen := make(map[string]bool, len(fields))
	for i, sf := range fields {
		base := signalIdent(sf.SignalName)
		ident := base
		for n := 2; seen[ident]; n++ {
			ident = base + itoa(n)
		}
		seen[ident] = true
		idents[i] = ident
	}
	return idents
}

// signalIdentMap maps each signal name to the identifier signalIdents gave it.
func signalIdentMap(fields []model.SubjectField, idents []string) map[string]string {
	m := make(map[string]string, len(fields))
	for i, sf := range fields {
		m[sf.SignalName] = idents[i]
	}
	return m
}

// signalIdent turns a signal name into an exported Go identifier.
func signalIdent(signalName string) string {
	var b strings.Builder
	newWord := true
	for _, r := range signalName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			if newWord {
				b.WriteRune(unicode.ToUpper(r))
				newWord = false
			} else {
				b.WriteRune(r)
			}
		case r >= '0' && r <= '9':
			if b.Len() == 0 {
				continue // An identifier must not start with a digit.
			}
			b.WriteRune(r)
			newWord = false
		default:
			newWord = true
		}
	}
	if b.Len() == 0 {
		return "Signal"
	}
	s := b.String()
	// Uppercase the common trailing initialism for idiomatic Go.
	if strings.HasSuffix(s, "Id") {
		s = s[:len(s)-2] + "ID"
	}
	return s
}
