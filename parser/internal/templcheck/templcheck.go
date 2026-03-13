// Package templcheck validates .templ files for common mistakes:
//   - hardcoded app-internal href and action attributes
//   - action helpers used outside Datastar action contexts
//   - cross-page action references (action from page A used in page B's template)
package templcheck

import (
	"go/ast"
	"go/constant"
	goparser "go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
	"strings"

	templparser "github.com/a-h/templ/parser/v2"
	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/hrefcheck"
	"github.com/romshark/datapages/parser/model"
)

// ErrFunc is called for each error found during checking.
type ErrFunc func(pos token.Position, err error)

// checker holds resolved state shared across all checks.
type checker struct {
	errFn           ErrFunc
	constValues     map[string]string
	hrefLocalName   string
	actionLocalName string
}

// Check validates .templ files in pkg and reports errors via errFn.
func Check(
	pkg *packages.Package,
	app *model.App,
	errFn ErrFunc,
) {
	c := checker{
		errFn:           errFn,
		constValues:     resolveConstValues(pkg),
		hrefLocalName:   findPkgLocalName(pkg, "/href", "href"),
		actionLocalName: findPkgLocalName(pkg, "/action", "action"),
	}
	templPaths := templFilesFromPackage(pkg)
	for _, templPath := range templPaths {
		c.checkTemplFile(templPath)
	}
	if app != nil {
		c.checkActionOwnership(pkg, app, templPaths)
	}
}

// resolveConstValues builds a map from package-level constant names to their
// string values. This allows the linter to detect hardcoded URLs hidden behind
// named constants. Only constants are trusted (variables can be reassigned).
func resolveConstValues(pkg *packages.Package) map[string]string {
	m := map[string]string{}
	for ident, obj := range pkg.TypesInfo.Defs {
		c, ok := obj.(*types.Const)
		if !ok || c.Val().Kind() != constant.String {
			continue
		}
		m[ident.Name] = constant.StringVal(c.Val())
	}
	return m
}

// templFilesFromPackage returns absolute paths of .templ files belonging
// to pkg by finding _templ.go compiled files and deriving the source name.
func templFilesFromPackage(pkg *packages.Package) []string {
	var paths []string
	for _, goFile := range pkg.GoFiles {
		if !strings.HasSuffix(goFile, "_templ.go") {
			continue
		}
		// foo_templ.go → foo.templ
		base := strings.TrimSuffix(filepath.Base(goFile), "_templ.go") + ".templ"
		templPath := filepath.Join(filepath.Dir(goFile), base)
		paths = append(paths, templPath)
	}
	return paths
}

// findPkgLocalName scans the _templ.go files in pkg for an import whose path
// ends with suffix and belongs to the same Go module. It returns the local name
// used for that import, or "" if no such import is found. defaultName is the
// fallback when no explicit alias is present (e.g. "href", "action").
func findPkgLocalName(pkg *packages.Package, suffix, defaultName string) string {
	for _, f := range pkg.Syntax {
		filename := pkg.Fset.Position(f.Pos()).Filename
		if !strings.HasSuffix(filename, "_templ.go") {
			continue
		}
		for _, imp := range f.Imports {
			importPath, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasSuffix(importPath, suffix) {
				continue
			}
			// Reject external packages by verifying the import
			// belongs to the same module as the package being checked.
			if pkg.Module != nil && !strings.HasPrefix(importPath, pkg.Module.Path+"/") {
				continue
			}
			if imp.Name != nil {
				return imp.Name.Name
			}
			return defaultName
		}
	}
	return ""
}

func (c *checker) checkTemplFile(path string) {
	tf, err := templparser.Parse(path)
	if err != nil {
		return
	}

	filename := filepath.Base(path)

	for _, node := range tf.Nodes {
		tmpl, ok := node.(*templparser.HTMLTemplate)
		if !ok {
			continue
		}
		c.walkChildren(filename, tmpl.Children)
	}
}

