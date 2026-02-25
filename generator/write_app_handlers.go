package generator

import (
	"go/types"
	"strings"

	"github.com/romshark/datapages/parser/model"
)

// writePageGETHandler generates the GET handler for a page.
func (w *Writer) writePageGETHandler(p *model.Page, m *model.App, appPkg string) {
	w.Line(0, "")
	w.Raw("func (s *Server) handle")
	w.Raw(p.TypeName)
	w.Raw("GET(w http.ResponseWriter, r *http.Request) {\n")

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
		w.Raw("\n\tp := ")
	} else {
		w.Raw("\tp := ")
	}
	w.writePageConstructor(p, appPkg)
	w.Byte('\n')

	// Call GET.
	w.writeGETMethodCall(p, m, appPkg)

	w.Line(0, "}")
}

func (w *Writer) writeGETMethodCall(p *model.Page, m *model.App, appPkg string) {
	h := p.GET.Handler

	// Build output list.
	var outsBuf [8]string
	outs := outsBuf[:0]
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
	var argsBuf [8]string
	args := argsBuf[:0]
	if h.InputRequest != nil {
		args = append(args, "r")
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

	w.Byte('\t')
	w.writeCommaSep(outs)
	w.Raw(" := ")
	w.writeCallExpr("p", "GET", args)
	w.Byte('\n')

	if h.OutputErr != nil {
		w.Line(1, "if err != nil {")
		w.Raw("\t\ts.httpErrIntern(w, r, nil, \"handling ")
		w.Raw(p.TypeName)
		w.Raw(".GET\", err)\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Redirect.
	if h.OutputRedirect != nil {
		statusArg := "0"
		if h.OutputRedirectStatus != nil {
			statusArg = h.OutputRedirectStatus.Name
		}
		w.Raw("\tif httpRedirect(w, r, ")
		w.Raw(h.OutputRedirect.Name)
		w.Raw(", ")
		w.Raw(statusArg)
		w.Raw(") {\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Generic head.
	if m.GlobalHeadGenerator != nil {
		w.Line(1, "genericHead, err := s.app.Head(r)")
		w.Line(1, "if err != nil {")
		w.Raw("\t\ts.httpErrIntern(w, r, nil, \"generating generic head for ")
		w.Raw(p.TypeName)
		w.Raw("\", err)\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Body attrs.
	w.writeGETBodyAttrs(p)

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
	w.Raw("\t\tw, r, ")
	w.Raw(sessArg)
	w.Raw(", ")
	w.Raw(genericHeadArg)
	w.Raw(", ")
	w.Raw(headArg)
	w.Raw(", ")
	w.Raw(bodyName)
	w.Raw(", bodyAttrs,\n")
	w.Line(1, "); err != nil {")
	w.Raw("\t\ts.logErr(\"rendering ")
	w.Raw(p.TypeName)
	w.Raw("\", err)\n")
	w.Line(2, "return")
	w.Line(1, "}")
}

func hasSessionInput(h *model.Handler) bool {
	return h.InputSession != nil
}

func (w *Writer) writeGETBodyAttrs(p *model.Page) {
	h := p.GET.Handler

	hasDisableRefresh := h.OutputDisableRefresh != nil
	hasEnableBgStream := h.OutputEnableBgStream != nil
	hasStream := pageHasStream(p)
	hasAnonStream := pageHasAnonStream(p, w.eventMap)

	var reflectFields []reflectSignalField
	if h.InputQuery != nil {
		fields := w.structFields(h.InputQuery.Type.Resolved)
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
		w.Raw("\t\tif !")
		w.Raw(h.OutputDisableRefresh.Name)
		w.Raw(" {\n")
		w.Line(3, "writeBodyAttrOnVisibilityChange(w)")
		w.Line(2, "}")
	} else if hasEnableBgStream {
		w.Raw("\t\tif !")
		w.Raw(h.OutputEnableBgStream.Name)
		w.Raw(" {\n")
		w.Line(3, "writeBodyAttrOnVisibilityChange(w)")
		w.Line(2, "}")
	} else {
		w.Line(2, "writeBodyAttrOnVisibilityChange(w)")
	}

	// Reflect signal attrs.
	for _, f := range reflectFields {
		if isStringType(f.Type) {
			w.Line(0, "")
			w.Raw("\t\t_, _ = io.WriteString(w, `data-signals:")
			w.Raw(f.SignalName)
			w.Raw("=\"'`)\n")
			w.Raw("\t\t_, _ = io.WriteString(w, query.")
			w.Raw(f.FieldName)
			w.Raw(")\n")
			w.Line(2, "_, _ = io.WriteString(w, `'\"`)")
		} else if isIntType(f.Type) {
			_, unsigned := intTypeParseInfo(f.Type)
			typeName := intTypeName(f.Type)
			w.Line(0, "")
			w.Raw("\t\t_, _ = io.WriteString(w, `data-signals:")
			w.Raw(f.SignalName)
			w.Raw("=\"`)\n")
			w.Raw("\t\t_, _ = io.WriteString(w, ")
			if unsigned {
				if typeName != "uint64" {
					w.Raw("strconv.FormatUint(uint64(query.")
				} else {
					w.Raw("strconv.FormatUint(query.")
				}
			} else {
				if typeName != "int64" {
					w.Raw("strconv.FormatInt(int64(query.")
				} else {
					w.Raw("strconv.FormatInt(query.")
				}
			}
			w.Raw(f.FieldName)
			w.Raw(", 10))\n")
			w.Line(2, "_, _ = io.WriteString(w, `\"`)")
		}
	}

	// Stream data-init attr.
	if hasStream && h.InputSession != nil {
		streamPath := routeStreamPath(p.Route)
		if hasAnonStream {
			// Mixed: auth -> /_$/ , anon -> /_$/anon/
			// Need to handle path variables.
			if h.InputPath != nil {
				// Dynamic path.
				w.Line(0, "")
				w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
				w.writeStreamPathSegments(p.Route, h.InputPath)
				w.Line(2, `if sess.UserID != "" {`)
				w.Line(3, "_, _ = io.WriteString(w, `/_$/')\"`)")
				w.Line(2, "} else {")
				w.Line(3, "_, _ = io.WriteString(w, `/_$/anon/')\"`)")
				w.Line(2, "}")
			} else {
				w.Line(0, "")
				w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
				w.Line(2, `if sess.UserID != "" {`)
				w.Raw("\t\t\t_, _ = io.WriteString(w, `")
				w.Raw(streamPath)
				w.Raw("')\"`)\n")
				w.Line(2, "} else {")
				w.Raw("\t\t\t_, _ = io.WriteString(w, `")
				w.Raw(streamPath)
				w.Raw("anon/')\"`)\n")
				w.Line(2, "}")
			}
		} else {
			// Auth-only stream.
			if hasEnableBgStream {
				w.Line(0, "")
				w.Line(2, `if sess.UserID != "" {`)
				w.Raw("\t\t\t_, _ = io.WriteString(w, `data-init=\"@get('")
				w.Raw(streamPath)
				w.Raw("'`)\n")
				w.Raw("\t\t\tif ")
				w.Raw(h.OutputEnableBgStream.Name)
				w.Raw(" {\n")
				w.Line(4, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
				w.Line(3, "} else {")
				w.Line(4, "_, _ = io.WriteString(w, `)\"`)")
				w.Line(3, "}")
				w.Line(2, "}")
			} else {
				w.Line(0, "")
				w.Line(2, `if sess.UserID != "" {`)
				w.Raw("\t\t\t_, _ = io.WriteString(w, `data-init=\"@get('")
				w.Raw(streamPath)
				w.Raw("')\"`)\n")
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
			w.Raw("\t\t\tif ($")
			w.Raw(f.SignalName)
			w.Raw(") params.set('")
			w.Raw(f.QueryTag)
			w.Raw("', $")
			w.Raw(f.SignalName)
			w.Raw(");\n")
		}
		w.Line(3, "const query = params.toString();")
		w.Raw("\t\t\twindow.history.replaceState(null, '', query ? '")
		w.Raw(route)
		w.Raw("?' + query : '")
		w.Raw(route)
		w.Raw("');\n")
		w.Line(2, "\"`)")
	}

	w.Line(1, "}")
}

func (w *Writer) writeStreamPathSegments(route string, pathInput *model.Input) {
	// Build a map from path: tag value to Go field name.
	fields := w.structFields(pathInput.Type.Resolved)
	tagToField := make(map[string]string, len(fields))
	for _, f := range fields {
		if tag := pathTagValue(f.Tag); tag != "" {
			tagToField[tag] = f.Name
		}
	}
	// Build the path prefix up to the variable, then write the variable.
	literals, vars := routeSegments(route)
	for i, lit := range literals {
		w.Raw("\t\t_, _ = io.WriteString(w, `")
		w.Raw(lit)
		w.Raw("`)\n")
		if i < len(vars) {
			w.Raw("\t\t_, _ = io.WriteString(w, path.")
			w.Raw(tagToField[vars[i]])
			w.Raw(")\n")
		}
	}
}

// writePageGETStreamHandler generates the authenticated stream handler for a page.
func (w *Writer) writePageGETStreamHandler(
	p *model.Page, m *model.App, appPkg string,
) {
	w.Line(0, "")
	w.Raw("func (s *Server) handle")
	w.Raw(p.TypeName)
	w.Raw("GETStream(w http.ResponseWriter, r *http.Request) {\n")

	w.Line(1, "if !s.checkIsDSReq(w, r) {")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "sess, sessToken, ok := s.auth(w, r)")
	w.Line(1, "if !ok {")
	w.Line(2, "return")
	w.Line(1, "}")

	// Check if anon stream exists - if so, redirect unauthenticated to anon.
	hasAnon := pageHasAnonStream(p, w.eventMap)
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
				w.Line(0, "")
				w.Raw("\tvar signals ")
				w.Raw(renderSignalsType(eh.InputSignals, m))
				w.Byte('\n')
				w.Line(1, "if err := datastar.ReadSignals(r, &signals); err != nil {")
				w.Line(2, `s.httpErrBad(w, "reading signals", err)`)
				w.Line(2, "return")
				w.Line(1, "}")
				break
			}
		}
	}

	// Page constructor.
	w.Raw("\n\tp := ")
	w.writePageConstructor(p, appPkg)
	w.Byte('\n')

	// evSubj call.
	evSubjName := "evSubj" + p.TypeName
	// Determine if the evSubj function takes a userID.
	hasPrivate := false
	for _, eh := range p.EventHandlers {
		ev := w.eventMap[eh.EventTypeName]
		if ev != nil && ev.HasTargetUserIDs {
			hasPrivate = true
			break
		}
	}
	w.Raw("\ts.handleStreamRequest(w, r, sessToken, sess, ")
	w.Raw(evSubjName)
	if hasPrivate {
		w.Raw("(sess.UserID), func(\n")
	} else {
		w.Raw("(), func(\n")
	}
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan msgbroker.Message,")
	w.Line(1, ") {")
	w.Line(2, "for msg := range ch {")
	w.Line(3, "switch {")

	// Generate case for each event handler.
	for _, eh := range p.EventHandlers {
		ev := w.eventMap[eh.EventTypeName]
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
		w.Raw("\t\t\tcase strings.HasPrefix(msg.Subject, EvSubjPref")
		w.Raw(constName)
		w.Raw("):\n")
	} else {
		w.Raw("\t\t\tcase msg.Subject == EvSubj")
		w.Raw(constName)
		w.Raw(":\n")
	}

	w.Raw("\t\t\t\tvar e ")
	w.Raw(appPkg)
	w.Byte('.')
	w.Raw(ev.TypeName)
	w.Byte('\n')
	w.Line(4, "if err := json.Unmarshal(msg.Data, &e); err != nil {")
	w.Raw("\t\t\t\t\ts.logErr(\"unmarshaling ")
	w.Raw(ev.TypeName)
	w.Raw(" JSON\", err)\n")
	w.Line(5, "continue")
	w.Line(4, "}")

	w.writeEventHandlerCall(p.TypeName, eh, "p")
}

func (w *Writer) writeEventHandlerCall(
	ownerLabel string, eh *model.EventHandler, receiver string,
) {
	// Build args.
	var argsBuf [8]string
	args := argsBuf[:0]
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

	if eh.OutputErr != nil {
		w.Raw("\t\t\t\tif err := ")
		w.writeCallExpr(receiver, methodName, args)
		w.Raw("; err != nil {\n")
		w.Raw("\t\t\t\t\ts.logErr(\"handling ")
		w.Raw(ownerLabel)
		w.Byte('.')
		w.Raw(methodName)
		w.Raw("\", err)\n")
		w.Line(4, "}")
	} else {
		w.Raw("\t\t\t\t")
		w.writeCallExpr(receiver, methodName, args)
		w.Byte('\n')
	}
}

// writePageGETStreamAnonHandler generates the anonymous stream handler for a page.
func (w *Writer) writePageGETStreamAnonHandler(
	p *model.Page, appPkg string,
) {
	w.Line(0, "")
	w.Raw("func (s *Server) handle")
	w.Raw(p.TypeName)
	w.Raw("GETStreamAnon(w http.ResponseWriter, r *http.Request) {\n")

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
	w.Raw("\n\tp := ")
	w.writePageConstructor(p, appPkg)
	w.Byte('\n')

	// evSubj call (for anon, pass empty userID to get public-only subjects).
	w.Raw("\ts.handleStreamRequest(w, r, sessToken, sess, evSubj")
	w.Raw(p.TypeName)
	w.Raw("(sess.UserID), func(\n")
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan msgbroker.Message,")
	w.Line(1, ") {")
	w.Line(2, "for msg := range ch {")

	// Only handle public events in anon stream.
	publicHandlers := 0
	for _, eh := range p.EventHandlers {
		ev := w.eventMap[eh.EventTypeName]
		if ev != nil && !ev.HasTargetUserIDs {
			publicHandlers++
		}
	}

	if publicHandlers == 1 {
		// Single event: use if instead of switch.
		for _, eh := range p.EventHandlers {
			ev := w.eventMap[eh.EventTypeName]
			if ev == nil || ev.HasTargetUserIDs {
				continue
			}
			w.Raw("\t\t\tif msg.Subject == EvSubj")
			w.Raw(eventConstName(ev.TypeName))
			w.Raw(" {\n")
			w.Raw("\t\t\t\tvar e ")
			w.Raw(appPkg)
			w.Byte('.')
			w.Raw(ev.TypeName)
			w.Byte('\n')
			w.Line(4, "if err := json.Unmarshal(msg.Data, &e); err != nil {")
			w.Raw("\t\t\t\t\ts.logErr(\"unmarshaling ")
			w.Raw(ev.TypeName)
			w.Raw(" JSON\", err)\n")
			w.Line(5, "continue")
			w.Line(4, "}")
			w.writeEventHandlerCall(p.TypeName, eh, "p")
			w.Line(3, "}")
		}
	} else {
		w.Line(3, "switch {")
		for _, eh := range p.EventHandlers {
			ev := w.eventMap[eh.EventTypeName]
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
	w.Line(0, "")
	w.Raw("func (s *Server) handle")
	w.Raw(p.TypeName)
	w.Raw(strings.ToUpper(h.HTTPMethod))
	w.Raw(h.Name)
	w.Raw("(\n")
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
		w.Raw("\tvar signals ")
		w.Raw(renderSignalsType(h.InputSignals, m))
		w.Byte('\n')
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
		w.writeDispatchClosure(h.InputDispatch, appPkg)
	}

	// SSE for actions that take it.
	if h.InputSSE != nil {
		w.Line(0, "")
		w.Line(1, "sse := datastar.NewSSE(w, r, datastar.WithCompression())")
	}

	// Page constructor.
	w.Raw("\tp := ")
	w.writePageConstructor(p, appPkg)
	w.Byte('\n')

	// Build the method call.
	w.writeActionMethodCall(p, h, m, appPkg)

	w.Line(0, "}")
}

func (w *Writer) writeActionMethodCall(
	p *model.Page, h *model.Handler, m *model.App, appPkg string,
) {
	// Build output list.
	var outsBuf [8]string
	outs := outsBuf[:0]
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
	var argsBuf [8]string
	args := argsBuf[:0]
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

	if len(outs) == 0 {
		if h.OutputErr != nil {
			sseRef := "nil"
			if h.InputSSE != nil {
				sseRef = "sse"
			}
			w.Raw("\tif err := ")
			w.writeCallExpr("p", methodName, args)
			w.Raw("; err != nil {\n")
			w.Raw("\t\ts.httpErrIntern(w, r, ")
			w.Raw(sseRef)
			w.Raw(", \"handling action ")
			w.Raw(p.TypeName)
			w.Byte('.')
			w.Raw(h.Name)
			w.Raw("\", err)\n")
			w.Line(2, "return")
			w.Line(1, "}")
		} else {
			w.Byte('\t')
			w.writeCallExpr("p", methodName, args)
			w.Byte('\n')
		}
		return
	}

	w.Byte('\t')
	w.writeCommaSep(outs)
	w.Raw(" := ")
	w.writeCallExpr("p", methodName, args)
	w.Byte('\n')

	if h.OutputErr != nil {
		sseRef := "nil"
		if h.InputSSE != nil {
			sseRef = "sse"
		}
		w.Line(1, "if err != nil {")
		w.Raw("\t\ts.httpErrIntern(w, r, ")
		w.Raw(sseRef)
		w.Raw(", \"handling action ")
		w.Raw(p.TypeName)
		w.Byte('.')
		w.Raw(h.Name)
		w.Raw("\", err)\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Close session.
	if h.OutputCloseSession != nil {
		w.Raw("\tif ")
		w.Raw(h.OutputCloseSession.Name)
		w.Raw(" {\n")
		w.Line(2, "if err := s.closeSession(w, r, sessToken); err != nil {")
		w.Line(3, `s.httpErrIntern(w, r, nil, "removing session", err)`)
		w.Line(3, "return")
		w.Line(2, "}")
		w.Line(1, "}")
	}

	// New session.
	if h.OutputNewSession != nil {
		w.Raw("\tif j := ")
		w.Raw(h.OutputNewSession.Name)
		w.Raw("; j.UserID != \"\" {\n")
		w.Raw("\t\tif err := s.createSession(w, r, ")
		w.Raw(h.OutputNewSession.Name)
		w.Raw("); err != nil {\n")
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
		w.Raw("\tif httpRedirect(w, r, ")
		w.Raw(h.OutputRedirect.Name)
		w.Raw(", ")
		w.Raw(statusArg)
		w.Raw(") {\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Render body (if action returns templ.Component).
	if h.OutputBody != nil {
		if m.GlobalHeadGenerator != nil {
			w.Line(1, "genericHead, err := s.app.Head(r)")
			w.Line(1, "if err != nil {")
			w.Raw("\t\ts.httpErrIntern(w, r, nil, \"generating generic head for ")
			w.Raw(p.TypeName)
			w.Byte('.')
			w.Raw(h.HTTPMethod)
			w.Raw(h.Name)
			w.Raw("\", err)\n")
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
		w.Raw("\t\tw, r, ")
		w.Raw(sessArg)
		w.Raw(", ")
		w.Raw(genericHeadArg)
		w.Raw(", nil, ")
		w.Raw(h.OutputBody.Name)
		w.Raw(", nil,\n")
		w.Line(1, "); err != nil {")
		w.Raw("\t\ts.logErr(\"rendering response of ")
		w.Raw(p.TypeName)
		w.Byte('.')
		w.Raw(h.HTTPMethod)
		w.Raw(h.Name)
		w.Raw("\", err)\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}
}

func (w *Writer) writeReadQuery(input *model.Input, m *model.App) {
	w.Line(0, "")
	w.Line(1, "q := r.URL.Query()")
	w.Raw("\tvar query ")
	w.Raw(renderQueryType(input, m))
	w.Byte('\n')
	fields := w.structFields(input.Type.Resolved)
	for _, f := range fields {
		tag := queryTagValue(f.Tag)
		if isIntType(f.Type) {
			bits, unsigned := intTypeParseInfo(f.Type)
			typeName := intTypeName(f.Type)
			w.Line(1, "{")
			w.Raw("\t\tif q := q.Get(")
			w.writeQuoted(tag)
			w.Raw("); q != \"\" {\n")
			if unsigned {
				w.Linef(3, "u, err := strconv.ParseUint(q, 10, %d)", bits)
			} else {
				w.Linef(3, "i, err := strconv.ParseInt(q, 10, %d)", bits)
			}
			w.Line(3, "if err != nil {")
			w.Raw("\t\t\t\ts.httpErrBad(w, \"unexpected value for query parameter: ")
			w.Raw(tag)
			w.Raw("\", err)\n")
			w.Line(4, "return")
			w.Line(3, "}")
			w.Raw("\t\t\tquery.")
			w.Raw(f.Name)
			w.Raw(" = ")
			if unsigned {
				if typeName != "uint64" {
					w.Raw(typeName + "(u)")
				} else {
					w.Raw("u")
				}
			} else {
				if typeName != "int64" {
					w.Raw(typeName + "(i)")
				} else {
					w.Raw("i")
				}
			}
			w.Raw("\n")
			w.Line(2, "}")
			w.Line(1, "}")
		} else {
			w.Raw("\tquery.")
			w.Raw(f.Name)
			w.Raw(" = q.Get(")
			w.writeQuoted(tag)
			w.Raw(")\n")
		}
	}
}

func (w *Writer) writeReadPath(input *model.Input, m *model.App) {
	w.Line(0, "")
	w.Raw("\tvar path ")
	w.Raw(renderPathType(input, m))
	w.Byte('\n')
	fields := w.structFields(input.Type.Resolved)
	for _, f := range fields {
		tag := pathTagValue(f.Tag)
		w.Raw("\tpath.")
		w.Raw(f.Name)
		w.Raw(" = r.PathValue(")
		w.writeQuoted(tag)
		w.Raw(")\n")
	}
}

type reflectSignalField struct {
	SignalName string
	FieldName  string
	Type       types.Type
	QueryTag   string
}
