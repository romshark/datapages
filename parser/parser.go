package parser

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/parser/internal/methodkind"
	"github.com/romshark/datapages/parser/internal/paramvalidation"
	"github.com/romshark/datapages/parser/internal/structinspect"
	"github.com/romshark/datapages/parser/internal/structtag"
	"github.com/romshark/datapages/parser/internal/typecheck"
	"github.com/romshark/datapages/parser/model"
	"github.com/romshark/datapages/parser/validate"
)

func Parse(appPackagePath string) (app *model.App, errs Errors) {
	defer sortErrors(&errs)

	pkg, err := loadPackage(appPackagePath)
	if err != nil {
		errs.Err(err)
		return nil, errs
	}

	if pkg.Types == nil || pkg.TypesInfo == nil {
		// Include package errors only when we don't have type information
		for _, pe := range pkg.Errors {
			errs.ErrAt(posFromPackagesError(pe), pe)
		}
		errs.ErrAt(earliestPkgPos(pkg),
			errors.New("missing source package type information"))
		return nil, errs
	}

	ctx := newParseCtx(pkg)
	indexTypes(&ctx)
	collectEventTypeNames(&ctx)
	initApp(&ctx, &errs)
	firstPassTypes(&ctx, &errs)
	validateEvents(&ctx, &errs)
	secondPassEmbeds(&ctx, &errs)
	thirdPassMethods(&ctx, &errs)
	flattenPages(&ctx, &errs)
	validateRequiredHandlers(&ctx, &errs)
	finalizePages(&ctx)
	assignSpecialPages(&ctx, &errs)

	if !ctx.appTypeFound {
		return nil, errs
	}
	return ctx.app, errs
}

type parseCtx struct {
	pkg *packages.Package

	typeSpecByName map[string]*ast.TypeSpec
	docByType      map[string]*ast.CommentGroup
	genDocByType   map[string]*ast.CommentGroup

	// Set of declared valid EventXXX names (same package),
	// used for validating OnXXX param types.
	eventTypeNames map[string]struct{}

	pages     map[string]*model.Page
	abstracts map[string]*model.AbstractPage

	// recv -> event type name -> first handler position
	seenEvHandlerByRecv map[string]map[string]token.Pos

	// Non-error outputs per handler, used by buildHandlerGET.
	handlerOutputs map[*model.Handler][]*model.Output

	app          *model.App
	appTypeFound bool
	basePos      token.Position
}

func newParseCtx(pkg *packages.Package) parseCtx {
	return parseCtx{
		pkg:                 pkg,
		typeSpecByName:      map[string]*ast.TypeSpec{},
		docByType:           map[string]*ast.CommentGroup{},
		genDocByType:        map[string]*ast.CommentGroup{},
		eventTypeNames:      map[string]struct{}{},
		pages:               map[string]*model.Page{},
		abstracts:           map[string]*model.AbstractPage{},
		seenEvHandlerByRecv: map[string]map[string]token.Pos{},
		handlerOutputs:      map[*model.Handler][]*model.Output{},
		basePos:             earliestPkgPos(pkg),
	}
}

func indexTypes(ctx *parseCtx) {
	for _, f := range ctx.pkg.Syntax {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, s := range gd.Specs {
				ts, ok := s.(*ast.TypeSpec)
				if !ok {
					continue
				}
				name := ts.Name.Name
				ctx.typeSpecByName[name] = ts
				if ts.Doc != nil {
					ctx.docByType[name] = ts.Doc
				} else if gd.Doc != nil {
					ctx.genDocByType[name] = gd.Doc
				}
			}
		}
	}
}

func collectEventTypeNames(ctx *parseCtx) {
	for name := range ctx.typeSpecByName {
		if err := validate.EventTypeName(name); err == nil {
			ctx.eventTypeNames[name] = struct{}{}
		}
	}
}

func initApp(ctx *parseCtx, errs *Errors) {
	ctx.app = &model.App{Fset: ctx.pkg.Fset, PkgPath: ctx.pkg.PkgPath}
	if appTS, ok := ctx.typeSpecByName["App"]; ok {
		ctx.app.Expr = appTS.Name
		ctx.appTypeFound = true
		return
	}
	errs.ErrAt(ctx.basePos, ErrAppMissingTypeApp)
}

func firstPassTypes(ctx *parseCtx, errs *Errors) {
	for _, name := range slices.Sorted(maps.Keys(ctx.typeSpecByName)) {
		ts := ctx.typeSpecByName[name]

		if name == "Session" {
			firstPassSessionType(ctx, errs, ts)
			continue
		}

		// Only treat valid EventXXX as event types.
		if err := validate.EventTypeName(name); err == nil {
			firstPassEventType(ctx, errs, name, ts)
			continue
		}

		// Pages / abstracts are structs only.
		firstPassPageOrAbstractType(ctx, errs, name, ts)
	}
}

func firstPassSessionType(
	ctx *parseCtx, errs *Errors, ts *ast.TypeSpec,
) {
	typePos := ctx.pkg.Fset.Position(ts.Name.Pos())

	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		errs.ErrAt(typePos, ErrSessionNotStruct)
		return
	}

	info := ctx.pkg.TypesInfo
	t := info.TypeOf(st)
	if t == nil {
		errs.ErrAt(typePos, ErrSessionNotStruct)
		return
	}
	underlying, ok := t.Underlying().(*types.Struct)
	if !ok {
		errs.ErrAt(typePos, ErrSessionNotStruct)
		return
	}

	hasUserID, hasIssuedAt := false, false
	for f := range underlying.Fields() {
		switch {
		case f.Name() == "UserID" && typecheck.IsString(f.Type()):
			hasUserID = true
		case f.Name() == "IssuedAt" && typecheck.IsTimeTime(f.Type()):
			hasIssuedAt = true
		}
	}
	if !hasUserID {
		errs.ErrAt(typePos, ErrSessionMissingUserID)
	}
	if !hasIssuedAt {
		errs.ErrAt(typePos, ErrSessionMissingIssuedAt)
	}

	ctx.app.Session = &model.SessionType{Expr: ts.Name}
}

func firstPassEventType(
	ctx *parseCtx, errs *Errors, name string, ts *ast.TypeSpec,
) {
	typePos := ctx.pkg.Fset.Position(ts.Name.Pos())
	doc := pickDoc(name, ctx.docByType, ctx.genDocByType)

	subj, err := extractEventSubject(name, doc)
	if err != nil {
		switch err {
		case validate.ErrEventCommMissing:
			errs.ErrAt(typePos, &ErrorEventCommMissing{TypeName: name})
		case validate.ErrEventCommInvalid:
			commPos := eventCommInvalidPos(doc, name, ctx.pkg.Fset, typePos)
			errs.ErrAt(commPos, &ErrorEventCommInvalid{TypeName: name})
		case validate.ErrEventSubjectInvalid:
			subjPos := eventSubjectPos(doc, name, ctx.pkg.Fset, typePos)
			errs.ErrAt(subjPos, fmt.Errorf("%w: %s", ErrEventSubjectInvalid, name))
		default:
			// Defensive fallback: treat as invalid comment.
			errs.ErrAt(typePos, &ErrorEventCommInvalid{TypeName: name})
		}
		return
	}

	ctx.app.Events = append(ctx.app.Events, &model.Event{
		Expr:             ts.Name,
		TypeName:         name,
		Subject:          subj,
		HasTargetUserIDs: structinspect.HasTargetUserIDs(ts, ctx.pkg.TypesInfo),
	})
}