// walkChildren recursively walks templ AST children looking for Element nodes
// with hardcoded href/action attributes.
func (c *checker) walkChildren(filename string, nodes []templparser.Node) {
	prevIsNolint := false
	for _, node := range nodes {
		switch n := node.(type) {
		case *templparser.GoComment:
			prevIsNolint = strings.Contains(n.Contents, "datapages:nolint")
			continue
		case *templparser.Whitespace:
			// Whitespace between a nolint comment and the next element
			// should not reset the nolint flag.
			continue
		case *templparser.Element:
			if !prevIsNolint {
				c.checkElementAttrs(filename, n)
			}
			c.walkChildren(filename, n.Children)
		case templparser.CompositeNode:
			c.walkChildren(filename, n.ChildNodes())
		}
		prevIsNolint = false
	}
}

func (c *checker) checkElementAttrs(filename string, el *templparser.Element) {
	for _, attr := range el.Attributes {
		switch a := attr.(type) {
		case *templparser.ConstantAttribute:
			key, ok := a.Key.(templparser.ConstantAttributeKey)
			if !ok {
				continue
			}
			switch key.Name {
			case "href":
				if el.Name != "a" || hrefcheck.IsAllowedNonRelativeHref(a.Value) {
					continue
				}
				pos := token.Position{
					Filename: filename,
					Line:     int(a.Range.From.Line) + 1,
					Column:   int(a.Range.From.Col) + 1,
				}
				c.errFn(pos, &ErrorHardcodedHref{URL: a.Value})
			case "action":
				if el.Name != "form" || hrefcheck.IsAllowedNonRelativeHref(a.Value) {
					continue
				}
				pos := token.Position{
					Filename: filename,
					Line:     int(a.Range.From.Line) + 1,
					Column:   int(a.Range.From.Col) + 1,
				}
				c.errFn(pos, &ErrorHardcodedAction{URL: a.Value})
			}
		case *templparser.ExpressionAttribute:
			key, ok := a.Key.(templparser.ConstantAttributeKey)
			if !ok {
				continue
			}
			exprPos := token.Position{
				Filename: filename,
				Line:     int(a.Expression.Range.From.Line) + 1,
				Column:   int(a.Expression.Range.From.Col) + 1,
			}
			switch key.Name {
			case "href":
				if el.Name != "a" {
					continue
				}
				checkHrefExpr(
					c.errFn, exprPos, a.Expression.Value,
					c.constValues, c.hrefLocalName,
				)
			}
			if isDatastarActionAttr(key.Name) {
				if c.hrefLocalName != "" {
					findPkgCalls(
						a.Expression.Value, c.hrefLocalName,
						func(funcName string) {
							c.errFn(exprPos, &ErrorHrefContext{
								AttrName: key.Name,
								HrefFunc: funcName,
							})
						},
					)
				}
				continue
			}
			if c.actionLocalName != "" {
				findPkgCalls(
					a.Expression.Value, c.actionLocalName,
					func(funcName string) {
						c.errFn(exprPos, &ErrorActionContext{
							AttrName:   key.Name,
							ActionFunc: funcName,
						})
					},
				)
			}
		}
	}
}

// isDatastarActionAttr reports whether the attribute name is a valid
// Datastar action context.
//
// Matched attributes:
//   - data-on:<event> — standard DOM events (open-ended: click, submit, load, …)
//   - data-on-intersect, data-on-interval, data-on-signal-patch — Datastar plugins
//   - data-init
//
// Plugin and data-init attributes may carry Datastar modifiers
// (e.g. data-on-intersect.once, data-on-interval__duration.500ms).
func isDatastarActionAttr(name string) bool {
	// data-on:<event> — DOM events are open-ended, prefix match is required.
	if strings.HasPrefix(name, "data-on:") {
		return true
	}
	// data-on-<plugin> — known Datastar plugin events.
	for _, plugin := range datastarOnPlugins {
		if hasAttrPrefix(name, plugin) {
			return true
		}
	}
	// data-init with optional modifiers.
	return hasAttrPrefix(name, "data-init")
}

// datastarOnPlugins lists the known Datastar data-on-<plugin> attribute prefixes.
var datastarOnPlugins = []string{
	"data-on-intersect",
	"data-on-interval",
	"data-on-signal-patch",
}

// hasAttrPrefix reports whether name equals prefix or starts with prefix
// followed by a Datastar modifier separator ('.' or '__').
func hasAttrPrefix(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	rest := name[len(prefix):]
	return rest[0] == '.' || strings.HasPrefix(rest, "__")
}

