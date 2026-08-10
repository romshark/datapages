// Package structinspect provides AST helpers for inspecting
// Go struct types and method receivers.
package structinspect

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"github.com/romshark/datapages/parser/validate"
)

// ReceiverTypeName extracts the type name from a method
// receiver expression, handling T, *T, and generic instantiations
// T[X], *T[X, Y], etc. Returns "" for anything that isn't rooted
// at a plain identifier.
func ReceiverTypeName(expr ast.Expr) string {
	if id := embeddedBaseIdent(expr); id != nil {
		return id.Name
	}
	return ""
}

// embeddedBaseIdent returns the base *ast.Ident of an embedded field
// type, unwrapping pointer and generic-instantiation syntax. It handles:
//
//	T           → T
//	*T          → T
//	T[A]        → T
//	T[A, B]     → T
//
// Returns nil when the field is not a plain embedded type (e.g. a
// qualified identifier from another package, unsupported here).
func embeddedBaseIdent(expr ast.Expr) *ast.Ident {
	switch t := expr.(type) {
	case *ast.Ident:
		return t
	case *ast.StarExpr:
		return embeddedBaseIdent(t.X)
	case *ast.IndexExpr:
		return embeddedBaseIdent(t.X)
	case *ast.IndexListExpr:
		return embeddedBaseIdent(t.X)
	}
	return nil
}

// EmbeddedTypeNames returns the names of all embedded types
// in a struct. Generic instantiations (e.g. Base[StateXXX]) are
// collapsed to their base type name (Base).
func EmbeddedTypeNames(st *ast.StructType) []string {
	var out []string
	for _, f := range st.Fields.List {
		if len(f.Names) != 0 {
			continue
		}
		if id := embeddedBaseIdent(f.Type); id != nil {
			out = append(out, id.Name)
		}
	}
	return out
}

// EmbeddedFieldPosMap returns a map from embedded type name
// to the position of the embedding field identifier.
func EmbeddedFieldPosMap(
	st *ast.StructType,
) map[string]token.Pos {
	out := map[string]token.Pos{}
	if st == nil || st.Fields == nil {
		return out
	}
	for _, f := range st.Fields.List {
		if len(f.Names) != 0 {
			continue
		}
		if id := embeddedBaseIdent(f.Type); id != nil {
			out[id.Name] = id.Pos()
		}
	}
	return out
}

// EmbeddedFieldTypeExprs returns a map from embedded type name to the
// type expression written at the embed site. For a generic embed the expression carries
// the type arguments, e.g. `Base` maps to the expression `Base[StateFoo]`.
func EmbeddedFieldTypeExprs(st *ast.StructType) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	if st == nil || st.Fields == nil {
		return out
	}
	for _, f := range st.Fields.List {
		if len(f.Names) != 0 {
			continue
		}
		if id := embeddedBaseIdent(f.Type); id != nil {
			out[id.Name] = f.Type
		}
	}
	return out
}

// EmbeddedTypeArgNames returns a map from embedded abstract-page
// type name to the list of type argument names written at the embed
// site. Non-identifier type arguments (e.g. `Base[*StateFoo]` where
// the argument is starred) are returned as their base identifier name.
//
// For a non-generic embed `Base`, the entry maps to nil. For a generic
// embed `Base[StateFoo]`, the entry is ["StateFoo"]. For a list
// instantiation `Base[StateA, StateB]`, the entry is ["StateA",
// "StateB"].
func EmbeddedTypeArgNames(st *ast.StructType) map[string][]string {
	out := map[string][]string{}
	if st == nil || st.Fields == nil {
		return out
	}
	for _, f := range st.Fields.List {
		if len(f.Names) != 0 {
			continue
		}
		base := embeddedBaseIdent(f.Type)
		if base == nil {
			continue
		}
		switch t := f.Type.(type) {
		case *ast.IndexExpr:
			if id := embeddedBaseIdent(t.Index); id != nil {
				out[base.Name] = []string{id.Name}
			}
		case *ast.IndexListExpr:
			args := make([]string, 0, len(t.Indices))
			for _, a := range t.Indices {
				if id := embeddedBaseIdent(a); id != nil {
					args = append(args, id.Name)
				}
			}
			out[base.Name] = args
		default:
			out[base.Name] = nil
		}
	}
	return out
}

// HasDisallowedNamedFields reports whether a page struct
// contains any named field besides the single allowed
// `App *App`. Embedded fields are ignored (validated
// separately).
func HasDisallowedNamedFields(st *ast.StructType) bool {
	if st == nil || st.Fields == nil {
		return false
	}
	appCount := 0
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue
		}
		if len(f.Names) != 1 {
			return true
		}
		if f.Names[0].Name != "App" {
			return true
		}
		appCount++
	}
	return appCount > 1
}