func extractEventSubject(
	typeName string, doc *ast.CommentGroup,
) (string, error) {
	// Validate first (sentinel errors).
	if err := validate.EventSubjectComment(typeName, doc); err != nil {
		return "", err
	}

	// Extract (validated => safe).
	for _, c := range doc.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		txt = strings.TrimSpace(txt)
		rest, ok := validate.CutEventIsPrefix(txt, typeName)
		if !ok {
			continue
		}
		// validate.EventSubjectCommentSubject guarantees:
		// - starts/ends with '"'
		// - non-empty payload
		if len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"' {
			return rest[1 : len(rest)-1], nil
		}
		break
	}
	// Should not happen if validation succeeded.
	return "", validate.ErrEventSubjectInvalid
}

// eventSubjectPos returns the position of the subject value (the quoted
// string after "is ") in the doc comment for an event type. Falls back
// to fallback when the comment cannot be located.
func eventSubjectPos(
	doc *ast.CommentGroup, typeName string,
	fset *token.FileSet, fallback token.Position,
) token.Position {
	if doc == nil || len(doc.List) == 0 {
		return fallback
	}
	c := doc.List[0]
	txt := c.Text
	// Find the subject within the raw comment text (including "// " prefix).
	// Look for " is " (with possible extra whitespace) after the type name.
	idx := strings.Index(txt, typeName)
	if idx < 0 {
		return fallback
	}
	// Skip past typeName, then whitespace, "is", then whitespace.
	off := idx + len(typeName)
	for off < len(txt) && (txt[off] == ' ' || txt[off] == '\t') {
		off++
	}
	if !strings.HasPrefix(txt[off:], "is") {
		return fallback
	}
	off += len("is")
	for off < len(txt) && (txt[off] == ' ' || txt[off] == '\t') {
		off++
	}
	pos := fset.Position(c.Pos())
	pos.Column += off
	return pos
}

// eventCommInvalidPos returns the position of the first unexpected token
// in an invalid event subject comment. When the type name matches but "is"
// is missing, it points at the token after the type name. When the type
// name doesn't match, it points at the start of the comment content.
func eventCommInvalidPos(
	doc *ast.CommentGroup, typeName string,
	fset *token.FileSet, fallback token.Position,
) token.Position {
	if doc == nil || len(doc.List) == 0 {
		return fallback
	}
	c := doc.List[0]
	txt := c.Text
	idx := strings.Index(txt, typeName)
	if idx < 0 {
		// Type name not found — point at the content start (after "// ").
		off := 0
		if strings.HasPrefix(txt, "//") {
			off = 2
			for off < len(txt) && (txt[off] == ' ' || txt[off] == '\t') {
				off++
			}
		}
		pos := fset.Position(c.Pos())
		pos.Column += off
		return pos
	}
	// Type name found — skip past it and whitespace, point at what follows.
	off := idx + len(typeName)
	for off < len(txt) && (txt[off] == ' ' || txt[off] == '\t') {
		off++
	}
	pos := fset.Position(c.Pos())
	pos.Column += off
	return pos
}

func firstPassPageOrAbstractType(
	ctx *parseCtx, errs *Errors, name string, ts *ast.TypeSpec,
) {
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return
	}

	if strings.HasPrefix(name, "Page") {
		typePos := ctx.pkg.Fset.Position(ts.Name.Pos())

		if err := validate.PageTypeName(name); err != nil {
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageNameInvalid, name))
		}
		if !structinspect.HasRequiredAppField(st, ctx.pkg.TypesInfo) {
			errs.ErrAt(typePos, &ErrorPageMissingFieldApp{TypeName: name})
		}
		if structinspect.HasDisallowedNamedFields(st) {
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageHasExtraFields, name))
		}

		route, found, ok := parseRoute(
			name, pickDoc(name, ctx.docByType, ctx.genDocByType),
		)
		if !found {
			errs.ErrAt(typePos, &ErrorPageMissingPathComm{TypeName: name})
		} else if !ok {
			errs.ErrAt(typePos, &ErrorPageInvalidPathComm{TypeName: name})
		} else if name == "PageIndex" && route != "/" {
			errs.ErrAt(typePos, &ErrorPageIndexPathMustBeRoot{Route: route})
		}

		ctx.pages[name] = &model.Page{
			Expr:               ts.Name,
			TypeName:           name,
			Route:              route,
			PageSpecialization: pageSpecialization(name),
		}
		return
	}

	// Abstract pages still require App *App.
	if !structinspect.HasRequiredAppField(st, ctx.pkg.TypesInfo) {
		return
	}
	ctx.abstracts[name] = &model.AbstractPage{
		Expr:     ts.Name,
		TypeName: name,
	}
}

func secondPassEmbeds(ctx *parseCtx, errs *Errors) {
	for _, pg := range ctx.pages {
		resolveEmbedsForStruct(ctx, errs, pg.TypeName, func(ap *model.AbstractPage) {
			pg.Embeds = append(pg.Embeds, ap)
		})
	}
	for _, ap := range ctx.abstracts {
		resolveEmbedsForStruct(ctx, errs, ap.TypeName, func(sub *model.AbstractPage) {
			ap.Embeds = append(ap.Embeds, sub)
		})
	}
}

func resolveEmbedsForStruct(
	ctx *parseCtx, errs *Errors, typeName string, add func(*model.AbstractPage),
) {
	ts := ctx.typeSpecByName[typeName]
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, emb := range structinspect.EmbeddedTypeNames(st) {
		if ap, ok := ctx.abstracts[emb]; ok {
			add(ap)
			continue
		}
		typePos := ctx.pkg.Fset.Position(ts.Name.Pos())
		errs.ErrAt(typePos,
			fmt.Errorf("%w: %s embeds %s", ErrPageHasExtraFields, typeName, emb))
	}
}

