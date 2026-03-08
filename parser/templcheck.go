package parser

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	templparser "github.com/a-h/templ/parser/v2"

	"golang.org/x/tools/go/packages"
)

// checkTemplFiles parses .templ files belonging to the app package and reports:
//   - hardcoded app-internal href and action attributes
//   - cross-page action references (action from page A used in page B's template)
func checkTemplFiles(ctx *parseCtx, errs *Errors) {
	templPaths := templFilesFromPackage(ctx.pkg)
	for _, templPath := range templPaths {
		checkTemplFile(errs, templPath, ctx.opts.StaticPrefix)
	}
	if ctx.app != nil {
		checkTemplActionOwnership(ctx, errs, templPaths)
	}
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

func checkTemplFile(errs *Errors, path, staticPrefix string) {
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
		walkChildren(errs, filename, tmpl.Children, staticPrefix)
	}
}

// walkChildren recursively walks templ AST children looking for Element nodes
// with hardcoded href/action attributes.
func walkChildren(errs *Errors, filename string, nodes []templparser.Node, staticPrefix string) {
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
				checkElementAttrs(errs, filename, n, staticPrefix)
			}
			walkChildren(errs, filename, n.Children, staticPrefix)
		case templparser.CompositeNode:
			walkChildren(errs, filename, n.ChildNodes(), staticPrefix)
		}
		prevIsNolint = false
	}
}

func checkElementAttrs(errs *Errors, filename string, el *templparser.Element, staticPrefix string) {
	for _, attr := range el.Attributes {
		ca, ok := attr.(*templparser.ConstantAttribute)
		if !ok {
			continue
		}
		key, ok := ca.Key.(templparser.ConstantAttributeKey)
		if !ok {
			continue
		}

		switch key.Name {
		case "href":
			if el.Name != "a" || isExemptURL(ca.Value, staticPrefix) {
				continue
			}
			pos := token.Position{
				Filename: filename,
				Line:     int(ca.Range.From.Line),
				Column:   int(ca.Range.From.Col),
			}
			errs.ErrAt(pos, &ErrorTemplHardcodedHref{URL: ca.Value})
		case "action":
			if isExemptURL(ca.Value, staticPrefix) {
				continue
			}
			pos := token.Position{
				Filename: filename,
				Line:     int(ca.Range.From.Line),
				Column:   int(ca.Range.From.Col),
			}
			errs.ErrAt(pos, &ErrorTemplHardcodedAction{URL: ca.Value})
		}
	}
}

// isExemptURL reports whether a URL should not be flagged as a hardcoded
// app-internal URL. External URLs, static assets, anchors, and special
// schemes are exempt.
func isExemptURL(url, staticPrefix string) bool {
	return url == "" ||
		!strings.HasPrefix(url, "/") ||
		strings.HasPrefix(url, staticPrefix) ||
		strings.HasPrefix(url, "//")
}

// actionRefMatch matches action.FuncName( in Go expressions.
var actionRefMatch = regexp.MustCompile(`\baction\.(\w+)\s*\(`)

// templFuncInfo holds information extracted from a single templ function definition.
type templFuncInfo struct {
	name       string
	filename   string // base filename
	childCalls []string
	actionRefs []templActionRef
}

type templActionRef struct {
	funcName string
	line     int
	col      int
}

// checkTemplActionOwnership verifies that action.XXX() calls in templ templates
// are only used in pages that own those actions.
func checkTemplActionOwnership(ctx *parseCtx, errs *Errors, templPaths []string) {
	// Build action ownership map: generated func name → page type name (or "App").
	actionOwner := buildActionOwnerMap(ctx)
	if len(actionOwner) == 0 {
		return
	}

	// Parse all templ files and extract function info.
	funcsByName := map[string]*templFuncInfo{}
	for _, path := range templPaths {
		for _, fi := range parseTemplFuncInfos(path) {
			funcsByName[fi.name] = fi
		}
	}
	if len(funcsByName) == 0 {
		return
	}

	// For each page, find GET handler entry templ functions, BFS through
	// the call graph, and check action ownership.
	for _, page := range ctx.app.Pages {
		if page.GET == nil {
			continue
		}
		fd := findGETFuncDecl(ctx, page.TypeName)
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
				errs.ErrAt(pos, &ErrorTemplActionWrongPage{
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
func buildActionOwnerMap(ctx *parseCtx) map[string]string {
	m := map[string]string{}
	for _, a := range ctx.app.Actions {
		funcName := strings.ToUpper(a.HTTPMethod) + "App" + a.Name
		m[funcName] = "App"
	}
	for _, p := range ctx.app.Pages {
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
func parseTemplFuncInfos(path string) []*templFuncInfo {
	tf, err := templparser.Parse(path)
	if err != nil {
		return nil
	}
	filename := filepath.Base(path)
	var funcs []*templFuncInfo
	for _, node := range tf.Nodes {
		tmpl, ok := node.(*templparser.HTMLTemplate)
		if !ok {
			continue
		}
		name := templFuncName(tmpl.Expression.Value)
		if name == "" {
			continue
		}
		fi := &templFuncInfo{name: name, filename: filename}
		collectTemplCalls(tmpl.Children, fi)
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
func collectTemplCalls(nodes []templparser.Node, fi *templFuncInfo) {
	for _, node := range nodes {
		switch n := node.(type) {
		case *templparser.TemplElementExpression:
			if name := templCallName(n.Expression.Value); name != "" {
				fi.childCalls = append(fi.childCalls, name)
			}
			collectActionRefs(n.Expression, fi)
			collectTemplCalls(n.Children, fi)
		case *templparser.CallTemplateExpression:
			if name := templCallName(n.Expression.Value); name != "" {
				fi.childCalls = append(fi.childCalls, name)
			}
			collectActionRefs(n.Expression, fi)
		case *templparser.Element:
			collectElementActionRefs(n, fi)
			collectTemplCalls(n.Children, fi)
		case *templparser.StringExpression:
			collectActionRefs(n.Expression, fi)
		case templparser.CompositeNode:
			collectTemplCalls(n.ChildNodes(), fi)
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

// collectActionRefs scans a Go expression for action.XXX() references.
func collectActionRefs(expr templparser.Expression, fi *templFuncInfo) {
	matches := actionRefMatch.FindAllStringSubmatchIndex(expr.Value, -1)
	for _, loc := range matches {
		funcName := expr.Value[loc[2]:loc[3]]
		fi.actionRefs = append(fi.actionRefs, templActionRef{
			funcName: funcName,
			line:     int(expr.Range.From.Line),
			col:      int(expr.Range.From.Col),
		})
	}
}

// collectElementActionRefs scans an element's expression attributes for
// action.XXX() references.
func collectElementActionRefs(el *templparser.Element, fi *templFuncInfo) {
	for _, attr := range el.Attributes {
		ea, ok := attr.(*templparser.ExpressionAttribute)
		if !ok {
			continue
		}
		collectActionRefs(ea.Expression, fi)
	}
}

// findGETFuncDecl finds the GET method FuncDecl for the given page type.
func findGETFuncDecl(ctx *parseCtx, pageTypeName string) *ast.FuncDecl {
	for _, f := range ctx.pkg.Syntax {
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
func extractTemplCallsFromBody(body *ast.BlockStmt, known map[string]*templFuncInfo) []string {
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
func bfsTemplFuncs(entries []string, funcs map[string]*templFuncInfo) []*templFuncInfo {
	visited := map[string]bool{}
	var result []*templFuncInfo
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
