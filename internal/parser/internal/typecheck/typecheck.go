// Package typecheck provides type-checking predicates for
// common Go types used in Datapages handler signatures.
package typecheck

import (
	"go/ast"
	"go/types"

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

// DispatchEventTypeName returns the name of the Event type argument of
// datapages.Dispatcher[Event]. ok is false if expr isn't an instantiation of
// datapages.Dispatcher, name is empty if the argument isn't a named type.
func DispatchEventTypeName(expr ast.Expr, info *types.Info) (name string, ok bool) {
	arg, ok := namedTypeArg(expr, info, "Dispatcher")
	if !ok {
		return "", false
	}
	named, isNamed := types.Unalias(arg).(*types.Named)
	if !isNamed || named.Obj() == nil {
		return "", true
	}
	return named.Obj().Name(), true
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
