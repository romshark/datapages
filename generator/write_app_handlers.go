package generator

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/romshark/datapages/parser/model"
)

// writePageGETHandler generates the GET handler for a page.
func (w *Writer) writePageGETHandler(p *model.Page, m *model.App, appPkg string) {
	funcName := "handle" + p.TypeName + "GET"
	w.Line(0, "")
	w.Linef(0, "func (s *Server) %s(w http.ResponseWriter, r *http.Request) {", funcName)

	h := p.GET.Handler

	hasBody := false

	// Auth.
	needsToken := h.InputSessionToken != nil
	if h.InputSession != nil || needsToken {
		hasBody = true
		if needsToken {
			w.Line(1, "sess, sessToken, ok := s.auth(w, r)")
		} else {
			w.Line(1, "sess, _, ok := s.auth(w, r)")
		}
		w.Line(1, "if !ok {")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Index page: 404 fallback for non-root paths.
	if p.PageSpecialization == model.PageTypeIndex {
		hasBody = true
		w.Line(0, "")
		w.Line(1, `if r.URL.Path != "/" {`)
		w.Line(2, "s.render404(w, r)")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Read query params.
	if h.InputQuery != nil {
		hasBody = true
		w.writeReadQuery(h.InputQuery, m)
	}

	// Read path params.
	if h.InputPath != nil {
		hasBody = true
		w.writeReadPath(h.InputPath, m)
	}

	// Page constructor.
	if hasBody {
		w.Buf = append(w.Buf, "\n\tp := "...)
	} else {
		w.Buf = append(w.Buf, "\tp := "...)
	}
	w.writePageConstructor(p, appPkg)
	w.Buf = append(w.Buf, '\n')

	// Call GET.
	w.writeGETMethodCall(p, m, appPkg)

	w.Line(0, "}")
}

func (w *Writer) writeGETMethodCall(p *model.Page, m *model.App, appPkg string) {
	h := p.GET.Handler

	// Build output list.
	var outs []string
	if p.GET.OutputBody != nil {
		outs = append(outs, p.GET.OutputBody.Name)
	}
	if p.GET.OutputHead != nil {
		outs = append(outs, p.GET.OutputHead.Name)
	}
	if h.OutputRedirect != nil {
		outs = append(outs, h.OutputRedirect.Name)
	}
	if h.OutputRedirectStatus != nil {
		outs = append(outs, h.OutputRedirectStatus.Name)
	}
	if h.OutputEnableBgStream != nil {
		outs = append(outs, h.OutputEnableBgStream.Name)
	}
	if h.OutputDisableRefresh != nil {
		outs = append(outs, h.OutputDisableRefresh.Name)
	}
	if h.OutputErr != nil {
		outs = append(outs, "err")
	}

	// Build input args.
	var args []string
	if h.InputRequest != nil {
		args = append(args, "r")
	}
	if h.InputSession != nil {
		args = append(args, "sess")
	}
	if h.InputPath != nil {
		args = append(args, "path")
	}
	if h.InputQuery != nil {
		args = append(args, "query")
	}

	w.Linef(1, "%s := p.GET(%s)", strings.Join(outs, ", "), strings.Join(args, ", "))

	if h.OutputErr != nil {
		w.Line(1, "if err != nil {")
		w.Linef(2, `s.httpErrIntern(w, r, nil, "handling %s.GET", err)`, p.TypeName)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Redirect.
	if h.OutputRedirect != nil {
		statusArg := "0"
		if h.OutputRedirectStatus != nil {
			statusArg = h.OutputRedirectStatus.Name
		}
		w.Linef(1, "if httpRedirect(w, r, %s, %s) {", h.OutputRedirect.Name, statusArg)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Generic head.
	if m.GlobalHeadGenerator != nil {
		w.Line(1, "genericHead, err := s.app.Head(r)")
		w.Line(1, "if err != nil {")
		w.Linef(2, `s.httpErrIntern(w, r, nil, "generating generic head for %s", err)`,
			p.TypeName)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Body attrs.
	w.writeGETBodyAttrs(p, m)

	// writeHTML call.
	sessArg := "sess"
	if p.PageSpecialization == model.PageTypeError500 || !hasSessionInput(h) {
		sessArg = appPkg + ".Session{}"
	}

	genericHeadArg := "nil"
	if m.GlobalHeadGenerator != nil {
		genericHeadArg = "genericHead"
	}

	headArg := "nil"
	if p.GET.OutputHead != nil {
		headArg = p.GET.OutputHead.Name
	}

	bodyName := "body"
	if p.GET.OutputBody != nil {
		bodyName = p.GET.OutputBody.Name
	}

	w.Line(0, "")
	w.Line(1, "if err := s.writeHTML(")
	w.Linef(2, "w, r, %s, %s, %s, %s, bodyAttrs,",
		sessArg, genericHeadArg, headArg, bodyName)
	w.Line(1, "); err != nil {")
	w.Linef(2, `s.logErr("rendering %s", err)`, p.TypeName)
	w.Line(2, "return")
	w.Line(1, "}")
}

func hasSessionInput(h *model.Handler) bool {
	return h.InputSession != nil
}

func (w *Writer) writeGETBodyAttrs(p *model.Page, m *model.App) {
	h := p.GET.Handler
	eventMap := buildEventMap(m.Events)

	hasDisableRefresh := h.OutputDisableRefresh != nil
	hasEnableBgStream := h.OutputEnableBgStream != nil
	hasStream := pageHasStream(p)
	hasAnonStream := pageHasAnonStream(p, eventMap)

	var reflectFields []reflectSignalField
	if h.InputQuery != nil {
		fields := structFields(h.InputQuery.Type.Resolved)
		for _, f := range fields {
			rs := reflectSignalTagValue(f.Tag)
			if rs != "" {
				reflectFields = append(reflectFields, reflectSignalField{
					SignalName: rs,
					FieldName:  f.Name,
					Type:       f.Type,
					QueryTag:   queryTagValue(f.Tag),
				})
			}
		}
	}
	hasReflectSignals := len(reflectFields) > 0

	w.Line(0, "")
	w.Line(1, "bodyAttrs := func(w http.ResponseWriter) {")

	if hasDisableRefresh {
		w.Linef(2, "if !%s {", h.OutputDisableRefresh.Name)
		w.Line(3, "writeBodyAttrOnVisibilityChange(w)")
		w.Line(2, "}")
	} else if hasEnableBgStream {
		w.Linef(2, "if !%s {", h.OutputEnableBgStream.Name)
		w.Line(3, "writeBodyAttrOnVisibilityChange(w)")
		w.Line(2, "}")
	} else {
		w.Line(2, "writeBodyAttrOnVisibilityChange(w)")
	}

	// Reflect signal attrs.
	for _, f := range reflectFields {
		if isStringType(f.Type) {
			w.Line(0, "")
			w.Linef(2, "_, _ = io.WriteString(w, `data-signals:%s=\"'`)", f.SignalName)
			w.Linef(2, "_, _ = io.WriteString(w, query.%s)", f.FieldName)
			w.Line(2, "_, _ = io.WriteString(w, `'\"`)")
		} else if isIntType(f.Type) {
			w.Line(0, "")
			w.Linef(2, "_, _ = io.WriteString(w, `data-signals:%s=\"`)", f.SignalName)
			w.Linef(2, "_, _ = io.WriteString(w, strconv.FormatInt(query.%s, 10))",
				f.FieldName)
			w.Line(2, "_, _ = io.WriteString(w, `\"`)")
		}
	}

	// Stream data-init attr.
	if hasStream && h.InputSession != nil {
		streamPath := routeStreamPath(p.Route)
		if hasAnonStream {
			// Mixed: auth → /_$/ , anon → /_$/anon/
			// Need to handle path variables.
			pathVars := routeVars(p.Route)
			if len(pathVars) > 0 {
				// Dynamic path.
				w.Line(0, "")
				w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
				w.writeStreamPathSegments(p.Route, pathVars)
				w.Line(2, `if sess.UserID != "" {`)
				w.Line(3, "_, _ = io.WriteString(w, `/_$/')\"`)")
				w.Line(2, "} else {")
				w.Line(3, "_, _ = io.WriteString(w, `/_$/anon/')\"`)")
				w.Line(2, "}")
			} else {
				w.Line(0, "")
				w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
				w.Line(2, `if sess.UserID != "" {`)
				w.Linef(3, "_, _ = io.WriteString(w, `%s')\"`)", streamPath)
				w.Line(2, "} else {")
				w.Linef(3, "_, _ = io.WriteString(w, `%sanon/')\"`)", streamPath)
				w.Line(2, "}")
			}
		} else {
			// Auth-only stream.
			if hasEnableBgStream {
				w.Line(0, "")
				w.Line(2, `if sess.UserID != "" {`)
				w.Linef(3, "_, _ = io.WriteString(w, `data-init=\"@get('%s'`)",
					streamPath)
				w.Linef(3, "if %s {", h.OutputEnableBgStream.Name)
				w.Line(4, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
				w.Line(3, "} else {")
				w.Line(4, "_, _ = io.WriteString(w, `)\"`)")
				w.Line(3, "}")
				w.Line(2, "}")
			} else {
				w.Line(0, "")
				w.Line(2, `if sess.UserID != "" {`)
				w.Linef(3, "_, _ = io.WriteString(w, `data-init=\"@get('%s')\"`)",
					streamPath)
				w.Line(2, "}")
			}
		}
	}

	// data-effect for URL sync with reflect signals.
	if hasReflectSignals {
		route := strings.TrimSuffix(p.Route, "{$}")
		route = strings.TrimSuffix(route, "/")
		if route == "" {
			route = "/"
		}

		w.Line(0, "")
		w.Line(2, "_, _ = io.WriteString(w, `data-effect=\"const params = new URLSearchParams();")
		for _, f := range reflectFields {
			if isStringType(f.Type) {
				w.Linef(3, "if ($%s) params.set('%s', $%s);",
					f.SignalName, f.QueryTag, f.SignalName)
			} else if isIntType(f.Type) {
				w.Linef(3, "if ($%s) params.set('%s', $%s);",
					f.SignalName, f.QueryTag, f.SignalName)
			}
		}
		w.Linef(3, "const query = params.toString();")
		w.Linef(3, "window.history.replaceState(null, '', query ? '%s?' + query : '%s');",
			route, route)
		w.Line(2, "\"`)")
	}

	w.Line(1, "}")
}

func (w *Writer) writeStreamPathSegments(route string, pathVars []string) {
	// Build the path prefix up to the variable, then write the variable.
	literals, _ := routeSegments(route)
	for i, lit := range literals {
		w.Linef(2, "_, _ = io.WriteString(w, `%s`)", lit)
		if i < len(pathVars) {
			w.Linef(2, "_, _ = io.WriteString(w, path.%s)",
				strings.ToUpper(pathVars[i][:1])+pathVars[i][1:])
		}
	}
}

// writePageGETStreamHandler generates the authenticated stream handler for a page.
func (w *Writer) writePageGETStreamHandler(
	p *model.Page, m *model.App, appPkg string, eventMap map[string]*model.Event,
) {
	funcName := "handle" + p.TypeName + "GETStream"
	w.Line(0, "")
	w.Linef(0, "func (s *Server) %s(w http.ResponseWriter, r *http.Request) {", funcName)

	w.Line(1, "if !s.checkIsDSReq(w, r) {")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "sess, sessToken, ok := s.auth(w, r)")
	w.Line(1, "if !ok {")
	w.Line(2, "return")
	w.Line(1, "}")

	// Check if anon stream exists - if so, redirect unauthenticated to anon.
	hasAnon := pageHasAnonStream(p, eventMap)
	if hasAnon {
		w.Line(0, "")
		w.Line(1, `if sess.UserID == "" {`)
		w.Line(2, `http.Redirect(w, r, r.URL.Path+"/anon", http.StatusSeeOther)`)
		w.Line(2, "return")
		w.Line(1, "}")
	} else {
		w.Line(0, "")
		w.Line(1, `if sess.UserID == "" {`)
		w.Line(2, "http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Read signals if any event handler takes signals.
	hasStreamSignals := false
	for _, eh := range p.EventHandlers {
		if eh.InputSignals != nil {
			hasStreamSignals = true
			break
		}
	}
	if hasStreamSignals {
		// Find the signals type from the first event handler that has it.
		for _, eh := range p.EventHandlers {
			if eh.InputSignals != nil {
				sigType := renderSignalsType(eh.InputSignals, m)
				w.Line(0, "")
				w.Linef(1, "var signals %s", sigType)
				w.Line(1, "if err := datastar.ReadSignals(r, &signals); err != nil {")
				w.Line(2, `s.httpErrBad(w, "reading signals", err)`)
				w.Line(2, "return")
				w.Line(1, "}")
				break
			}
		}
	}

	// Page constructor.
	w.Buf = append(w.Buf, "\n\tp := "...)
	w.writePageConstructor(p, appPkg)
	w.Buf = append(w.Buf, '\n')

	// evSubj call.
	evSubjName := "evSubj" + p.TypeName
	// Determine if the evSubj function takes a userID.
	hasPrivate := false
	for _, eh := range p.EventHandlers {
		ev := eventMap[eh.EventTypeName]
		if ev != nil && ev.HasTargetUserIDs {
			hasPrivate = true
			break
		}
	}
	if hasPrivate {
		w.Linef(1, "s.handleStreamRequest(w, r, sessToken, sess, %s(sess.UserID), func(",
			evSubjName)
	} else {
		w.Linef(1, "s.handleStreamRequest(w, r, sessToken, sess, %s(), func(", evSubjName)
	}
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan msgbroker.Message,")
	w.Line(1, ") {")
	w.Line(2, "for msg := range ch {")
	w.Line(3, "switch {")

	// Generate case for each event handler.
	for _, eh := range p.EventHandlers {
		ev := eventMap[eh.EventTypeName]
		if ev == nil {
			continue
		}
		w.writeStreamEventCase(p, eh, ev, appPkg)
	}

	w.Line(3, "}")
	w.Line(2, "}")
	w.Line(1, "})")
	w.Line(0, "}")
}

func (w *Writer) writeStreamEventCase(
	p *model.Page, eh *model.EventHandler, ev *model.Event, appPkg string,
) {
	constName := eventConstName(ev.TypeName)

	if ev.HasTargetUserIDs {
		w.Linef(3, "case strings.HasPrefix(msg.Subject, EvSubjPref%s):", constName)
	} else {
		w.Linef(3, "case msg.Subject == EvSubj%s:", constName)
	}

	w.Linef(4, "var e %s.%s", appPkg, ev.TypeName)
	w.Line(4, "if err := json.Unmarshal(msg.Data, &e); err != nil {")
	w.Linef(5, `s.logErr("unmarshaling %s JSON", err)`, ev.TypeName)
	w.Line(5, "continue")
	w.Line(4, "}")

	w.writeEventHandlerCall(p.TypeName, eh, "p")
}

func (w *Writer) writeEventHandlerCall(
	ownerLabel string, eh *model.EventHandler, receiver string,
) {
	// Build args.
	var args []string
	if eh.InputEvent != nil {
		args = append(args, "e")
	}
	if eh.InputSSE != nil {
		args = append(args, "sse")
	}
	if eh.InputSessionToken != nil {
		args = append(args, "sessToken")
	}
	if eh.InputSession != nil {
		args = append(args, "sess")
	}
	if eh.InputSignals != nil {
		args = append(args, "signals")
	}

	methodName := "On" + eh.Name
	logLabel := fmt.Sprintf("%s.%s", ownerLabel, methodName)

	call := fmt.Sprintf("%s.%s(%s)", receiver, methodName, strings.Join(args, ", "))

	if eh.OutputErr != nil {
		w.Linef(4, "if err := %s; err != nil {", call)
		w.Linef(5, `s.logErr("handling %s", err)`, logLabel)
		w.Line(4, "}")
	} else {
		w.Linef(4, "%s", call)
	}
}

// writePageGETStreamAnonHandler generates the anonymous stream handler for a page.
func (w *Writer) writePageGETStreamAnonHandler(
	p *model.Page, appPkg string, eventMap map[string]*model.Event,
) {
	funcName := "handle" + p.TypeName + "GETStreamAnon"
	w.Line(0, "")
	w.Linef(0, "func (s *Server) %s(w http.ResponseWriter, r *http.Request) {", funcName)

	w.Line(1, "if !s.checkIsDSReq(w, r) {")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "sess, sessToken, ok := s.auth(w, r)")
	w.Line(1, "if !ok {")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(0, "")
	w.Line(1, `if sess.UserID != "" {`)
	w.Line(2, `s.httpErrBad(w, "authenticated client on anonymous stream", nil)`)
	w.Line(2, "return")
	w.Line(1, "}")

	// Page constructor.
	w.Buf = append(w.Buf, "\n\tp := "...)
	w.writePageConstructor(p, appPkg)
	w.Buf = append(w.Buf, '\n')

	// evSubj call (for anon, pass empty userID to get public-only subjects).
	evSubjName := "evSubj" + p.TypeName
	w.Linef(1, "s.handleStreamRequest(w, r, sessToken, sess, %s(sess.UserID), func(",
		evSubjName)
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan msgbroker.Message,")
	w.Line(1, ") {")
	w.Line(2, "for msg := range ch {")

	// Only handle public events in anon stream.
	publicHandlers := 0
	for _, eh := range p.EventHandlers {
		ev := eventMap[eh.EventTypeName]
		if ev != nil && !ev.HasTargetUserIDs {
			publicHandlers++
		}
	}

	if publicHandlers == 1 {
		// Single event: use if instead of switch.
		for _, eh := range p.EventHandlers {
			ev := eventMap[eh.EventTypeName]
			if ev == nil || ev.HasTargetUserIDs {
				continue
			}
			constName := eventConstName(ev.TypeName)
			w.Linef(3, "if msg.Subject == EvSubj%s {", constName)
			w.Linef(4, "var e %s.%s", appPkg, ev.TypeName)
			w.Line(4, "if err := json.Unmarshal(msg.Data, &e); err != nil {")
			w.Linef(5, `s.logErr("unmarshaling %s JSON", err)`, ev.TypeName)
			w.Line(5, "continue")
			w.Line(4, "}")
			w.writeEventHandlerCall(p.TypeName, eh, "p")
			w.Line(3, "}")
		}
	} else {
		w.Line(3, "switch {")
		for _, eh := range p.EventHandlers {
			ev := eventMap[eh.EventTypeName]
			if ev == nil || ev.HasTargetUserIDs {
				continue
			}
			w.writeStreamEventCase(p, eh, ev, appPkg)
		}
		w.Line(3, "}")
	}

	w.Line(2, "}")
	w.Line(1, "})")
	w.Line(0, "}")
}

// writePageActionHandler generates a page-level action handler.
func (w *Writer) writePageActionHandler(
	p *model.Page, h *model.Handler, m *model.App, appPkg string,
) {
	funcName := "handle" + p.TypeName + strings.ToUpper(h.HTTPMethod) + h.Name
	w.Line(0, "")
	w.Linef(0, "func (s *Server) %s(", funcName)
	w.Line(1, "w http.ResponseWriter, r *http.Request,")
	w.Line(0, ") {")

	if h.InputSSE != nil || h.InputSignals != nil {
		w.Line(1, "if !s.checkIsDSReq(w, r) {")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Auth.
	needsToken := h.InputSessionToken != nil || h.OutputCloseSession != nil
	if h.InputSession != nil || needsToken {
		if needsToken {
			w.Line(1, "sess, sessToken, ok := s.auth(w, r)")
		} else {
			w.Line(1, "sess, _, ok := s.auth(w, r)")
		}
		w.Line(1, "if !ok {")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Body size limit.
	if h.InputSignals != nil && h.InputSSE == nil {
		w.Line(1, "r.Body = http.MaxBytesReader(w, r.Body, DefaultBodySizeLimit)")
	}

	// Read signals.
	if h.InputSignals != nil {
		sigType := renderSignalsType(h.InputSignals, m)
		w.Linef(1, "var signals %s", sigType)
		w.Line(1, "if err := datastar.ReadSignals(r, &signals); err != nil {")
		w.Line(2, `s.httpErrBad(w, "reading signals", err)`)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Read query params.
	if h.InputQuery != nil {
		w.writeReadQuery(h.InputQuery, m)
	}

	// Read path params.
	if h.InputPath != nil {
		w.writeReadPath(h.InputPath, m)
	}

	// Dispatch closure.
	if h.InputDispatch != nil {
		w.writeDispatchClosure(h.InputDispatch, m, appPkg)
	}

	// SSE for actions that take it.
	if h.InputSSE != nil {
		w.Line(0, "")
		w.Line(1, "sse := datastar.NewSSE(w, r, datastar.WithCompression())")
	}

	// Page constructor.
	w.Buf = append(w.Buf, "\tp := "...)
	w.writePageConstructor(p, appPkg)
	w.Buf = append(w.Buf, '\n')

	// Build the method call.
	w.writeActionMethodCall(p, h, m, appPkg)

	w.Line(0, "}")
}

func (w *Writer) writeActionMethodCall(
	p *model.Page, h *model.Handler, m *model.App, appPkg string,
) {
	// Build output list.
	var outs []string
	if h.OutputBody != nil {
		outs = append(outs, h.OutputBody.Name)
	}
	if h.OutputCloseSession != nil {
		outs = append(outs, h.OutputCloseSession.Name)
	}
	if h.OutputRedirect != nil {
		outs = append(outs, h.OutputRedirect.Name)
	}
	if h.OutputRedirectStatus != nil {
		outs = append(outs, h.OutputRedirectStatus.Name)
	}
	if h.OutputNewSession != nil {
		outs = append(outs, h.OutputNewSession.Name)
	}
	if h.OutputErr != nil {
		outs = append(outs, "err")
	}

	// Build input args.
	var args []string
	if h.InputRequest != nil {
		args = append(args, "r")
	}
	if h.InputSSE != nil {
		args = append(args, "sse")
	}
	if h.InputSessionToken != nil {
		args = append(args, "sessToken")
	}
	if h.InputSession != nil {
		args = append(args, "sess")
	}
	if h.InputPath != nil {
		args = append(args, "path")
	}
	if h.InputQuery != nil {
		args = append(args, "query")
	}
	if h.InputSignals != nil {
		args = append(args, "signals")
	}
	if h.InputDispatch != nil {
		args = append(args, "dispatch")
	}

	methodName := h.HTTPMethod + h.Name
	callExpr := fmt.Sprintf("p.%s(%s)", methodName, strings.Join(args, ", "))

	if len(outs) == 0 {
		if h.OutputErr != nil {
			sseRef := "nil"
			if h.InputSSE != nil {
				sseRef = "sse"
			}
			w.Linef(1, "if err := %s; err != nil {", callExpr)
			w.Linef(2, `s.httpErrIntern(w, r, %s, "handling action %s.%s", err)`,
				sseRef, p.TypeName, h.Name)
			w.Line(2, "return")
			w.Line(1, "}")
		} else {
			w.Linef(1, "%s", callExpr)
		}
		return
	}

	w.Linef(1, "%s := %s", strings.Join(outs, ", "), callExpr)

	if h.OutputErr != nil {
		sseRef := "nil"
		if h.InputSSE != nil {
			sseRef = "sse"
		}
		w.Line(1, "if err != nil {")
		w.Linef(2, `s.httpErrIntern(w, r, %s, "handling action %s.%s", err)`,
			sseRef, p.TypeName, h.Name)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Close session.
	if h.OutputCloseSession != nil {
		w.Linef(1, "if %s {", h.OutputCloseSession.Name)
		w.Line(2, "if err := s.closeSession(w, r, sessToken); err != nil {")
		w.Line(3, `s.httpErrIntern(w, r, nil, "removing session", err)`)
		w.Line(3, "return")
		w.Line(2, "}")
		w.Line(1, "}")
	}

	// New session.
	if h.OutputNewSession != nil {
		w.Linef(1, `if j := %s; j.UserID != "" {`, h.OutputNewSession.Name)
		w.Linef(2, "if err := s.createSession(w, r, %s); err != nil {",
			h.OutputNewSession.Name)
		w.Line(3, `s.httpErrIntern(w, r, nil, "creating session", err)`)
		w.Line(2, "}")
		w.Line(1, "}")
	}

	// Redirect.
	if h.OutputRedirect != nil {
		statusArg := "0"
		if h.OutputRedirectStatus != nil {
			statusArg = h.OutputRedirectStatus.Name
		}
		w.Linef(1, "if httpRedirect(w, r, %s, %s) {", h.OutputRedirect.Name, statusArg)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Render body (if action returns templ.Component).
	if h.OutputBody != nil {
		if m.GlobalHeadGenerator != nil {
			w.Line(1, "genericHead, err := s.app.Head(r)")
			w.Line(1, "if err != nil {")
			w.Linef(2, `s.httpErrIntern(w, r, nil, "generating generic head for %s.%s%s", err)`,
				p.TypeName, h.HTTPMethod, h.Name)
			w.Line(2, "return")
			w.Line(1, "}")
		}
		genericHeadArg := "nil"
		if m.GlobalHeadGenerator != nil {
			genericHeadArg = "genericHead"
		}
		sessArg := "sess"
		if !hasSessionInput(h) {
			sessArg = appPkg + ".Session{}"
		}
		w.Line(1, "if err := s.writeHTML(")
		w.Linef(2, "w, r, %s, %s, nil, %s, nil,",
			sessArg, genericHeadArg, h.OutputBody.Name)
		w.Line(1, "); err != nil {")
		w.Linef(2, `s.logErr("rendering response of %s.%s%s", err)`,
			p.TypeName, h.HTTPMethod, h.Name)
		w.Line(2, "return")
		w.Line(1, "}")
	}
}

func (w *Writer) writeReadQuery(input *model.Input, m *model.App) {
	queryType := renderQueryType(input, m)
	w.Line(0, "")
	w.Line(1, "q := r.URL.Query()")
	w.Linef(1, "var query %s", queryType)
	fields := structFields(input.Type.Resolved)
	for _, f := range fields {
		tag := queryTagValue(f.Tag)
		if isIntType(f.Type) {
			w.Line(1, "{")
			w.Linef(2, "if q := q.Get(%q); q != \"\" {", tag)
			w.Line(3, "i, err := strconv.ParseInt(q, 10, 64)")
			w.Line(3, "if err != nil {")
			w.Linef(4,
				"s.httpErrBad(w, \"unexpected value for query parameter: %s\", err)",
				tag)
			w.Line(4, "return")
			w.Line(3, "}")
			w.Linef(3, "query.%s = i", f.Name)
			w.Line(2, "}")
			w.Line(1, "}")
		} else {
			w.Linef(1, "query.%s = q.Get(%q)", f.Name, tag)
		}
	}
}

func (w *Writer) writeReadPath(input *model.Input, m *model.App) {
	w.Line(0, "")
	pathType := renderPathType(input, m)
	w.Linef(1, "var path %s", pathType)
	fields := structFields(input.Type.Resolved)
	for _, f := range fields {
		tag := pathTagValue(f.Tag)
		w.Linef(1, "path.%s = r.PathValue(%q)", f.Name, tag)
	}
}

type reflectSignalField struct {
	SignalName string
	FieldName  string
	Type       types.Type
	QueryTag   string
}
