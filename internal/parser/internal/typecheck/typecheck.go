// Package typecheck provides type-checking predicates for
// common Go types used in Datapages handler signatures.
package typecheck

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/internal/gotypes"
	"github.com/romshark/datapages/internal/parser/model"
)

// IsInputFieldType reports whether t is a supported type for
// path and query struct fields: string, bool, integers
// (int, int8, int16, int32, int64, uint, uint8, uint16,
// uint32, uint64), floats (float32, float64),
// or any type that implements encoding.TextUnmarshaler.
func IsInputFieldType(t types.Type) bool {
	if isBasicInputType(t) {
		return true
	}
	return gotypes.ImplementsTextUnmarshaler(t)
}

// isBasicInputType reports whether t is a basic scalar type
// supported for path/query fields.
func isBasicInputType(t types.Type) bool {
	return gotypes.IsString(t) || gotypes.IsBool(t) ||
		gotypes.IsInt(t) || gotypes.IsFloat(t)
}

// IsTimeTime reports whether t is time.Time from the standard library.
func IsTimeTime(t types.Type) bool {
	if t == nil {
		return false
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == "time" && obj.Name() == "Time"
}

// IsError reports whether t is the builtin "error" type.
func IsError(t types.Type) bool {
	if t == nil {
		return false
	}
	// builtin "error" is a named interface in Universe.
	return t.String() == "error"
}

// IsComponent reports whether t is datapages.Component.
func IsComponent(t types.Type) bool {
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == datapagesPkgPath &&
		obj.Name() == "Component"
}

// IsPtrToNetHTTPReq reports whether expr resolves to
// *net/http.Request.
func IsPtrToNetHTTPReq(
	expr ast.Expr, info *types.Info,
) bool {
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
	return obj.Pkg().Path() == "net/http" &&
		obj.Name() == "Request"
}

// IsPtrToDatastarSSE reports whether expr resolves to
// *datastar.ServerSentEventGenerator.
func IsPtrToDatastarSSE(
	expr ast.Expr, info *types.Info,
) bool {
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
	return obj.Pkg().Path() ==
		"github.com/starfederation/datastar-go/datastar" &&
		obj.Name() == "ServerSentEventGenerator"
}

// datapagesPkgPath is the import path of the core datapages package that owns
// the abstract handler parameter and return types (SSE, Session, Redirect).
const datapagesPkgPath = "github.com/romshark/datapages"

// IsDatapagesSSE reports whether expr resolves to datapages.SSE.
func IsDatapagesSSE(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "SSE")
}

// IsSSEParam reports whether expr is the SSE handler parameter type.
// datapages.SSE is the only accepted form; IsPtrToDatastarSSE exists solely to
// detect the raw Datastar generator and report it as an error.
func IsSSEParam(expr ast.Expr, info *types.Info) bool {
	return IsDatapagesSSE(expr, info)
}

func isNamedFromPkg(
	expr ast.Expr, info *types.Info, pkgPath, name string,
) bool {
	t := info.TypeOf(expr)
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == pkgPath && obj.Name() == name
}

// IsHeadType reports whether expr resolves to datapages.Head.
func IsHeadType(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "Head")
}

// IsCloseSessionType reports whether expr resolves to datapages.CloseSession.
func IsCloseSessionType(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "CloseSession")
}

// IsEnableBgStreamType reports whether expr resolves to
// datapages.EnableBackgroundStreaming.
func IsEnableBgStreamType(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "EnableBackgroundStreaming")
}

// IsDisableRefreshType reports whether expr resolves to
// datapages.DisableRefreshAfterHidden.
func IsDisableRefreshType(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "DisableRefreshAfterHidden")
}

// IsStreamIDType reports whether expr resolves to datapages.StreamID.
func IsStreamIDType(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "StreamID")
}

// IsRedirectType reports whether expr resolves to datapages.Redirect.
func IsRedirectType(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "Redirect")
}

// IsSessionType reports whether expr resolves to datapages.Session[Data].
func IsSessionType(expr ast.Expr, info *types.Info) bool {
	_, ok := SessionDataType(expr, info)
	return ok
}

// IsNewSessionType reports whether expr resolves to datapages.NewSession[Data].
func IsNewSessionType(expr ast.Expr, info *types.Info) bool {
	_, ok := namedTypeArg(expr, info, "NewSession")
	return ok
}

