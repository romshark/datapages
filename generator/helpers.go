package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"sync"

	"github.com/romshark/datapages/parser/model"
)

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

// evSubjPrefConst returns the subject prefix constant name for per-user events.
// Returns "" for public events (no TargetUserIDs).
// "EventMessagingSent" -> "EvSubjPrefMessagingSent"
func evSubjPrefConst(e *model.Event) string {
	if !e.HasTargetUserIDs {
		return ""
	}
	return "EvSubjPref" + eventConstName(e.TypeName)
}

// evSubjValue returns the subject constant value.
// For HasTargetUserIDs: "messaging.sent.*"
// For public: "posts.archived"
func evSubjValue(e *model.Event) string {
	if e.HasTargetUserIDs {
		return e.Subject + ".*"
	}
	return e.Subject
}

// evSubjPrefValue returns the subject prefix value for per-user events.
// "messaging.sent."
func evSubjPrefValue(e *model.Event) string {
	return e.Subject + "."
}

// stripPagePrefix strips "Page" prefix from type name: "PageSettings" -> "Settings"
func stripPagePrefix(typeName string) string {
	return strings.TrimPrefix(typeName, "Page")
}

// pageNameForHref returns the function name for the href package.
// "PageIndex" -> "Index", "PageError404" -> "Error404"
func pageNameForHref(typeName string) string {
	return stripPagePrefix(typeName)
}

// pageHasStream returns true if the page has event handlers and needs a stream.
func pageHasStream(p *model.Page) bool {
	return len(p.EventHandlers) > 0
}

// pageHasAnonStream returns true if a page needs an anonymous stream endpoint.
// This happens when the page has both public (no TargetUserIDs) AND private events.
func pageHasAnonStream(p *model.Page, eventByName map[string]*model.Event) bool {
	hasPublic := false
	hasPrivate := false
	for _, eh := range p.EventHandlers {
		e, ok := eventByName[eh.EventTypeName]
		if !ok {
			continue
		}
		if e.HasTargetUserIDs {
			hasPrivate = true
		} else {
			hasPublic = true
		}
	}
	return hasPublic && hasPrivate
}

// routeStreamPath returns the SSE stream path for a page route.
// "/settings/" -> "/settings/_$/"
// "/post/{slug}/" -> "/post/{slug}/_$/"
// "/" -> "/_$/"
func routeStreamPath(route string) string {
	r := routeWithTrailingSlash(route)
	return r + "_$/"
}

// routeWithTrailingSlash strips any {$} suffix and ensures the route
// has a trailing slash.
// "/settings" -> "/settings/"
// "/user/{name}/{$}" -> "/user/{name}/"
// "/" -> "/"
func routeWithTrailingSlash(route string) string {
	route = strings.TrimSuffix(route, "{$}")
	if !strings.HasSuffix(route, "/") {
		return route + "/"
	}
	return route
}

// renderType renders a Go type using types.TypeString with a qualifier
// that maps the app package to its short name.
func renderType(t model.Type, appPkgPath string) string {
	pkg := appPkgName(appPkgPath)
	qualifier := func(p *types.Package) string {
		if p.Path() == appPkgPath {
			return pkg
		}
		return p.Name()
	}
	return types.TypeString(t.Resolved, qualifier)
}

// renderAnonStructType renders an anonymous struct type from its AST expression,
// preserving struct tags. Uses go/format.Node.
func renderAnonStructType(t model.Type, fset *token.FileSet) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, t.TypeExpr); err != nil {
		return ""
	}
	return buf.String()
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

// queryTagValue extracts the value from a `query:"value"` struct tag.
func queryTagValue(tag string) string {
	return reflect.StructTag(tag).Get("query")
}

// reflectSignalTagValue extracts the value from a `reflectsignal:"value"` struct tag.
func reflectSignalTagValue(tag string) string {
	return reflect.StructTag(tag).Get("reflectsignal")
}

// pathTagValue extracts the value from a `path:"value"` struct tag.
func pathTagValue(tag string) string {
	return reflect.StructTag(tag).Get("path")
}

// appUsage tracks which optional helpers are referenced by the generated handler code.
// It is computed once from the model before any code is emitted, and used to
// conditionally emit helper functions/methods that would otherwise be dead code.
type appUsage struct {
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
	// dsRequest: func (s *Server) checkIsDSReq(...)
	dsRequest bool
	// recover500: isDSReq called in httpErrIntern when Recover500+PageError500 exist
	recover500 bool
}

