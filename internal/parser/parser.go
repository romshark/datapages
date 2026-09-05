package parser

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/internal/gotypes"
	"github.com/romshark/datapages/internal/parser/internal/methodkind"
	"github.com/romshark/datapages/internal/parser/internal/paramvalidation"
	"github.com/romshark/datapages/internal/parser/internal/structinspect"
	"github.com/romshark/datapages/internal/parser/internal/templcheck"
	"github.com/romshark/datapages/internal/parser/internal/typecheck"
	"github.com/romshark/datapages/internal/parser/internal/urlpath"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/parser/validate"
	"github.com/romshark/datapages/internal/routepattern"
	"github.com/romshark/datapages/internal/subject"
)

func Parse(appPackagePath string) (app *model.App, errs Errors) {
	defer sortErrors(&errs)

	pkg, err := loadPackage(appPackagePath)
	if err != nil {
		errs.Err(err)
		return nil, errs
	}

	// A package that does not compile has no model to validate.
	// Every rule below reads a type, and a type the compiler could not
	// resolve turns into framework errors that name the wrong thing:
	// a project whose *_templ.go files were never written reports an
	// invalid datapages.Component as a wrong GET signature.
	for _, pe := range pkg.Errors {
		// Only the message: the position is reported separately and
		// packages.Error prints it as part of its own text.
		errs.ErrAt(posFromPackagesError(pe), errors.New(pe.Msg))
	}
	if pkg.Types == nil || pkg.TypesInfo == nil {
		errs.ErrAt(earliestPkgPos(pkg),
			errors.New("missing source package type information"))
		return nil, errs
	}
	if len(pkg.Errors) > 0 {
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
	collectAssets(&ctx, &errs)
	// Both run after flattenPages: a session-carrying handler declared on an
	// abstract page is only reachable from ctx.pages once it has been adopted.
	flattenPages(&ctx, &errs)
	collectSessionType(&ctx, &errs)
	validateEventsNeedSession(&ctx, &errs)
	validateRequiredHandlers(&ctx, &errs)
	finalizePages(&ctx)
	assignSpecialPages(&ctx, &errs)
	validateRouteConflicts(&ctx, &errs)
	checkTemplFiles(&ctx, &errs)

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

	// subject -> the event type that claimed it first.
	eventSubjects map[string]string

	// Every subject claimed so far, in declaration order,
	// for the overlap check an exact map cannot make.
	eventClaims []eventClaim

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
		eventSubjects:       map[string]string{},
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
	ctx.app = &model.App{
		Fset:    ctx.pkg.Fset,
		PkgPath: ctx.pkg.PkgPath,
		PkgName: ctx.pkg.Name,
	}
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
		checkTypeParams(ctx, errs, name, ts)

		// Only treat valid EventXXX as event types.
		if err := validate.EventTypeName(name); err == nil {
			firstPassEventType(ctx, errs, name, ts)
			continue
		}

		// Pages / abstracts are structs only.
		firstPassPageOrAbstractType(ctx, errs, name, ts)
	}
}

// collectSessionType decides whether the application uses sessions and which
// datapages.Session instantiation it uses. Sessions are in play as soon as any
// handler has a session-related input or output. Only session and newSession
// name the Data type, an application that merely returns closeSession gets
// datapages.Session[struct{}].
func collectSessionType(ctx *parseCtx, errs *Errors) {
	usesSession := false
	noteHandler := func(h *model.Handler) {
		if h == nil {
			return
		}
		for _, in := range []*model.Input{h.InputSession} {
			if in == nil {
				continue
			}
			usesSession = true
			if in.Type.TypeExpr != nil {
				noteSessionType(ctx, errs, in.Type.TypeExpr, ctx.pkg.TypesInfo)
			}
		}
		for _, o := range []*model.Output{h.OutputNewSession, h.OutputCloseSession} {
			if o == nil {
				continue
			}
			usesSession = true
			if o.Type.TypeExpr != nil {
				noteSessionType(ctx, errs, o.Type.TypeExpr, ctx.pkg.TypesInfo)
			}
		}
	}
	for _, p := range ctx.pages {
		if p.GET != nil {
			noteHandler(p.GET.Handler)
		}
		noteHandler(p.StreamOpen)
		noteHandler(p.StreamClose)
		for _, h := range p.Actions {
			noteHandler(h)
		}
		for _, eh := range p.EventHandlers {
			if eh.InputSession != nil {
				usesSession = true
				if eh.InputSession.Type.TypeExpr != nil {
					noteSessionType(ctx, errs,
						eh.InputSession.Type.TypeExpr, ctx.pkg.TypesInfo)
				}
			}
		}
	}
	for _, h := range ctx.app.Actions {
		noteHandler(h)
	}
	if gh := ctx.app.GlobalHeadGenerator; gh != nil && gh.InputSession {
		usesSession = true
	}
	if usesSession && ctx.app.Session == nil {
		// Sessions are used, but no handler names the Data type.
		ctx.app.Session = &model.SessionType{
			Data: model.Type{Resolved: types.NewStruct(nil, nil)},
		}
	}
}