func thirdPassMethods(ctx *parseCtx, errs *Errors) {
	for _, f := range ctx.pkg.Syntax {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			recv := structinspect.ReceiverTypeName(fd.Recv.List[0].Type)

			// App hooks: (*App).Head, (*App).Recover500, and App-level actions.
			if recv == "App" {
				switch fd.Name.Name {
				case "Head":
					info := ctx.pkg.TypesInfo
					pos := ctx.pkg.Fset.Position(fd.Name.Pos())

					// Head must return exactly templ.Component.
					results := fd.Type.Results
					if results == nil || results.NumFields() != 1 ||
						!typecheck.IsTemplComponent(info.TypeOf(results.List[0].Type)) {
						errs.ErrAt(pos, ErrAppHeadMustReturnTemplComponent)
						continue
					}

					// Head must accept *http.Request as first param,
					// optionally followed by session and/or sessionToken.
					params := fd.Type.Params
					if params == nil || params.NumFields() < 1 ||
						!typecheck.IsPtrToNetHTTPReq(params.List[0].Type, info) {
						errs.ErrAt(pos, ErrAppHeadMustTakeRequest)
						continue
					}

					gh := &model.GlobalHead{Expr: fd.Name}
					valid := true
					for i := 1; i < params.NumFields(); i++ {
						f := params.List[i]
						switch {
						case typecheck.IsSessionType(f.Type, info):
							gh.InputSession = true
						case typecheck.IsString(info.TypeOf(f.Type)) &&
							len(f.Names) > 0 && f.Names[0].Name == "sessionToken":
							gh.InputSessionToken = true
						default:
							errs.ErrAt(pos, ErrAppHeadUnsupportedInput)
							valid = false
						}
					}
					if !valid {
						continue
					}

					ctx.app.GlobalHeadGenerator = gh
				case "Recover500":
					info := ctx.pkg.TypesInfo
					pos := ctx.pkg.Fset.Position(fd.Name.Pos())
					params := fd.Type.Params
					results := fd.Type.Results
					if params == nil || params.NumFields() != 2 ||
						!typecheck.IsError(info.TypeOf(params.List[0].Type)) ||
						!typecheck.IsPtrToDatastarSSE(params.List[1].Type, info) ||
						results == nil || results.NumFields() != 1 ||
						!typecheck.IsError(info.TypeOf(results.List[0].Type)) {
						errs.ErrAt(pos, ErrAppRecover500InvalidSignature)
						continue
					}
					ctx.app.Recover500 = fd.Name
				default:
					kind, suffix := methodkind.Classify(fd.Name.Name)
					if kind.IsAction() {
						pos := ctx.pkg.Fset.Position(fd.Name.Pos())
						if suffix == "" {
							errs.ErrAt(pos,
								fmt.Errorf("%w: %s", ErrActionNameMissing, fd.Name.Name))
						} else if err := validate.ActionMethodName(fd.Name.Name); err != nil {
							errs.ErrAt(pos,
								fmt.Errorf("%w: %s", ErrActionNameInvalid, fd.Name.Name))
						}
						attachAppAction(ctx, errs, fd, kind, suffix)
					}
				}
				continue
			}

			pg, isPage := ctx.pages[recv]
			ap, isAbs := ctx.abstracts[recv]
			if !isPage && !isAbs {
				continue
			}

			kind, suffix := methodkind.Classify(fd.Name.Name)
			if kind == 0 {
				continue
			}

			pos := ctx.pkg.Fset.Position(fd.Name.Pos())

			// Validate action method names early.
			if kind.IsAction() {
				if suffix == "" {
					errs.ErrAt(pos,
						fmt.Errorf("%w: %s", ErrActionNameMissing, fd.Name.Name))
				} else if err := validate.ActionMethodName(fd.Name.Name); err != nil {
					errs.ErrAt(pos,
						fmt.Errorf("%w: %s", ErrActionNameInvalid, fd.Name.Name))
				}
			}

			switch kind {
			case methodkind.EventHandler:
				if err := validate.EventHandlerMethodName(fd.Name.Name); err != nil {
					errs.ErrAt(pos,
						fmt.Errorf("%w: %s.%s",
							validate.ErrEventHandlerNameInvalid, recv, fd.Name.Name))
				}
				validateAndAttachEventHandler(ctx, errs, recv, fd, pg, ap, suffix)
			default:
				attachHTTPHandler(ctx, errs, recv, fd, pg, ap, kind, suffix)
			}
		}
	}
}

func validateAndAttachEventHandler(
	ctx *parseCtx,
	errs *Errors,
	recv string,
	fd *ast.FuncDecl,
	pg *model.Page,
	ap *model.AbstractPage,
	suffix string,
) {
	pos := ctx.pkg.Fset.Position(fd.Name.Pos())

	// Invariants for OnXXX handlers:
	//   - Must have a parameter named "event" of an EventXXX type
	//   - Must have a *datastar.ServerSentEventGenerator parameter
	//   - Only one handler per EventXXX per receiver type
	//   - Parameters may be in any order
	params := fd.Type.Params
	var evName string

	// Find and validate the event parameter (by name "event").
	foundEvent := false
	if params != nil {
		for _, f := range params.List {
			if len(f.Names) == 1 && f.Names[0].Name == "event" {
				var ok bool
				evName, ok = typecheck.EventTypeNameOf(
					f.Type, ctx.pkg.TypesInfo, ctx.eventTypeNames,
				)
				if !ok {
					errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
						ErrSignatureEvHandMissingEvent, recv, fd.Name.Name))
				}
				foundEvent = true
				break
			}
		}
	}
	if !foundEvent {
		errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
			ErrSignatureEvHandMissingEvent, recv, fd.Name.Name))
	}

	// Check for duplicate event handlers.
	if evName != "" {
		m := ctx.seenEvHandlerByRecv[recv]
		if m == nil {
			m = map[string]token.Pos{}
			ctx.seenEvHandlerByRecv[recv] = m
		}
		if prev, dup := m[evName]; dup {
			errs.ErrAt(pos, fmt.Errorf(
				"%w: %s.%s handles %s (previous at %s)",
				ErrEvHandDuplicate,
				recv,
				fd.Name.Name,
				evName,
				ctx.pkg.Fset.Position(prev),
			))
		} else {
			m[evName] = fd.Name.Pos()
		}
	}

	// Find SSE parameter (by type).
	foundSSE := false
	if params != nil {
		for _, f := range params.List {
			if typecheck.IsPtrToDatastarSSE(f.Type, ctx.pkg.TypesInfo) {
				foundSSE = true
				break
			}
		}
	}
	if !foundSSE {
		errs.ErrAt(pos,
			fmt.Errorf("%w: %s.%s", ErrSignatureEvHandMissingSSE, recv, fd.Name.Name))
	}

	// Validate remaining recognized parameters (order-independent).
	if params != nil {
		for _, f := range params.List {
			switch {
			case len(f.Names) == 1 && f.Names[0].Name == "event":
				// Already validated above.
			case typecheck.IsPtrToDatastarSSE(f.Type, ctx.pkg.TypesInfo):
				// Already validated above.
			case paramvalidation.IsSessionTokenParam(f):
				if !typecheck.IsString(ctx.pkg.TypesInfo.TypeOf(f.Type)) {
					errs.ErrAt(ctx.pkg.Fset.Position(f.Type.Pos()), fmt.Errorf(
						"%w: %s.%s",
						ErrSessionTokenParamNotString,
						recv, fd.Name.Name,
					))
				}
			case paramvalidation.IsSessionParam(f):
				if !typecheck.IsSessionType(f.Type, ctx.pkg.TypesInfo) {
					errs.ErrAt(ctx.pkg.Fset.Position(f.Type.Pos()), fmt.Errorf(
						"%w: %s.%s",
						ErrSessionParamNotSessionType,
						recv, fd.Name.Name,
					))
				}
			case paramvalidation.IsSignalsParam(f):
				// Valid, no extra validation needed.
			default:
				p := f.Type.Pos()
				if len(f.Names) > 0 {
					p = f.Names[0].Pos()
				}
				errs.ErrAt(ctx.pkg.Fset.Position(p), unsupportedInputError(
					f, nil, ctx.pkg.TypesInfo, recv, fd.Name.Name,
				))
			}
		}
	}

	// OnXXX must return exactly one result of type error.
	if !eventHandlerReturnsOnlyError(fd, ctx.pkg.TypesInfo) {
		retPos := pos
		if fd.Type.Results != nil {
			retPos = ctx.pkg.Fset.Position(fd.Type.Results.Pos())
		}
		errs.ErrAt(retPos, fmt.Errorf("%w: %s.%s",
			ErrSignatureEvHandReturnMustBeError, recv, fd.Name.Name))
	}

	h := parseEventHandler(fd, ctx.pkg.TypesInfo, suffix, evName)

	// If it was valid, or best-effort (even if invalid arguments), we attach it.
	// But if evName is empty, it won't be useful for flattening override checks.
	// We attach it anyway so that AST info is there.

	if pg != nil {
		pg.EventHandlers = append(pg.EventHandlers, h)
	} else {
		ap.EventHandlers = append(ap.EventHandlers, h)
	}
}

