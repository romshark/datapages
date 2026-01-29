package parser

import (
	"datapages/parser/model"
	"datapages/parser/validate"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Parse(appPackagePath string) (app *model.App, errs Errors) {
	defer sortErrors(&errs)

	pkg, err := loadPackage(appPackagePath)
	if err != nil {
		errs.Err(err)
		return nil, errs
	}
	for _, pe := range pkg.Errors {
		errs.ErrAt(posFromPackagesError(pe), pe)
	}

	if pkg.Types == nil || pkg.TypesInfo == nil {
		errs.ErrAt(earliestPkgPos(pkg),
			errors.New("missing source package type information"))
		return nil, errs
	}

	ctx := p.newParseCtx(pkg)
	p.indexTypes(&ctx)
	p.collectEventTypeNames(&ctx)
	p.initApp(&ctx, &errs)
	p.firstPassTypes(&ctx, &errs)
	p.validateEvents(&ctx, &errs)
	p.secondPassEmbeds(&ctx, &errs)
	p.thirdPassMethods(&ctx, &errs)
	p.flattenPages(&ctx)
	p.validateRequiredHandlers(&ctx, &errs)
	p.finalizePages(&ctx)
	p.assignSpecialPages(&ctx, &errs)

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

	app          *model.App
	appTypeFound bool
	basePos      token.Position
}

func (p *Parser) newParseCtx(pkg *packages.Package) parseCtx {
	return parseCtx{
		pkg:                 pkg,
		typeSpecByName:      map[string]*ast.TypeSpec{},
		docByType:           map[string]*ast.CommentGroup{},
		genDocByType:        map[string]*ast.CommentGroup{},
		eventTypeNames:      map[string]struct{}{},
		pages:               map[string]*model.Page{},
		abstracts:           map[string]*model.AbstractPage{},
		seenEvHandlerByRecv: map[string]map[string]token.Pos{},
		basePos:             earliestPkgPos(pkg),
	}
}

func (p *Parser) indexTypes(ctx *parseCtx) {
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

func (p *Parser) collectEventTypeNames(ctx *parseCtx) {
	for name := range ctx.typeSpecByName {
		if err := validate.EventTypeName(name); err == nil {
			ctx.eventTypeNames[name] = struct{}{}
		}
	}
}

func (p *Parser) initApp(ctx *parseCtx, errs *Errors) {
	ctx.app = &model.App{Fset: ctx.pkg.Fset}
	if appTS, ok := ctx.typeSpecByName["App"]; ok {
		ctx.app.Expr = appTS.Name
		ctx.appTypeFound = true
		return
	}
	errs.ErrAt(ctx.basePos, ErrAppMissingTypeApp)
}

func (p *Parser) firstPassTypes(ctx *parseCtx, errs *Errors) {
	typeNames := make([]string, 0, len(ctx.typeSpecByName))
	for name := range ctx.typeSpecByName {
		typeNames = append(typeNames, name)
	}
	slices.Sort(typeNames)

	for _, name := range typeNames {
		ts := ctx.typeSpecByName[name]

		// Only treat valid EventXXX as event types.
		if err := validate.EventTypeName(name); err == nil {
			p.firstPassEventType(ctx, errs, name, ts)
			continue
		}

		// Pages / abstracts are structs only.
		p.firstPassPageOrAbstractType(ctx, errs, name, ts)
	}
}

func (p *Parser) firstPassEventType(
	ctx *parseCtx, errs *Errors, name string, ts *ast.TypeSpec,
) {
	typePos := ctx.pkg.Fset.Position(ts.Name.Pos())
	doc := pickDoc(name, ctx.docByType, ctx.genDocByType)

	subj, err := p.extractEventSubject(name, doc)
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
		HasTargetUserIDs: hasTargetUserIDs(ts, ctx.pkg.TypesInfo),
	})
}

func (p *Parser) extractEventSubject(
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

func (p *Parser) firstPassPageOrAbstractType(
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
		if !hasRequiredAppField(st, ctx.pkg.TypesInfo) {
			errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageMissingFieldApp, name))
		}
		if hasDisallowedNamedFields(st) {
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
	if !hasRequiredAppField(st, ctx.pkg.TypesInfo) {
		return
	}
	ctx.abstracts[name] = &model.AbstractPage{
		Expr:     ts.Name,
		TypeName: name,
	}
}

func (p *Parser) secondPassEmbeds(ctx *parseCtx, errs *Errors) {
	for _, pg := range ctx.pages {
		p.resolveEmbedsForStruct(ctx, errs, pg.TypeName, func(ap *model.AbstractPage) {
			pg.Embeds = append(pg.Embeds, ap)
		})
	}
	for _, ap := range ctx.abstracts {
		p.resolveEmbedsForStruct(ctx, errs, ap.TypeName, func(sub *model.AbstractPage) {
			ap.Embeds = append(ap.Embeds, sub)
		})
	}
}

