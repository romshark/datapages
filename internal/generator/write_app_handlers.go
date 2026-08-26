package generator

import (
	"go/types"
	"slices"
	"strings"

	"github.com/romshark/datapages/internal/gotypes"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/routepattern"
	"github.com/romshark/datapages/internal/structtag"
)

// stateArgExpr returns the expression a stateful handler receives its state as.
// The slot holds the checked-out *T, the handler takes datapages.State[T].
// The wrapper is that same pointer: nothing is copied.
func stateArgExpr(appPkgQual, stateTypeName string) string {
	return "datapages.State[" + appPkgQual + "." + stateTypeName +
		"]{Values: slot.state}"
}

// handlerArgVar maps an InputKind constant to the local variable name
// used in generated code. skipSSE causes SSE to be omitted (for app-level actions).
// stateExpr is what a state input is passed as, built by stateArgExpr.
func handlerArgVar(kind string, skipSSE bool, stateExpr string) string {
	switch kind {
	case model.InputKindRequest:
		return "r"
	case model.InputKindStreamID:
		return "streamID"
	case model.InputKindSSE:
		if skipSSE {
			return ""
		}
		// Handlers receive datapages.SSE, never the raw Datastar generator.
		return "dpsse.New(sse)"
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
		// The generated wrapper holds the checked-out *T as slot.state.
		return stateExpr
	case model.InputKindStateID:
		// The derived routing key of the tab, not the instance id.
		// See writeStateRouteKeyVar.
		return "stateID"
	default:
		return ""
	}
}

// outputVar returns the generated variable name for an output.
// Outputs are matched by type, so the name the app picked is not the one
// generated code binds them to.
func outputVar(out *model.Output) string {
	switch out.Kind {
	case model.OutputKindErr:
		return "err"
	case model.OutputKindBody:
		return "body"
	case model.OutputKindHead:
		return "head"
	case model.OutputKindRedirect:
		return "redirect"
	case model.OutputKindNewSession:
		return "newSession"
	case model.OutputKindCloseSession:
		return "closeSession"
	case model.OutputKindEnableBgStream:
		return "enableBackgroundStreaming"
	case model.OutputKindDisableRefresh:
		return "disableRefreshAfterHidden"
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
		outs = append(outs, outputVar(get.OutputBody.Output))
	}
	if get.OutputHead != nil {
		outs = append(outs, outputVar(get.OutputHead.Output))
	}
	if h.OutputRedirect != nil {
		outs = append(outs, outputVar(h.OutputRedirect))
	}
	if h.OutputEnableBgStream != nil {
		outs = append(outs, outputVar(h.OutputEnableBgStream))
	}
	if h.OutputDisableRefresh != nil {
		outs = append(outs, outputVar(h.OutputDisableRefresh))
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
		outs = append(outs, outputVar(h.OutputBody.Output))
	}
	if h.OutputCloseSession != nil {
		outs = append(outs, outputVar(h.OutputCloseSession))
	}
	if h.OutputRedirect != nil {
		outs = append(outs, outputVar(h.OutputRedirect))
	}
	if h.OutputNewSession != nil {
		outs = append(outs, outputVar(h.OutputNewSession))
	}
	if h.OutputEnableBgStream != nil {
		outs = append(outs, outputVar(h.OutputEnableBgStream))
	}
	if h.OutputDisableRefresh != nil {
		outs = append(outs, outputVar(h.OutputDisableRefresh))
	}
	if h.OutputErr != nil {
		outs = append(outs, "err")
	}
	return outs
}