func eventHandlerReturnsOnlyError(fd *ast.FuncDecl, info *types.Info) bool {
	if fd == nil || fd.Type == nil || fd.Type.Results == nil {
		return false
	}
	results := fd.Type.Results.List
	if len(results) == 0 {
		return false
	}

	// Count actual result values (a single field can declare multiple named results).
	total := 0
	for _, f := range results {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		total += n
	}
	if total != 1 {
		return false
	}

	// The sole result type must be `error`.
	t := info.TypeOf(results[0].Type)
	return typecheck.IsError(t)
}

func attachHTTPHandler(
	ctx *parseCtx,
	errs *Errors,
	recv string,
	fd *ast.FuncDecl,
	pg *model.Page,
	ap *model.AbstractPage,
	kind methodkind.Kind,
	suffix string,
) {
	pos := ctx.pkg.Fset.Position(fd.Name.Pos())

	h, outputs, herr := parseHandler(
		recv, fd, ctx.pkg.TypesInfo, ctx.pkg.Fset, ctx.eventTypeNames, kind, suffix,
	)
	if herr != nil {
		// Keep going; still attach a best-effort handler model.
		// herr may contain multiple joined errors (e.g. several
		// unsupported params); report each one separately.
		reportErrorsWithFset(errs, ctx.pkg.Fset, pos, herr)
	}
	ctx.handlerOutputs[h] = outputs

	if kind.IsAction() {
		r, found, valid := parseRoute(fd.Name.Name, fd.Doc)
		h.Route = r

		if !found {
			pagePath := ""
			if pg != nil {
				pagePath = pg.Route
			}
			errs.ErrAt(pos,
				&ErrorActionMissingPathComm{PagePath: pagePath, Recv: recv, MethodName: fd.Name.Name})
		} else if !valid {
			errs.ErrAt(pos,
				&ErrorActionInvalidPathComm{Recv: recv, MethodName: fd.Name.Name})
		} else if pg != nil && pg.Route != "" && !actionIsUnderPage(pg.Route, r) {
			errs.ErrAt(pos,
				&ErrorActionPathNotUnderPage{PagePath: pg.Route, Recv: recv, MethodName: fd.Name.Name})
		}
	} else if kind == methodkind.GETHandler && pg != nil {
		h.Route = pg.Route
	}

	// Validate path struct fields against route variables.
	if herr == nil && h.Route != "" {
		if err := paramvalidation.ValidatePathAgainstRoute(
			h, recv, fd.Name.Name,
		); err != nil {
			p := pos
			if h.InputPath != nil {
				p = ctx.pkg.Fset.Position(h.InputPath.Expr.Pos())
			}
			reportErrorsWithFset(errs, ctx.pkg.Fset, p, err)
		}
	}

	// Validate reflectsignal tags on query fields reference actual signals.
	if herr == nil {
		if rsErr := structtag.ValidateReflectSignal(h, recv, fd.Name.Name); rsErr != nil {
			p := pos
			if h.InputQuery != nil {
				p = ctx.pkg.Fset.Position(h.InputQuery.Expr.Pos())
			}
			reportErrorsWithFset(errs, ctx.pkg.Fset, p, rsErr)
		}
	}

	if pg != nil {
		if kind == methodkind.GETHandler {
			if herr != nil {
				// Handler parsing failed; attach a minimal GET so the
				// page is not flagged as missing a GET handler, but skip
				// output validation and code generation details.
				pg.GET = &model.HandlerGET{Handler: h}
			} else {
				get, getErr := buildHandlerGET(h, outputs, ctx.pkg.Fset)
				pg.GET = get
				if getErr != nil {
					p := resolveErrorPos(getErr, ctx.pkg.Fset, pos)
					errs.ErrAt(p,
						fmt.Errorf("%w in %s.%s",
							unwrapPositioned(getErr), recv, fd.Name.Name))
				}
			}
		} else {
			pg.Actions = append(pg.Actions, h)
		}
		return
	}
	ap.Methods = append(ap.Methods, h)
}

func attachAppAction(
	ctx *parseCtx,
	errs *Errors,
	fd *ast.FuncDecl,
	kind methodkind.Kind,
	suffix string,
) {
	pos := ctx.pkg.Fset.Position(fd.Name.Pos())

	h, outputs, herr := parseHandler(
		"App", fd, ctx.pkg.TypesInfo, ctx.pkg.Fset, ctx.eventTypeNames, kind, suffix,
	)
	if herr != nil {
		reportErrorsWithFset(errs, ctx.pkg.Fset, pos, herr)
	}
	ctx.handlerOutputs[h] = outputs

	r, found, valid := parseRoute(fd.Name.Name, fd.Doc)
	h.Route = r

	if !found {
		errs.ErrAt(pos,
			&ErrorActionMissingPathComm{Recv: "App", MethodName: fd.Name.Name})
	} else if !valid {
		errs.ErrAt(pos,
			&ErrorActionInvalidPathComm{Recv: "App", MethodName: fd.Name.Name})
	}

	// Validate path struct fields against route variables.
	if herr == nil && h.Route != "" {
		if err := paramvalidation.ValidatePathAgainstRoute(
			h, "App", fd.Name.Name,
		); err != nil {
			errs.ErrAt(pos, err)
		}
	}

	ctx.app.Actions = append(ctx.app.Actions, h)
}

func flattenPages(ctx *parseCtx, errs *Errors) {
	for _, name := range slices.Sorted(maps.Keys(ctx.pages)) {
		flattenPage(ctx, errs, ctx.pages[name])
	}
}