func (p *Parser) resolveEmbedsForStruct(
	ctx *parseCtx, errs *Errors, typeName string, add func(*model.AbstractPage),
) {
	ts := ctx.typeSpecByName[typeName]
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, emb := range embeddedTypeNames(st) {
		if ap, ok := ctx.abstracts[emb]; ok {
			add(ap)
			continue
		}
		typePos := ctx.pkg.Fset.Position(ts.Name.Pos())
		errs.ErrAt(typePos,
			fmt.Errorf("%w: %s embeds %s", ErrPageHasExtraFields, typeName, emb))
	}
}

func (p *Parser) thirdPassMethods(ctx *parseCtx, errs *Errors) {
	for _, f := range ctx.pkg.Syntax {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			recv := receiverTypeName(fd.Recv.List[0].Type)

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

			kind, suffix := classifyMethodName(fd.Name.Name)
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
			case methodKindEventHandler:
				if !isPage && !isAbs {
					break
				}
				if err := validate.EventHandlerMethodName(fd.Name.Name); err != nil {
					errs.ErrAt(pos,
						fmt.Errorf("%w: %s.%s",
							validate.ErrEventHandlerNameInvalid,
							recv, fd.Name.Name))
				}
				p.validateAndAttachEventHandler(ctx, errs, recv, fd, pg, ap, suffix)
			default:
				p.attachHTTPHandler(ctx, errs, recv, fd, pg, ap, kind, suffix)
			}
		}
	}
}

