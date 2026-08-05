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

// IsUint64 reports whether t's underlying type is uint64.
func IsUint64(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint64
}

// IsBool reports whether t's underlying type is bool.
func IsBool(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Bool
}

// IsInputFieldType reports whether t is a supported type for
// path and query struct fields: string, bool, integers
// (int, int8, int16, int32, int64, uint, uint8, uint16,
// uint32, uint64), floats (float32, float64), or any type
// that implements encoding.TextUnmarshaler.
func IsInputFieldType(t types.Type) bool {
	if isBasicInputType(t) {
		return true
	}
	return ImplementsTextUnmarshaler(t)
}

// isBasicInputType reports whether t is a basic scalar type
// supported for path/query fields.
func isBasicInputType(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch b.Kind() {
	case types.String, types.Bool,
		types.Int, types.Int8, types.Int16,
		types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16,
		types.Uint32, types.Uint64,
		types.Float32, types.Float64:
		return true
	default:
		return false
	}
}

// textUnmarshaler is the method set of encoding.TextUnmarshaler.
var textUnmarshaler = func() *types.Interface {
	sig := types.NewSignatureType(
		nil, nil, nil,
		types.NewTuple(types.NewVar(
			0, nil, "text", types.NewSlice(types.Typ[types.Byte]),
		)),
		types.NewTuple(types.NewVar(
			0, nil, "", types.Universe.Lookup("error").Type(),
		)),
		false,
	)
	return types.NewInterfaceType(
		[]*types.Func{types.NewFunc(
			0, nil, "UnmarshalText", sig,
		)},
		nil,
	).Complete()
}()

// ImplementsTextUnmarshaler reports whether t or *t implements
// encoding.TextUnmarshaler.
func ImplementsTextUnmarshaler(t types.Type) bool {
	if t == nil {
		return false
	}
	if types.Implements(t, textUnmarshaler) {
		return true
	}
	return types.Implements(types.NewPointer(t), textUnmarshaler)
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

// datapagesPkgPath is the import path of the core datapages package that owns
// the abstract handler-parameter types (SSE, Offline).
const datapagesPkgPath = "github.com/romshark/datapages"

// IsDatapagesSSE reports whether expr resolves to datapages.SSE.
func IsDatapagesSSE(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "SSE")
}

// IsDatapagesPageCache reports whether expr resolves to
// datapages.PageCacheWriter.
func IsDatapagesPageCache(expr ast.Expr, info *types.Info) bool {
	return isNamedFromPkg(expr, info, datapagesPkgPath, "PageCacheWriter")
}

// isSSEParam reports whether expr is a valid SSE parameter type: either
// *datastar.ServerSentEventGenerator or the datapages.SSE interface.
// IsSSEParam reports whether expr is the SSE handler parameter type. datapages.SSE
// is the only accepted form; IsPtrToDatastarSSE exists solely to detect the raw
// Datastar generator and report it as an error.
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
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == pkgPath && obj.Name() == name
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
	t := info.TypeOf(expr)
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj() == nil {
		return false
	}
	return named.Obj().Name() == eventTypeName
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