// checkHrefExpr validates an expression href attribute on an <a> tag.
// It parses the expression as Go source and walks the AST to determine:
//  1. If expr calls href.Xxx() → OK (but check href.External for disallowed URLs).
//  2. If expr contains any other function call → ErrHrefUnverifiable.
//  3. If expr contains unresolved identifiers (variables) → ErrHrefUnverifiable.
//  4. If expr contains disallowed string literals or constants → ErrHardcodedHref.
//  5. If expr contains only allowed literals/constants → OK.
//  6. Otherwise → ErrHrefUnverifiable.
func checkHrefExpr(
	errFn ErrFunc,
	pos token.Position,
	expr string,
	constValues map[string]string,
	hrefLocalName string,
) {
	exprAST, err := goparser.ParseExpr(expr)
	if err != nil {
		errFn(pos, &ErrorHrefUnverifiable{Expr: expr})
		return
	}

	info := analyzeHrefExpr(exprAST, constValues, hrefLocalName)

	// 1. Uses href package → check External for disallowed URLs, otherwise OK.
	if info.usesHrefPkg {
		if info.externalURL != "" &&
			!hrefcheck.IsAllowedNonRelativeHref(info.externalURL) {
			errFn(pos, &ErrorExternalWithInternal{URL: info.externalURL})
		}
		return
	}

	// 2. Any non-href function call makes the expression unverifiable.
	if info.hasCall {
		errFn(pos, &ErrorHrefUnverifiable{Expr: expr})
		return
	}

	// 3. Unresolved identifiers (variables, parameters) make the expression unverifiable.
	if info.hasUnresolved {
		errFn(pos, &ErrorHrefUnverifiable{Expr: expr})
		return
	}

	// 4. Disallowed string literal or constant value.
	if info.disallowedURL != "" {
		errFn(pos, &ErrorHardcodedHref{URL: info.disallowedURL})
		return
	}

	// 5. All resolved values are allowed external URLs → OK.
	if info.hasAllowed {
		return
	}

	// 6. Expression doesn't use href package at all.
	errFn(pos, &ErrorHrefUnverifiable{Expr: expr})
}

// hrefExprInfo holds the results of analyzing a Go expression used as an
// href attribute value.
type hrefExprInfo struct {
	usesHrefPkg   bool   // expression calls href.Xxx(...)
	externalURL   string // first arg to href.External if statically known
	hasCall       bool   // expression contains a non-href function call
	hasUnresolved bool   // expression contains an identifier not in constValues
	disallowedURL string // first disallowed URL from a literal or constant
	hasAllowed    bool   // at least one allowed literal or constant
}

// analyzeHrefExpr walks a parsed Go expression and populates hrefExprInfo.
// hrefLocalName is the local import name for the generated href package
// (e.g. "href"); if empty, no href package is imported and all pkg.Xxx()
// calls are treated as non-href calls.
func analyzeHrefExpr(
	node ast.Expr, constValues map[string]string, hrefLocalName string,
) hrefExprInfo {
	var info hrefExprInfo
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if hrefLocalName != "" && isPkgCall(x, hrefLocalName) {
				info.usesHrefPkg = true
				if isExternalCall(x) {
					info.externalURL = resolveCallArg(x, constValues)
				}
				return false // don't recurse into href.Xxx() args
			}
			info.hasCall = true
			return false
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				val, _ := strconv.Unquote(x.Value)
				classifyURL(&info, val)
			}
		case *ast.Ident:
			if val, ok := constValues[x.Name]; ok {
				classifyURL(&info, val)
			} else {
				info.hasUnresolved = true
			}
		}
		return true
	})
	return info
}

// isPkgCall reports whether call is <localName>.Xxx(...).
func isPkgCall(call *ast.CallExpr, localName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == localName
}

// isExternalCall reports whether call is <pkg>.External(...).
func isExternalCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "External"
}

// resolveCallArg returns the string value of the first argument to a call
// if it is a string literal or a known constant. Returns "" otherwise.
func resolveCallArg(call *ast.CallExpr, constValues map[string]string) string {
	if len(call.Args) == 0 {
		return ""
	}
	switch arg := call.Args[0].(type) {
	case *ast.BasicLit:
		if arg.Kind == token.STRING {
			val, _ := strconv.Unquote(arg.Value)
			return val
		}
	case *ast.Ident:
		if val, ok := constValues[arg.Name]; ok {
			return val
		}
	}
	return ""
}

