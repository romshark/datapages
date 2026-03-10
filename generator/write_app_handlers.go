package generator

import (
	"go/types"
	"strings"

	"github.com/romshark/datapages/parser/model"
)

// handlerArgVar maps an InputKind constant to the local variable name
// used in generated code. skipSSE causes SSE to be omitted (for app-level actions).
func handlerArgVar(kind string, skipSSE bool) string {
	switch kind {
	case model.InputKindRequest:
		return "r"
	case model.InputKindSSE:
		if skipSSE {
			return ""
		}
		return "sse"
	case model.InputKindSessionToken:
		return "sessToken"
	case model.InputKindSession:
		return "sess"
	case model.InputKindPath:
		return "path"
	case model.InputKindQuery:
		return "query"
	case model.InputKindSignals:
		return "signals"
	case model.InputKindDispatch:
		return "dispatch"
	case model.InputKindEvent:
		return "e"
	default:
		return ""
	}
}

// outputVar returns the generated variable name for an output.
// Error outputs always use "err"; others use their source name.
func outputVar(out *model.Output) string {
	if out.Kind == model.OutputKindErr {
		return "err"
	}
	return out.Name
}

// handlerGETOutputVars builds the output variable list for a GET handler call
// in the order defined by h.OrderedOutputs.
func handlerGETOutputVars(
	h *model.Handler, get *model.HandlerGET,
) []string {
	if len(h.OrderedOutputs) > 0 {
		outs := make([]string, 0, len(h.OrderedOutputs))
		for _, out := range h.OrderedOutputs {
			outs = append(outs, outputVar(out))
		}
		return outs
	}
	// Fallback for manually constructed Handler (e.g. in tests).
	var outsBuf [8]string
	outs := outsBuf[:0]
	if get.OutputBody != nil {
		outs = append(outs, get.OutputBody.Name)
	}
	if get.OutputHead != nil {
		outs = append(outs, get.OutputHead.Name)
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
	return outs
}

// handlerOutputVars builds the output variable list for a handler call
// in the order defined by h.OrderedOutputs.
func handlerOutputVars(h *model.Handler) []string {
	if len(h.OrderedOutputs) > 0 {
		outs := make([]string, 0, len(h.OrderedOutputs))
		for _, out := range h.OrderedOutputs {
			outs = append(outs, outputVar(out))
		}
		return outs
	}
	// Fallback for manually constructed Handler (e.g. in tests).
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
	if h.OutputEnableBgStream != nil {
		outs = append(outs, h.OutputEnableBgStream.Name)
	}
	if h.OutputDisableRefresh != nil {
		outs = append(outs, h.OutputDisableRefresh.Name)
	}
	if h.OutputErr != nil {
		outs = append(outs, "err")
	}
	return outs
}

// handlerInputArgs builds the argument list for a handler call
// in the order defined by h.OrderedInputs.
func handlerInputArgs(h *model.Handler, skipSSE bool) []string {
	if len(h.OrderedInputs) > 0 {
		args := make([]string, 0, len(h.OrderedInputs))
		for _, inp := range h.OrderedInputs {
			if v := handlerArgVar(inp.Kind, skipSSE); v != "" {
				args = append(args, v)
			}
		}
		return args
	}
	// Fallback for manually constructed Handler (e.g. in tests).
	var args []string
	if h.InputRequest != nil {
		args = append(args, "r")
	}
	if h.InputSSE != nil && !skipSSE {
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
	return args
}

// eventHandlerInputArgs builds the argument list for an event handler call
// in the order defined by eh.OrderedInputs.
func eventHandlerInputArgs(eh *model.EventHandler) []string {
	if len(eh.OrderedInputs) > 0 {
		args := make([]string, 0, len(eh.OrderedInputs))
		for _, inp := range eh.OrderedInputs {
			if v := handlerArgVar(inp.Kind, false); v != "" {
				args = append(args, v)
			}
		}
		return args
	}
	// Fallback for manually constructed EventHandler (e.g. in tests).
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
	return args
}

// writePageGETHandler generates the GET handler for a page.
func (w *Writer) writePageGETHandler(p *model.Page, m *model.App, appPkg string) {
	w.Line(0, "")
	w.Raw("func (s *Server) handle")
	w.Raw(p.TypeName)
	w.Raw("GET(w http.ResponseWriter, r *http.Request) {\n")

	h := p.GET.Handler

	hasBody := false

	// Auth.
	needsSession := h.InputSession != nil ||
		(m.GlobalHeadGenerator != nil && m.GlobalHeadGenerator.InputSession)
	needsToken := h.InputSessionToken != nil ||
		(m.GlobalHeadGenerator != nil && m.GlobalHeadGenerator.InputSessionToken)
	if needsSession || needsToken {
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
		if hasBody {
			w.Line(0, "")
		}
		hasBody = true
		w.Line(1, `if r.URL.Path != "/" {`)
		if m.PageError404 != nil {
			w.Line(2, "s.render404(w, r)")
		} else {
			w.Line(2, "http.NotFound(w, r)")
		}
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

	// Dispatch closure.
	if h.InputDispatch != nil {
		hasBody = true
		w.writeDispatchClosure(h.InputDispatch, appPkg)
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

	// Build output list in user-defined order.
	outs := handlerGETOutputVars(h, p.GET)

	// Build input args in user-defined order.
	args := handlerInputArgs(h, false)

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
	if gh := m.GlobalHeadGenerator; gh != nil {
		hasSess := h.InputSession != nil || h.InputSessionToken != nil ||
			gh.InputSession || gh.InputSessionToken
		hasSessToken := h.InputSessionToken != nil || gh.InputSessionToken
		w.writeGenericHeadCall(gh, appPkg, hasSess, hasSessToken)
	}

	// Body attrs.
	w.writeGETBodyAttrs(p)

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
	if m.Session != nil {
		sessArg := "sess"
		headNeedsSession := m.GlobalHeadGenerator != nil &&
			(m.GlobalHeadGenerator.InputSession || m.GlobalHeadGenerator.InputSessionToken)
		if p.PageSpecialization == model.PageTypeError500 ||
			(!hasSessionInput(h) && !headNeedsSession) {
			sessArg = appPkg + ".Session{}"
		}
		w.Raw(sessArg)
		w.Raw(", ")
	}
	if m.GlobalHeadGenerator != nil {
		w.Raw("genericHead, ")
	}
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

// writeGenericHeadCall emits: genericHead := s.app.Head(r[, sess][, sessToken])
// hasSess indicates whether a "sess" variable is in scope.
// hasSessToken indicates whether a "sessToken" variable is in scope.
func (w *Writer) writeGenericHeadCall(
	gh *model.GlobalHead, appPkg string, hasSess, hasSessToken bool,
) {
	w.Raw("\tgenericHead := s.app.Head(r")
	if gh.InputSession {
		if hasSess {
			w.Raw(", sess")
		} else {
			w.Raw(", ")
			w.Raw(appPkg)
			w.Raw(".Session{}")
		}
	}
	if gh.InputSessionToken {
		if hasSessToken {
			w.Raw(", sessToken")
		} else {
			w.Raw(`, ""`)
		}
	}
	w.Raw(")\n")
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
		fi := structFieldInfo{Name: f.FieldName, Type: f.Type}
		if isStringType(f.Type) {
			w.Line(0, "")
			w.Raw("\t\t_, _ = io.WriteString(w, `data-signals:")
			w.Raw(f.SignalName)
			w.Raw("=\"'`)\n")
			w.Raw("\t\t_, _ = io.WriteString(w, query.")
			w.Raw(f.FieldName)
			w.Raw(")\n")
			w.Line(2, "_, _ = io.WriteString(w, `'\"`)")
		} else {
			w.Line(0, "")
			w.Raw("\t\t_, _ = io.WriteString(w, `data-signals:")
			w.Raw(f.SignalName)
			w.Raw("=\"`)\n")
			w.Raw("\t\t_, _ = io.WriteString(w, ")
			w.writeFieldToString("query", fi)
			w.Raw(")\n")
			w.Line(2, "_, _ = io.WriteString(w, `\"`)")
		}
	}

	// Stream data-init attr.
	// Fun fact: this is a writer writing a writer writing an attribute.
	if hasStream {
		hasPrivate := pageHasPrivateEvent(p, w.eventMap)
		streamPath := routeStreamPath(p.Route)
		if hasPrivate && h.InputSession != nil {
			if hasAnonStream {
				// Mixed: authenticated -> "/_$/"; anonymous -> "/_$/anon/"
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
		} else if !hasPrivate {
			// Public-only stream: always emit data-init unconditionally.
			if h.InputPath != nil {
				w.Line(0, "")
				w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
				w.writeStreamPathSegments(p.Route, h.InputPath)
				if hasEnableBgStream {
					w.Line(2, "_, _ = io.WriteString(w, `/_$/'`)")
					w.Raw("\t\tif ")
					w.Raw(h.OutputEnableBgStream.Name)
					w.Raw(" {\n")
					w.Line(3, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
					w.Line(2, "} else {")
					w.Line(3, "_, _ = io.WriteString(w, `)\"`)")
					w.Line(2, "}")
				} else {
					w.Line(2, "_, _ = io.WriteString(w, `/_$/')\"`)")
				}
			} else if hasEnableBgStream {
				w.Line(0, "")
				w.Raw("\t\t_, _ = io.WriteString(w, `data-init=\"@get('")
				w.Raw(streamPath)
				w.Raw("'`)\n")
				w.Raw("\t\tif ")
				w.Raw(h.OutputEnableBgStream.Name)
				w.Raw(" {\n")
				w.Line(3, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
				w.Line(2, "} else {")
				w.Line(3, "_, _ = io.WriteString(w, `)\"`)")
				w.Line(2, "}")
			} else {
				w.Line(0, "")
				w.Raw("\t\t_, _ = io.WriteString(w, `data-init=\"@get('")
				w.Raw(streamPath)
				w.Raw("')\"`)\n")
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
	// Build a map from path: tag value to field info.
	fields := w.structFields(pathInput.Type.Resolved)
	tagToField := make(map[string]structFieldInfo, len(fields))
	for _, f := range fields {
		if tag := pathTagValue(f.Tag); tag != "" {
			tagToField[tag] = f
		}
	}
	// Build the path prefix up to the variable, then write the variable.
	literals, vars := routeSegments(route)
	for i, lit := range literals {
		w.Raw("\t\t_, _ = io.WriteString(w, `")
		w.Raw(lit)
		w.Raw("`)\n")
		if i < len(vars) {
			f := tagToField[vars[i]]
			w.Raw("\t\t_, _ = io.WriteString(w, ")
			w.writeFieldToString("path", f)
			w.Raw(")\n")
		}
	}
}

// writeFieldToString emits an expression that converts a struct field to a string.
// For string fields it emits "varName.FieldName"; for other types it wraps with
// strconv.Format* or fmt.Sprint.
func (w *Writer) writeFieldToString(varName string, f structFieldInfo) {
	ref := varName + "." + f.Name
	if isStringType(f.Type) {
		w.Raw(ref)
	} else if isIntType(f.Type) {
		_, unsigned := intTypeParseInfo(f.Type)
		typeName := intTypeName(f.Type)
		if unsigned {
			if typeName != "uint64" {
				w.Rawf("strconv.FormatUint(uint64(%s), 10)", ref)
			} else {
				w.Rawf("strconv.FormatUint(%s, 10)", ref)
			}
		} else {
			if typeName != "int64" {
				w.Rawf("strconv.FormatInt(int64(%s), 10)", ref)
			} else {
				w.Rawf("strconv.FormatInt(%s, 10)", ref)
			}
		}
	} else if isFloatType(f.Type) {
		bits := floatBits(f.Type)
		typeName := floatTypeName(f.Type)
		if typeName != "float64" {
			w.Rawf("strconv.FormatFloat(float64(%s), 'f', -1, %d)", ref, bits)
		} else {
			w.Rawf("strconv.FormatFloat(%s, 'f', -1, %d)", ref, bits)
		}
	} else if isBoolType(f.Type) {
		w.Rawf("strconv.FormatBool(%s)", ref)
	} else {
		// TextUnmarshaler or other — use fmt.Sprint as fallback.
		w.Rawf("fmt.Sprint(%s)", ref)
	}
}

// writePageGETStreamHandler generates the stream handler for a page.
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

	hasPrivate := pageHasPrivateEvent(p, w.eventMap)
	if hasPrivate {
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
	w.Raw("\ts.handleStreamRequest(w, r,")
	if hasPrivate {
		w.Raw(" sessToken, sess,")
	} else if w.usage.streamAuth {
		w.Raw(` "", `)
		w.Raw(appPkg)
		w.Raw(`.Session{},`)
	}
	w.Raw(" ")
	w.Raw(evSubjName)
	if hasPrivate {
		w.Raw("(sess.UserID), func(\n")
	} else {
		w.Raw("(), func(\n")
	}
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan msgbroker.Message,")
	w.Line(1, ") {")
	w.Line(2, "for msg := range ch {")
	if hasPrivate {
		w.Line(3, "switch {")
	} else {
		w.Line(3, "switch msg.Subject {")
	}

	// Generate case for each event handler.
	for _, eh := range p.EventHandlers {
		ev := w.eventMap[eh.EventTypeName]
		if ev == nil {
			continue
		}
		w.writeStreamEventCase(p, eh, ev, appPkg, !hasPrivate)
	}

	w.Line(3, "}")
	w.Line(2, "}")
	w.Line(1, "})")
	w.Line(0, "}")
}

func (w *Writer) writeStreamEventCase(
	p *model.Page, eh *model.EventHandler, ev *model.Event,
	appPkg string, tagged bool,
) {
	constName := eventConstName(ev.TypeName)

	if ev.HasTargetUserIDs {
		w.Raw("\t\t\tcase strings.HasPrefix(msg.Subject, EvSubjPref")
		w.Raw(constName)
		w.Raw("):\n")
	} else if tagged {
		w.Raw("\t\t\tcase EvSubj")
		w.Raw(constName)
		w.Raw(":\n")
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
	// Build args in user-defined order.
	args := eventHandlerInputArgs(eh)

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
		w.Line(3, "switch msg.Subject {")
		for _, eh := range p.EventHandlers {
			ev := w.eventMap[eh.EventTypeName]
			if ev == nil || ev.HasTargetUserIDs {
				continue
			}
			w.writeStreamEventCase(p, eh, ev, appPkg, true)
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
	headNeedsSess := h.OutputBody != nil && m.GlobalHeadGenerator != nil &&
		m.GlobalHeadGenerator.InputSession
	headNeedsToken := h.OutputBody != nil && m.GlobalHeadGenerator != nil &&
		m.GlobalHeadGenerator.InputSessionToken
	if h.InputSession != nil || needsToken || headNeedsSess || headNeedsToken {
		if needsToken || headNeedsToken {
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
	// Build output list in user-defined order.
	outs := handlerOutputVars(h)

	// Build input args in user-defined order.
	args := handlerInputArgs(h, false)

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
			w.writeGenericHeadCall(m.GlobalHeadGenerator, appPkg, hasSessionInput(h), false)
		}
		w.Line(1, "if err := s.writeHTML(")
		w.Raw("\t\tw, r, ")
		if m.Session != nil {
			sessArg := "sess"
			headNeedsSession := m.GlobalHeadGenerator != nil &&
				(m.GlobalHeadGenerator.InputSession || m.GlobalHeadGenerator.InputSessionToken)
			if !hasSessionInput(h) && !headNeedsSession {
				sessArg = appPkg + ".Session{}"
			}
			w.Raw(sessArg)
			w.Raw(", ")
		}
		if m.GlobalHeadGenerator != nil {
			w.Raw("genericHead, nil, ")
		} else {
			w.Raw("nil, ")
		}
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
		if isStringType(f.Type) {
			w.Raw("\tquery.")
			w.Raw(f.Name)
			w.Raw(" = q.Get(")
			w.writeQuoted(tag)
			w.Raw(")\n")
		} else {
			w.Line(1, "{")
			w.Raw("\t\tif q := q.Get(")
			w.writeQuoted(tag)
			w.Raw("); q != \"\" {\n")
			w.writeParseField("query", f, tag, "query parameter", 3)
			w.Line(2, "}")
			w.Line(1, "}")
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
		if isStringType(f.Type) {
			w.Raw("\tpath.")
			w.Raw(f.Name)
			w.Raw(" = r.PathValue(")
			w.writeQuoted(tag)
			w.Raw(")\n")
		} else {
			w.Line(1, "{")
			w.Raw("\t\tv := r.PathValue(")
			w.writeQuoted(tag)
			w.Raw(")\n")
			w.writeParseField("path", f, tag, "path parameter", 2)
			w.Line(1, "}")
		}
	}
}

// writeParseField emits code that parses a raw string value into a typed
// struct field. For writeReadQuery the raw variable is named "q" (from the
// if-guard); for writeReadPath it is "v" (set before the call).
//
// varName is "path" or "query" (the struct being populated).
// label is "path parameter" or "query parameter" (for error messages).
// indent is the base indentation level for the generated code.
func (w *Writer) writeParseField(
	varName string, f structFieldInfo, tag, label string, indent int,
) {
	// Determine the raw-string variable name: "q" for query, "v" for path.
	raw := "q"
	if varName == "path" {
		raw = "v"
	}

	tabs := func(n int) {
		for range n {
			w.Byte('\t')
		}
	}

	if isIntType(f.Type) {
		bits, unsigned := intTypeParseInfo(f.Type)
		typeName := intTypeName(f.Type)
		if unsigned {
			tabs(indent)
			w.Rawf("u, err := strconv.ParseUint(%s, 10, %d)\n", raw, bits)
		} else {
			tabs(indent)
			w.Rawf("i, err := strconv.ParseInt(%s, 10, %d)\n", raw, bits)
		}
		tabs(indent)
		w.Raw("if err != nil {\n")
		tabs(indent + 1)
		w.Rawf("s.httpErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
		tabs(indent + 1)
		w.Raw("return\n")
		tabs(indent)
		w.Raw("}\n")
		tabs(indent)
		w.Raw(varName)
		w.Byte('.')
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
		w.Byte('\n')
	} else if isFloatType(f.Type) {
		bits := floatBits(f.Type)
		typeName := floatTypeName(f.Type)
		tabs(indent)
		w.Rawf("f, err := strconv.ParseFloat(%s, %d)\n", raw, bits)
		tabs(indent)
		w.Raw("if err != nil {\n")
		tabs(indent + 1)
		w.Rawf("s.httpErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
		tabs(indent + 1)
		w.Raw("return\n")
		tabs(indent)
		w.Raw("}\n")
		tabs(indent)
		w.Raw(varName)
		w.Byte('.')
		w.Raw(f.Name)
		w.Raw(" = ")
		if typeName != "float64" {
			w.Raw("float32(f)")
		} else {
			w.Raw("f")
		}
		w.Byte('\n')
	} else if isBoolType(f.Type) {
		tabs(indent)
		w.Rawf("b, err := strconv.ParseBool(%s)\n", raw)
		tabs(indent)
		w.Raw("if err != nil {\n")
		tabs(indent + 1)
		w.Rawf("s.httpErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
		tabs(indent + 1)
		w.Raw("return\n")
		tabs(indent)
		w.Raw("}\n")
		tabs(indent)
		w.Raw(varName)
		w.Byte('.')
		w.Raw(f.Name)
		w.Raw(" = b\n")
	} else if isTextUnmarshaler(f.Type) {
		tabs(indent)
		w.Rawf("if err := %s.%s.UnmarshalText([]byte(%s)); err != nil {\n",
			varName, f.Name, raw)
		tabs(indent + 1)
		w.Rawf("s.httpErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
		tabs(indent + 1)
		w.Raw("return\n")
		tabs(indent)
		w.Raw("}\n")
	}
}

type reflectSignalField struct {
	SignalName string
	FieldName  string
	Type       types.Type
	QueryTag   string
}