func flattenPage(ctx *parseCtx, errs *Errors, pg *model.Page) {
	if len(pg.Embeds) == 0 {
		return
	}

	visited := map[string]bool{}
	ownedMethods := map[string]bool{}
	handledEvents := map[string]string{}
	handledEventPos := map[string]token.Pos{}

	// GET ownership tracking
	getOwner := ""             // "page" or abstract type name
	getOwnerPos := token.NoPos // IMPORTANT: now points to embed site for embedded GET

	// Register own methods
	if pg.GET != nil {
		ownedMethods["GET"] = true
		getOwner = "page"
		if pg.GET.Handler != nil && pg.GET.Expr != nil {
			getOwnerPos = pg.GET.Expr.Pos()
		}
	}
	for _, a := range pg.Actions {
		ownedMethods[a.Name] = true
	}
	for _, h := range pg.EventHandlers {
		if h.EventTypeName != "" {
			handledEvents[h.EventTypeName] = "page"
			if h.Expr != nil {
				handledEventPos[h.EventTypeName] = h.Expr.Pos()
			}
		} else {
			ownedMethods[h.Name] = true
		}
	}

	// Queue items carry the embed site position that introduced this abstract.
	type qitem struct {
		ap       *model.AbstractPage
		embedPos token.Pos // position of the embedded field identifier
	}

	// seed queue from the page's struct embed sites
	pageEmbPos := structinspect.EmbeddedFieldPosMap(typeStruct(ctx, pg.TypeName))
	queue := make([]qitem, 0, len(pg.Embeds))
	for _, ap := range pg.Embeds {
		queue = append(queue, qitem{
			ap:       ap,
			embedPos: pageEmbPos[ap.TypeName],
		})
	}

	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		ap := it.ap

		if visited[ap.TypeName] {
			continue
		}
		visited[ap.TypeName] = true

		// enqueue children, carrying THEIR embed positions (in the parent abstract)
		apEmbPos := structinspect.EmbeddedFieldPosMap(typeStruct(ctx, ap.TypeName))
		for _, child := range ap.Embeds {
			queue = append(queue, qitem{
				ap:       child,
				embedPos: apEmbPos[child.TypeName],
			})
		}

		// Methods
		for _, m := range ap.Methods {
			if m.HTTPMethod == "GET" {
				// Page's own GET always wins; no conflict in that case.
				if getOwner == "page" {
					continue
				}
				// First embedded GET wins (record embed site).
				if getOwner == "" {
					get, getErr := buildHandlerGET(m, ctx.handlerOutputs[m], ctx.pkg.Fset)
					pg.GET = get
					if getErr != nil {
						fallback := ctx.pkg.Fset.Position(m.Expr.Pos())
						p := resolveErrorPos(getErr, ctx.pkg.Fset, fallback)
						errs.ErrAt(p, fmt.Errorf("%w in %s.%s",
							unwrapPositioned(getErr), ap.TypeName, m.Name))
					}
					getOwner = ap.TypeName
					getOwnerPos = it.embedPos
					continue
				}
				if getOwner == ap.TypeName {
					continue
				}

				// Conflicting embedded GETs -> report at embed site of the second one.
				pos := ctx.pkg.Fset.Position(pg.Expr.Pos())
				if it.embedPos != token.NoPos {
					pos = ctx.pkg.Fset.Position(it.embedPos)
				}

				prevPos := token.Position{}
				if getOwnerPos != token.NoPos {
					prevPos = ctx.pkg.Fset.Position(getOwnerPos)
				}

				errs.ErrAt(pos, fmt.Errorf(
					"%w: %s inherits %s and %s which both define GET (previous at %s)",
					ErrPageConflictingGETEmbed,
					pg.TypeName,
					getOwner,
					ap.TypeName,
					prevPos,
				))
				continue
			}

			// Non-GET methods: name-based dedup.
			if ownedMethods[m.Name] {
				continue
			}
			ownedMethods[m.Name] = true
			pg.Actions = append(pg.Actions, m)
		}

		// EventHandlers
		for _, h := range ap.EventHandlers {
			ev := h.EventTypeName
			if ev == "" {
				if ownedMethods[h.Name] {
					continue
				}
				ownedMethods[h.Name] = true
				pg.EventHandlers = append(pg.EventHandlers, h)
				continue
			}

			if prevOwner, exists := handledEvents[ev]; exists {
				if prevOwner == "page" {
					// The page overrides this inherited handler; skip.
					continue
				}
				if prevOwner == ap.TypeName {
					continue
				}

				pos := ctx.pkg.Fset.Position(pg.Expr.Pos())
				if h.Expr != nil {
					pos = ctx.pkg.Fset.Position(h.Expr.Pos())
				}
				prevPos := token.Position{}
				if ppos, ok := handledEventPos[ev]; ok && ppos != token.NoPos {
					prevPos = ctx.pkg.Fset.Position(ppos)
				}
				errs.ErrAt(pos, fmt.Errorf(
					"%w: %s inherits %s and %s which both handle %s (previous at %s)",
					ErrEvHandDuplicateEmbed,
					pg.TypeName,
					prevOwner,
					ap.TypeName,
					ev,
					prevPos,
				))
				continue
			}

			handledEvents[ev] = ap.TypeName
			if h.Expr != nil {
				handledEventPos[ev] = h.Expr.Pos()
			}
			pg.EventHandlers = append(pg.EventHandlers, h)
		}
	}
}

func validateRequiredHandlers(ctx *parseCtx, errs *Errors) {
	// Every page type must have a GET handler.
	for _, name := range slices.Sorted(maps.Keys(ctx.pages)) {
		if ctx.pages[name].GET == nil {
			ts := ctx.typeSpecByName[name]
			errs.ErrAt(ctx.pkg.Fset.Position(ts.Name.Pos()),
				&ErrorPageMissingGET{TypeName: name})
		}
	}
}

func finalizePages(ctx *parseCtx) {
	for _, name := range slices.Sorted(maps.Keys(ctx.pages)) {
		ctx.app.Pages = append(ctx.app.Pages, ctx.pages[name])
	}
}

func assignSpecialPages(ctx *parseCtx, errs *Errors) {
	ctx.app.PageIndex = ctx.pages["PageIndex"]
	ctx.app.PageError404 = ctx.pages["PageError404"]
	ctx.app.PageError500 = ctx.pages["PageError500"]

	if ctx.app.PageIndex == nil {
		errs.ErrAt(ctx.basePos, ErrAppMissingPageIndex)
	}
}

func parseEventHandler(
	fd *ast.FuncDecl, info *types.Info, name, eventTypeName string,
) *model.EventHandler {
	params := fd.Type.Params.List

	h := &model.EventHandler{
		Expr:          fd.Name,
		Name:          name,
		EventTypeName: eventTypeName,
	}

	// Match parameters by name/type in any order.
	for _, f := range params {
		switch {
		case len(f.Names) == 1 && f.Names[0].Name == "event":
			h.InputEvent = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindEvent)
		case typecheck.IsPtrToDatastarSSE(f.Type, info):
			h.InputSSE = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSSE)
		case paramvalidation.IsSessionTokenParam(f):
			h.InputSessionToken = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSessionToken)
		case paramvalidation.IsSessionParam(f):
			h.InputSession = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSession)
		case paramvalidation.IsSignalsParam(f):
			h.InputSignals = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSignals)
		}
	}

	if fd.Type.Results != nil && len(fd.Type.Results.List) > 0 {
		h.OutputErr = &model.Output{
			Type: makeType(fd.Type.Results.List[0].Type, info),
		}
	}

	return h
}