// HasRequiredAppField reports whether the struct has the
// required `App *App` field.
func HasRequiredAppField(
	st *ast.StructType, info *types.Info,
) bool {
	for _, f := range st.Fields.List {
		if len(f.Names) != 1 || f.Names[0].Name != "App" {
			continue
		}
		t := info.TypeOf(f.Type)
		if t == nil {
			continue
		}
		ptr, ok := t.(*types.Pointer)
		if !ok {
			continue
		}
		named, ok := ptr.Elem().(*types.Named)
		if !ok {
			continue
		}
		if named.Obj() != nil &&
			named.Obj().Name() == "App" {
			return true
		}
	}
	return false
}

// SubjectField describes a Subject-prefixed field found in a struct.
type SubjectField struct {
	FieldName  string    // e.g. "SubjectUser"
	Name       string    // e.g. "User"
	SignalName string    // e.g. "instance_id" (from signal:"instance_id" tag, empty otherwise)
	Singular   bool      // true when the field type is string (not []string)
	Pos        token.Pos // position of the field name identifier
}

// SubjectFieldResult holds the result of inspecting a struct for Subject fields.
type SubjectFieldResult struct {
	// Fields are the valid Subject fields found, in definition order.
	Fields []SubjectField
	// AfterPayload is non-nil when a Subject field appears after a
	// non-Subject (payload) field. It points to the offending field.
	AfterPayload *SubjectField
	// DuplicateSignal is non-nil when two subject fields share the
	// same signal:"..." tag value. It points to the second occurrence.
	// DuplicateSignalFirst names the first field that used the signal.
	DuplicateSignal      *SubjectField
	DuplicateSignalFirst string
	// UserWithSignal is non-nil when SubjectUser has a signal:"..." tag.
	// SubjectUser is auth-scoped and must not be bound to a signal.
	UserWithSignal *SubjectField
	// InvalidSignal is non-nil when a signal:"..." tag value is malformed.
	InvalidSignal *SubjectField
}

// SubjectFields inspects a type spec for Subject-prefixed fields.
// A valid Subject field has type string or []string.
// Returns the list of subject fields and whether any appear after payload fields.
func SubjectFields(
	ts *ast.TypeSpec, info *types.Info,
) SubjectFieldResult {
	var result SubjectFieldResult
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return result
	}

	seenPayload := false
	for _, f := range st.Fields.List {
		if len(f.Names) != 1 {
			continue
		}
		name := f.Names[0].Name
		suffix, isSubject := strings.CutPrefix(name, "Subject")
		if !isSubject || suffix == "" {
			// Not a Subject field — it's a payload field.
			if len(f.Names) == 1 && f.Names[0].IsExported() {
				seenPayload = true
			}
			continue
		}

		// Validate type is string or []string.
		t := info.TypeOf(f.Type)
		if t == nil {
			continue
		}
		singular := false
		switch ut := t.(type) {
		case *types.Basic:
			if ut.Kind() != types.String {
				continue
			}
			singular = true
		case *types.Slice:
			basic, ok := ut.Elem().(*types.Basic)
			if !ok || basic.Kind() != types.String {
				continue
			}
		default:
			continue
		}

		// Extract optional signal:"xxx" tag for signal-scoped subject fields.
		var signalName string
		if f.Tag != nil {
			if tag, err := strconv.Unquote(f.Tag.Value); err == nil {
				signalName = reflect.StructTag(tag).Get("signal")
			}
		}

		sf := SubjectField{
			FieldName:  name,
			Name:       suffix,
			SignalName: signalName,
			Singular:   singular,
			Pos:        f.Names[0].Pos(),
		}

		if seenPayload && result.AfterPayload == nil {
			result.AfterPayload = &sf
		}
		if suffix == "User" && signalName != "" && result.UserWithSignal == nil {
			result.UserWithSignal = &sf
		}
		if signalName != "" && validate.SignalTagName(signalName) != nil && result.InvalidSignal == nil {
			result.InvalidSignal = &sf
		}
		if signalName != "" && result.DuplicateSignal == nil {
			for _, prev := range result.Fields {
				if prev.SignalName == signalName {
					result.DuplicateSignal = &sf
					result.DuplicateSignalFirst = prev.FieldName
					break
				}
			}
		}

		result.Fields = append(result.Fields, sf)
	}

	return result
}
