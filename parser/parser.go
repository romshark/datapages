package parser

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"datapages/parser/internal/methodkind"
	"datapages/parser/internal/paramvalidation"
	"datapages/parser/internal/structinspect"
	"datapages/parser/internal/structtag"
	"datapages/parser/internal/typecheck"
	"datapages/parser/model"
	"datapages/parser/validate"

	"golang.org/x/tools/go/packages"
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
	ctx.app = &model.App{Fset: ctx.pkg.Fset}
	if appTS, ok := ctx.typeSpecByName["App"]; ok {
		ctx.app.Expr = appTS.Name
		ctx.appTypeFound = true
		return
	}
	errs.ErrAt(ctx.basePos, ErrAppMissingTypeApp)
}

func firstPassTypes(ctx *parseCtx, errs *Errors) {
	typeNames := make([]string, 0, len(ctx.typeSpecByName))
	for name := range ctx.typeSpecByName {
		typeNames = append(typeNames, name)
	}
	slices.Sort(typeNames)

	for _, name := range typeNames {
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

	hasUserID := false
	for i := range underlying.NumFields() {
		f := underlying.Field(i)
		if f.Name() == "UserID" && typecheck.IsString(f.Type()) {
			hasUserID = true
			break
		}
	}
	if !hasUserID {
		errs.ErrAt(typePos, ErrSessionMissingUserID)
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
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrEventCommMissing, name))
		case validate.ErrEventCommInvalid:
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrEventCommInvalid, name))
		case validate.ErrEventSubjectInvalid:
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrEventSubjectInvalid, name))
		default:
			// Defensive fallback: treat as invalid comment.
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrEventCommInvalid, name))
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
	want := typeName + " is "
	for _, c := range doc.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if !strings.HasPrefix(txt, want) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(txt, want))
		rest = strings.TrimSpace(rest)

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
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageMissingFieldApp, name))
		}
		if structinspect.HasDisallowedNamedFields(st) {
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageHasExtraFields, name))
		}

		route, found, ok := parseRoute(
			name, pickDoc(name, ctx.docByType, ctx.genDocByType),
		)
		if !found {
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageMissingPathComm, name))
		} else if !ok {
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageInvalidPathComm, name))
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

			// App hooks: (*App).Head and (*App).Recover500
			if recv == "App" {
				switch fd.Name.Name {
				case "Head":
					ctx.app.GlobalHeadGenerator = fd.Name
				case "Recover500":
					ctx.app.Recover500 = fd.Name
				}
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
				if err := validate.ActionMethodName(fd.Name.Name); err != nil {
					errs.ErrAt(pos,
						fmt.Errorf("%w: %s", ErrActionNameInvalid, fd.Name.Name))
				}
			}

			switch kind {
			case methodkind.EventHandler:
				if !isPage && !isAbs {
					break
				}
				if err := validate.EventHandlerMethodName(fd.Name.Name); err != nil {
					errs.ErrAt(pos,
						fmt.Errorf("%w: %s.%s",
							validate.ErrEventHandlerNameInvalid,
							recv, fd.Name.Name))
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
	//   - First parameter must be named exactly `event`
	//   - First parameter type must be an EventXXX type
	//   - Only one handler per EventXXX per receiver type
	params := fd.Type.Params
	var evName string

	if params == nil || len(params.List) == 0 {
		errs.ErrAt(pos,
			fmt.Errorf("%w: %s.%s",
				ErrSignatureEvHandFirstArgNotEvent, recv, fd.Name.Name))
	} else {
		first := params.List[0]
		if len(first.Names) != 1 || first.Names[0].Name != "event" {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s",
					ErrSignatureEvHandFirstArgNotEvent, recv, fd.Name.Name))
		} else {
			var ok bool
			evName, ok = typecheck.EventTypeNameOf(
				first.Type, ctx.pkg.TypesInfo, ctx.eventTypeNames,
			)
			if !ok {
				errs.ErrAt(pos,
					fmt.Errorf("%w: %s.%s",
						ErrSignatureEvHandFirstArgTypeNotEvent, recv, fd.Name.Name))
			} else {
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
		}
	}

	// Second param must be *datastar.ServerSentEventGenerator.
	if params != nil && len(params.List) > 0 {
		if len(params.List) < 2 {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s", ErrSignatureSecondArgNotSSE, recv, fd.Name.Name))
		} else if !typecheck.IsPtrToDatastarSSE(params.List[1].Type, ctx.pkg.TypesInfo) {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s", ErrSignatureSecondArgNotSSE, recv, fd.Name.Name))
		}
	}

	// Optional sessionToken parameter (third position).
	nextIdx := 2
	if params != nil && len(params.List) > nextIdx &&
		paramvalidation.IsSessionTokenParam(
			params.List[nextIdx],
		) {
		if !typecheck.IsString(
			ctx.pkg.TypesInfo.TypeOf(
				params.List[nextIdx].Type,
			),
		) {
			errs.ErrAt(pos, fmt.Errorf(
				"%w: %s.%s",
				ErrSessionTokenParamNotString,
				recv, fd.Name.Name,
			))
		}
		nextIdx++
	}

	// Optional session parameter.
	if params != nil && len(params.List) > nextIdx &&
		paramvalidation.IsSessionParam(
			params.List[nextIdx],
		) {
		if !typecheck.IsSessionType(
			params.List[nextIdx].Type,
			ctx.pkg.TypesInfo,
		) {
			errs.ErrAt(pos, fmt.Errorf(
				"%w: %s.%s",
				ErrSessionParamNotSessionType,
				recv, fd.Name.Name,
			))
		}
		nextIdx++
	}

	// No additional parameters beyond expected.
	if params != nil && len(params.List) > nextIdx {
		errs.ErrAt(pos,
			fmt.Errorf("%w: %s.%s",
				ErrSignatureUnknownInput, recv, fd.Name.Name))
	}

	// OnXXX must return exactly one result of type error.
	if !eventHandlerReturnsOnlyError(fd, ctx.pkg.TypesInfo) {
		errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
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
	h, outputs, herr := parseHandler(
		recv, fd, ctx.pkg.TypesInfo,
		ctx.eventTypeNames, kind, suffix,
	)
	if herr != nil {
		// Keep going; still attach a best-effort handler model.
		errs.ErrAt(ctx.pkg.Fset.Position(fd.Name.Pos()), herr)
	}
	ctx.handlerOutputs[h] = outputs

	if kind.IsAction() {
		r, found, valid := parseRoute(fd.Name.Name, fd.Doc)
		h.Route = r

		pos := ctx.pkg.Fset.Position(fd.Name.Pos())
		if !found {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s", ErrActionMissingPathComm, recv, fd.Name.Name))
		} else if !valid {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s", ErrActionInvalidPathComm, recv, fd.Name.Name))
		} else if pg != nil && pg.Route != "" && !actionIsUnderPage(pg.Route, r) {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s", ErrActionPathNotUnderPage, recv, fd.Name.Name))
		}
	} else if kind == methodkind.GETHandler && pg != nil {
		h.Route = pg.Route
	}

	// Validate path struct fields against route variables.
	if herr == nil && h.Route != "" {
		if pathErr := paramvalidation.ValidatePathAgainstRoute(h, recv, fd.Name.Name); pathErr != nil {
			errs.ErrAt(ctx.pkg.Fset.Position(fd.Name.Pos()), pathErr)
		}
	}

	// Validate reflectsignal tags on query fields reference actual signals.
	if herr == nil {
		if rsErr := structtag.ValidateReflectSignal(h, recv, fd.Name.Name); rsErr != nil {
			errs.ErrAt(ctx.pkg.Fset.Position(fd.Name.Pos()), rsErr)
		}
	}

	if pg != nil {
		if kind == methodkind.GETHandler {
			get, getErr := buildHandlerGET(h, outputs)
			pg.GET = get
			// Only report GET validation errors if handler parsing succeeded
			if getErr != nil && herr == nil {
				errs.ErrAt(ctx.pkg.Fset.Position(fd.Name.Pos()),
					fmt.Errorf("%w in %s.%s", getErr, recv, fd.Name.Name))
			}
		} else {
			pg.Actions = append(pg.Actions, h)
		}
		return
	}
	ap.Methods = append(ap.Methods, h)
}

func flattenPages(ctx *parseCtx, errs *Errors) {
	// deterministic iteration
	names := make([]string, 0, len(ctx.pages))
	for name := range ctx.pages {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
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
					get, getErr := buildHandlerGET(m, ctx.handlerOutputs[m])
					pg.GET = get
					if getErr != nil {
						pos := ctx.pkg.Fset.Position(m.Expr.Pos())
						errs.ErrAt(pos, fmt.Errorf("%w in %s.%s",
							getErr, ap.TypeName, m.Name))
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
	pageNames := make([]string, 0, len(ctx.pages))
	for name := range ctx.pages {
		pageNames = append(pageNames, name)
	}
	slices.Sort(pageNames)

	for _, name := range pageNames {
		if ctx.pages[name].GET == nil {
			ts := ctx.typeSpecByName[name]
			errs.ErrAt(ctx.pkg.Fset.Position(ts.Name.Pos()),
				fmt.Errorf("%w: %s", ErrPageMissingGET, name))
		}
	}
}

func finalizePages(ctx *parseCtx) {
	names := make([]string, 0, len(ctx.pages))
	for name := range ctx.pages {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
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

	// First param is the event
	if len(params) > 0 {
		h.InputEvent = parseInput(params[0], info)
	}

	// Second param is sse (required for event handlers)
	if len(params) > 1 &&
		typecheck.IsPtrToDatastarSSE(params[1].Type, info) {
		h.InputSSE = parseInput(params[1], info)
	}

	// Remaining optional params: sessionToken, session
	idx := 2
	if len(params) > idx &&
		paramvalidation.IsSessionTokenParam(params[idx]) {
		h.InputSessionToken = parseInput(
			params[idx], info,
		)
		idx++
	}
	if len(params) > idx &&
		paramvalidation.IsSessionParam(params[idx]) {
		h.InputSession = parseInput(params[idx], info)
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
		pkgs, err := packages.Load(cfg, ".")
		if err != nil {
			return nil, err
		}
		if len(pkgs) != 1 {
			return nil, fmt.Errorf("expected 1 package, got %d", len(pkgs))
		}
		return pkgs[0], nil
	}

	// If it looks like a filesystem path but doesn't exist, keep as pattern anyway.
	pattern := appPackagePath
	if filepath.IsAbs(appPackagePath) {
		// go list doesn’t like absolute patterns;
		// fallback to directory load if possible.
		dir := filepath.Dir(appPackagePath)
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			cfg.Dir = dir
			pkgs, err := packages.Load(cfg, ".")
			if err != nil {
				return nil, err
			}
			if len(pkgs) != 1 {
				return nil, fmt.Errorf("expected 1 package, got %d", len(pkgs))
			}
			return pkgs[0], nil
		}
	}

	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("expected 1 package, got %d", len(pkgs))
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

func buildHandlerGET(h *model.Handler, outputs []*model.Output) (*model.HandlerGET, error) {
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
		return get, ErrSignatureGETBodyWrongName
	}
	get.OutputBody = &model.TemplComponent{Output: firstComp}

	// Validate: If there's a second templ.Component, it must be named "head"
	if len(templComponents) > 1 {
		secondComp := templComponents[1]
		if secondComp.Name != "head" {
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
		return h, nil, fmt.Errorf("%w in %s.%s", ErrSignatureMissingReq, recv, fd.Name.Name)
	}
	// First param must be *http.Request
	if !typecheck.IsPtrToNetHTTPReq(params.List[0].Type, info) {
		return h, nil, fmt.Errorf("%w in %s.%s", ErrSignatureMissingReq, recv, fd.Name.Name)
	}
	h.InputRequest = parseInput(params.List[0], info)

	// Check if second param is sse (built-in feature for actions)
	remainingParams := params.List[1:]
	if len(remainingParams) > 0 && typecheck.IsPtrToDatastarSSE(remainingParams[0].Type, info) {
		h.InputSSE = parseInput(remainingParams[0], info)
		remainingParams = remainingParams[1:]
	}

	// Check if next param is sessionToken.
	if len(remainingParams) > 0 &&
		paramvalidation.IsSessionTokenParam(
			remainingParams[0],
		) {
		if !typecheck.IsString(
			info.TypeOf(remainingParams[0].Type),
		) {
			return h, nil, fmt.Errorf(
				"%w in %s.%s",
				ErrSessionTokenParamNotString,
				recv, fd.Name.Name,
			)
		}
		h.InputSessionToken = parseInput(
			remainingParams[0], info,
		)
		remainingParams = remainingParams[1:]
	}

	// Check if next param is session.
	if len(remainingParams) > 0 &&
		paramvalidation.IsSessionParam(remainingParams[0]) {
		if !typecheck.IsSessionType(
			remainingParams[0].Type, info,
		) {
			return h, nil, fmt.Errorf(
				"%w in %s.%s",
				ErrSessionParamNotSessionType,
				recv, fd.Name.Name,
			)
		}
		h.InputSession = parseInput(remainingParams[0], info)
		remainingParams = remainingParams[1:]
	}

	// Check if next param is path struct.
	if len(remainingParams) > 0 && paramvalidation.IsPathParam(remainingParams[0]) {
		pathErr := paramvalidation.ValidatePathStruct(remainingParams[0], info, recv, fd.Name.Name)
		if pathErr != nil {
			return h, nil, pathErr
		}
		h.InputPath = parseInput(remainingParams[0], info)
		remainingParams = remainingParams[1:]
	}

	// Check if next param is query struct.
	if len(remainingParams) > 0 && paramvalidation.IsQueryParam(remainingParams[0]) {
		queryErr := paramvalidation.ValidateQueryStruct(remainingParams[0], info, recv, fd.Name.Name)
		if queryErr != nil {
			return h, nil, queryErr
		}
		h.InputQuery = parseInput(remainingParams[0], info)
		remainingParams = remainingParams[1:]
	}

	// Check if next param is signals struct.
	if len(remainingParams) > 0 && paramvalidation.IsSignalsParam(remainingParams[0]) {
		sigErr := paramvalidation.ValidateSignalsStruct(remainingParams[0], info, recv, fd.Name.Name)
		if sigErr != nil {
			return h, nil, sigErr
		}
		h.InputSignals = parseInput(remainingParams[0], info)
		remainingParams = remainingParams[1:]
	}

	// Check if next param is dispatch function.
	if len(remainingParams) > 0 &&
		paramvalidation.IsDispatchParam(remainingParams[0]) {
		eventNames, dispErr := paramvalidation.ValidateDispatchFunc(
			remainingParams[0], info, eventTypeNames,
			recv, fd.Name.Name,
		)
		if dispErr != nil {
			return h, nil, dispErr
		}
		h.InputDispatch = &model.InputDispatch{
			Expr: remainingParams[0].Names[0],
			Name: "dispatch",
			Type: makeType(
				remainingParams[0].Type, info,
			),
			EventTypeNames: eventNames,
		}
		remainingParams = remainingParams[1:]
	}

	// No additional parameters allowed in handler signature.
	if len(remainingParams) > 0 {
		return h, nil, fmt.Errorf("%w in %s.%s",
			ErrSignatureUnknownInput, recv, fd.Name.Name)
	}

	// Results
	if fd.Type.Results == nil {
		return h, nil, nil
	}

	var outputs []*model.Output
	var multiErr bool
	for _, r := range fd.Type.Results.List {
		t := makeType(r.Type, info)

		if len(r.Names) == 0 {
			if typecheck.IsError(t.Resolved) {
				if h.OutputErr != nil {
					multiErr = true
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
					multiErr = true
					continue
				}
				h.OutputErr = out
				continue
			}
			if n.Name == "redirect" {
				if !typecheck.IsString(t.Resolved) {
					return h, nil, fmt.Errorf(
						"%w in %s.%s",
						ErrRedirectNotString,
						recv, fd.Name.Name,
					)
				}
				h.OutputRedirect = out
				continue
			}
			if n.Name == "redirectStatus" {
				if !typecheck.IsInt(t.Resolved) {
					return h, nil, fmt.Errorf(
						"%w in %s.%s",
						ErrRedirectStatusNotInt,
						recv, fd.Name.Name,
					)
				}
				h.OutputRedirectStatus = out
				continue
			}
			if n.Name == "newSession" {
				if !typecheck.IsSessionType(
					r.Type, info,
				) {
					return h, nil, fmt.Errorf(
						"%w in %s.%s",
						ErrNewSessionNotSessionType,
						recv, fd.Name.Name,
					)
				}
				h.OutputNewSession = out
				continue
			}
			if n.Name == "closeSession" {
				if !typecheck.IsBool(t.Resolved) {
					return h, nil, fmt.Errorf(
						"%w in %s.%s",
						ErrCloseSessionNotBool,
						recv, fd.Name.Name,
					)
				}
				h.OutputCloseSession = out
				continue
			}
			outputs = append(outputs, out)
		}
	}

	if multiErr {
		return h, outputs, fmt.Errorf(
			"%w in %s.%s",
			ErrSignatureMultiErrRet,
			recv, fd.Name.Name,
		)
	}
	if h.OutputRedirectStatus != nil &&
		h.OutputRedirect == nil {
		return h, outputs, fmt.Errorf(
			"%w in %s.%s",
			ErrRedirectStatusWithoutRedirect,
			recv, fd.Name.Name,
		)
	}
	if h.OutputNewSession != nil && h.InputSSE != nil {
		return h, outputs, fmt.Errorf(
			"%w in %s.%s",
			ErrNewSessionWithSSE,
			recv, fd.Name.Name,
		)
	}
	if h.OutputCloseSession != nil &&
		h.InputSSE != nil {
		return h, outputs, fmt.Errorf(
			"%w in %s.%s",
			ErrCloseSessionWithSSE,
			recv, fd.Name.Name,
		)
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