// noteSessionType records the datapages.Session[Data] instantiation used by a handler.
// All handlers of an application must use the same one,
// since the server holds a single session manager.
func noteSessionType(
	ctx *parseCtx, errs *Errors, expr ast.Expr, info *types.Info,
) {
	data, ok := typecheck.SessionDataType(expr, info)
	if !ok {
		if data, ok = typecheck.NewSessionDataType(expr, info); !ok {
			return
		}
	}
	pos := ctx.pkg.Fset.Position(expr.Pos())
	if ctx.app.Session == nil {
		ctx.app.Session = &model.SessionType{
			Expr: expr,
			Data: model.Type{Resolved: data, TypeExpr: expr},
		}
		return
	}
	if !types.Identical(ctx.app.Session.Data.Resolved, data) {
		errs.ErrAt(pos, fmt.Errorf("%w: %s and %s",
			ErrSessionTypeConflict,
			types.TypeString(ctx.app.Session.Data.Resolved, nil),
			types.TypeString(data, nil)))
	}
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
			// validate returns no other error for a comment, an unknown one
			// still means the comment cannot be read.
			errs.ErrAt(typePos, &ErrorEventCommInvalid{TypeName: name})
		}
		return
	}

	sfResult := structinspect.SubjectFields(ts, ctx.pkg.TypesInfo)
	if sfResult.AfterPayload != nil {
		errs.ErrAt(
			ctx.pkg.Fset.Position(sfResult.AfterPayload.Pos),
			&ErrorEventSubjectAfterPayload{
				FieldName: sfResult.AfterPayload.FieldName,
				TypeName:  name,
			},
		)
	}
	if sfResult.DuplicateSignal != nil {
		errs.ErrAt(
			ctx.pkg.Fset.Position(sfResult.DuplicateSignal.Pos),
			&ErrorEventSubjectDuplicateSignal{
				FieldName:      sfResult.DuplicateSignal.FieldName,
				FirstFieldName: sfResult.DuplicateSignalFirst,
				SignalName:     sfResult.DuplicateSignal.SignalName,
				TypeName:       name,
			},
		)
	}
	if sfResult.UserWithSignal != nil {
		errs.ErrAt(
			ctx.pkg.Fset.Position(sfResult.UserWithSignal.Pos),
			&ErrorEventSubjectUserSignal{TypeName: name},
		)
	}
	if sfResult.InvalidSignal != nil {
		errs.ErrAt(
			ctx.pkg.Fset.Position(sfResult.InvalidSignal.Pos),
			&ErrorEventSubjectSignalInvalid{
				FieldName:  sfResult.InvalidSignal.FieldName,
				SignalName: sfResult.InvalidSignal.SignalName,
				TypeName:   name,
			},
		)
	}

	for _, sf := range sfResult.Unexported {
		errs.ErrAt(
			ctx.pkg.Fset.Position(sf.Pos),
			fmt.Errorf("%w: field %s in %s",
				ErrEventFieldUnexported, sf.FieldName, name),
		)
	}
	for _, sf := range sfResult.Prefixed {
		errs.ErrAt(
			ctx.pkg.Fset.Position(sf.Pos),
			&ErrorEventSubjectPrefixedField{
				FieldName: sf.FieldName,
				TypeName:  name,
			},
		)
	}

	var subjectFields []model.SubjectField
	for _, sf := range sfResult.Fields {
		subjectFields = append(subjectFields, model.SubjectField{
			FieldName:  sf.FieldName,
			Kind:       sf.Kind,
			SignalName: sf.SignalName,
		})
	}

	if first, ok := ctx.eventSubjects[subj]; ok {
		errs.ErrAt(typePos, &ErrorEventSubjectDuplicate{
			Subject:       subj,
			TypeName:      name,
			FirstTypeName: first,
		})
		return
	}

	claim := eventClaim{
		Subject: subj, HasFields: len(subjectFields) > 0,
		typeName: name,
	}
	for _, first := range ctx.eventClaims {
		if !claim.Overlaps(first.Claim) {
			continue
		}
		errs.ErrAt(typePos, &ErrorEventSubjectOverlap{
			Subject:       subj,
			TypeName:      name,
			FirstSubject:  first.Subject,
			FirstTypeName: first.typeName,
		})
		return
	}

	ctx.eventSubjects[subj] = name
	ctx.eventClaims = append(ctx.eventClaims, claim)

	ctx.app.Events = append(ctx.app.Events, &model.Event{
		Expr:          ts.Name,
		TypeName:      name,
		Subject:       subj,
		SubjectFields: subjectFields,
	})
}

// eventClaim names the event behind a subject claim.
type eventClaim struct {
	subject.Claim
	typeName string
}

func extractEventSubject(
	typeName string, doc *ast.CommentGroup,
) (string, error) {
	if err := validate.EventSubjectComment(typeName, doc); err != nil {
		return "", err
	}

	for _, c := range doc.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		txt = strings.TrimSpace(txt)
		rest, ok := validate.CutEventIsPrefix(txt, typeName)
		if !ok {
			continue
		}
		// [validate.EventSubjectComment] passed, hence the subject is quoted
		// and carries a payload.
		if len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"' {
			return rest[1 : len(rest)-1], nil
		}
		break
	}
	// Unreachable while validation and extraction read the same comment.
	return "", validate.ErrEventSubjectInvalid
}

// eventSubjectPos returns the position of the subject value
// (the quoted string after "is ") in the doc comment for an event type.
// Falls back to fallback when the comment cannot be located.
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

// checkTypeParams reports a type parameter list on a type datapages generates for.
// Generated code names such a type without type arguments, which doesn't compile.
// Any other generic type of the app package is left alone.
func checkTypeParams(ctx *parseCtx, errs *Errors, name string, ts *ast.TypeSpec) {
	if ts.TypeParams == nil || len(ts.TypeParams.List) == 0 {
		return
	}
	generated := name == "App" || strings.HasPrefix(name, "Page") ||
		validate.EventTypeName(name) == nil
	if !generated {
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return
		}
		// An abstract page is a struct carrying App *App.
		generated = structinspect.HasRequiredAppField(st, ctx.pkg.TypesInfo)
	}
	if !generated {
		return
	}
	errs.ErrAt(ctx.pkg.Fset.Position(ts.Name.Pos()),
		fmt.Errorf("%w: %s", ErrTypeParams, name))
}