// classifyURL records a resolved URL value as allowed or disallowed in info.
func classifyURL(info *hrefExprInfo, val string) {
	if hrefcheck.IsAllowedNonRelativeHref(val) {
		info.hasAllowed = true
	} else if info.disallowedURL == "" {
		info.disallowedURL = val
	}
}

// funcInfo holds information extracted from a single templ function definition.
type funcInfo struct {
	name       string
	filename   string // base filename
	childCalls []string
	actionRefs []actionRef
}

type actionRef struct {
	funcName string
	line     int
	col      int
}

// checkActionOwnership verifies that action.XXX() calls in templ templates
// are only used in pages that own those actions.
func (c *checker) checkActionOwnership(
	pkg *packages.Package,
	app *model.App,
	templPaths []string,
) {
	// Build action ownership map: generated func name → page type name (or "App").
	actionOwner := buildActionOwnerMap(app)
	if len(actionOwner) == 0 {
		return
	}

	// Parse all templ files and extract function info.
	funcsByName := map[string]*funcInfo{}
	for _, path := range templPaths {
		for _, fi := range c.parseTemplFuncInfos(path) {
			funcsByName[fi.name] = fi
		}
	}
	if len(funcsByName) == 0 {
		return
	}

	// For each page, find GET handler entry templ functions, BFS through
	// the call graph, and check action ownership.
	for _, page := range app.Pages {
		if page.GET == nil {
			continue
		}
		fd := findGETFuncDecl(pkg, page.TypeName)
		if fd == nil {
			continue
		}
		entries := extractTemplCallsFromBody(fd.Body, funcsByName)
		reachable := bfsTemplFuncs(entries, funcsByName)

		for _, fi := range reachable {
			for _, ref := range fi.actionRefs {
				owner, ok := actionOwner[ref.funcName]
				if !ok {
					continue
				}
				if owner == "App" || owner == page.TypeName {
					continue
				}
				pos := token.Position{
					Filename: fi.filename,
					Line:     ref.line,
					Column:   ref.col,
				}
				c.errFn(pos, &ErrorActionWrongPage{
					ActionFunc: ref.funcName,
					PageType:   page.TypeName,
					OwnerPage:  owner,
				})
			}
		}
	}
}

// buildActionOwnerMap returns a map from generated action function name
// to the owning page type name (or "App" for app-level actions).
func buildActionOwnerMap(app *model.App) map[string]string {
	m := map[string]string{}
	for _, a := range app.Actions {
		funcName := strings.ToUpper(a.HTTPMethod) + "App" + a.Name
		m[funcName] = "App"
	}
	for _, p := range app.Pages {
		pageSuffix := strings.TrimPrefix(p.TypeName, "Page")
		for _, a := range p.Actions {
			funcName := strings.ToUpper(a.HTTPMethod) + "Page" + pageSuffix + a.Name
			m[funcName] = p.TypeName
		}
	}
	return m
}

// parseTemplFuncInfos parses a .templ file and returns info for each
// templ function defined in it.
func (c *checker) parseTemplFuncInfos(path string) []*funcInfo {
	tf, err := templparser.Parse(path)
	if err != nil {
		return nil
	}
	filename := filepath.Base(path)
	var funcs []*funcInfo
	for _, node := range tf.Nodes {
		tmpl, ok := node.(*templparser.HTMLTemplate)
		if !ok {
			continue
		}
		name := templFuncName(tmpl.Expression.Value)
		if name == "" {
			continue
		}
		fi := &funcInfo{name: name, filename: filename}
		c.collectTemplCalls(tmpl.Children, fi)
		funcs = append(funcs, fi)
	}
	return funcs
}

// templFuncName extracts the function name from a templ expression like
// "page()" or "Name(p Parameter)".
func templFuncName(expr string) string {
	if i := strings.IndexByte(expr, '('); i > 0 {
		return strings.TrimSpace(expr[:i])
	}
	return ""
}