func loadPackage(appPackagePath string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax |
			packages.NeedModule,
	}

	// Accept either an import path/pattern or a directory.
	if st, err := os.Stat(appPackagePath); err == nil && st.IsDir() {
		cfg.Dir = appPackagePath
		return loadSingle(cfg, ".")
	}

	// If it looks like a filesystem path but doesn't exist,
	// keep as pattern anyway.
	pattern := appPackagePath
	if filepath.IsAbs(appPackagePath) {
		// go list doesn't like absolute patterns;
		// fallback to directory load if possible.
		dir := filepath.Dir(appPackagePath)
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			cfg.Dir = dir
			return loadSingle(cfg, ".")
		}
	}

	return loadSingle(cfg, pattern)
}

func loadSingle(
	cfg *packages.Config, pattern string,
) (*packages.Package, error) {
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf(
			"expected 1 package, got %d", len(pkgs),
		)
	}
	return pkgs[0], nil
}

func pickDoc(
	typeName string, docByType, genDocByType map[string]*ast.CommentGroup,
) *ast.CommentGroup {
	if d := docByType[typeName]; d != nil {
		return d
	}
	return genDocByType[typeName]
}

func pageSpecialization(typeName string) model.PageSpecialization {
	switch typeName {
	case "PageIndex":
		return model.PageTypeIndex
	case "PageError404":
		return model.PageTypeError404
	case "PageError500":
		return model.PageTypeError500
	default:
		return 0
	}
}

// parseRoute parses lines like:
//
//	// PageFoo is /foo
//	// POSTDoThing is /foo/do-thing
//
// found=true means there was an attempt to define a route for `symbol`.
// valid=true means the attempt was well-formed and the route itself is valid.
func parseRoute(
	symbol string, cg *ast.CommentGroup,
) (route string, found bool, valid bool) {
	if cg == nil || len(cg.List) == 0 {
		return "", false, false
	}

	// The definition MUST be on the first line.
	c := cg.List[0]
	txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))

	want := symbol + " is "
	attemptPrefix := symbol + " "

	if !strings.HasPrefix(txt, attemptPrefix) {
		return "", false, false
	}

	// We *will* return found=true from here on.
	if !strings.HasPrefix(txt, want) {
		return "", true, false
	}

	route = strings.TrimSpace(strings.TrimPrefix(txt, want))
	if route == "" {
		return route, true, false
	}
	if !strings.HasPrefix(route, "/") {
		return route, true, false
	}
	if strings.IndexFunc(route, unicode.IsSpace) >= 0 {
		return route, true, false
	}

	// The empty // line between the is comment and the description is mandatory.
	if len(cg.List) > 1 {
		second := strings.TrimSpace(strings.TrimPrefix(cg.List[1].Text, "//"))
		if second != "" {
			return route, true, false
		}
	}

	return route, true, true
}

// expandFieldList splits multi-name fields (e.g. "r, a *http.Request")
// into individual single-name fields so each represents one parameter.
func expandFieldList(fields []*ast.Field) []*ast.Field {
	var out []*ast.Field
	for _, f := range fields {
		if len(f.Names) <= 1 {
			out = append(out, f)
			continue
		}
		for _, name := range f.Names {
			out = append(out, &ast.Field{
				Names: []*ast.Ident{name},
				Type:  f.Type,
			})
		}
	}
	return out
}

// positionedError wraps an error with a specific source position,
// overriding the default position used by reportErrors.
type positionedError struct {
	pos token.Position
	err error
}

func (e *positionedError) Error() string { return e.err.Error() }
func (e *positionedError) Unwrap() error { return e.err }

// resolveErrorPos returns the most specific position for an error.
// It checks positionedError first, then the ASTPos() interface,
// falling back to the provided fset and default position.
func resolveErrorPos(e error, fset *token.FileSet, fallback token.Position) token.Position {
	var pe *positionedError
	if errors.As(e, &pe) {
		return pe.pos
	}
	if ap, ok := e.(interface{ ASTPos() token.Pos }); ok {
		if p := ap.ASTPos(); p.IsValid() && fset != nil {
			return fset.Position(p)
		}
	}
	return fallback
}

// unwrapPositioned returns the inner error if e is a positionedError,
// otherwise returns e unchanged.
func unwrapPositioned(e error) error {
	var pe *positionedError
	if errors.As(e, &pe) {
		return pe.err
	}
	return e
}

// reportErrorsWithFset is like reportErrors but also resolves ASTPos()
// positions using the provided FileSet.
func reportErrorsWithFset(errs *Errors, fset *token.FileSet, pos token.Position, err error) {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			p := resolveErrorPos(e, fset, pos)
			errs.ErrAt(p, unwrapPositioned(e))
		}
		return
	}
	p := resolveErrorPos(err, fset, pos)
	errs.ErrAt(p, unwrapPositioned(err))
}

// appendPositioned wraps err (or each sub-error of a joined error)
// with the given AST position and appends them to dst.
// If an individual sub-error implements ASTPos() token.Pos and that
// position is valid, it overrides the fallback position.
func appendPositioned(dst *[]error, fset *token.FileSet, fallback token.Pos, err error) {
	resolvePos := func(e error) token.Position {
		if ap, ok := e.(interface{ ASTPos() token.Pos }); ok {
			if p := ap.ASTPos(); p.IsValid() {
				return fset.Position(p)
			}
		}
		return fset.Position(fallback)
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			*dst = append(*dst, &positionedError{pos: resolvePos(e), err: e})
		}
	} else {
		*dst = append(*dst, &positionedError{pos: resolvePos(err), err: err})
	}
}

// knownParamNames lists the recognized handler parameter names
// used for fuzzy matching in unsupportedInputError.
var knownParamNames = []string{
	"sessionToken", "session", "path", "query", "signals", "dispatch",
}

// unsupportedInputError builds an ErrorSignatureUnsupportedInput for a
// parameter that doesn't match any recognized handler input.
// It checks whether the parameter's type matches a known input whose
// name-based check failed, and if so, sets ExpectedName for a rename
// suggestion. When h is non-nil, it checks whether the expected name
// would collide with an already-consumed input.
// As a fallback, it performs fuzzy matching against known parameter names.
func unsupportedInputError(
	f *ast.Field, h *model.Handler, info *types.Info, recv, method string,
) *ErrorSignatureUnsupportedInput {
	paramName := "_"
	if len(f.Names) > 0 {
		paramName = f.Names[0].Name
	}
	paramType := types.ExprString(f.Type)
	if t := info.TypeOf(f.Type); t != nil {
		paramType = t.String()
	}

	e := &ErrorSignatureUnsupportedInput{
		ParamName:  paramName,
		ParamType:  paramType,
		Recv:       recv,
		MethodName: method,
	}

	// Check if the type matches a known input but the name is wrong.
	// Only suggest renaming when the slot is not already consumed
	// and the parameter doesn't already have the expected name.
	if h != nil {
		switch {
		case typecheck.IsPtrToNetHTTPReq(f.Type, info):
			// Already consumed — this is a duplicate *http.Request.
		case typecheck.IsPtrToDatastarSSE(f.Type, info):
			// Already consumed — duplicate SSE parameter.
		default:
			e.CandidateNames = typeCandidates(f, h, info)
		}
	}

	// Try fuzzy name matching for the best rename suggestion.
	if paramName != "_" {
		if best, ok := fuzzyMatchParamName(paramName, h); ok {
			e.ExpectedName = best
		}
	}

	return e
}