func (p *Parser) validateAndAttachEventHandler(
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
			fmt.Errorf("%w: %s.%s", ErrEvHandFirstArgNotEvent, recv, fd.Name.Name))
	} else {
		first := params.List[0]
		if len(first.Names) != 1 || first.Names[0].Name != "event" {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s", ErrEvHandFirstArgNotEvent, recv, fd.Name.Name))
		} else {
			var ok bool
			evName, ok = eventTypeNameOf(
				first.Type, ctx.pkg.TypesInfo, ctx.eventTypeNames,
			)
			if !ok {
				errs.ErrAt(pos,
					fmt.Errorf("%w: %s.%s",
						ErrEvHandFirstArgTypeNotEvent, recv, fd.Name.Name))
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

	// OnXXX must return exactly one result of type error.
	if !eventHandlerReturnsOnlyError(fd, ctx.pkg.TypesInfo) {
		errs.ErrAt(pos, fmt.Errorf("%w: %s.%s", ErrEvHandReturnMustBeError, recv, fd.Name.Name))
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
	return isErrorType(t)
}

func (p *Parser) attachHTTPHandler(
	ctx *parseCtx,
	errs *Errors,
	recv string,
	fd *ast.FuncDecl,
	pg *model.Page,
	ap *model.AbstractPage,
	kind methodKind,
	suffix string,
) {
	h, herr := parseHandler(recv, fd, ctx.pkg.TypesInfo, kind, suffix)
	if herr != nil {
		// Keep going; still attach a best-effort handler model.
		errs.ErrAt(ctx.pkg.Fset.Position(fd.Name.Pos()), herr)
	}

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
		} else if pg != nil && pg.Route != "" && !isUnderPage(pg.Route, r) {
			errs.ErrAt(pos,
				fmt.Errorf("%w: %s.%s", ErrActionPathNotUnderPage, recv, fd.Name.Name))
		}
	} else if kind == methodKindGETHandler && pg != nil {
		h.Route = pg.Route
	}

	if pg != nil {
		if kind == methodKindGETHandler {
			pg.GET = h
		} else {
			pg.Actions = append(pg.Actions, h)
		}
		return
	}
	ap.Methods = append(ap.Methods, h)
}

func (p *Parser) flattenPages(ctx *parseCtx) {
	for _, pg := range ctx.pages {
		p.flattenPage(pg)
	}
}

func (p *Parser) flattenPage(pg *model.Page) {
	if len(pg.Embeds) == 0 {
		return
	}

	visited := map[string]bool{}
	ownedMethods := map[string]bool{}
	handledEvents := map[string]bool{}

	// Register own methods
	if pg.GET != nil {
		ownedMethods["GET"] = true
	}
	for _, a := range pg.Actions {
		ownedMethods[a.Name] = true
	}
	for _, h := range pg.EventHandlers {
		ownedMethods[h.Name] = true
		handledEvents[h.EventTypeName] = true
	}

	queue := make([]*model.AbstractPage, len(pg.Embeds))
	copy(queue, pg.Embeds)

	for len(queue) > 0 {
		ap := queue[0]
		queue = queue[1:]

		if visited[ap.TypeName] {
			continue
		}
		visited[ap.TypeName] = true

		// Add children to queue
		queue = append(queue, ap.Embeds...)

		// Methods
		for _, m := range ap.Methods {
			if ownedMethods[m.Name] {
				continue
			}
			ownedMethods[m.Name] = true
			if m.HTTPMethod == "GET" {
				pg.GET = m
			} else {
				pg.Actions = append(pg.Actions, m)
			}
		}

		// EventHandlers
		for _, h := range ap.EventHandlers {
			if ownedMethods[h.Name] {
				continue
			}
			if handledEvents[h.EventTypeName] {
				continue
			}
			ownedMethods[h.Name] = true
			handledEvents[h.EventTypeName] = true
			pg.EventHandlers = append(pg.EventHandlers, h)
		}
	}
}

func (p *Parser) validateRequiredHandlers(ctx *parseCtx, errs *Errors) {
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

func (p *Parser) finalizePages(ctx *parseCtx) {
	names := make([]string, 0, len(ctx.pages))
	for name := range ctx.pages {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		ctx.app.Pages = append(ctx.app.Pages, ctx.pages[name])
	}
}

func (p *Parser) assignSpecialPages(ctx *parseCtx, errs *Errors) {
	ctx.app.PageIndex = ctx.pages["PageIndex"]
	ctx.app.PageError404 = ctx.pages["PageError404"]
	ctx.app.PageError500 = ctx.pages["PageError500"]

	if ctx.app.PageIndex == nil {
		errs.ErrAt(ctx.basePos, ErrAppMissingPageIndex)
	}
}

// hasDisallowedNamedFields reports whether a page struct contains any named field
// besides the single allowed `App *App`.
//
// Embedded fields are handled separately (and must be abstract page types).
func hasDisallowedNamedFields(st *ast.StructType) bool {
	if st == nil || st.Fields == nil {
		return false
	}
	appCount := 0
	for _, f := range st.Fields.List {
		// Embedded field: allowed only if it’s an abstract page type (validated later)
		if len(f.Names) == 0 {
			continue
		}
		// Any multi-name field is disallowed.
		if len(f.Names) != 1 {
			return true
		}
		if f.Names[0].Name != "App" {
			return true
		}
		appCount++
	}
	// More than one App field is disallowed (the missing case is handled elsewhere).
	return appCount > 1
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

	if len(params) > 0 {
		h.InputSSE = parseInput(params[0], info)
	}
	for _, p := range params[1:] {
		h.Inputs = append(h.Inputs, parseInput(p, info))
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

func hasTargetUserIDs(ts *ast.TypeSpec, info *types.Info) bool {
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return false
	}
	for _, f := range st.Fields.List {
		// name
		if len(f.Names) != 1 || f.Names[0].Name != "TargetUserIDs" {
			continue
		}
		// type must be []string
		t := info.TypeOf(f.Type)
		if t == nil {
			continue
		}
		slice, ok := t.(*types.Slice)
		if !ok {
			continue
		}
		basic, ok := slice.Elem().(*types.Basic)
		if !ok || basic.Kind() != types.String {
			continue
		}
		// tag must be json:"-"
		if f.Tag == nil {
			return false
		}
		tag, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			return false
		}
		// Very small parser: look for `json:"-"`
		return strings.Contains(tag, `json:"-"`)
	}
	return false
}

func hasRequiredAppField(st *ast.StructType, info *types.Info) bool {
	// Must contain exported field App *App.
	for _, f := range st.Fields.List {
		if len(f.Names) != 1 || f.Names[0].Name != "App" {
			continue
		}
		t := info.TypeOf(f.Type)
		if t == nil {
			continue
		}
		// want: *App (named type App in same package)
		ptr, ok := t.(*types.Pointer)
		if !ok {
			continue
		}
		named, ok := ptr.Elem().(*types.Named)
		if !ok {
			continue
		}
		if named.Obj() != nil && named.Obj().Name() == "App" {
			return true
		}
	}
	return false
}

func embeddedTypeNames(st *ast.StructType) []string {
	var out []string
	for _, f := range st.Fields.List {
		// Embedded field: Names == nil
		if len(f.Names) != 0 {
			continue
		}
		switch t := f.Type.(type) {
		case *ast.Ident:
			out = append(out, t.Name)
		case *ast.StarExpr:
			if id, ok := t.X.(*ast.Ident); ok {
				out = append(out, id.Name)
			}
		}
	}
	return out
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

type methodKind int8

const (
	_ methodKind = iota
	methodKindGETHandler
	methodKindActionPOSTHandler
	methodKindActionPUTHandler
	methodKindActionDELETEHandler
	methodKindEventHandler
)

func (m methodKind) IsAction() bool {
	switch m {
	case methodKindActionPOSTHandler,
		methodKindActionPUTHandler,
		methodKindActionDELETEHandler:
		return true
	}
	return false
}

func (k methodKind) HTTPMethod() string {
	switch k {
	case methodKindGETHandler:
		return "GET"
	case methodKindActionPOSTHandler:
		return "POST"
	case methodKindActionPUTHandler:
		return "PUT"
	case methodKindActionDELETEHandler:
		return "DELETE"
	}
	return ""
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

func isErrorType(t types.Type) bool {
	if t == nil {
		return false
	}
	// builtin "error" is a named interface in Universe.
	return t.String() == "error"
}

func classifyMethodName(name string) (kind methodKind, suffix string) {
	if name == "" {
		return 0, ""
	}
	// Only treat exported identifiers as framework-reserved handlers.
	// This makes pOST / postX / onFoo etc. normal methods.
	if name[0] < 'A' || name[0] > 'Z' {
		return 0, ""
	}

	switch {
	case name == "GET":
		return methodKindGETHandler, ""

	case strings.HasPrefix(name, "POST"):
		// Always classify; validation will decide if it's valid (incl. missing suffix).
		return methodKindActionPOSTHandler, name[len("POST"):]

	case strings.HasPrefix(name, "PUT"):
		return methodKindActionPUTHandler, name[len("PUT"):]

	case strings.HasPrefix(name, "DELETE"):
		return methodKindActionDELETEHandler, name[len("DELETE"):]

	case strings.HasPrefix(name, "On"):
		return methodKindEventHandler, name[len("On"):]

	default:
		return 0, ""
	}
}

func parseHandler(
	recv string,
	fd *ast.FuncDecl,
	info *types.Info,
	kind methodKind,
	name string,
) (*model.Handler, error) {
	h := &model.Handler{
		Expr:       fd.Name,
		Name:       name,
		HTTPMethod: kind.HTTPMethod(),
	}

	params := fd.Type.Params
	if params == nil || len(params.List) == 0 {
		return h, fmt.Errorf("%w in %s.%s", ErrSignatureMissingReq, recv, fd.Name.Name)
	}
	// First param must be *http.Request
	if !isPtrToNetHTTPReq(params.List[0].Type, info) {
		return h, fmt.Errorf("%w in %s.%s", ErrSignatureMissingReq, recv, fd.Name.Name)
	}
	h.InputRequest = parseInput(params.List[0], info)

	// Remaining params are plugins
	for _, p := range params.List[1:] {
		if len(p.Names) == 0 {
			h.Inputs = append(h.Inputs, &model.Input{
				Type: makeType(p.Type, info),
			})
			continue
		}
		for _, n := range p.Names {
			h.Inputs = append(h.Inputs, &model.Input{
				Expr: n,
				Name: n.Name,
				Type: makeType(p.Type, info),
			})
		}
	}

	// Results
	if fd.Type.Results == nil {
		return h, nil
	}

	var multiErr bool
	for _, r := range fd.Type.Results.List {
		t := makeType(r.Type, info)

		if len(r.Names) == 0 {
			if isErrorType(t.Resolved) {
				if h.OutputErr != nil {
					multiErr = true
					continue
				}
				h.OutputErr = &model.Output{Type: t}
				continue
			}
			h.Outputs = append(h.Outputs, &model.Output{Type: t})
			continue
		}

		for _, n := range r.Names {
			out := &model.Output{
				Expr: n,
				Name: n.Name,
				Type: t,
			}
			if isErrorType(t.Resolved) {
				if h.OutputErr != nil {
					multiErr = true
					continue
				}
				h.OutputErr = out
				continue
			}
			h.Outputs = append(h.Outputs, out)
		}
	}

	if multiErr {
		return h, fmt.Errorf("%w in %s.%s", ErrSignatureMultiErrRet, recv, fd.Name.Name)
	}
	return h, nil
}

func isPtrToNetHTTPReq(expr ast.Expr, info *types.Info) bool {
	t := info.TypeOf(expr)
	if t == nil {
		return false
	}
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	// Require exactly net/http.Request
	return obj.Pkg().Path() == "net/http" && obj.Name() == "Request"
}

// eventTypeNameOf returns the EventXXX type name for expr if it is (or points to) a
// named type declared in this package whose name is in eventTypeNames.
func eventTypeNameOf(
	expr ast.Expr, info *types.Info, eventTypeNames map[string]struct{},
) (string, bool) {
	t := info.TypeOf(expr)
	if t == nil {
		return "", false
	}
	// Allow both EventFoo and *EventFoo as the parameter type.
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return "", false
	}
	name := named.Obj().Name()
	if _, ok := eventTypeNames[name]; !ok {
		return "", false
	}
	// Ensure the event type is from the same package.
	if named.Obj().Pkg().Path() == "" {
		return "", false
	}
	return name, true
}