func firstPassPageOrAbstractType(
	ctx *parseCtx, errs *Errors, name string, ts *ast.TypeSpec,
) {
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		// A defined non-struct type and an alias both claim a page name
		// without being a page. Without this the route is simply absent.
		if strings.HasPrefix(name, "Page") {
			errs.ErrAt(ctx.pkg.Fset.Position(ts.Name.Pos()),
				fmt.Errorf("%w: %s", ErrPageNotStruct, name))
		}
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

			// App hooks: (*App).Head, (*App).RecoverError, and App-level actions.
			if recv == "App" {
				switch fd.Name.Name {
				case "Head":
					info := ctx.pkg.TypesInfo
					pos := ctx.pkg.Fset.Position(fd.Name.Pos())

					// Head must return exactly datapages.Head.
					results := fd.Type.Results
					if results == nil || results.NumFields() != 1 ||
						!typecheck.IsHeadType(results.List[0].Type, info) {
						errs.ErrAt(pos, ErrAppHeadMustReturnHead)
						continue
					}

					// Head must take *http.Request and may take a session.
					// Both are matched by type in any order.
					gh := &model.GlobalHead{Expr: fd.Name}
					valid := true
					nReq, nSess := 0, 0
					if fd.Type.Params != nil {
						for _, f := range expandFieldList(fd.Type.Params.List) {
							switch {
							case typecheck.IsPtrToNetHTTPReq(f.Type, info):
								nReq++
								gh.OrderedInputs = append(
									gh.OrderedInputs, model.InputKindRequest,
								)
							case typecheck.IsSessionType(f.Type, info):
								nSess++
								gh.InputSession = true
								noteSessionType(ctx, errs, f.Type, info)
								gh.OrderedInputs = append(
									gh.OrderedInputs, model.InputKindSession,
								)
							default:
								errs.ErrAt(pos, ErrAppHeadUnsupportedInput)
								valid = false
							}
						}
					}
					if !valid {
						continue
					}
					if nReq != 1 || nSess > 1 {
						errs.ErrAt(pos, ErrAppHeadMustTakeRequest)
						continue
					}

					ctx.app.GlobalHeadGenerator = gh
				case "RecoverError":
					info := ctx.pkg.TypesInfo
					pos := ctx.pkg.Fset.Position(fd.Name.Pos())
					results := fd.Type.Results
					if results == nil || results.NumFields() != 1 ||
						!typecheck.IsError(info.TypeOf(results.List[0].Type)) {
						errs.ErrAt(pos, ErrAppRecoverErrorInvalidSignature)
						continue
					}
					// The error and the SSE are matched by type in any order.
					var ordered []string
					nErr, nSSE := 0, 0
					if fd.Type.Params != nil {
						for _, f := range expandFieldList(fd.Type.Params.List) {
							switch {
							case typecheck.IsError(info.TypeOf(f.Type)):
								nErr++
								ordered = append(ordered, model.InputKindErr)
							case typecheck.IsSSEParam(f.Type, info):
								nSSE++
								ordered = append(ordered, model.InputKindSSE)
							default:
								ordered = append(ordered, "")
							}
						}
					}
					if nErr != 1 || nSSE != 1 || len(ordered) != 2 {
						errs.ErrAt(pos, ErrAppRecoverErrorInvalidSignature)
						continue
					}
					ctx.app.RecoverError = &model.RecoverError{
						Expr:          fd.Name,
						OrderedInputs: ordered,
					}
				default:
					kind, suffix := methodkind.Classify(fd.Name.Name)
					if kind != 0 && !kind.IsAction() {
						// A name the framework reserves on a page means nothing on App.
						// Silence would leave the method compiled,
						// never called and never mentioned.
						errs.ErrAt(ctx.pkg.Fset.Position(fd.Name.Pos()),
							fmt.Errorf("%w: App.%s",
								ErrAppUnsupportedMethod, fd.Name.Name))
					}
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
				if fd.Name.IsExported() {
					errs.ErrAt(
						ctx.pkg.Fset.Position(fd.Name.Pos()),
						fmt.Errorf("%w: %s.%s", ErrUnsupportedMethod, recv, fd.Name.Name),
					)
				}
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
			case methodkind.StreamOpenHook, methodkind.StreamCloseHook:
				validateAndAttachStreamHook(ctx, errs, recv, fd, pg, ap, kind)
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
	//   - Must have exactly one parameter of an EventXXX type
	//   - Must have a *datastar.ServerSentEventGenerator parameter
	//   - Only one handler per EventXXX per receiver type
	//   - Parameters may be in any order
	params := fd.Type.Params
	var evName string

	// Find the event parameter by type. The event a handler takes is the one
	// parameter carrying a declared event type, whatever the parameter is named.
	evParams := 0
	if params != nil {
		for _, f := range params.List {
			name, ok := typecheck.EventTypeNameOf(
				f.Type, ctx.pkg.TypesInfo, ctx.eventTypeNames,
			)
			if !ok {
				continue
			}
			evParams += max(len(f.Names), 1)
			if evName == "" {
				evName = name
			}
		}
	}
	switch {
	case evParams == 0:
		errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
			ErrSignatureEvHandMissingEvent, recv, fd.Name.Name))
	case evParams > 1:
		errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
			ErrSignatureEvHandMultipleEvents, recv, fd.Name.Name))
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
			if typecheck.IsSSEParam(f.Type, ctx.pkg.TypesInfo) {
				foundSSE = true
				break
			}
		}
	}
	if !foundSSE {
		errs.ErrAt(pos,
			fmt.Errorf("%w: %s.%s", ErrSignatureEvHandMissingSSE, recv, fd.Name.Name))
	}

	// What is left after the event, the SSE, the session and the stream ID,
	// each of which was matched by type already, is unsupported.
	if params != nil {
		for _, f := range params.List {
			switch {
			case typecheck.IsEventType(f.Type, ctx.pkg.TypesInfo, evName):
			case typecheck.IsSSEParam(f.Type, ctx.pkg.TypesInfo):
			case paramvalidation.IsSessionParam(f, ctx.pkg.TypesInfo):
			case typecheck.IsStreamIDType(f.Type, ctx.pkg.TypesInfo):
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

	// A handler with an empty evName carries no event for the override checks
	// to match on. It is attached anyway, for its AST position.
	if pg != nil {
		pg.EventHandlers = append(pg.EventHandlers, h)
	} else {
		ap.EventHandlers = append(ap.EventHandlers, h)
	}
}

func validateAndAttachStreamHook(
	ctx *parseCtx,
	errs *Errors,
	recv string,
	fd *ast.FuncDecl,
	pg *model.Page,
	ap *model.AbstractPage,
	kind methodkind.Kind,
) {
	pos := ctx.pkg.Fset.Position(fd.Name.Pos())

	h, herr := parseStreamHook(
		recv, fd, ctx.pkg.TypesInfo, ctx.pkg.Fset, ctx.eventTypeNames, kind,
	)
	if herr != nil {
		reportErrorsWithFset(errs, ctx.pkg.Fset, pos, herr)
	}

	if pg != nil {
		if kind == methodkind.StreamOpenHook {
			pg.StreamOpen = h
		} else {
			pg.StreamClose = h
		}
		return
	}
	if kind == methodkind.StreamOpenHook {
		ap.StreamOpen = h
	} else {
		ap.StreamClose = h
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
				&ErrorActionMissingPathComm{
					PagePath: pagePath, Recv: recv, MethodName: fd.Name.Name,
				})
		} else if !valid {
			errs.ErrAt(pos,
				&ErrorActionInvalidPathComm{Recv: recv, MethodName: fd.Name.Name})
		} else if pg != nil && pg.Route != "" && !actionIsUnderPage(pg.Route, r) {
			errs.ErrAt(pos,
				&ErrorActionPathNotUnderPage{
					PagePath: pg.Route, Recv: recv, MethodName: fd.Name.Name,
				})
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
		if rsErr := paramvalidation.ValidateReflectSignal(h, recv, fd.Name.Name); rsErr != nil {
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
				get, getErr := buildHandlerGET(h, outputs)
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

	getOwner := ""             // "page" or abstract type name
	getOwnerPos := token.NoPos // Embed site for an inherited GET
	streamOpenOwner := ""
	streamOpenOwnerPos := token.NoPos
	streamClosedOwner := ""
	streamClosedOwnerPos := token.NoPos

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
	if pg.StreamOpen != nil {
		streamOpenOwner = "page"
		if pg.StreamOpen.Expr != nil {
			streamOpenOwnerPos = pg.StreamOpen.Expr.Pos()
		}
	}
	if pg.StreamClose != nil {
		streamClosedOwner = "page"
		if pg.StreamClose.Expr != nil {
			streamClosedOwnerPos = pg.StreamClose.Expr.Pos()
		}
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

		// A child carries the position of its embed site in the parent abstract,
		// which is where a conflict it introduces is reported.
		apEmbPos := structinspect.EmbeddedFieldPosMap(typeStruct(ctx, ap.TypeName))
		for _, child := range ap.Embeds {
			queue = append(queue, qitem{
				ap:       child,
				embedPos: apEmbPos[child.TypeName],
			})
		}

		for _, m := range ap.Methods {
			if m.HTTPMethod == "GET" {
				// Page's own GET always wins; no conflict in that case.
				if getOwner == "page" {
					continue
				}
				// First embedded GET wins (record embed site).
				if getOwner == "" {
					get, getErr := buildHandlerGET(m, ctx.handlerOutputs[m])
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

		if ap.StreamOpen != nil {
			switch streamOpenOwner {
			case "":
				streamOpenOwner = ap.TypeName
				if ap.StreamOpen.Expr != nil {
					streamOpenOwnerPos = ap.StreamOpen.Expr.Pos()
				}
				pg.StreamOpen = ap.StreamOpen
			case "page", ap.TypeName:
				// Page-owned or already inherited from the same abstract wins.
			default:
				pos := ctx.pkg.Fset.Position(pg.Expr.Pos())
				if it.embedPos != token.NoPos {
					pos = ctx.pkg.Fset.Position(it.embedPos)
				}
				prevPos := token.Position{}
				if streamOpenOwnerPos != token.NoPos {
					prevPos = ctx.pkg.Fset.Position(streamOpenOwnerPos)
				}
				errs.ErrAt(pos, fmt.Errorf(
					"%w: %s inherits %s and %s which both "+
						"define StreamOpen (previous at %s)",
					ErrStreamHookDuplicateEmbed,
					pg.TypeName,
					streamOpenOwner,
					ap.TypeName,
					prevPos,
				))
			}
		}

		if ap.StreamClose != nil {
			switch streamClosedOwner {
			case "":
				streamClosedOwner = ap.TypeName
				if ap.StreamClose.Expr != nil {
					streamClosedOwnerPos = ap.StreamClose.Expr.Pos()
				}
				pg.StreamClose = ap.StreamClose
			case "page", ap.TypeName:
				// Page-owned or already inherited from the same abstract wins.
			default:
				pos := ctx.pkg.Fset.Position(pg.Expr.Pos())
				if it.embedPos != token.NoPos {
					pos = ctx.pkg.Fset.Position(it.embedPos)
				}
				prevPos := token.Position{}
				if streamClosedOwnerPos != token.NoPos {
					prevPos = ctx.pkg.Fset.Position(streamClosedOwnerPos)
				}
				errs.ErrAt(pos, fmt.Errorf(
					"%w: %s inherits %s and %s which both define StreamClose "+
						"(previous at %s)",
					ErrStreamHookDuplicateEmbed,
					pg.TypeName,
					streamClosedOwner,
					ap.TypeName,
					prevPos,
				))
			}
		}

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

// validateRouteConflicts reports routes that cannot live in one router.
// The generated server registers them on a net/http ServeMux,
// which rejects a pattern matching the same requests as one already registered.
// Asking the same mux keeps the answer identical to the one the server gets on startup.
func validateRouteConflicts(ctx *parseCtx, errs *Errors) {
	mux := http.NewServeMux()
	claim := func(method, route string, expr ast.Expr, owner string) {
		// A route that is not a path is already reported where it is read.
		if !strings.HasPrefix(route, "/") {
			return
		}
		pattern := method + " " + route
		if err := registerRoute(mux, pattern); err != nil {
			errs.ErrAt(ctx.pkg.Fset.Position(expr.Pos()), &ErrorRouteConflict{
				Pattern: pattern,
				Owner:   owner,
				Reason:  err.Error(),
			})
		}
	}

	// The core registers the assets prefix on the same mux. Claiming it first
	// makes the page that wants the same requests the one reported,
	// which is the one the user can move.
	if prefix := ctx.app.Assets.URLPrefix; prefix != "" {
		_ = registerRoute(mux, http.MethodGet+" "+prefix)
	}

	events := map[string]*model.Event{}
	for _, e := range ctx.app.Events {
		events[e.TypeName] = e
	}

	for _, p := range ctx.app.Pages {
		if p.GET != nil && p.GET.Handler != nil {
			claim(http.MethodGet, pageRoutePattern(p), p.Expr, p.TypeName)
		}
		switch {
		case !pageHasStream(p):
		case routepattern.EndsInWildcard(p.Route):
			// The stream path of such a route parses nowhere,
			// which is what this reports. Claiming it would report it a second time.
			errs.ErrAt(ctx.pkg.Fset.Position(p.Expr.Pos()),
				&ErrorRouteWildcardStream{TypeName: p.TypeName, Route: p.Route})
		default:
			stream := routepattern.StreamPath(p.Route)
			claim(http.MethodGet, stream+"{$}", p.Expr, p.TypeName+" stream")
			if pageHasAnonStream(p, events) {
				claim(http.MethodGet, stream+"anon/{$}", p.Expr,
					p.TypeName+" anonymous stream")
			}
		}
		for _, h := range p.Actions {
			claim(h.HTTPMethod, actionRoutePattern(h.Route), h.Expr,
				p.TypeName+"."+h.Name)
		}
	}
	for _, h := range ctx.app.Actions {
		claim(h.HTTPMethod, actionRoutePattern(h.Route), h.Expr, "App."+h.Name)
	}
}

// pageRoutePattern is the pattern the generated setupHandlers registers the page under.
// A route that is not a path is left alone, [claim] drops it.
func pageRoutePattern(p *model.Page) string {
	switch {
	case p.PageSpecialization == model.PageTypeIndex:
		return "/"
	case !strings.HasPrefix(p.Route, "/"):
		return p.Route
	case routepattern.EndsInWildcard(p.Route):
		// A wildcard runs to the end of the path. Marking the end after it
		// puts the wildcard in the middle, which net/http will not parse.
		return p.Route
	}
	return routepattern.WithTrailingSlash(p.Route) + "{$}"
}

// actionRoutePattern is the pattern the generated setupHandlers registers an
// action under.
func actionRoutePattern(route string) string {
	if !strings.HasPrefix(route, "/") {
		return route
	}
	return routepattern.WithTrailingSlash(route) + "{$}"
}

// pageHasAnonStream reports whether the page is served a second, anonymous stream.
// A page mixing public and private events gets one.
func pageHasAnonStream(p *model.Page, events map[string]*model.Event) bool {
	hasPublic, hasPrivate := false, false
	for _, eh := range p.EventHandlers {
		e, ok := events[eh.EventTypeName]
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

// pageHasStream reports whether the page is served an SSE stream of its own.
func pageHasStream(p *model.Page) bool {
	return len(p.EventHandlers) > 0 || p.StreamOpen != nil || p.StreamClose != nil
}

// registerRoute reports what ServeMux says about a pattern.
// ServeMux says it by panicking, which this turns into a value.
func registerRoute(mux *http.ServeMux, pattern string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(routerReason(fmt.Sprint(r)))
		}
	}()
	mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return nil
}

// routerReason takes the sentence out of a ServeMux panic.
// A conflict is reported over two lines, the second of which says what the conflict is.
// The first names the file registering each pattern, which is this file.
func routerReason(msg string) string {
	if _, rest, ok := strings.Cut(msg, ":\n"); ok {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(msg)
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
		case typecheck.IsEventType(f.Type, info, eventTypeName):
			h.InputEvent = parseInput(f, f.Type, info)
			h.InputEvent.Kind = model.InputKindEvent
			h.OrderedInputs = append(h.OrderedInputs, h.InputEvent)
		case typecheck.IsSSEParam(f.Type, info):
			h.InputSSE = parseInput(f, f.Type, info)
			h.InputSSE.Kind = model.InputKindSSE
			h.OrderedInputs = append(h.OrderedInputs, h.InputSSE)
		case typecheck.IsStreamIDType(f.Type, info):
			h.InputStreamID = parseInput(f, f.Type, info)
			h.InputStreamID.Kind = model.InputKindStreamID
			h.OrderedInputs = append(h.OrderedInputs, h.InputStreamID)
		case paramvalidation.IsSessionParam(f, info):
			h.InputSession = parseInput(f, f.Type, info)
			h.InputSession.Kind = model.InputKindSession
			h.OrderedInputs = append(h.OrderedInputs, h.InputSession)
		}
	}

	if fd.Type.Results != nil && len(fd.Type.Results.List) > 0 {
		h.OutputErr = &model.Output{
			Kind: model.OutputKindErr,
			Type: makeType(fd.Type.Results.List[0].Type, info),
		}
	}

	return h
}

func parseStreamHook(
	recv string,
	fd *ast.FuncDecl,
	info *types.Info,
	fset *token.FileSet,
	eventTypeNames map[string]struct{},
	kind methodkind.Kind,
) (*model.Handler, error) {
	h := &model.Handler{
		Expr: fd.Name,
		Name: fd.Name.Name,
	}

	params := fd.Type.Params
	if params == nil || len(params.List) == 0 {
		return h, fmt.Errorf("%w in %s.%s",
			ErrSignatureMissingReq, recv, fd.Name.Name)
	}
	expandedParams := expandFieldList(params.List)

	var unsupErrs []error
	foundReq := false
	foundStreamID := false
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
			h.InputRequest = parseInput(f, f.Type, info)
			h.InputRequest.Kind = model.InputKindRequest
			h.OrderedInputs = append(h.OrderedInputs, h.InputRequest)
			foundReq = true

		case typecheck.IsStreamIDType(f.Type, info):
			if h.InputStreamID != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			h.InputStreamID = parseInput(f, f.Type, info)
			h.InputStreamID.Kind = model.InputKindStreamID
			h.OrderedInputs = append(h.OrderedInputs, h.InputStreamID)
			foundStreamID = true

		case typecheck.IsSSEParam(f.Type, info):
			if kind != methodkind.StreamOpenHook {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			if h.InputSSE != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			h.InputSSE = parseInput(f, f.Type, info)
			h.InputSSE.Kind = model.InputKindSSE
			h.OrderedInputs = append(h.OrderedInputs, h.InputSSE)

		case paramvalidation.IsSessionParam(f, info):
			if h.InputSession != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			h.InputSession = parseInput(f, f.Type, info)
			h.InputSession.Kind = model.InputKindSession
			h.OrderedInputs = append(h.OrderedInputs, h.InputSession)

		case paramvalidation.IsSignalsParam(f, info):
			if kind != methodkind.StreamOpenHook {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			if h.InputSignals != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			values := typecheck.TypeArgExpr(f.Type)
			sigErr := paramvalidation.ValidateSignalsStruct(
				values, info, recv, fd.Name.Name,
			)
			if sigErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), sigErr)
				continue
			}
			h.InputSignals = parseInput(f, values, info)
			h.InputSignals.Kind = model.InputKindSignals
			h.OrderedInputs = append(h.OrderedInputs, h.InputSignals)

		case paramvalidation.IsDispatchParam(f, info):
			eventName, dispErr := paramvalidation.ValidateDispatch(
				f, info, eventTypeNames, recv, fd.Name.Name,
			)
			if dispErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), dispErr)
				continue
			}
			if slices.ContainsFunc(h.InputDispatches,
				func(d *model.InputDispatch) bool {
					return d.EventTypeName == eventName
				}) {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(),
					&ErrorDispatchDuplicate{
						Recv:          recv,
						MethodName:    fd.Name.Name,
						EventTypeName: eventName,
					})
				continue
			}
			inp := parseInput(f, f.Type, info)
			inp.Kind = model.InputKindDispatch
			h.InputDispatches = append(h.InputDispatches, &model.InputDispatch{
				Input:         inp,
				EventTypeName: eventName,
			})
			h.OrderedInputs = append(h.OrderedInputs, inp)

		default:
			unsupErrs = append(unsupErrs,
				fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
		}
	}

	if !foundReq {
		return h, fmt.Errorf("%w in %s.%s",
			ErrSignatureMissingReq, recv, fd.Name.Name)
	}
	if !foundStreamID {
		return h, fmt.Errorf("%w in %s.%s",
			ErrSignatureMissingStreamID, recv, fd.Name.Name)
	}
	if len(unsupErrs) > 0 {
		return h, errors.Join(unsupErrs...)
	}
	// Stream hooks may return error or nothing.
	noReturn := fd.Type.Results == nil || len(fd.Type.Results.List) == 0
	if !noReturn && !eventHandlerReturnsOnlyError(fd, info) {
		retPos := fset.Position(fd.Type.Results.Pos())
		return h, &positionedError{
			pos: retPos,
			err: fmt.Errorf("%w: %s.%s",
				ErrSignatureStreamHookReturnMustBeError, recv, fd.Name.Name),
		}
	}

	if !noReturn {
		h.OutputErr = &model.Output{
			Kind: model.OutputKindErr,
			Type: makeType(fd.Type.Results.List[0].Type, info),
		}
	}
	return h, nil
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

	// A path that does not exist is loaded as an import pattern instead.
	pattern := appPackagePath
	if filepath.IsAbs(appPackagePath) {
		// go list rejects an absolute pattern, load its directory instead.
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

	// The route must stand on the first line of the doc comment.
	c := cg.List[0]
	txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))

	want := symbol + " is "
	attemptPrefix := symbol + " "

	if !strings.HasPrefix(txt, attemptPrefix) {
		return "", false, false
	}

	// The comment names the symbol, hence it is an attempt at a route:
	// found is true from here on, valid is not.
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
func resolveErrorPos(
	e error, fset *token.FileSet, fallback token.Position,
) token.Position {
	if pe, ok := errors.AsType[*positionedError](e); ok {
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
	if pe, ok := errors.AsType[*positionedError](e); ok {
		return pe.err
	}
	return e
}

// reportErrorsWithFset is like reportErrors but also resolves ASTPos()
// positions using the provided FileSet.
func reportErrorsWithFset(
	errs *Errors, fset *token.FileSet, pos token.Position, err error,
) {
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

// unsupportedInputError builds an ErrorSignatureUnsupportedInput for a
// parameter that doesn't match any recognized handler input.
// When h is non-nil, it names the inputs whose type the parameter could carry
// and whose slot the handler hasn't filled yet.
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

	// Name the inputs the parameter's type could stand for,
	// skipping the slots the handler has already filled.
	if h != nil {
		switch {
		case typecheck.IsPtrToNetHTTPReq(f.Type, info):
			// Already consumed — this is a duplicate *http.Request.
		case typecheck.IsSSEParam(f.Type, info):
			// Already consumed — duplicate SSE parameter.
		case typecheck.IsPtrToDatastarSSE(f.Type, info):
			// The raw Datastar generator is not a supported parameter type.
			e.CandidateNames = []string{"sse datapages.SSE"}
		default:
			e.CandidateNames = typeCandidates(f, h, info)
		}
	}

	return e
}

// typeCandidates returns the unconsumed handler inputs whose
// expected type matches the field's type.
func typeCandidates(
	f *ast.Field, h *model.Handler, info *types.Info,
) []string {
	t := info.TypeOf(f.Type)
	if t == nil {
		return nil
	}

	isSession := typecheck.IsSessionType(f.Type, info)
	isUint64 := gotypes.IsUint64(t)
	isStruct := isStructType(t)

	type candidate struct {
		name     string
		consumed bool
		match    bool
	}
	all := []candidate{
		{"datapages.StreamID", h.InputStreamID != nil, isUint64},
		{"datapages.Session[Data]", h.InputSession != nil, isSession},
		// The path, query and signals wrappers all carry a plain struct,
		// hence a plain struct suggests wrapping. A named type with
		// a more specific match, Session for one, is not a candidate.
		{"datapages.Path[...]", h.InputPath != nil, isStruct && !isSession},
		{"datapages.Query[...]", h.InputQuery != nil, isStruct && !isSession},
		{"datapages.Signals[...]", h.InputSignals != nil, isStruct && !isSession},
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

// parseInput builds the model input for a handler parameter.
// typeExpr is the type the model records, which is f.Type for a plain
// parameter and the Values type argument for a wrapped one
// (datapages.Path, datapages.Query, datapages.Signals).
func parseInput(f *ast.Field, typeExpr ast.Expr, info *types.Info) *model.Input {
	// An unnamed parameter still yields an input, with an empty name.
	name := ""
	var expr ast.Expr
	if len(f.Names) > 0 {
		name = f.Names[0].Name
		expr = f.Names[0]
	}
	return &model.Input{
		Expr: expr,
		Name: name,
		Type: makeType(typeExpr, info),
	}
}

func makeType(typeExpr ast.Expr, info *types.Info) model.Type {
	return model.Type{
		Resolved: info.TypeOf(typeExpr),
		TypeExpr: typeExpr,
	}
}

func buildHandlerGET(
	h *model.Handler, outputs []*model.Output,
) (*model.HandlerGET, error) {
	get := &model.HandlerGET{
		Handler: h,
	}

	for _, out := range outputs {
		switch out.Kind {
		case model.OutputKindBody:
			if get.OutputBody == nil {
				get.OutputBody = &model.TemplComponent{Output: out}
			}
		case model.OutputKindHead:
			if get.OutputHead == nil {
				get.OutputHead = &model.TemplComponent{Output: out}
			}
		}
	}

	// Validate: GET must return a body datapages.Component
	if get.OutputBody == nil {
		return get, ErrSignatureGETMissingBody
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
	// Expand multi-name fields (e.g. "r, a *http.Request" -> two fields)
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
			h.InputRequest = parseInput(f, f.Type, info)
			h.InputRequest.Kind = model.InputKindRequest
			h.OrderedInputs = append(h.OrderedInputs, h.InputRequest)
			foundReq = true

		case typecheck.IsSSEParam(f.Type, info):
			if h.InputSSE != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			h.InputSSE = parseInput(f, f.Type, info)
			h.InputSSE.Kind = model.InputKindSSE
			h.OrderedInputs = append(h.OrderedInputs, h.InputSSE)

		case paramvalidation.IsSessionParam(f, info):
			if h.InputSession != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			h.InputSession = parseInput(f, f.Type, info)
			h.InputSession.Kind = model.InputKindSession
			h.OrderedInputs = append(h.OrderedInputs, h.InputSession)

		case paramvalidation.IsPathParam(f, info):
			if h.InputPath != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			values := typecheck.TypeArgExpr(f.Type)
			pathErr := paramvalidation.ValidatePathStruct(
				values, info, recv, fd.Name.Name,
			)
			if pathErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), pathErr)
				continue
			}
			h.InputPath = parseInput(f, values, info)
			h.InputPath.Kind = model.InputKindPath
			h.OrderedInputs = append(h.OrderedInputs, h.InputPath)

		case paramvalidation.IsQueryParam(f, info):
			if h.InputQuery != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			values := typecheck.TypeArgExpr(f.Type)
			queryErr := paramvalidation.ValidateQueryStruct(
				values, info, recv, fd.Name.Name,
			)
			if queryErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), queryErr)
				continue
			}
			h.InputQuery = parseInput(f, values, info)
			h.InputQuery.Kind = model.InputKindQuery
			h.OrderedInputs = append(h.OrderedInputs, h.InputQuery)

		case paramvalidation.IsSignalsParam(f, info):
			if h.InputSignals != nil {
				unsupErrs = append(unsupErrs,
					fieldErr(unsupportedInputError(f, h, info, recv, fd.Name.Name)))
				continue
			}
			values := typecheck.TypeArgExpr(f.Type)
			sigErr := paramvalidation.ValidateSignalsStruct(
				values, info, recv, fd.Name.Name,
			)
			if sigErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), sigErr)
				continue
			}
			h.InputSignals = parseInput(f, values, info)
			h.InputSignals.Kind = model.InputKindSignals
			h.OrderedInputs = append(h.OrderedInputs, h.InputSignals)

		case paramvalidation.IsDispatchParam(f, info):
			eventName, dispErr := paramvalidation.ValidateDispatch(
				f, info, eventTypeNames, recv, fd.Name.Name,
			)
			if dispErr != nil {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(), dispErr)
				continue
			}
			if slices.ContainsFunc(h.InputDispatches,
				func(d *model.InputDispatch) bool {
					return d.EventTypeName == eventName
				}) {
				appendPositioned(&unsupErrs, fset, f.Type.Pos(),
					&ErrorDispatchDuplicate{
						Recv:          recv,
						MethodName:    fd.Name.Name,
						EventTypeName: eventName,
					})
				continue
			}
			inp := parseInput(f, f.Type, info)
			inp.Kind = model.InputKindDispatch
			h.InputDispatches = append(h.InputDispatches, &model.InputDispatch{
				Input:         inp,
				EventTypeName: eventName,
			})
			h.OrderedInputs = append(h.OrderedInputs, inp)

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

	if fd.Type.Results == nil {
		return h, nil, nil
	}

	var outputs []*model.Output
	var multiErrPos token.Pos
	var seenBody, seenHead bool
	for _, r := range fd.Type.Results.List {
		t := makeType(r.Type, info)

		// A result field declares one value per name, or a single unnamed one.
		// Every output is matched by its type, the name is the app's to pick.
		exprs := make([]ast.Expr, 0, max(len(r.Names), 1))
		names := make([]string, 0, max(len(r.Names), 1))
		if len(r.Names) == 0 {
			exprs, names = append(exprs, nil), append(names, "")
		} else {
			for _, n := range r.Names {
				exprs, names = append(exprs, n), append(names, n.Name)
			}
		}

		for i, name := range names {
			out := &model.Output{Expr: exprs[i], Name: name, Type: t}
			pos := r.Type.Pos()
			if exprs[i] != nil {
				pos = exprs[i].Pos()
			}
			retErr := func(err error) *positionedError {
				return &positionedError{pos: fset.Position(pos), err: err}
			}

			dup := func() *positionedError {
				return retErr(fmt.Errorf("%w %s %s in %s.%s",
					ErrSignatureDuplicateOutput, name, t.Resolved,
					recv, fd.Name.Name))
			}

			switch {
			case typecheck.IsError(t.Resolved):
				if h.OutputErr != nil {
					multiErrPos = pos
					continue
				}
				out.Kind = model.OutputKindErr
				h.OutputErr = out
				h.OrderedOutputs = append(h.OrderedOutputs, out)
				continue

			case typecheck.IsRedirectType(r.Type, info):
				if h.OutputRedirect != nil {
					return h, nil, dup()
				}
				out.Kind = model.OutputKindRedirect
				h.OutputRedirect = out

			case typecheck.IsNewSessionType(r.Type, info):
				if h.OutputNewSession != nil {
					return h, nil, dup()
				}
				out.Kind = model.OutputKindNewSession
				h.OutputNewSession = out

			case typecheck.IsCloseSessionType(r.Type, info):
				if h.OutputCloseSession != nil {
					return h, nil, dup()
				}
				out.Kind = model.OutputKindCloseSession
				h.OutputCloseSession = out

			case typecheck.IsEnableBgStreamType(r.Type, info):
				if kind != methodkind.GETHandler {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrEnableBgStreamNotGET, recv, fd.Name.Name))
				}
				if h.OutputEnableBgStream != nil {
					return h, nil, dup()
				}
				out.Kind = model.OutputKindEnableBgStream
				h.OutputEnableBgStream = out

			case typecheck.IsDisableRefreshType(r.Type, info):
				if kind != methodkind.GETHandler {
					return h, nil, retErr(fmt.Errorf("%w in %s.%s",
						ErrDisableRefreshNotGET, recv, fd.Name.Name))
				}
				if h.OutputDisableRefresh != nil {
					return h, nil, dup()
				}
				out.Kind = model.OutputKindDisableRefresh
				h.OutputDisableRefresh = out

			case typecheck.IsHeadType(r.Type, info):
				if seenHead {
					return h, nil, dup()
				}
				seenHead = true
				out.Kind = model.OutputKindHead

			case typecheck.IsComponent(t.Resolved):
				if seenBody {
					return h, nil, dup()
				}
				seenBody = true
				out.Kind = model.OutputKindBody

			default:
				return h, nil, retErr(fmt.Errorf(
					"%w %s %s in %s.%s",
					ErrSignatureUnsupportedOutput, name, t.Resolved,
					recv, fd.Name.Name,
				))
			}

			h.OrderedOutputs = append(h.OrderedOutputs, out)
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
	if h.OutputNewSession != nil && h.InputSSE != nil {
		return h, outputs, fmt.Errorf("%w in %s.%s",
			ErrNewSessionWithSSE, recv, fd.Name.Name)
	}
	if h.OutputCloseSession != nil && h.InputSSE != nil {
		return h, outputs, fmt.Errorf("%w in %s.%s",
			ErrCloseSessionWithSSE, recv, fd.Name.Name)
	}

	// For action handlers, pick up the body and head the same way a GET does.
	if kind.IsAction() {
		for _, out := range outputs {
			switch out.Kind {
			case model.OutputKindBody:
				if h.OutputBody == nil {
					h.OutputBody = &model.TemplComponent{Output: out}
				}
			case model.OutputKindHead:
				if h.OutputHead == nil {
					h.OutputHead = &model.TemplComponent{Output: out}
				}
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
	page = urlpath.Clean(page)
	action = urlpath.Clean(action)

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
		return true // exact match: action at same path as page
	}
	return action[len(page)] == '/'
}

// checkTemplFiles delegates to the templcheck subpackage.
func checkTemplFiles(ctx *parseCtx, errs *Errors) {
	templcheck.Check(ctx.pkg, ctx.app, errs.ErrAt)
}