// collectTemplCalls recursively walks templ AST nodes collecting child
// template calls and action.XXX() references.
func (c *checker) collectTemplCalls(nodes []templparser.Node, fi *funcInfo) {
	for _, node := range nodes {
		switch n := node.(type) {
		case *templparser.TemplElementExpression:
			if name := templCallName(n.Expression.Value); name != "" {
				fi.childCalls = append(fi.childCalls, name)
			}
			c.collectActionRefs(n.Expression, fi)
			c.collectTemplCalls(n.Children, fi)
		case *templparser.CallTemplateExpression:
			if name := templCallName(n.Expression.Value); name != "" {
				fi.childCalls = append(fi.childCalls, name)
			}
			c.collectActionRefs(n.Expression, fi)
		case *templparser.Element:
			c.collectElementActionRefs(n, fi)
			c.collectTemplCalls(n.Children, fi)
		case *templparser.StringExpression:
			c.collectActionRefs(n.Expression, fi)
		case templparser.CompositeNode:
			c.collectTemplCalls(n.ChildNodes(), fi)
		}
	}
}

// templCallName extracts a local function name from a templ call expression.
// "header()" → "header", "pkg.Foo()" → "" (not local).
func templCallName(expr string) string {
	expr = strings.TrimSpace(expr)
	i := strings.IndexByte(expr, '(')
	if i <= 0 {
		return ""
	}
	name := strings.TrimSpace(expr[:i])
	if strings.ContainsAny(name, ". ") {
		return "" // qualified or complex expression
	}
	return name
}

// collectActionRefs parses a Go expression and collects action.XXX() references.
func (c *checker) collectActionRefs(expr templparser.Expression, fi *funcInfo) {
	if c.actionLocalName == "" {
		return
	}
	findPkgCalls(expr.Value, c.actionLocalName, func(funcName string) {
		fi.actionRefs = append(fi.actionRefs, actionRef{
			funcName: funcName,
			line:     int(expr.Range.From.Line) + 1,
			col:      int(expr.Range.From.Col) + 1,
		})
	})
}

// findPkgCalls parses expr as a Go expression and calls fn for each
// <localName>.Xxx() call found. fn receives the called method name (Xxx).
func findPkgCalls(expr, localName string, fn func(funcName string)) {
	exprAST, err := goparser.ParseExpr(expr)
	if err != nil {
		return
	}
	ast.Inspect(exprAST, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isPkgCall(call, localName) {
			return true
		}
		sel := call.Fun.(*ast.SelectorExpr) // safe: isPkgCall verified
		fn(sel.Sel.Name)
		return false
	})
}

// collectElementActionRefs scans an element's expression attributes for
// action.XXX() references.
func (c *checker) collectElementActionRefs(el *templparser.Element, fi *funcInfo) {
	for _, attr := range el.Attributes {
		ea, ok := attr.(*templparser.ExpressionAttribute)
		if !ok {
			continue
		}
		c.collectActionRefs(ea.Expression, fi)
	}
}

// findGETFuncDecl finds the GET method FuncDecl for the given page type.
func findGETFuncDecl(pkg *packages.Package, pageTypeName string) *ast.FuncDecl {
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			if fd.Name.Name != "GET" {
				continue
			}
			recv := recvTypeName(fd.Recv.List[0].Type)
			if recv == pageTypeName {
				return fd
			}
		}
	}
	return nil
}

// recvTypeName extracts the type name from a receiver expression,
// handling both value and pointer receivers.
func recvTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// extractTemplCallsFromBody walks a Go function body and returns the names
// of templ functions that are called (identified by matching known templ
// function names).
func extractTemplCallsFromBody(body *ast.BlockStmt, known map[string]*funcInfo) []string {
	if body == nil {
		return nil
	}
	seen := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		if _, exists := known[ident.Name]; exists {
			seen[ident.Name] = true
		}
		return true
	})
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}

// bfsTemplFuncs returns all templ functions reachable from the given
// entry points via the call graph.
func bfsTemplFuncs(entries []string, funcs map[string]*funcInfo) []*funcInfo {
	visited := map[string]bool{}
	var result []*funcInfo
	queue := entries
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if visited[name] {
			continue
		}
		visited[name] = true
		fi, ok := funcs[name]
		if !ok {
			continue
		}
		result = append(result, fi)
		queue = append(queue, fi.childCalls...)
	}
	return result
}