// needsIsDSReq returns true if the isDSReq helper must be emitted.
func (u appUsage) needsIsDSReq() bool {
	return u.stream || u.dsRequest || u.httpRedirect || u.recover500
}

// needsCheckIsDSReq returns true if the checkIsDSReq method must be emitted.
func (u appUsage) needsCheckIsDSReq() bool {
	return u.stream || u.dsRequest
}

// needsSetSessionCookie returns true if setSessionCookie must be emitted.
func (u appUsage) needsSetSessionCookie() bool {
	return u.auth || u.createSession || u.closeSession
}

// computeAppUsage scans the model to determine which optional helpers are needed.
func computeAppUsage(m *model.App) appUsage {
	var u appUsage

	if m.Recover500 != nil && m.PageError500 != nil {
		u.recover500 = true
	}

	checkHandler := func(h *model.Handler) {
		if h.InputSession != nil || h.InputSessionToken != nil {
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
	}

	for _, h := range m.Actions {
		checkHandler(h)
	}
	for _, p := range m.Pages {
		if p.GET != nil {
			checkHandler(p.GET.Handler)
		}
		if len(p.EventHandlers) > 0 {
			u.stream = true
			u.auth = true // stream handlers always call s.auth
		}
		for _, h := range p.Actions {
			checkHandler(h)
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
	Buf        []byte
	eventMap   map[string]*model.Event // built once per WriteApp, reused
	fields     []structFieldInfo       // reusable scratch for structFields
	prometheus bool                    // whether to generate Prometheus metrics code
	usage      appUsage                // computed once per WriteApp
}

func (w *Writer) Reset() {
	w.Buf = w.Buf[:0]
	clear(w.eventMap)
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
		w.writeEmbedInit(embed, appPkg, "\t")
	}
	w.Byte('}')
}

// writeEmbedInit recursively appends an embed field initialization.
func (w *Writer) writeEmbedInit(ap *model.AbstractPage, appPkg, indent string) {
	w.Raw(indent)
	w.Raw(ap.TypeName)
	w.Raw(": ")
	w.Raw(appPkg)
	w.Byte('.')
	w.Raw(ap.TypeName)
	w.Raw("{\n")
	w.Raw(indent)
	w.Raw("\tApp: s.app,\n")
	for _, sub := range ap.Embeds {
		w.writeEmbedInit(sub, appPkg, indent+"\t")
	}
	w.Raw(indent)
	w.Raw("},\n")
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

// isStringType returns true if the type is string.
func isStringType(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

// isIntType returns true if the type is int64, int, etc.
func isIntType(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch basic.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
		return true
	}
	return false
}

// intTypeName returns the Go identifier for an integer type (e.g. "int", "uint32").
// Precondition: isIntType(t) must be true.
func intTypeName(t types.Type) string {
	switch t.Underlying().(*types.Basic).Kind() {
	case types.Int:
		return "int"
	case types.Int8:
		return "int8"
	case types.Int16:
		return "int16"
	case types.Int32:
		return "int32"
	case types.Int64:
		return "int64"
	case types.Uint:
		return "uint"
	case types.Uint8:
		return "uint8"
	case types.Uint16:
		return "uint16"
	case types.Uint32:
		return "uint32"
	default: // Uint64
		return "uint64"
	}
}

// intTypeParseInfo returns the strconv bit-size argument and whether the type
// is unsigned, for use with strconv.ParseInt / strconv.ParseUint.
// Precondition: isIntType(t) must be true.
func intTypeParseInfo(t types.Type) (bits int, unsigned bool) {
	switch t.Underlying().(*types.Basic).Kind() {
	case types.Int:
		return 0, false
	case types.Int8:
		return 8, false
	case types.Int16:
		return 16, false
	case types.Int32:
		return 32, false
	case types.Int64:
		return 64, false
	case types.Uint:
		return 0, true
	case types.Uint8:
		return 8, true
	case types.Uint16:
		return 16, true
	case types.Uint32:
		return 32, true
	default: // Uint64
		return 64, true
	}
}

// appPkgName returns the short package name from an import path.
// "github.com/romshark/datapages/example/classifieds/app" -> "app"
func appPkgName(pkgPath string) string {
	if i := strings.LastIndex(pkgPath, "/"); i >= 0 {
		return pkgPath[i+1:]
	}
	return pkgPath
}
