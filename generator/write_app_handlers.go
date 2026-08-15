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
	case model.InputKindStreamID:
		return "streamID"
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
	case model.InputKindState:
		// The generated handler wrapper exposes the checked-out
		// *T as `slot.state`.
		return "slot.state"
	case model.InputKindStateID:
		// The derived routing key of the tab, not the instance id.
		// See writeStateRouteKeyVar.
		return "stateID"
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
	if h.InputStreamID != nil {
		args = append(args, "streamID")
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

func handlerInputArgsWithDispatchVar(
	h *model.Handler, skipSSE bool, dispatchVar string,
) []string {
	if len(h.OrderedInputs) > 0 {
		args := make([]string, 0, len(h.OrderedInputs))
		for _, inp := range h.OrderedInputs {
			if inp.Kind == model.InputKindDispatch {
				if dispatchVar != "" {
					args = append(args, dispatchVar)
				}
				continue
			}
			if v := handlerArgVar(inp.Kind, skipSSE); v != "" {
				args = append(args, v)
			}
		}
		return args
	}
	return handlerInputArgs(h, skipSSE)
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
	if eh.InputStreamID != nil {
		args = append(args, "streamID")
	}
	if eh.InputSessionToken != nil {
		args = append(args, "sessToken")
	}
	if eh.InputSession != nil {
		args = append(args, "sess")
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

	// Read signals.
	//
	// A page load carries them in the datastar query parameter,
	// which the Datastar client adds and a visitor typing the URL does not.
	// Their absence is the ordinary case and leaves the handler the zero value;
	// only a malformed value is an error.
	if h.InputSignals != nil {
		hasBody = true
		w.Line(0, "")
		w.Raw("\tvar signals ")
		w.Raw(renderSignalsType(h.InputSignals, m))
		w.Byte('\n')
		w.Line(1, `if r.URL.Query().Has("datastar") {`)
		w.Line(2, "if err := datastar.ReadSignals(r, &signals); err != nil {")
		w.Line(3, `s.httpErrBad(w, "reading signals", err)`)
		w.Line(3, "return")
		w.Line(2, "}")
		w.Line(1, "}")
	}

	// Dispatch closure.
	if h.InputDispatch != nil {
		hasBody = true
		w.writeDispatchClosure(h.InputDispatch, appPkg)
	}

	// Stateful page: mint the Datapages-Instance header so the client can
	// echo it on actions and on the SSE stream connect.
	if p.State != nil {
		hasBody = true
		w.writeMintInstanceIDOnGET()
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

// writeSessionOutputs emits what a handler's newSession and closeSession
// outputs ask for. A handler that returns them and is answered by neither has
// them declared and unused, which is a package that does not compile.
// writeSubjectSignalsRead emits the read of the signal values a page subscribes by.
// Both stream handlers need it: the subjects a stream subscribes to do not depend on
// whether the client holds a session.
func (w *Writer) writeSubjectSignalsRead(signalFields []model.SubjectField) {
	w.Line(0, "")
	w.Line(1, "var subjSignals struct {")
	for _, sf := range signalFields {
		w.Raw("\t\t")
		w.Raw(sf.Name)
		w.Raw(` string `)
		w.Raw("`json:\"")
		w.Raw(sf.SignalName)
		w.Raw("\"`")
		w.Byte('\n')
	}
	w.Line(1, "}")
	w.Line(1, "if err := datastar.ReadSignals(r, &subjSignals); err != nil {")
	w.Line(2, `s.httpErrBad(w, "reading signals", err)`)
	w.Line(2, "return")
	w.Line(1, "}")
	for _, sf := range signalFields {
		w.Raw("\tif subjSignals.")
		w.Raw(sf.Name)
		w.Raw(" == \"\" {\n")
		w.Raw("\t\ts.httpErrBad(w, \"missing required signal\", fmt.Errorf(\"signal %q is required\", ")
		w.writeQuoted(sf.SignalName)
		w.Raw("))\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}
}

func (w *Writer) writeSessionOutputs(h *model.Handler) {
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
	if h.OutputNewSession != nil {
		w.Raw("\tif j := ")
		w.Raw(h.OutputNewSession.Name)
		w.Raw("; j.UserID != \"\" {\n")
		w.Raw("\t\tif err := s.createSession(w, r, ")
		w.Raw(h.OutputNewSession.Name)
		w.Raw("); err != nil {\n")
		w.Line(3, `s.httpErrIntern(w, r, nil, "creating session", err)`)
		w.Line(3, "return")
		w.Line(2, "}")
		w.Line(1, "}")
	}
}

func (w *Writer) writeGETMethodCall(p *model.Page, m *model.App, appPkg string) {
	h := p.GET.Handler

	// Build output list in user-defined order.
	outs := handlerGETOutputVars(h, p.GET)

	// enableBackgroundStreaming is used in bodyAttrs (when there's no
	// disableRefresh) and in bodySuffix (when the page has a stream).
	// If neither applies, blank it to avoid an unused-variable error.
	if h.OutputEnableBgStream != nil &&
		h.OutputDisableRefresh != nil && !pageHasStream(p) {
		for i, o := range outs {
			if o == h.OutputEnableBgStream.Name {
				outs[i] = "_"
				break
			}
		}
	}

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

	// Close and create session, before anything is written: both set a cookie,
	// and a cookie set after the body has started is dropped.
	w.writeSessionOutputs(h)

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

	// Body attrs and suffix.
	hasBodySuffix := w.writeGETBodyAttrs(p)

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
			(m.GlobalHeadGenerator.InputSession ||
				m.GlobalHeadGenerator.InputSessionToken)
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
	w.Raw(", bodyAttrs, ")
	if hasBodySuffix {
		w.Raw("bodySuffix,\n")
	} else {
		w.Raw("nil,\n")
	}
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

func (w *Writer) writeGETBodyAttrs(p *model.Page) (hasBodySuffix bool) {
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

	// bodyAttrs: visibility change + reflect signal attrs.
	// Written as attributes on the <body> tag.
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

	w.Line(1, "}")

	// bodySuffix: data-init (SSE stream) + data-effect (URL sync).
	// Written as attributes on a <div> after the body content so that
	// signals defined in child elements are available in the Datastar
	// store when these attributes are processed.
	needsSuffix := hasStream || hasReflectSignals
	if !needsSuffix {
		return false
	}

	w.Line(0, "")
	w.Line(1, "bodySuffix := func(w http.ResponseWriter) {")

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
				if h.InputPath != nil {
					w.Line(0, "")
					w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
					w.writeStreamPathSegments(p.Route, h.InputPath)
					if hasEnableBgStream {
						w.Line(2, `if sess.UserID != "" {`)
						w.Line(3, "_, _ = io.WriteString(w, `/_$/'`)")
						w.Raw("\t\t\tif ")
						w.Raw(h.OutputEnableBgStream.Name)
						w.Raw(" {\n")
						w.Line(4, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
						w.Line(3, "} else {")
						w.Line(4, "_, _ = io.WriteString(w, `)\"`)")
						w.Line(3, "}")
						w.Line(2, "}")
					} else {
						w.Line(2, `if sess.UserID != "" {`)
						w.Line(3, "_, _ = io.WriteString(w, `/_$/')\"`)")
						w.Line(2, "}")
					}
				} else if hasEnableBgStream {
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
		if h.InputPath != nil {
			// Route has path parameters that must be interpolated with HTML escaping.
			fields := w.structFields(h.InputPath.Type.Resolved)
			tagToField := make(map[string]structFieldInfo, len(fields))
			for _, f := range fields {
				if tag := pathTagValue(f.Tag); tag != "" {
					tagToField[tag] = f
				}
			}
			// writeRoute writes the route with path params replaced by
			// template.HTMLEscape calls. It assumes we are mid-backtick
			// in an io.WriteString and leaves us mid-backtick.
			writeRoute := func(r string) {
				for {
					i := strings.IndexByte(r, '{')
					if i < 0 {
						w.Raw(r)
						return
					}
					j := strings.IndexByte(r[i:], '}')
					w.Raw(r[:i])
					w.Raw("`)\n")
					f := tagToField[r[i+1:i+j]]
					w.Raw("\t\ttemplate.HTMLEscape(w, []byte(")
					w.writeFieldToString("path", f)
					w.Raw("))\n")
					w.Raw("\t\t_, _ = io.WriteString(w, `")
					r = r[i+j+1:]
				}
			}
			w.Raw("\t\t\twindow.history.replaceState(null, '', query ? '")
			writeRoute(route)
			w.Raw("?' + query : '")
			writeRoute(route)
			w.Raw("');\n")
			w.Line(2, "\"`)")
		} else {
			w.Raw("\t\t\twindow.history.replaceState(null, '', query ? '")
			w.Raw(route)
			w.Raw("?' + query : '")
			w.Raw(route)
			w.Raw("');\n")
			w.Line(2, "\"`)")
		}
	}

	w.Line(1, "}")
	return true
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

	needsAuth := pageStreamNeedsAuth(p, w.eventMap)
	hasPrivate := pageHasPrivateEvent(p, w.eventMap)
	hasSignalScoped := pageHasSignalScopedEvent(p, w.eventMap)
	if needsAuth {
		w.Line(1, "sess, sessToken, ok := s.auth(w, r)")
		w.Line(1, "if !ok {")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	if hasPrivate {
		// Check if anon stream exists - if so, redirect unauthenticated to anon.
		hasAnon := pageHasAnonStream(p, w.eventMap)
		if hasAnon {
			w.Line(0, "")
			w.Line(1, `if sess.UserID == "" {`)
			w.Line(2, "// The query carries the signals a stream subscribes by,")
			w.Line(2, "// which the anonymous route needs as much as this one.")
			w.Line(2, `target := r.URL.Path + "/anon"`)
			w.Line(2, `if r.URL.RawQuery != "" {`)
			w.Line(3, `target += "?" + r.URL.RawQuery`)
			w.Line(2, "}")
			w.Line(2, "http.Redirect(w, r, target, http.StatusSeeOther)")
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

	// Read signal-scoped subject values for subscription.
	signalFields := pageSignalSubjectFields(p, w.eventMap)
	if hasSignalScoped {
		w.writeSubjectSignalsRead(signalFields)
	}

	if p.StreamOpen != nil && p.StreamOpen.InputSignals != nil {
		w.Line(0, "")
		w.Raw("\tvar signals ")
		w.Raw(renderSignalsType(p.StreamOpen.InputSignals, m))
		w.Byte('\n')
		w.Line(1, "if err := datastar.ReadSignals(r, &signals); err != nil {")
		w.Line(2, `s.httpErrBad(w, "reading signals", err)`)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	if p.StreamOpen != nil && p.StreamOpen.InputDispatch != nil {
		w.writeDispatchClosureAs(
			"dispatchOpen", p.StreamOpen.InputDispatch, appPkg,
			"context.WithoutCancel(r.Context())",
		)
	}
	if p.StreamClose != nil && p.StreamClose.InputDispatch != nil {
		w.writeDispatchClosureAs(
			"dispatchClosed", p.StreamClose.InputDispatch, appPkg,
			"context.WithoutCancel(r.Context())",
		)
	}

	// Stateful page: verify Datapages-Instance header and declare the slot
	// handle that is captured by the onOpen/onClose/event-loop closures.
	if p.State != nil {
		w.Line(0, "")
		w.writeVerifyInstanceIDHeader()
		w.writeStateCapacityCheck(p.State)
		if pageNeedsStateRouteKey(p, w.eventMap) {
			w.writeStateRouteKeyVar()
		}
		w.Linef(1, "var slot *%s", pageStateSlotTypeName(p))
	}

	// Page constructor.
	w.Raw("\n\tp := ")
	w.writePageConstructor(p, appPkg)
	w.Byte('\n')

	// evSubj call.
	evSubjName := "evSubj" + p.TypeName
	w.Raw("\ts.handleStreamRequest(w, r,")
	if needsAuth {
		w.Raw(" sessToken, sess,")
	} else if w.usage.streamAuth {
		w.Raw(` "", `)
		w.Raw(appPkg)
		w.Raw(`.Session{},`)
	}
	w.Raw(" ")
	w.Raw(evSubjName)
	switch {
	case pageHasStateIDScopedEvent(p, w.eventMap):
		w.Raw("(stateID),\n")
	case hasPrivate && hasSignalScoped:
		w.Raw("(sess.UserID")
		for _, sf := range signalFields {
			w.Raw(", subjSignals.")
			w.Raw(sf.Name)
		}
		w.Raw("),\n")
	case hasPrivate:
		w.Raw("(sess.UserID),\n")
	case hasSignalScoped:
		w.Raw("(")
		for i, sf := range signalFields {
			if i > 0 {
				w.Raw(", ")
			}
			w.Raw("subjSignals.")
			w.Raw(sf.Name)
		}
		w.Raw("),\n")
	default:
		w.Raw("(),\n")
	}
	w.writePageStreamOpenHook(p)
	w.writePageStreamCloseHook(p)
	w.Line(1, "func(")
	w.Line(2, "streamID uint64,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan msgbroker.Message,")
	w.Line(1, ") {")
	if len(p.EventHandlers) == 0 {
		w.Line(2, "for range ch {")
		w.Line(2, "}")
	} else {
		w.Line(2, "for msg := range ch {")
		// One event matched by prefix makes the whole switch untagged:
		// a tagged switch can only compare, and a pattern never equals the
		// subject a message carries.
		needsPrefixMatch := false
		for _, eh := range p.EventHandlers {
			if ev := w.eventMap[eh.EventTypeName]; ev != nil &&
				evUsesPrefixMatch(ev) {
				needsPrefixMatch = true
				break
			}
		}
		if needsPrefixMatch {
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
			w.writeStreamEventCase(p, eh, ev, appPkg, !needsPrefixMatch)
		}

		w.Line(3, "}")
		w.Line(2, "}")
	}
	w.Line(1, "})")
	w.Line(0, "}")
}

func (w *Writer) writeStreamEventCase(
	p *model.Page, eh *model.EventHandler, ev *model.Event,
	appPkg string, tagged bool,
) {
	constName := eventConstName(ev.TypeName)

	if evUsesPrefixMatch(ev) {
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

	// Stateful event handlers run under the per-slot mutex to serialize with
	// concurrent action calls and with StreamOpen / StreamClose.
	// The loop can outlive the close that returns the state to the pool,
	// which is what the liveness check guards against.
	stateful := eh.InputState != nil
	if stateful {
		w.Line(4, "slot.mu.Lock()")
		w.Line(4, "if slot.dead {")
		w.Line(5, "slot.mu.Unlock()")
		w.Line(5, "continue")
		w.Line(4, "}")
	}

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

	if stateful {
		w.Line(4, "slot.mu.Unlock()")
	}
}

func (w *Writer) writePageStreamOpenHook(p *model.Page) {
	// Stateful page: always emit an onOpen closure so the slot is
	// allocated even when the user did not define StreamOpen.
	if p.State != nil {
		w.writeStatefulStreamOpenHook(p)
		return
	}
	if p.StreamOpen == nil {
		w.Line(1, "nil,")
		return
	}
	dispatchVar := ""
	if p.StreamOpen.InputDispatch != nil {
		dispatchVar = "dispatchOpen"
	}
	w.Line(1, "func(")
	w.Line(2, "streamID uint64,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator,")
	w.Line(1, ") error {")
	if p.StreamOpen.OutputErr != nil {
		w.Raw("\t\treturn ")
	} else {
		w.Raw("\t\t")
	}
	w.writeCallExpr(
		"p", "StreamOpen",
		handlerInputArgsWithDispatchVar(p.StreamOpen, false, dispatchVar),
	)
	w.Byte('\n')
	if p.StreamOpen.OutputErr == nil {
		w.Line(2, "return nil")
	}
	w.Line(1, "},")
}

// writeStatefulStreamOpenHook emits the onOpen closure for a stateful page.
// The closure allocates a fresh slot from the pool, stashes it in the outer
// `slot` variable, and, if the user defined StreamOpen, calls it under the slot mutex.
//
// Every stream gets its own instance. A client that reconnects under an id it
// already used is a new stream and starts from a zeroed state.
//
// The instance is reserved before the user hook runs, and the close hook that
// gives it back is wired up only once this one has returned nil. An open that
// ends any other way therefore releases what it took, here, or nothing ever does.
func (w *Writer) writeStatefulStreamOpenHook(p *model.Page) {
	suffix := stateSuffix(p.State)
	w.Line(1, "func(")
	w.Line(2, "streamID uint64,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator,")
	w.Line(1, ") error {")
	w.Linef(2, "slot = s.allocate%s(instanceID)", suffix)
	w.Line(2, "// A stream that gets no slot never opens: the event loop and the")
	w.Line(2, "// close hook run only once this hook has returned nil,")
	w.Line(2, "// which is why neither of them checks again.")
	w.Line(2, "if slot == nil {")
	w.Line(3, "return errStateAtCapacity")
	w.Line(2, "}")
	if p.StreamOpen == nil {
		// Nothing between the allocation and the return can fail,
		// so there is nothing to give back.
		w.Line(2, "slot.mu.Lock()")
		w.Line(2, "defer slot.mu.Unlock()")
		w.Line(2, "return nil")
		w.Line(1, "},")
		return
	}
	// The hook the user wrote can return an error or panic. Either way this
	// stream never opens and never closes, which leaves nothing else to return
	// the instance. Only this defer does. It runs after the unlock below,
	// whose mutex the release takes.
	w.Line(2, "// The close hook is wired up only once this one has returned nil.")
	w.Line(2, "// An open that ends any other way gives the instance back here,")
	w.Line(2, "// since nothing else is left to do it.")
	w.Line(2, "opened := false")
	w.Line(2, "defer func() {")
	w.Line(3, "if !opened {")
	w.Linef(4, "s.release%s(instanceID, slot)", suffix)
	w.Line(3, "}")
	w.Line(2, "}()")
	w.Line(2, "slot.mu.Lock()")
	w.Line(2, "defer slot.mu.Unlock()")
	dispatchVar := ""
	if p.StreamOpen.InputDispatch != nil {
		dispatchVar = "dispatchOpen"
	}
	if p.StreamOpen.OutputErr != nil {
		w.Raw("\t\tif err := ")
	} else {
		w.Raw("\t\t")
	}
	w.writeCallExpr(
		"p", "StreamOpen",
		handlerInputArgsWithDispatchVar(p.StreamOpen, false, dispatchVar),
	)
	if p.StreamOpen.OutputErr != nil {
		w.Raw("; err != nil {\n")
		w.Line(3, "return err")
		w.Line(2, "}")
	} else {
		w.Byte('\n')
	}
	w.Line(2, "opened = true")
	w.Line(2, "return nil")
	w.Line(1, "},")
}

func (w *Writer) writePageStreamCloseHook(p *model.Page) {
	// Stateful page: always emit an onClose closure so the instance is given back,
	// even when the user did not define StreamClose.
	if p.State != nil {
		w.writeStatefulStreamCloseHook(p)
		return
	}
	if p.StreamClose == nil {
		w.Line(1, "nil,")
		return
	}
	dispatchVar := ""
	if p.StreamClose.InputDispatch != nil {
		dispatchVar = "dispatchClosed"
	}
	w.Line(1, "func(streamID uint64) {")
	if p.StreamClose.OutputErr != nil {
		w.Raw("\t\tif err := ")
		w.writeCallExpr(
			"p", "StreamClose",
			handlerInputArgsWithDispatchVar(p.StreamClose, false, dispatchVar),
		)
		w.Raw("; err != nil {\n")
		w.Raw("\t\t\ts.logErr(\"handling ")
		w.Raw(p.TypeName)
		w.Raw(".StreamClose\", err)\n")
		w.Line(2, "}")
	} else {
		w.Raw("\t\t")
		w.writeCallExpr(
			"p", "StreamClose",
			handlerInputArgsWithDispatchVar(p.StreamClose, false, dispatchVar),
		)
		w.Byte('\n')
	}
	w.Line(1, "},")
}

// writeStatefulStreamCloseHook emits the onClose closure for a stateful page.
// If the user defined StreamClose, it runs under the slot mutex.
// In all cases the state goes back to the pool before this returns.
//
// This closure runs on the watchdog goroutine, outside net/http.
// handleStreamRequest recovers a panic raised here, which leaves this one
// responsible for unlocking the slot and releasing the instance on that path as well.
// Both are deferred. A StreamClose that reads state still runs only
// while the slot is alive.
func (w *Writer) writeStatefulStreamCloseHook(p *model.Page) {
	suffix := stateSuffix(p.State)
	w.Line(1, "func(streamID uint64) {")

	// The slot is taken only to call the user's hook under it. Without a hook
	// there is nothing to serialize, and the release below takes the slot on its own.
	if p.StreamClose == nil {
		w.Linef(2, "s.release%s(instanceID, slot)", suffix)
		w.Line(1, "},")
		return
	}

	w.Line(2, "// Both of these run even when the hook below panics.")
	w.Line(2, "// Deferred calls unwind in reverse, which puts the release after the unlock,")
	w.Line(2, "// and the release takes this same mutex.")
	w.Linef(2, "defer s.release%s(instanceID, slot)", suffix)
	w.Line(2, "slot.mu.Lock()")
	w.Line(2, "defer slot.mu.Unlock()")

	ind := 2
	guard := p.StreamClose.InputState != nil
	if guard {
		w.Line(2, "if !slot.dead {")
		ind = 3
	}
	tabs := strings.Repeat("\t", ind)

	if p.StreamClose != nil {
		dispatchVar := ""
		if p.StreamClose.InputDispatch != nil {
			dispatchVar = "dispatchClosed"
		}
		if p.StreamClose.OutputErr != nil {
			w.Raw(tabs)
			w.Raw("if err := ")
			w.writeCallExpr(
				"p", "StreamClose",
				handlerInputArgsWithDispatchVar(p.StreamClose, false, dispatchVar),
			)
			w.Raw("; err != nil {\n")
			w.Raw(tabs)
			w.Raw("\ts.logErr(\"handling ")
			w.Raw(p.TypeName)
			w.Raw(".StreamClose\", err)\n")
			w.Line(ind, "}")
		} else {
			w.Raw(tabs)
			w.writeCallExpr(
				"p", "StreamClose",
				handlerInputArgsWithDispatchVar(p.StreamClose, false, dispatchVar),
			)
			w.Byte('\n')
		}
	}
	if guard {
		w.Line(2, "}")
	}

	w.Line(1, "},")
}

// writePageGETStreamAnonHandler generates the anonymous stream handler for a page.
func (w *Writer) writePageGETStreamAnonHandler(
	p *model.Page, m *model.App, appPkg string,
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

	if fields := pageSignalSubjectFields(p, w.eventMap); len(fields) > 0 {
		w.writeSubjectSignalsRead(fields)
	}

	if p.StreamOpen != nil && p.StreamOpen.InputSignals != nil {
		w.Line(0, "")
		w.Raw("\tvar signals ")
		w.Raw(renderSignalsType(p.StreamOpen.InputSignals, m))
		w.Byte('\n')
		w.Line(1, "if err := datastar.ReadSignals(r, &signals); err != nil {")
		w.Line(2, `s.httpErrBad(w, "reading signals", err)`)
		w.Line(2, "return")
		w.Line(1, "}")
	}
	if p.StreamOpen != nil && p.StreamOpen.InputDispatch != nil {
		w.writeDispatchClosureAs(
			"dispatchOpen", p.StreamOpen.InputDispatch, appPkg,
			"context.WithoutCancel(r.Context())",
		)
	}
	if p.StreamClose != nil && p.StreamClose.InputDispatch != nil {
		w.writeDispatchClosureAs(
			"dispatchClosed", p.StreamClose.InputDispatch, appPkg,
			"context.WithoutCancel(r.Context())",
		)
	}

	// Per-tab state is unrelated to the session. A signed-out client on
	// this endpoint holds an instance like any other client,
	// and the stream hooks below expect the same locals as the main handler.
	if p.State != nil {
		w.Line(0, "")
		w.writeVerifyInstanceIDHeader()
		w.writeStateCapacityCheck(p.State)
		if pageNeedsStateRouteKey(p, w.eventMap) {
			w.writeStateRouteKeyVar()
		}
		w.Linef(1, "var slot *%s", pageStateSlotTypeName(p))
	}

	// Page constructor.
	w.Raw("\n\tp := ")
	w.writePageConstructor(p, appPkg)
	w.Byte('\n')

	// evSubj call. The client holds no session, so the user id is empty and
	// only public subjects come back — but a page that scopes by signals
	// still subscribes by their values.
	anonSignalFields := pageSignalSubjectFields(p, w.eventMap)
	w.Raw("\ts.handleStreamRequest(w, r, sessToken, sess, evSubj")
	w.Raw(p.TypeName)
	w.Raw("(")
	if pageHasPrivateEvent(p, w.eventMap) {
		w.Raw("sess.UserID")
		if len(anonSignalFields) > 0 {
			w.Raw(", ")
		}
	}
	for i, sf := range anonSignalFields {
		if i > 0 {
			w.Raw(", ")
		}
		w.Raw("subjSignals.")
		w.Raw(sf.Name)
	}
	w.Raw("),\n")
	w.writePageStreamOpenHook(p)
	w.writePageStreamCloseHook(p)
	w.Line(1, "func(")
	w.Line(2, "streamID uint64,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan msgbroker.Message,")
	w.Line(1, ") {")
	w.Line(2, "for msg := range ch {")

	// Only public events reach an anonymous stream.
	var publicHandlers []*model.EventHandler
	needsPrefixMatch := false
	for _, eh := range p.EventHandlers {
		ev := w.eventMap[eh.EventTypeName]
		if ev == nil || ev.IsPrivate() {
			continue
		}
		publicHandlers = append(publicHandlers, eh)
		if evUsesPrefixMatch(ev) {
			needsPrefixMatch = true
		}
	}

	// An event matched by prefix cannot be compared against: its constant is
	// the pattern the stream subscribed by, and a message carries the values.
	if needsPrefixMatch {
		w.Line(3, "switch {")
	} else {
		w.Line(3, "switch msg.Subject {")
	}
	for _, eh := range publicHandlers {
		w.writeStreamEventCase(p, eh, w.eventMap[eh.EventTypeName], appPkg,
			!needsPrefixMatch)
	}
	w.Line(3, "}")

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

	// Stateful action: verify Datapages-Instance header and find the live
	// slot. On a miss the client is told to reconnect and retry,
	// before the request body is read. The mutex that serializes the
	// handlers of this state is taken further down, once nothing is left to read.
	if h.InputState != nil {
		w.writeVerifyInstanceIDHeader()
		w.writeLookupSlotOrReject(p.State)
		if h.InputStateID != nil {
			w.writeStateRouteKeyVar()
		}
	}

	// Auth.
	needsToken := h.InputSessionToken != nil || h.OutputCloseSession != nil
	headNeedsSess := h.OutputBody != nil && m.GlobalHeadGenerator != nil &&
		m.GlobalHeadGenerator.InputSession
	headNeedsToken := h.OutputBody != nil && m.GlobalHeadGenerator != nil &&
		m.GlobalHeadGenerator.InputSessionToken
	switch {
	case h.InputSession != nil || needsToken || headNeedsSess || headNeedsToken:
		// Each part of the session helper's result is taken only when the handler,
		// or the app-wide head, has a use for it. A local nobody
		// reads is a package that does not compile.
		sessVar := "_"
		if h.InputSession != nil || headNeedsSess {
			sessVar = "sess"
		}
		if needsToken || headNeedsToken {
			w.Linef(1, "%s, sessToken, ok := s.auth(w, r)", sessVar)
		} else {
			w.Linef(1, "%s, _, ok := s.auth(w, r)", sessVar)
		}
		w.Line(1, "if !ok {")
		w.Line(2, "return")
		w.Line(1, "}")
	case needsCSRFOnly(h, m):
		w.writeCSRFOnlyCheck()
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

	// The request is read; claim the state for the duration of the handler.
	// Before the SSE generator, which commits the response headers and would
	// leave the rejection below nowhere to go.
	if h.InputState != nil {
		w.writeLockSlotOrReject()
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
			w.writeActionErrCheck(p, h, sseRef)
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
		w.writeActionErrCheck(p, h, sseRef)
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
			w.writeGenericHeadCall(
				m.GlobalHeadGenerator, appPkg, hasSessionInput(h), false,
			)
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
			w.Raw("genericHead, ")
		}
		// An action may return a head of its own, which the response it
		// renders has to carry. Anything else leaves the value the handler
		// returned unused, which is also a package that does not compile.
		if h.OutputHead != nil {
			w.Raw(h.OutputHead.Name)
		} else {
			w.Raw("nil")
		}
		w.Raw(", ")
		w.Raw(h.OutputBody.Name)
		w.Raw(", nil, nil,\n")
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

// writeActionErrCheck emits the error-handling body for action handlers.
// All errors are routed through httpErrIntern, which calls RecoverError
// if available and falls back to httperr sentinel checks or 500.
func (w *Writer) writeActionErrCheck(
	p *model.Page, h *model.Handler, sseRef string,
) {
	w.Raw("\t\ts.httpErrIntern(w, r, ")
	w.Raw(sseRef)
	w.Raw(", \"handling action ")
	w.Raw(p.TypeName)
	w.Byte('.')
	w.Raw(h.Name)
	w.Raw("\", err)\n")
	w.Line(2, "return")
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

// writeParseField emits code that parses a raw string value into a typed struct field.
// For writeReadQuery the raw variable is named "q" (from the if-guard);
// for writeReadPath it is "v" (set before the call).
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