// NewSessionDataType returns the Data type argument of datapages.NewSession[Data].
func NewSessionDataType(expr ast.Expr, info *types.Info) (types.Type, bool) {
	return namedTypeArg(expr, info, "NewSession")
}

// SessionDataType returns the Data type argument of datapages.Session[Data].
// ok is false if expr isn't an instantiation of datapages.Session.
func SessionDataType(expr ast.Expr, info *types.Info) (data types.Type, ok bool) {
	return namedTypeArg(expr, info, "Session")
}

// namedTypeArg returns the single type argument of the datapages generic type
// name that expr resolves to.
func namedTypeArg(
	expr ast.Expr, info *types.Info, name string,
) (arg types.Type, ok bool) {
	t := info.TypeOf(expr)
	if t == nil {
		return nil, false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return nil, false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil ||
		obj.Pkg().Path() != datapagesPkgPath || obj.Name() != name {
		return nil, false
	}
	args := named.TypeArgs()
	if args == nil || args.Len() != 1 {
		return nil, false
	}
	return args.At(0), true
}

// IsDispatchType reports whether expr resolves to datapages.Dispatcher[Event].
func IsDispatchType(expr ast.Expr, info *types.Info) bool {
	_, ok := namedTypeArg(expr, info, "Dispatcher")
	return ok
}

// DispatchEventNamed returns the Event type argument of
// datapages.Dispatcher[Event]. ok is false if expr isn't an instantiation of
// datapages.Dispatcher, named is nil if the argument isn't a named type.
func DispatchEventNamed(
	expr ast.Expr, info *types.Info,
) (named *types.Named, ok bool) {
	arg, ok := namedTypeArg(expr, info, "Dispatcher")
	if !ok {
		return nil, false
	}
	named, isNamed := types.Unalias(arg).(*types.Named)
	if !isNamed || named.Obj() == nil || named.Obj().Pkg() == nil {
		return nil, true
	}
	return named, true
}

// IsEventType reports whether the expression resolves to the
// named event type eventTypeName.
func IsEventType(
	expr ast.Expr,
	info *types.Info,
	eventTypeName string,
) bool {
	if eventTypeName == "" {
		return false
	}
	named, ok := EventNamedOf(expr, info)
	if !ok {
		return false
	}
	return named.Obj().Name() == eventTypeName
}

// EventNamedOf returns the named type expr denotes, a pointer to one
// counting as that type. The caller decides whether the name is an event.
//
// A type without a package is no event: every event is declared at package level,
// in the app package or in a package it imports.
func EventNamedOf(expr ast.Expr, info *types.Info) (*types.Named, bool) {
	t := info.TypeOf(expr)
	if t == nil {
		return nil, false
	}
	// Allow both EventFoo and *EventFoo.
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil ||
		named.Obj().Pkg().Path() == "" {
		return nil, false
	}
	return named, true
}

// SubjectKindOf reports which datapages subject segment type t is,
// model.SubjectKindNone if it's none of them.
func SubjectKindOf(t types.Type) model.SubjectKind {
	if t == nil {
		return model.SubjectKindNone
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return model.SubjectKindNone
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != datapagesPkgPath {
		return model.SubjectKindNone
	}
	switch obj.Name() {
	case "Subject":
		return model.SubjectKindValue
	case "SubjectUser":
		return model.SubjectKindUser
	}
	return model.SubjectKindNone
}

// PathValuesType returns the Values type argument of datapages.Path[Values].
// ok is false if expr isn't an instantiation of datapages.Path.
func PathValuesType(expr ast.Expr, info *types.Info) (types.Type, bool) {
	return namedTypeArg(expr, info, "Path")
}

// QueryValuesType returns the Values type argument of datapages.Query[Values].
// ok is false if expr isn't an instantiation of datapages.Query.
func QueryValuesType(expr ast.Expr, info *types.Info) (types.Type, bool) {
	return namedTypeArg(expr, info, "Query")
}

// SignalsValuesType returns the Values type argument of datapages.Signals[Values].
// ok is false if expr isn't an instantiation of datapages.Signals.
func SignalsValuesType(expr ast.Expr, info *types.Info) (types.Type, bool) {
	return namedTypeArg(expr, info, "Signals")
}

// TypeArgExpr returns the type argument expression of a generic type
// instantiation such as datapages.Path[struct{...}].
// It returns expr unchanged if expr isn't an instantiation.
func TypeArgExpr(expr ast.Expr) ast.Expr {
	switch t := ast.Unparen(expr).(type) {
	case *ast.IndexExpr:
		return t.Index
	case *ast.IndexListExpr:
		if len(t.Indices) == 1 {
			return t.Indices[0]
		}
	}
	return expr
}

// maxSubjectDeriveDepth caps the declaration chain [DerivedSubjectKindOf] follows.
// Go rejects a cycle among type declarations:
// the cap only guards against an unexpected type graph.
const maxSubjectDeriveDepth = 16

// DerivedSubjectKindOf reports which datapages subject segment type t is declared from,
// e.g. [model.SubjectKindUser] for `type UserID datapages.SubjectUser`.
// It returns [model.SubjectKindNone] for the two framework types themselves,
// which [SubjectKindOf] reports, and for every other type.
//
// pkg is the package t is written in. Both framework types are strings
// and a declaration from either keeps that underlying type, which erases the
// derivation from go/types: the declaration is read from the syntax of pkg and
// of its imports instead.
func DerivedSubjectKindOf(t types.Type, pkg *packages.Package) model.SubjectKind {
	if t == nil || pkg == nil || SubjectKindOf(t).IsSubject() {
		return model.SubjectKindNone
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || !gotypes.IsString(named.Underlying()) {
		return model.SubjectKindNone
	}
	for range maxSubjectDeriveDepth {
		obj := named.Obj()
		if obj == nil || obj.Pkg() == nil {
			return model.SubjectKindNone
		}
		declPkg := packageByPath(pkg, obj.Pkg().Path())
		if declPkg == nil || declPkg.TypesInfo == nil {
			return model.SubjectKindNone
		}
		rhs := typeSpecRHS(declPkg, obj.Pos())
		if rhs == nil {
			return model.SubjectKindNone
		}
		next, ok := types.Unalias(declPkg.TypesInfo.TypeOf(rhs)).(*types.Named)
		if !ok {
			return model.SubjectKindNone
		}
		if kind := SubjectKindOf(next); kind.IsSubject() {
			return kind
		}
		named, pkg = next, declPkg
	}
	return model.SubjectKindNone
}

// TypeDeclOf returns the package-level type declaration of obj, the package it
// is written in and the doc comment above it. It reads the syntax of pkg and of
// its imports, which is as far as a declaration an app package names can sit.
func TypeDeclOf(obj *types.TypeName, pkg *packages.Package) (
	declPkg *packages.Package, ts *ast.TypeSpec, doc *ast.CommentGroup, ok bool,
) {
	if obj == nil || obj.Pkg() == nil || pkg == nil {
		return nil, nil, nil, false
	}
	declPkg = packageByPath(pkg, obj.Pkg().Path())
	if declPkg == nil {
		return nil, nil, nil, false
	}
	for _, file := range declPkg.Syntax {
		for _, decl := range file.Decls {
			gd, isGen := decl.(*ast.GenDecl)
			if !isGen || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, isType := spec.(*ast.TypeSpec)
				if !isType || ts.Name.Pos() != obj.Pos() {
					continue
				}
				doc = ts.Doc
				if doc == nil {
					doc = gd.Doc
				}
				return declPkg, ts, doc, true
			}
		}
	}
	return nil, nil, nil, false
}

// packageByPath returns pkg itself or the package with the given path among
// what pkg imports, directly or further down. An alias re-exporting a type puts
// the declaration one package further out than the app package imports.
func packageByPath(pkg *packages.Package, path string) *packages.Package {
	if pkg.PkgPath == path {
		return pkg
	}
	if p, ok := pkg.Imports[path]; ok {
		return p
	}
	seen := map[*packages.Package]bool{pkg: true}
	queue := []*packages.Package{pkg}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if p, ok := cur.Imports[path]; ok {
			return p
		}
		for _, imp := range cur.Imports {
			if !seen[imp] {
				seen[imp] = true
				queue = append(queue, imp)
			}
		}
	}
	return nil
}

// typeSpecRHS returns the right-hand side of the package-level type
// declaration whose name identifier sits at pos, nil if pkg declares none.
func typeSpecRHS(pkg *packages.Package, pos token.Pos) ast.Expr {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok && ts.Name.Pos() == pos {
					return ts.Type
				}
			}
		}
	}
	return nil
}