// typeCandidates returns the unconsumed known parameter names whose
// expected type matches the field's type.
func typeCandidates(
	f *ast.Field, h *model.Handler, info *types.Info,
) []string {
	t := info.TypeOf(f.Type)
	if t == nil {
		return nil
	}

	isSession := typecheck.IsSessionType(f.Type, info)
	isString := typecheck.IsString(t)
	isStruct := isStructType(t)
	isFunc := isFuncType(t)

	type candidate struct {
		name     string
		consumed bool
		match    bool
	}
	all := []candidate{
		{"session", h.InputSession != nil, isSession},
		{"sessionToken", h.InputSessionToken != nil, isString},
		// Only suggest path/query/signals for plain structs,
		// not for named types that have a more specific match (e.g. Session).
		{"path", h.InputPath != nil, isStruct && !isSession},
		{"query", h.InputQuery != nil, isStruct && !isSession},
		{"signals", h.InputSignals != nil, isStruct && !isSession},
		{"dispatch", h.InputDispatch != nil, isFunc},
	}

	var names []string
	for _, c := range all {
		if c.match && !c.consumed {
			names = append(names, c.name)
		}
	}
	return names
}

func isStructType(t types.Type) bool {
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

func isFuncType(t types.Type) bool {
	_, ok := t.Underlying().(*types.Signature)
	return ok
}

// fuzzyMatchParamName returns the closest known parameter name to name,
// if one is close enough. It skips names whose slot is already consumed in h.
func fuzzyMatchParamName(name string, h *model.Handler) (string, bool) {
	bestName := ""
	bestDist := -1
	for _, known := range knownParamNames {
		if h != nil && isParamConsumed(h, known) {
			continue
		}
		d := fuzzy.LevenshteinDistance(
			strings.ToLower(name), strings.ToLower(known),
		)
		// Accept if distance is at most ~1/3 of the longer name's length.
		maxDist := max(max(len(name), len(known))/3, 1)
		if d <= maxDist && (bestDist < 0 || d < bestDist) {
			bestName = known
			bestDist = d
		}
	}
	if bestDist < 0 {
		return "", false
	}
	return bestName, true
}

// isParamConsumed reports whether the named parameter slot is already
// consumed in h.
func isParamConsumed(h *model.Handler, name string) bool {
	switch name {
	case "sessionToken":
		return h.InputSessionToken != nil
	case "session":
		return h.InputSession != nil
	case "path":
		return h.InputPath != nil
	case "query":
		return h.InputQuery != nil
	case "signals":
		return h.InputSignals != nil
	case "dispatch":
		return h.InputDispatch != nil
	}
	return false
}

func parseInput(f *ast.Field, info *types.Info) *model.Input {
	// request param should be named, but keep best-effort
	name := ""
	var expr ast.Expr
	if len(f.Names) > 0 {
		name = f.Names[0].Name
		expr = f.Names[0]
	}
	return &model.Input{
		Expr: expr,
		Name: name,
		Type: makeType(f.Type, info),
	}
}

func makeType(typeExpr ast.Expr, info *types.Info) model.Type {
	return model.Type{
		Resolved: info.TypeOf(typeExpr),
		TypeExpr: typeExpr,
	}
}

func buildHandlerGET(
	h *model.Handler, outputs []*model.Output, fset *token.FileSet,
) (*model.HandlerGET, error) {
	get := &model.HandlerGET{
		Handler: h,
	}

	// Collect all templ.Component outputs
	var templComponents []*model.Output
	for _, out := range outputs {
		if typecheck.IsTemplComponent(out.Type.Resolved) {
			templComponents = append(templComponents, out)
		}
	}

	// Validate: GET must have at least one templ.Component return (body)
	if len(templComponents) == 0 {
		return get, ErrSignatureGETMissingBody
	}

	// Validate: First templ.Component must be named "body"
	firstComp := templComponents[0]
	if firstComp.Name != "body" {
		if firstComp.Expr != nil {
			return get, &positionedError{
				pos: fset.Position(firstComp.Expr.Pos()),
				err: ErrSignatureGETBodyWrongName,
			}
		}
		return get, ErrSignatureGETBodyWrongName
	}
	get.OutputBody = &model.TemplComponent{Output: firstComp}

	// Validate: If there's a second templ.Component, it must be named "head"
	if len(templComponents) > 1 {
		secondComp := templComponents[1]
		if secondComp.Name != "head" {
			if secondComp.Expr != nil {
				return get, &positionedError{
					pos: fset.Position(secondComp.Expr.Pos()),
					err: ErrSignatureGETHeadWrongName,
				}
			}
			return get, ErrSignatureGETHeadWrongName
		}
		get.OutputHead = &model.TemplComponent{Output: secondComp}
	}

	return get, nil
}

func parseHandler(
	recv string,
	fd *ast.FuncDecl,
	info *types.Info,
	fset *token.FileSet,
	eventTypeNames map[string]struct{},
	kind methodkind.Kind,
	name string,
) (*model.Handler, []*model.Output, error) {
	h := &model.Handler{
		Expr:       fd.Name,
		Name:       name,
		HTTPMethod: kind.HTTPMethod(),
	}

	params := fd.Type.Params
	if params == nil || len(params.List) == 0 {
		return h, nil, fmt.Errorf("%w in %s.%s",
			ErrSignatureMissingReq, recv, fd.Name.Name)
	}
	// Expand multi-name fields (e.g. "r, a *http.Request" → two fields)
	// so that each field represents exactly one parameter.
	expandedParams := expandFieldList(params.List)

	// Match parameters by name/type in any order.
	var unsupErrs []error
	foundReq := false
	for _, f := range expandedParams {
		fieldErr := func(err error) *positionedError {
			p := f.Type.Pos()
			if len(f.Names) > 0 {
				p = f.Names[0].Pos()
			}
			return &positionedError{pos: fset.Position(p), err: err}
		}

		switch {
		case typecheck.IsPtrToNetHTTPReq(f.Type, info):
			if h.InputRequest != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			h.InputRequest = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindRequest)
			foundReq = true

		case typecheck.IsPtrToDatastarSSE(f.Type, info):
			if h.InputSSE != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			h.InputSSE = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSSE)

		case paramvalidation.IsSessionTokenParam(f):
			if h.InputSessionToken != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			if !typecheck.IsString(info.TypeOf(f.Type)) {
				return h, nil, fieldErr(fmt.Errorf("%w in %s.%s",
					ErrSessionTokenParamNotString, recv, fd.Name.Name))
			}
			h.InputSessionToken = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSessionToken)

		case paramvalidation.IsSessionParam(f):
			if h.InputSession != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			if !typecheck.IsSessionType(f.Type, info) {
				return h, nil, fieldErr(fmt.Errorf("%w in %s.%s",
					ErrSessionParamNotSessionType, recv, fd.Name.Name))
			}
			h.InputSession = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSession)

		case paramvalidation.IsPathParam(f):
			if h.InputPath != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			pathErr := paramvalidation.ValidatePathStruct(
				f, info, recv, fd.Name.Name,
			)
			if pathErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), pathErr)
				continue
			}
			h.InputPath = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindPath)

		case paramvalidation.IsQueryParam(f):
			if h.InputQuery != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			queryErr := paramvalidation.ValidateQueryStruct(
				f, info, recv, fd.Name.Name,
			)
			if queryErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), queryErr)
				continue
			}
			h.InputQuery = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindQuery)

		case paramvalidation.IsSignalsParam(f):
			if h.InputSignals != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			sigErr := paramvalidation.ValidateSignalsStruct(
				f, info, recv, fd.Name.Name,
			)
			if sigErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), sigErr)
				continue
			}
			h.InputSignals = parseInput(f, info)
			h.InputOrder = append(h.InputOrder, model.InputKindSignals)

		case paramvalidation.IsDispatchParam(f):
			if h.InputDispatch != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			eventNames, dispErr := paramvalidation.ValidateDispatchFunc(
				f, info, eventTypeNames, recv, fd.Name.Name,
			)
			if dispErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), dispErr)
				continue
			}
			h.InputDispatch = &model.InputDispatch{
				Expr:           f.Names[0],
				Name:           "dispatch",
				Type:           makeType(f.Type, info),
				EventTypeNames: eventNames,
			}
			h.InputOrder = append(h.InputOrder, model.InputKindDispatch)

		default:
			unsupErrs = append(unsupErrs,
				fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
		}
	}

	if !foundReq {
		return h, nil, fmt.Errorf("%w in %s.%s",
			ErrSignatureMissingReq, recv, fd.Name.Name)
	}

	if len(unsupErrs) > 0 {
		return h, nil, errors.Join(unsupErrs...)
	}

	// Results
	if fd.Type.Results == nil {
		return h, nil, nil
	}

	var outputs []*model.Output
	var multiErrPos token.Pos
	for _, r := range fd.Type.Results.List {
		t := makeType(r.Type, info)

		if len(r.Names) == 0 {
			if typecheck.IsError(t.Resolved) {
				if h.OutputErr != nil {
					multiErrPos = r.Type.Pos()
					continue
				}
				h.OutputErr = &model.Output{Type: t}
				continue
			}
			outputs = append(outputs, &model.Output{Type: t})
			continue
		}

		for _, n := range r.Names {
			out := &model.Output{
				Expr: n,
				Name: n.Name,
				Type: t,
			}
			if typecheck.IsError(t.Resolved) {
				if h.OutputErr != nil {
					multiErrPos = n.Pos()
					continue
				}
				h.OutputErr = out
				continue
			}
			retErr := func(err error) *positionedError {
				return &positionedError{pos: fset.Position(r.Type.Pos()), err: err}
			}

			switch n.Name {
			case "redirect":
				if !typecheck.IsString(t.Resolved) {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrRedirectNotString, recv, fd.Name.Name))
				}
				h.OutputRedirect = out
				continue
			case "redirectStatus":
				if !typecheck.IsInt(t.Resolved) {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrRedirectStatusNotInt, recv, fd.Name.Name))
				}
				h.OutputRedirectStatus = out
				continue
			case "newSession":
				if !typecheck.IsSessionType(
					r.Type, info,
				) {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrNewSessionNotSessionType, recv, fd.Name.Name))
				}
				h.OutputNewSession = out
				continue
			case "closeSession":
				if !typecheck.IsBool(t.Resolved) {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrCloseSessionNotBool, recv, fd.Name.Name))
				}
				h.OutputCloseSession = out
				continue
			case "enableBackgroundStreaming":
				if kind != methodkind.GETHandler {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrEnableBgStreamNotGET, recv, fd.Name.Name))
				}
				if !typecheck.IsBool(t.Resolved) {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrEnableBgStreamNotBool, recv, fd.Name.Name))
				}
				h.OutputEnableBgStream = out
				continue

			case "disableRefreshAfterHidden":
				if kind != methodkind.GETHandler {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrDisableRefreshNotGET, recv, fd.Name.Name))
				}
				if !typecheck.IsBool(t.Resolved) {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrDisableRefreshNotBool, recv, fd.Name.Name))
				}
				h.OutputDisableRefresh = out
				continue
			}
			outputs = append(outputs, out)
		}
	}

	if multiErrPos.IsValid() {
		return h, outputs, &positionedError{
			pos: fset.Position(multiErrPos),
			err: fmt.Errorf("%w in %s.%s",
				ErrSignatureMultiErrRet, recv, fd.Name.Name),
		}
	}
	if h.OutputRedirectStatus != nil && h.OutputRedirect == nil {
		return h, outputs, fmt.Errorf("%w in %s.%s",
			ErrRedirectStatusWithoutRedirect, recv, fd.Name.Name)
	}
	if h.OutputNewSession != nil && h.InputSSE != nil {
		return h, outputs, fmt.Errorf("%w in %s.%s",
			ErrNewSessionWithSSE, recv, fd.Name.Name)
	}
	if h.OutputCloseSession != nil && h.InputSSE != nil {
		return h, outputs, fmt.Errorf("%w in %s.%s",
			ErrCloseSessionWithSSE, recv, fd.Name.Name)
	}

	// For action handlers, detect templ.Component body output.
	if kind.IsAction() {
		for _, out := range outputs {
			if typecheck.IsTemplComponent(out.Type.Resolved) {
				h.OutputBody = &model.TemplComponent{Output: out}
				break
			}
		}
	}

	return h, outputs, nil
}

func typeStruct(ctx *parseCtx, typeName string) *ast.StructType {
	ts := ctx.typeSpecByName[typeName]
	if ts == nil {
		return nil
	}
	st, _ := ts.Type.(*ast.StructType)
	return st
}

// actionIsUnderPage reports whether action is under page.
// Rules:
//   - page must be prefix of action
//   - boundary: either page=="/" OR next char after prefix is '/'
//   - disallow exact equality (action == page) to avoid colliding with GET route
func actionIsUnderPage(page, action string) bool {
	page = cleanPath(page)
	action = cleanPath(action)

	if page == "" || action == "" {
		return false
	}
	if page == "/" {
		return strings.HasPrefix(action, "/")
	}
	if !strings.HasPrefix(action, page) {
		return false
	}
	if len(action) == len(page) {
		return false // disallow exact match
	}
	return action[len(page)] == '/'
}