// handlerInputArgs builds the argument list for a handler call
// in the order defined by h.OrderedInputs. dispatchPrefix names the dispatch
// closures the call passes, see dispatchVarName.
func handlerInputArgs(
	h *model.Handler, skipSSE bool, dispatchPrefix, appPkgQual string,
) []string {
	stateExpr := ""
	if h.InputState != nil {
		stateExpr = stateArgExpr(appPkgQual, h.InputState.StateTypeName)
	}
	if len(h.OrderedInputs) > 0 {
		args := make([]string, 0, len(h.OrderedInputs))
		for _, inp := range h.OrderedInputs {
			if inp.Kind == model.InputKindDispatch {
				args = append(args, dispatchArgVar(h, inp, dispatchPrefix))
				continue
			}
			if v := handlerArgVar(inp.Kind, skipSSE, stateExpr); v != "" {
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
		args = append(args, "dpsse.New(sse)")
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
	for _, d := range h.InputDispatches {
		args = append(args, dispatchVarName(dispatchPrefix, d.EventTypeName))
	}
	return args
}

// dispatchArgVar returns the closure variable a dispatch input is passed as.
func dispatchArgVar(h *model.Handler, inp *model.Input, prefix string) string {
	for _, d := range h.InputDispatches {
		if d.Input == inp {
			return dispatchVarName(prefix, d.EventTypeName)
		}
	}
	return "nil"
}

// eventHandlerInputArgs builds the argument list for an event handler call
// in the order defined by eh.OrderedInputs.
func eventHandlerInputArgs(
	eh *model.EventHandler, eventVar, appPkgQual string,
) []string {
	stateExpr := ""
	if eh.InputState != nil {
		stateExpr = stateArgExpr(appPkgQual, eh.InputState.StateTypeName)
	}
	if len(eh.OrderedInputs) > 0 {
		args := make([]string, 0, len(eh.OrderedInputs))
		for _, inp := range eh.OrderedInputs {
			v := handlerArgVar(inp.Kind, false, stateExpr)
			if inp.Kind == model.InputKindEvent {
				v = eventVar
			}
			if v != "" {
				args = append(args, v)
			}
		}
		return args
	}
	// Fallback for manually constructed EventHandler (e.g. in tests).
	var args []string
	if eh.InputEvent != nil {
		args = append(args, eventVar)
	}
	if eh.InputSSE != nil {
		args = append(args, "dpsse.New(sse)")
	}
	if eh.InputStreamID != nil {
		args = append(args, "streamID")
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
	if needsSession {
		hasBody = true
		w.Line(1, "sess, _, ok := s.ReadSession(w, r)")
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
		w.writeReadPath(h.InputPath, m, h.Route)
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
		w.Line(1, `if httpread.QueryHas(r.URL.RawQuery, "datastar") {`)
		w.Line(2, "if err := datastar.ReadSignals(r, &"+varSignals+"); err != nil {")
		w.Line(3, `s.HTTPErrBad(w, "reading signals", err)`)
		w.Line(3, "return")
		w.Line(2, "}")
		w.Line(1, "}")
	}

	// Dispatch closures.
	if len(h.InputDispatches) > 0 {
		hasBody = true
		w.writeDispatchers(h, "dispatch", "r.Context()")
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
	w.writeGETMethodCall(p, m)

	w.Line(0, "}")
}

// writeSessionOutputs emits what a handler's newSession and closeSession
// outputs ask for. A handler that returns them and is answered by neither has
// them declared and unused, which is a package that does not compile.
// writeSubjectSignalsRead emits the read of the signal values a page subscribes by.
// Both stream handlers need it: the subjects a stream subscribes to do not depend on
// whether the client holds a session.
func (w *Writer) writeSubjectSignalsRead(signalFields []model.SubjectField) {
	idents := signalIdents(signalFields)
	w.Line(0, "")
	w.Line(1, "var subjSignals struct {")
	for i, sf := range signalFields {
		w.Raw("\t\t")
		w.Raw(idents[i])
		w.Raw(` string `)
		w.Raw("`json:\"")
		w.Raw(sf.SignalName)
		w.Raw("\"`")
		w.Byte('\n')
	}
	w.Line(1, "}")
	w.Line(1, "if err := datastar.ReadSignals(r, &subjSignals); err != nil {")
	w.Line(2, `s.HTTPErrBad(w, "reading signals", err)`)
	w.Line(2, "return")
	w.Line(1, "}")
	for i, sf := range signalFields {
		w.Raw("\tif subjSignals.")
		w.Raw(idents[i])
		w.Raw(" == \"\" {\n")
		w.Raw("\t\ts.HTTPErrBad(w, \"invalid signal\",\n")
		w.Raw("\t\t\tfmt.Errorf(\"signal %q must not be empty\", ")
		w.writeQuoted(sf.SignalName)
		w.Raw("))\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}
}

func (w *Writer) writeGETMethodCall(p *model.Page, m *model.App) {
	h := p.GET.Handler

	// Build output list in user-defined order.
	outs := handlerGETOutputVars(h, p.GET)

	// enableBackgroundStreaming is used in bodyAttrs (when there's no
	// disableRefresh) and in bodySuffix (when the page has a stream).
	// If neither applies, blank it to avoid an unused-variable error.
	if h.OutputEnableBgStream != nil &&
		h.OutputDisableRefresh != nil && !pageHasStream(p) {
		for i, o := range outs {
			if o == outputVar(h.OutputEnableBgStream) {
				outs[i] = "_"
				break
			}
		}
	}

	// Build input args in user-defined order.
	args := handlerInputArgs(h, false, "dispatch", w.appPkgQual)

	w.Byte('\t')
	w.writeCommaSep(outs)
	w.Raw(" := ")
	w.writeCallExpr("p", "GET", args)
	w.Byte('\n')

	if h.OutputErr != nil {
		w.Line(1, "if err != nil {")
		if m.PageError500 != nil && p == m.PageError500 {
			// httpErrIntern renders PageError500. The error page can't use it.
			w.Raw("\t\ts.httpErrFinal(w, \"handling ")
		} else {
			w.Raw("\t\ts.httpErrIntern(w, r, nil, \"handling ")
		}
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
		w.Raw("\tif httpserve.Redirect(w, r, ")
		w.Raw(outputVar(h.OutputRedirect))
		w.Raw(") {\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Generic head.
	if gh := m.GlobalHeadGenerator; gh != nil {
		hasSess := h.InputSession != nil || gh.InputSession
		w.writeGenericHeadCall(gh, hasSess)
	}

	// Body attrs and suffix.
	hasBodySuffix := w.writeGETBodyAttrs(p)

	headArg := "nil"
	if p.GET.OutputHead != nil {
		headArg = outputVar(p.GET.OutputHead.Output)
	}

	bodyName := "body"
	if p.GET.OutputBody != nil {
		bodyName = outputVar(p.GET.OutputBody.Output)
	}

	w.Line(0, "")
	w.Line(1, "if err := s.writeHTML(")
	w.Raw("\t\tw, r, ")
	if m.Session != nil {
		sessArg := "sess"
		headNeedsSession := m.GlobalHeadGenerator != nil &&
			m.GlobalHeadGenerator.InputSession
		if p.PageSpecialization == model.PageTypeError500 ||
			(!hasSessionInput(h) && !headNeedsSession) {
			sessArg = w.sessionType + "{}"
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
	w.Raw("\t\ts.LogErr(\"rendering ")
	w.Raw(p.TypeName)
	w.Raw("\", err)\n")
	w.Line(2, "return")
	w.Line(1, "}")
}

func hasSessionInput(h *model.Handler) bool {
	return h.InputSession != nil
}

// writeSessionOutputs emits what a handler's newSession and closeSession
// outputs ask for. Both set a cookie, so both run before the response body.
//
// A handler that returns a session and never has it acted on leaves the value unused,
// which is a generated package that does not compile.
func (w *Writer) writeSessionOutputs(h *model.Handler) {
	if h.OutputCloseSession != nil {
		w.Raw("\tif ")
		w.Raw(outputVar(h.OutputCloseSession))
		w.Raw(" {\n")
		w.Line(2, "if err := s.CloseSession(w, r, sessToken); err != nil {")
		w.Line(3, `s.httpErrIntern(w, r, nil, "removing session", err)`)
		w.Line(3, "return")
		w.Line(2, "}")
		w.Line(1, "}")
	}
	if h.OutputNewSession != nil {
		w.Raw("\tif j := ")
		w.Raw(outputVar(h.OutputNewSession))
		w.Raw("; j.UserID != \"\" {\n")
		w.Raw("\t\tif err := s.CreateSession(w, r, ")
		w.Raw(outputVar(h.OutputNewSession))
		w.Raw("); err != nil {\n")
		w.Line(3, `s.httpErrIntern(w, r, nil, "creating session", err)`)
		w.Line(3, "return")
		w.Line(2, "}")
		w.Line(1, "}")
	}
}

// writeGenericHeadCall emits: genericHead := s.app.Head(r[, sess])
// hasSess indicates whether a "sess" variable is in scope.
func (w *Writer) writeGenericHeadCall(gh *model.GlobalHead, hasSess bool) {
	w.Raw("\tgenericHead := s.app.Head(")
	for i, kind := range gh.OrderedInputs {
		if i > 0 {
			w.Raw(", ")
		}
		if kind == model.InputKindRequest {
			w.Raw("r")
			continue
		}
		if hasSess {
			w.Raw("sess")
			continue
		}
		w.Raw(w.sessionType)
		w.Raw("{}")
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
			rs := structtag.ReflectSignalTagValue(f.Tag)
			if rs != "" {
				reflectFields = append(reflectFields, reflectSignalField{
					SignalName: rs,
					FieldName:  f.Name,
					Type:       f.Type,
					QueryTag:   structtag.QueryTagValue(f.Tag),
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
		w.Raw(outputVar(h.OutputDisableRefresh))
		w.Raw(" {\n")
		w.Line(3, "httpserve.WriteReloadOnVisibility(w)")
		w.Line(2, "}")
	} else if hasEnableBgStream {
		w.Raw("\t\tif !")
		w.Raw(outputVar(h.OutputEnableBgStream))
		w.Raw(" {\n")
		w.Line(3, "httpserve.WriteReloadOnVisibility(w)")
		w.Line(2, "}")
	} else {
		w.Line(2, "httpserve.WriteReloadOnVisibility(w)")
	}

	// Reflect signal attrs.
	for _, f := range reflectFields {
		fi := structFieldInfo{Name: f.FieldName, Type: f.Type}
		if gotypes.IsString(f.Type) {
			w.Line(0, "")
			w.Raw("\t\t_, _ = io.WriteString(w, `data-signals:")
			w.Raw(f.SignalName)
			w.Raw("=\"'`)\n")
			w.Raw("\t\thtmlattr.WriteSignalString(w, " + varQuery + ".")
			w.Raw(f.FieldName)
			w.Raw(")\n")
			w.Line(2, "_, _ = io.WriteString(w, `'\"`)")
		} else {
			w.Line(0, "")
			w.Raw("\t\t_, _ = io.WriteString(w, `data-signals:")
			w.Raw(f.SignalName)
			w.Raw("=\"`)\n")
			w.Raw("\t\thtmlattr.WriteSignalValue(w, ")
			w.writeFieldToString(varQuery, fi)
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
		streamPath := routepattern.StreamPath(p.Route)
		if hasPrivate && h.InputSession != nil {
			if hasAnonStream {
				// Mixed: authenticated -> "/_$/"; anonymous -> "/_$/anon/"
				// Need to handle path variables.
				if h.InputPath != nil {
					// Dynamic path.
					w.Line(0, "")
					w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
					w.writeStreamPathSegments(p.Route, h.InputPath)
					w.Line(2, `if sess.UserID() != "" {`)
					w.Line(3, "_, _ = io.WriteString(w, `_$/')\"`)")
					w.Line(2, "} else {")
					w.Line(3, "_, _ = io.WriteString(w, `_$/anon/')\"`)")
					w.Line(2, "}")
				} else {
					w.Line(0, "")
					w.Line(2, "_, _ = io.WriteString(w, `data-init=\"@get('`)")
					w.Line(2, `if sess.UserID() != "" {`)
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
						w.Line(2, `if sess.UserID() != "" {`)
						w.Line(3, "_, _ = io.WriteString(w, `_$/'`)")
						w.Raw("\t\t\tif ")
						w.Raw(outputVar(h.OutputEnableBgStream))
						w.Raw(" {\n")
						w.Line(4, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
						w.Line(3, "} else {")
						w.Line(4, "_, _ = io.WriteString(w, `)\"`)")
						w.Line(3, "}")
						w.Line(2, "}")
					} else {
						w.Line(2, `if sess.UserID() != "" {`)
						w.Line(3, "_, _ = io.WriteString(w, `_$/')\"`)")
						w.Line(2, "}")
					}
				} else if hasEnableBgStream {
					w.Line(0, "")
					w.Line(2, `if sess.UserID() != "" {`)
					w.Raw("\t\t\t_, _ = io.WriteString(w, `data-init=\"@get('")
					w.Raw(streamPath)
					w.Raw("'`)\n")
					w.Raw("\t\t\tif ")
					w.Raw(outputVar(h.OutputEnableBgStream))
					w.Raw(" {\n")
					w.Line(4, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
					w.Line(3, "} else {")
					w.Line(4, "_, _ = io.WriteString(w, `)\"`)")
					w.Line(3, "}")
					w.Line(2, "}")
				} else {
					w.Line(0, "")
					w.Line(2, `if sess.UserID() != "" {`)
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
					w.Line(2, "_, _ = io.WriteString(w, `_$/'`)")
					w.Raw("\t\tif ")
					w.Raw(outputVar(h.OutputEnableBgStream))
					w.Raw(" {\n")
					w.Line(3, "_, _ = io.WriteString(w, `,{openWhenHidden:true})\"`)")
					w.Line(2, "} else {")
					w.Line(3, "_, _ = io.WriteString(w, `)\"`)")
					w.Line(2, "}")
				} else {
					w.Line(2, "_, _ = io.WriteString(w, `_$/')\"`)")
				}
			} else if hasEnableBgStream {
				w.Line(0, "")
				w.Raw("\t\t_, _ = io.WriteString(w, `data-init=\"@get('")
				w.Raw(streamPath)
				w.Raw("'`)\n")
				w.Raw("\t\tif ")
				w.Raw(outputVar(h.OutputEnableBgStream))
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
				if tag := structtag.PathTagValue(f.Tag); tag != "" {
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
					w.writeFieldToString(varPath, f)
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

// writeStreamPathSegments writes the page route with its path values filled in.
// The last literal of [routepattern.Segments] carries a trailing slash,
// which is why the caller appends the stream suffix without one.
func (w *Writer) writeStreamPathSegments(route string, pathInput *model.Input) {
	// Build a map from path: tag value to field info.
	fields := w.structFields(pathInput.Type.Resolved)
	tagToField := make(map[string]structFieldInfo, len(fields))
	for _, f := range fields {
		if tag := structtag.PathTagValue(f.Tag); tag != "" {
			tagToField[tag] = f
		}
	}
	// Build the path prefix up to the variable, then write the variable.
	literals, vars := routepattern.Segments(route)
	for i, lit := range literals {
		w.Raw("\t\t_, _ = io.WriteString(w, `")
		w.Raw(lit)
		w.Raw("`)\n")
		if i < len(vars) {
			f := tagToField[vars[i]]
			w.Raw("\t\thtmlattr.WritePathValue(w, ")
			w.writeFieldToString(varPath, f)
			w.Raw(")\n")
		}
	}
}

// writeFieldToString emits an expression that converts a struct field to a string.
// For string fields it emits "varName.FieldName"; for other types it wraps with
// strconv.Format* or fmt.Sprint.
func (w *Writer) writeFieldToString(varName string, f structFieldInfo) {
	ref := varName + "." + f.Name
	if gotypes.IsString(f.Type) {
		if gotypes.IsNamedString(f.Type) {
			w.Rawf("string(%s)", ref)
			return
		}
		w.Raw(ref)
	} else if gotypes.IsInt(f.Type) {
		_, unsigned := gotypes.IntParseInfo(f.Type)
		typeName := gotypes.IntTypeName(f.Type)
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
	} else if gotypes.IsFloat(f.Type) {
		bits := gotypes.FloatBits(f.Type)
		typeName := gotypes.FloatTypeName(f.Type)
		if typeName != "float64" {
			w.Rawf("strconv.FormatFloat(float64(%s), 'f', -1, %d)", ref, bits)
		} else {
			w.Rawf("strconv.FormatFloat(%s, 'f', -1, %d)", ref, bits)
		}
	} else if gotypes.IsBool(f.Type) {
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

	w.Line(1, "if !s.CheckDatastarRequest(w, r) {")
	w.Line(2, "return")
	w.Line(1, "}")

	needsAuth := pageStreamNeedsAuth(p, w.eventMap)
	hasPrivate := pageHasPrivateEvent(p, w.eventMap)
	hasSignalScoped := pageHasSignalScopedEvent(p, w.eventMap)
	if needsAuth {
		w.Line(1, "sess, sessToken, ok := s.ReadSession(w, r)")
		w.Line(1, "if !ok {")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	if hasPrivate {
		// Check if anon stream exists - if so, redirect unauthenticated to anon.
		hasAnon := pageHasAnonStream(p, w.eventMap)
		if hasAnon {
			w.Line(0, "")
			w.Line(1, `if sess.UserID() == "" {`)
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
			w.Line(1, `if sess.UserID() == "" {`)
			w.Line(2, "http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)")
			w.Line(2, "return")
			w.Line(1, "}")
		}
	}

	// Read signal-scoped subject values for subscription.
	signalFields := pageSignalSubjectFields(p, w.eventMap)
	signalIdents := signalIdents(signalFields)
	if hasSignalScoped {
		w.writeSubjectSignalsRead(signalFields)
	}

	if p.StreamOpen != nil && p.StreamOpen.InputSignals != nil {
		w.Line(0, "")
		w.Raw("\tvar signals ")
		w.Raw(renderSignalsType(p.StreamOpen.InputSignals, m))
		w.Byte('\n')
		w.Line(1, "if err := datastar.ReadSignals(r, &"+varSignals+"); err != nil {")
		w.Line(2, `s.HTTPErrBad(w, "reading signals", err)`)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	if p.StreamOpen != nil {
		w.writeDispatchers(p.StreamOpen, "dispatchOpen",
			"context.WithoutCancel(r.Context())")
	}
	if p.StreamClose != nil {
		w.writeDispatchers(p.StreamClose, "dispatchClosed",
			"context.WithoutCancel(r.Context())")
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
	if pageStreamCallsPage(p) {
		w.Raw("\n\tp := ")
		w.writePageConstructor(p, appPkg)
		w.Byte('\n')
	}

	// evSubj call.
	evSubjName := "evSubj" + p.TypeName
	w.Raw("\ts.handleStreamRequest(w, r,")
	if needsAuth {
		w.Raw(" sessToken, sess,")
	} else if w.usage.streamAuth {
		w.Raw(` "", `)
		w.Raw(w.sessionType)
		w.Raw(`{},`)
	}
	w.Raw(" ")
	w.Raw(evSubjName)
	switch {
	case pageHasStateIDScopedEvent(p, w.eventMap):
		w.Raw("(stateID),\n")
	case hasPrivate && hasSignalScoped:
		w.Raw("(sess.UserID()")
		for _, ident := range signalIdents {
			w.Raw(", subjSignals.")
			w.Raw(ident)
		}
		w.Raw("),\n")
	case hasPrivate:
		w.Raw("(sess.UserID()),\n")
	case hasSignalScoped:
		w.Raw("(")
		for i, ident := range signalIdents {
			if i > 0 {
				w.Raw(", ")
			}
			w.Raw("subjSignals.")
			w.Raw(ident)
		}
		w.Raw("),\n")
	default:
		w.Raw(",\n")
	}
	w.writePageStreamOpenHook(p)
	w.writePageStreamCloseHook(p)
	w.Line(1, "func(")
	w.Line(2, "streamID datapages.StreamID,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan messaging.Message,")
	w.Line(1, ") {")
	if len(p.EventHandlers) == 0 {
		w.Line(2, "for range ch {")
		w.Line(2, "}")
	} else {
		w.writeStreamEventVars(p.EventHandlers, appPkg)
		w.Line(2, "for msg := range ch {")
		// An event matched by prefix cannot be compared against: its constant
		// is the pattern the stream subscribed by, and a message carries the values.
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

// writeStreamEventVars declares the event of every case of the message loop.
// One declaration per stream keeps the decode of every message off the heap.
func (w *Writer) writeStreamEventVars(
	handlers []*model.EventHandler, appPkg string,
) {
	var declared []string
	for _, eh := range handlers {
		ev := w.eventMap[eh.EventTypeName]
		if ev == nil || slices.Contains(declared, ev.TypeName) {
			continue
		}
		declared = append(declared, ev.TypeName)
		w.Raw("\t\tvar ")
		w.Raw(eventVarName(ev.TypeName))
		w.Byte(' ')
		w.Raw(appPkg)
		w.Byte('.')
		w.Raw(ev.TypeName)
		w.Byte('\n')
	}
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

	eventVar := eventVarName(ev.TypeName)
	w.Byte('\t')
	w.Raw("\t\t\t")
	w.Raw(eventVar)
	w.Raw(" = ")
	w.Raw(appPkg)
	w.Byte('.')
	w.Raw(ev.TypeName)
	w.Raw("{}\n")
	w.Line(4, "if err := json.Unmarshal(msg.Data, &"+eventVar+"); err != nil {")
	w.Raw("\t\t\t\t\ts.LogErr(\"unmarshaling ")
	w.Raw(ev.TypeName)
	w.Raw(" JSON\", err)\n")
	w.Line(5, "continue")
	w.Line(4, "}")

	w.writeEventHandlerCall(p.TypeName, eh, "p", eventVar)
}

func (w *Writer) writeEventHandlerCall(
	ownerLabel string, eh *model.EventHandler, receiver, eventVar string,
) {
	// Build args in user-defined order.
	args := eventHandlerInputArgs(eh, eventVar, w.appPkgQual)

	methodName := "On" + eh.Name

	// Stateful event handlers run under the per-slot mutex to serialize with
	// concurrent action calls and with StreamOpen / StreamClose.
	// The loop can outlive the close that drops the state,
	// which is what the liveness check guards against.
	stateful := eh.InputState != nil
	if stateful {
		w.Line(4, "slot.mu.Lock()")
		w.Line(4, "if slot.dead {")
		w.Line(5, "slot.mu.Unlock()")
		w.Line(5, "continue")
		w.Line(4, "}")
		w.Line(4, "func() {")
		w.Line(5, "defer slot.mu.Unlock()")
	}
	ind := 4
	if stateful {
		ind = 5
	}
	tabs := strings.Repeat("\t", ind)

	if eh.OutputErr != nil {
		w.Raw(tabs)
		w.Raw("if err := ")
		w.writeMultilineCallExpr(receiver, methodName, args, ind)
		w.Raw("; err != nil {\n")
		w.Raw(tabs)
		w.Raw("\ts.LogErr(\"handling ")
		w.Raw(ownerLabel)
		w.Byte('.')
		w.Raw(methodName)
		w.Raw("\", err)\n")
		w.Line(ind, "}")
	} else {
		w.Raw(tabs)
		w.writeMultilineCallExpr(receiver, methodName, args, ind)
		w.Byte('\n')
	}

	if stateful {
		w.Line(4, "}()")
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
	w.Line(1, "func(")
	w.Line(2, "streamID datapages.StreamID,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator,")
	w.Line(1, ") error {")
	if p.StreamOpen.OutputErr != nil {
		w.Raw("\t\treturn ")
	} else {
		w.Raw("\t\t")
	}
	w.writeCallExpr(
		"p", "StreamOpen",
		handlerInputArgs(p.StreamOpen, false, "dispatchOpen", w.appPkgQual),
	)
	w.Byte('\n')
	if p.StreamOpen.OutputErr == nil {
		w.Line(2, "return nil")
	}
	w.Line(1, "},")
}

// writeStatefulStreamOpenHook emits the onOpen closure for a stateful page.
// The closure allocates a fresh slot, stashes it in the outer
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
	w.Line(2, "streamID datapages.StreamID,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator,")
	w.Line(1, ") error {")
	w.Linef(2, "slot = s.allocate%s(instanceID)", suffix)
	w.Line(2, "// A stream that gets no slot never opens: the event loop and the")
	w.Line(2, "// close hook run only once this hook has returned nil,")
	w.Line(2, "// which is why neither of them checks again.")
	w.Line(2, "if slot == nil {")
	w.Line(3, "return httpserve.ErrStateAtCapacity")
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
	if p.StreamOpen.OutputErr != nil {
		w.Raw("\t\tif err := ")
	} else {
		w.Raw("\t\t")
	}
	w.writeCallExpr(
		"p", "StreamOpen",
		handlerInputArgs(p.StreamOpen, false, "dispatchOpen", w.appPkgQual),
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
	w.Line(1, "func(streamID datapages.StreamID) {")
	if p.StreamClose.OutputErr != nil {
		w.Raw("\t\tif err := ")
		w.writeCallExpr(
			"p", "StreamClose",
			handlerInputArgs(p.StreamClose, false, "dispatchClosed", w.appPkgQual),
		)
		w.Raw("; err != nil {\n")
		w.Raw("\t\t\ts.LogErr(\"handling ")
		w.Raw(p.TypeName)
		w.Raw(".StreamClose\", err)\n")
		w.Line(2, "}")
	} else {
		w.Raw("\t\t")
		w.writeCallExpr(
			"p", "StreamClose",
			handlerInputArgs(p.StreamClose, false, "dispatchClosed", w.appPkgQual),
		)
		w.Byte('\n')
	}
	w.Line(1, "},")
}

// writeStatefulStreamCloseHook emits the onClose closure for a stateful page.
// If the user defined StreamClose, it runs under the slot mutex.
// In all cases the state is dropped before this returns.
//
// This closure runs outside the goroutine net/http recovers on.
// handleStreamRequest recovers a panic raised here, which leaves this one
// responsible for unlocking the slot and releasing the instance on that path as well.
// Both are deferred. A StreamClose that reads state still runs only
// while the slot is alive.
func (w *Writer) writeStatefulStreamCloseHook(p *model.Page) {
	suffix := stateSuffix(p.State)
	w.Line(1, "func(streamID datapages.StreamID) {")

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
		if p.StreamClose.OutputErr != nil {
			w.Raw(tabs)
			w.Raw("if err := ")
			w.writeCallExpr(
				"p", "StreamClose",
				handlerInputArgs(p.StreamClose, false, "dispatchClosed", w.appPkgQual),
			)
			w.Raw("; err != nil {\n")
			w.Raw(tabs)
			w.Raw("\ts.LogErr(\"handling ")
			w.Raw(p.TypeName)
			w.Raw(".StreamClose\", err)\n")
			w.Line(ind, "}")
		} else {
			w.Raw(tabs)
			w.writeCallExpr(
				"p", "StreamClose",
				handlerInputArgs(p.StreamClose, false, "dispatchClosed", w.appPkgQual),
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

	w.Line(1, "if !s.CheckDatastarRequest(w, r) {")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "sess, sessToken, ok := s.ReadSession(w, r)")
	w.Line(1, "if !ok {")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(0, "")
	w.Line(1, `if sess.UserID() != "" {`)
	w.Line(2, `s.HTTPErrBad(w, "authenticated client on anonymous stream", nil)`)
	w.Line(2, "return")
	w.Line(1, "}")

	// Read signal-scoped subject values for subscription.
	//
	// The anonymous stream carries what is public, and a signal-scoped event is public.
	// It therefore has to subscribe by the same values as the authenticated one,
	// which it cannot do without reading them.
	signalFields := pageSignalSubjectFields(p, w.eventMap)
	signalIdents := signalIdents(signalFields)
	if len(signalFields) > 0 {
		w.writeSubjectSignalsRead(signalFields)
	}

	if p.StreamOpen != nil && p.StreamOpen.InputSignals != nil {
		w.Line(0, "")
		w.Raw("\tvar signals ")
		w.Raw(renderSignalsType(p.StreamOpen.InputSignals, m))
		w.Byte('\n')
		w.Line(1, "if err := datastar.ReadSignals(r, &"+varSignals+"); err != nil {")
		w.Line(2, `s.HTTPErrBad(w, "reading signals", err)`)
		w.Line(2, "return")
		w.Line(1, "}")
	}
	if p.StreamOpen != nil {
		w.writeDispatchers(p.StreamOpen, "dispatchOpen",
			"context.WithoutCancel(r.Context())")
	}
	if p.StreamClose != nil {
		w.writeDispatchers(p.StreamClose, "dispatchClosed",
			"context.WithoutCancel(r.Context())")
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
	if pageStreamCallsPage(p) {
		w.Raw("\n\tp := ")
		w.writePageConstructor(p, appPkg)
		w.Byte('\n')
	}

	// evSubj call. The client holds no session. The user id is empty and only
	// public subjects come back, though a page that scopes by signals still
	// subscribes by their values.
	w.Raw("\ts.handleStreamRequest(w, r, sessToken, sess, evSubj")
	w.Raw(p.TypeName)
	w.Raw("(sess.UserID()")
	for _, ident := range signalIdents {
		w.Raw(", subjSignals.")
		w.Raw(ident)
	}
	w.Raw("),\n")
	w.writePageStreamOpenHook(p)
	w.writePageStreamCloseHook(p)
	w.Line(1, "func(")
	w.Line(2, "streamID datapages.StreamID,")
	w.Line(2, "sse *datastar.ServerSentEventGenerator, ch <-chan messaging.Message,")
	w.Line(1, ") {")
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

	w.writeStreamEventVars(publicHandlers, appPkg)
	w.Line(2, "for msg := range ch {")

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
		w.Line(1, "if !s.CheckDatastarRequest(w, r) {")
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
	needsToken := h.OutputCloseSession != nil
	headNeedsSess := h.OutputBody != nil && m.GlobalHeadGenerator != nil &&
		m.GlobalHeadGenerator.InputSession
	switch {
	case h.InputSession != nil || needsToken || headNeedsSess:
		// A local nobody reads is a package that does not compile.
		sessVar := "_"
		if h.InputSession != nil || headNeedsSess {
			sessVar = "sess"
		}
		if needsToken {
			w.Linef(1, "%s, sessToken, ok := s.ReadSession(w, r)", sessVar)
		} else {
			w.Linef(1, "%s, _, ok := s.ReadSession(w, r)", sessVar)
		}
		w.Line(1, "if !ok {")
		w.Line(2, "return")
		w.Line(1, "}")
	case needsCSRFOnly(h, m):
		w.writeCSRFOnlyCheck()
	}

	// Body size limit.
	if h.InputSignals != nil {
		w.Line(1, "r.Body = http.MaxBytesReader(w, r.Body, s.BodySizeLimit())")
	}

	// Read signals.
	if h.InputSignals != nil {
		w.Raw("\tvar signals ")
		w.Raw(renderSignalsType(h.InputSignals, m))
		w.Byte('\n')
		w.Line(1, "if err := datastar.ReadSignals(r, &"+varSignals+"); err != nil {")
		w.Line(2, `s.HTTPErrBad(w, "reading signals", err)`)
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Read query params.
	if h.InputQuery != nil {
		w.writeReadQuery(h.InputQuery, m)
	}

	// Read path params.
	if h.InputPath != nil {
		w.writeReadPath(h.InputPath, m, h.Route)
	}

	// The request is read; claim the state for the duration of the handler.
	// Before the SSE generator, which commits the response headers and would
	// leave the rejection below nowhere to go.
	if h.InputState != nil {
		w.writeLockSlotOrReject()
	}

	// Dispatch closures.
	w.writeDispatchers(h, "dispatch", "r.Context()")

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
	w.writeActionMethodCall(p, h, m)

	w.Line(0, "}")
}

func (w *Writer) writeActionMethodCall(
	p *model.Page, h *model.Handler, m *model.App,
) {
	// Build output list in user-defined order.
	outs := handlerOutputVars(h)

	// Build input args in user-defined order.
	args := handlerInputArgs(h, false, "dispatch", w.appPkgQual)

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

	// Close and create session.
	w.writeSessionOutputs(h)

	// Redirect.
	if h.OutputRedirect != nil {
		w.Raw("\tif httpserve.Redirect(w, r, ")
		w.Raw(outputVar(h.OutputRedirect))
		w.Raw(") {\n")
		w.Line(2, "return")
		w.Line(1, "}")
	}

	// Render body (if action returns templ.Component).
	if h.OutputBody != nil {
		if m.GlobalHeadGenerator != nil {
			w.writeGenericHeadCall(m.GlobalHeadGenerator, hasSessionInput(h))
		}
		w.Line(1, "if err := s.writeHTML(")
		w.Raw("\t\tw, r, ")
		if m.Session != nil {
			sessArg := "sess"
			headNeedsSession := m.GlobalHeadGenerator != nil &&
				m.GlobalHeadGenerator.InputSession
			if !hasSessionInput(h) && !headNeedsSession {
				sessArg = w.sessionType + "{}"
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
			w.Raw(outputVar(h.OutputHead.Output))
		} else {
			w.Raw("nil")
		}
		w.Raw(", ")
		w.Raw(outputVar(h.OutputBody.Output))
		w.Raw(", nil, nil,\n")
		w.Line(1, "); err != nil {")
		w.Raw("\t\ts.LogErr(\"rendering response of ")
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
// if available and falls back to the datapages error sentinels or 500.
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
	w.Raw("\tvar query ")
	w.Raw(renderQueryType(input, m))
	w.Byte('\n')
	fields := w.structFields(input.Type.Resolved)
	for _, f := range fields {
		tag := structtag.QueryTagValue(f.Tag)
		if isPlainString(f.Type) {
			w.Raw("\t" + varQuery + ".")
			w.Raw(f.Name)
			w.Raw(" = ")
			w.writeStringConv(f.Type, func() {
				w.Raw("httpread.QueryValue(r.URL.RawQuery, ")
				w.writeQuoted(tag)
				w.Raw(")")
			})
			w.Byte('\n')
		} else {
			w.Line(1, "{")
			w.Raw("\t\tif q := httpread.QueryValue(r.URL.RawQuery, ")
			w.writeQuoted(tag)
			w.Raw("); q != \"\" {\n")
			w.writeParseField(varQuery, "q", f, tag, "query parameter", 3)
			w.Line(2, "}")
			w.Line(1, "}")
		}
	}
}

func (w *Writer) writeReadPath(input *model.Input, m *model.App, route string) {
	w.Line(0, "")
	w.Raw("\tvar path ")
	w.Raw(renderPathType(input, m))
	w.Byte('\n')
	wildcard := wildcardVar(route)
	fields := w.structFields(input.Type.Resolved)
	for _, f := range fields {
		tag := structtag.PathTagValue(f.Tag)
		read := func() {
			if tag != "" && tag == wildcard {
				w.Raw("httpserve.WildcardPathValue(r, ")
			} else {
				w.Raw("r.PathValue(")
			}
			w.writeQuoted(tag)
			w.Raw(")")
		}
		if isPlainString(f.Type) {
			w.Raw("\t" + varPath + ".")
			w.Raw(f.Name)
			w.Raw(" = ")
			w.writeStringConv(f.Type, read)
			w.Byte('\n')
		} else {
			w.Line(1, "{")
			w.Raw("\t\tv := ")
			read()
			w.Byte('\n')
			w.writeParseField(varPath, "v", f, tag, "path parameter", 2)
			w.Line(1, "}")
		}
	}
}

// wildcardVar names the {name...} variable a route ends in, empty when it ends
// in anything else. It is the only path variable whose value carries the slash
// [github.com/romshark/datapages/runtime/httpserve.Core.ServeHTTP] appends.
func wildcardVar(route string) string {
	if !routepattern.EndsInWildcard(route) {
		return ""
	}
	last := ""
	for v := range routepattern.Vars(route) {
		last = v
	}
	return last
}

// writeParseField emits code that parses a raw string value into a typed struct field.
//
// varName is the struct being populated, raw is the variable holding the
// string to parse: "q" from the if-guard writeReadQuery emits, "v" as
// writeReadPath sets it before the call.
// label is "path parameter" or "query parameter" (for error messages).
// indent is the base indentation level for the generated code.
func (w *Writer) writeParseField(
	varName, raw string, f structFieldInfo, tag, label string, indent int,
) {
	tabs := func(n int) {
		for range n {
			w.Byte('\t')
		}
	}

	// UnmarshalText comes first: a type that declares one parses through it,
	// however its underlying kind reads.
	if gotypes.ImplementsTextUnmarshaler(f.Type) {
		tabs(indent)
		w.Rawf("if err := %s.%s.UnmarshalText([]byte(%s)); err != nil {\n",
			varName, f.Name, raw)
		tabs(indent + 1)
		w.Rawf("s.HTTPErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
		tabs(indent + 1)
		w.Raw("return\n")
		tabs(indent)
		w.Raw("}\n")
		return
	}

	if gotypes.IsInt(f.Type) {
		bits, unsigned := gotypes.IntParseInfo(f.Type)
		typeName := gotypes.IntTypeName(f.Type)
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
		w.Rawf("s.HTTPErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
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
			w.writeConv(convTypeName(f.Type, typeName), "uint64", "u")
		} else {
			w.writeConv(convTypeName(f.Type, typeName), "int64", "i")
		}
		w.Byte('\n')
	} else if gotypes.IsFloat(f.Type) {
		bits := gotypes.FloatBits(f.Type)
		typeName := gotypes.FloatTypeName(f.Type)
		tabs(indent)
		w.Rawf("f, err := strconv.ParseFloat(%s, %d)\n", raw, bits)
		tabs(indent)
		w.Raw("if err != nil {\n")
		tabs(indent + 1)
		w.Rawf("s.HTTPErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
		tabs(indent + 1)
		w.Raw("return\n")
		tabs(indent)
		w.Raw("}\n")
		tabs(indent)
		w.Raw(varName)
		w.Byte('.')
		w.Raw(f.Name)
		w.Raw(" = ")
		w.writeConv(convTypeName(f.Type, typeName), "float64", "f")
		w.Byte('\n')
	} else if gotypes.IsBool(f.Type) {
		tabs(indent)
		w.Rawf("b, err := strconv.ParseBool(%s)\n", raw)
		tabs(indent)
		w.Raw("if err != nil {\n")
		tabs(indent + 1)
		w.Rawf("s.HTTPErrBad(w, \"unexpected value for %s: %s\", err)\n", label, tag)
		tabs(indent + 1)
		w.Raw("return\n")
		tabs(indent)
		w.Raw("}\n")
		tabs(indent)
		w.Raw(varName)
		w.Byte('.')
		w.Raw(f.Name)
		w.Raw(" = ")
		w.writeConv(convTypeName(f.Type, "bool"), "bool", "b")
		w.Byte('\n')
	}
}

type reflectSignalField struct {
	SignalName string
	FieldName  string
	Type       types.Type
	QueryTag   string
}

// isPlainString reports a string field that parses by conversion alone.
// One that declares an UnmarshalText parses through it instead.
func isPlainString(t types.Type) bool {
	return gotypes.IsString(t) && !gotypes.ImplementsTextUnmarshaler(t)
}

// convTypeName is the type a parsed value is assigned as: the declared type
// when the field names one, the basic type otherwise.
// strconv returns a basic value, which a named field cannot take without a conversion.
func convTypeName(t types.Type, basic string) string {
	if _, isBasic := t.(*types.Basic); isBasic {
		return basic
	}
	return gotypes.QualifiedTypeName(t)
}

// writeConv writes expr converted to typeName, or expr alone when strconv
// already returns that type. parsed is what strconv hands back.
func (w *Writer) writeConv(typeName, parsed, expr string) {
	if typeName == parsed {
		w.Raw(expr)
		return
	}
	w.Raw(typeName + "(" + expr + ")")
}

// writeStringConv writes inner, converted to t when t is
// a string type with a name of its own.
func (w *Writer) writeStringConv(t types.Type, inner func()) {
	if !gotypes.IsNamedString(t) {
		inner()
		return
	}
	w.Raw(gotypes.QualifiedTypeName(t))
	w.Byte('(')
	inner()
	w.Byte(')')
}
