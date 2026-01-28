package parser

import (
	"datapages/parser/model"
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

func Parse(appPackagePath string) (app *model.App, errs Errors) {
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

	typeSpecByName := map[string]*ast.TypeSpec{}
	docByType := map[string]*ast.CommentGroup{}
	genDocByType := map[string]*ast.CommentGroup{}

	for _, f := range pkg.Syntax {
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
				typeSpecByName[ts.Name.Name] = ts
				if ts.Doc != nil {
					docByType[ts.Name.Name] = ts.Doc
				} else if gd.Doc != nil {
					genDocByType[ts.Name.Name] = gd.Doc
				}
			}
		}
	}

	// Build an App model even if App type is missing, so we can still parse pages/events
	// and collect all errors. We'll return nil if App is missing.
	app = &model.App{
		Fset: pkg.Fset,
	}
	var appTypeFound bool

	basePos := earliestPkgPos(pkg)
	if appTS, ok := typeSpecByName["App"]; ok {
		app.Expr = appTS.Name
		appTypeFound = true
	} else {
		errs.ErrAt(basePos, ErrAppMissingTypeApp)
	}

	pages := map[string]*model.Page{}
	abstracts := map[string]*model.AbstractPage{}

	// --- First pass: collect types
	typeNames := make([]string, 0, len(typeSpecByName))
	for name := range typeSpecByName {
		typeNames = append(typeNames, name)
	}
	slices.Sort(typeNames)
	for _, name := range typeNames {
		ts := typeSpecByName[name]
		if strings.HasPrefix(name, "Event") {
			if subj, ok := parseEventSubject(name, pickDoc(name, docByType, genDocByType)); ok {
				app.Events = append(app.Events, &model.Event{
					Expr:             ts.Name,
					TypeName:         name,
					Subject:          subj,
					HasTargetUserIDs: hasTargetUserIDs(ts, pkg.TypesInfo),
				})
			}
			continue
		}

		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}

		if strings.HasPrefix(name, "Page") {
			typePos := pkg.Fset.Position(ts.Name.Pos())
			if !isValidPageTypeName(name) {
				errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageNameInvalid, name))
				// keep going; still register it (or choose to skip; see note below)
			}
			if !hasRequiredAppField(st, pkg.TypesInfo) {
				errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageMissingFieldApp, name))
				// Keep registering the page so PageIndex is still "found".
			}
			if hasDisallowedNamedFields(st) {
				errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageHasExtraFields, name))
			}
			route, found, ok := parseRoute(name, pickDoc(name, docByType, genDocByType))
			if !found {
				errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageMissingPathComm, name))
			} else if !ok {
				errs.ErrAt(typePos, fmt.Errorf("%w: %s", ErrPageInvalidPathComm, name))
			}
			pages[name] = &model.Page{
				Expr:               ts.Name,
				TypeName:           name,
				Route:              route,
				PageSpecialization: pageSpecialization(name),
			}
		} else {
			// Abstract pages still require App *App.
			if !hasRequiredAppField(st, pkg.TypesInfo) {
				continue
			}
			abstracts[name] = &model.AbstractPage{
				Expr:     ts.Name,
				TypeName: name,
			}
		}
	}

	// --- Second pass: embeds
	for _, p := range pages {
		ts := typeSpecByName[p.TypeName]
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, emb := range embeddedTypeNames(st) {
			if ap, ok := abstracts[emb]; ok {
				p.Embeds = append(p.Embeds, ap)
				continue
			}
			// Embedded types are only allowed for abstract page types.
			typePos := pkg.Fset.Position(ts.Name.Pos())
			errs.ErrAt(typePos,
				fmt.Errorf("%w: %s embeds %s", ErrPageHasExtraFields, p.TypeName, emb))
		}
	}

	// --- Third pass: methods
	for _, f := range pkg.Syntax {
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
					app.GlobalHeadGenerator = fd.Name
				case "Recover500":
					app.Recover500 = fd.Name
				}
				// keep scanning; App methods are not also page methods
			}

			p, isPage := pages[recv]
			ap, isAbs := abstracts[recv]
			if !isPage && !isAbs {
				continue
			}

			kind, suffix := classifyMethodName(fd.Name.Name)
			if kind == 0 {
				continue
			}

			pos := pkg.Fset.Position(fd.Name.Pos())

			// Validate action method names early.
			if kind.IsAction() && !isValidActionName(fd.Name.Name) {
				errs.ErrAt(pos, fmt.Errorf("%w: %s", ErrActionNameInvalid, fd.Name.Name))
			}

			switch kind {
			case methodKindEventHandler:
				if isPage {
					p.EventHandlers = append(p.EventHandlers,
						parseEventHandler(fd, pkg.TypesInfo, suffix))
				}
			default:
				h, herr := parseHandler(recv, fd, pkg.TypesInfo, kind, suffix)
				if herr != nil {
					// Keep going; still attach a best-effort handler model.
					errs.ErrAt(pkg.Fset.Position(fd.Name.Pos()), herr)
				}
				if kind.IsAction() {
					r, found, valid := parseRoute(fd.Name.Name, fd.Doc)
					h.Route = r

					pos := pkg.Fset.Position(fd.Name.Pos())

					if !found {
						errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
							ErrActionMissingPathComm, recv, fd.Name.Name))
					} else if !valid {
						errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
							ErrActionInvalidPathComm, recv, fd.Name.Name))
					} else if p != nil && p.Route != "" && !isUnderPage(p.Route, r) {
						// Only check if the page route is known;
						// if the page route is missing/invalid,
						// the page-level errors already cover that and
						// this avoids noisy cascades.
						errs.ErrAt(pos, fmt.Errorf("%w: %s.%s",
							ErrActionPathNotUnderPage, recv, fd.Name.Name))
					}
				} else if kind == methodKindGETHandler {
					h.Route = p.Route
				}

				if isPage {
					if kind == methodKindGETHandler {
						p.GET = h
					} else {
						p.Actions = append(p.Actions, h)
					}
				} else {
					ap.Methods = append(ap.Methods, h)
				}
			}
		}
	}

	// --- Validate required handlers
	// Every page type must have a GET handler.
	pageNames := make([]string, 0, len(pages))
	for name := range pages {
		pageNames = append(pageNames, name)
	}
	slices.Sort(pageNames)
	for _, name := range pageNames {
		if pages[name].GET == nil {
			ts := typeSpecByName[name]
			errs.ErrAt(pkg.Fset.Position(ts.Name.Pos()),
				fmt.Errorf("%w: %s", ErrPageMissingGET, name))
		}
	}

	// --- Finalize pages deterministically
	names := make([]string, 0, len(pages))
	for name := range pages {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		app.Pages = append(app.Pages, pages[name])
	}

	// --- Assign special pages
	app.PageIndex = pages["PageIndex"]
	app.PageError404 = pages["PageError404"]
	app.PageError500 = pages["PageError500"]

	if app.PageIndex == nil {
		errs.ErrAt(basePos, ErrAppMissingPageIndex)
	}

	if !appTypeFound {
		// Caller wants nil app when App is missing.
		return nil, errs
	}
	return app, errs
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
	fd *ast.FuncDecl, info *types.Info, name string,
) *model.EventHandler {
	params := fd.Type.Params.List

	h := &model.EventHandler{
		Expr: fd.Name,
		Name: name,
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
		// go list doesn’t like absolute patterns; fallback to directory load if possible.
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

func pickDoc(typeName string, docByType, genDocByType map[string]*ast.CommentGroup) *ast.CommentGroup {
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
func parseRoute(symbol string, cg *ast.CommentGroup) (route string, found bool, valid bool) {
	if cg == nil {
		return "", false, false
	}

	want := symbol + " is "
	attemptPrefix := symbol + " "

	for _, c := range cg.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))

		if !strings.HasPrefix(txt, attemptPrefix) {
			continue
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
		return route, true, true
	}

	return "", false, false
}

// parseEventSubject parses:
//
//	// EventX is "x.y"
func parseEventSubject(typeName string, cg *ast.CommentGroup) (string, bool) {
	if cg == nil {
		return "", false
	}
	prefix := typeName + " is "
	for _, c := range cg.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if !strings.HasPrefix(txt, prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(txt, prefix))
		if len(rest) < 2 {
			return "", false
		}
		quote := rest[0]
		if quote != '"' && quote != '`' {
			return "", false
		}
		end := strings.LastIndexByte(rest[1:], quote)
		if end < 0 {
			return "", false
		}
		return rest[1 : 1+end], true
	}
	return "", false
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
	switch {
	case name == "GET":
		return methodKindGETHandler, ""
	case strings.HasPrefix(name, "POST") && len(name) > 4:
		return methodKindActionPOSTHandler, name[len("POST"):]
	case strings.HasPrefix(name, "PUT") && len(name) > 3:
		return methodKindActionPUTHandler, name[len("PUT"):]
	case strings.HasPrefix(name, "DELETE") && len(name) > 6:
		return methodKindActionDELETEHandler, name[len("DELETE"):]
	case strings.HasPrefix(name, "On") && len(name) > 2:
		return methodKindEventHandler, name[len("On"):]
	}
	return 0, ""
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

func isValidPageTypeName(name string) bool {
	s, ok := strings.CutPrefix(name, "Page")
	if !ok || s == "" {
		return false
	}
	r0 := s[0]
	if r0 < 'A' || r0 > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

func isValidActionName(name string) bool {
	isValidActionSuffix := func(name string, prefixLen int) bool {
		if len(name) <= prefixLen {
			return false
		}
		s := name[prefixLen:]
		// First char must be A-Z.
		c0 := s[0]
		if c0 < 'A' || c0 > 'Z' {
			return false
		}
		// Remaining chars must be alnum.
		for i := 1; i < len(s); i++ {
			c := s[i]
			if (c >= 'A' && c <= 'Z') ||
				(c >= 'a' && c <= 'z') ||
				(c >= '0' && c <= '9') {
				continue
			}
			return false
		}
		return true
	}

	// Valid action names are:
	//   POSTX...
	//   PUTX...
	//   DELETEX...
	// where X is [A-Z], followed by [A-Za-z0-9]*.
	switch {
	case strings.HasPrefix(name, "POST"):
		return isValidActionSuffix(name, 4)
	case strings.HasPrefix(name, "PUT"):
		return isValidActionSuffix(name, 3)
	case strings.HasPrefix(name, "DELETE"):
		return isValidActionSuffix(name, 6)
	default:
		return false
	}
}
