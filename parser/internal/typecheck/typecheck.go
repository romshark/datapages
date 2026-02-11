// Package typecheck provides type-checking predicates for
// common Go types used in Datapages handler signatures.
package typecheck

import (
	"go/ast"
	"go/types"
)

// IsString reports whether t's underlying type is string.
func IsString(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// IsInt reports whether t's underlying type is int.
func IsInt(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Int
}

// IsError reports whether t is the builtin "error" type.
func IsError(t types.Type) bool {
	if t == nil {
		return false
	}
	// builtin "error" is a named interface in Universe.
	return t.String() == "error"
}

// IsTemplComponent reports whether t is
// github.com/a-h/templ.Component.
func IsTemplComponent(t types.Type) bool {
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
	return obj.Pkg().Path() == "github.com/a-h/templ" &&
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

// IsSessionType reports whether expr resolves to a named
// type called "Session".
func IsSessionType(
	expr ast.Expr, info *types.Info,
) bool {
	t := info.TypeOf(expr)
	if t == nil {
		return false
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Name() == "Session"
}

// EventTypeNameOf returns the EventXXX type name for expr
// if it is (or points to) a named type whose name is in
// eventTypeNames.
func EventTypeNameOf(
	expr ast.Expr,
	info *types.Info,
	eventTypeNames map[string]struct{},
) (string, bool) {
	t := info.TypeOf(expr)
	if t == nil {
		return "", false
	}
	// Allow both EventFoo and *EventFoo.
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
	if named.Obj().Pkg().Path() == "" {
		return "", false
	}
	return name, true
}
