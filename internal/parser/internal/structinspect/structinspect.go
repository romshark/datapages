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

	"github.com/romshark/datapages/internal/parser/internal/typecheck"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/parser/validate"
)

// baseIdent unwraps a type expression to the identifier naming its type:
// T, *T, T[A] and *T[A, B] all yield T. A qualified name yields nil,
// the passes key on names declared in the app package.
func baseIdent(expr ast.Expr) *ast.Ident {
	for {
		switch t := expr.(type) {
		case *ast.Ident:
			return t
		case *ast.StarExpr:
			expr = t.X
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		default:
			return nil
		}
	}
}

// ReceiverTypeName extracts the type name from a method
// receiver expression, handling the T, *T and generic forms.
func ReceiverTypeName(expr ast.Expr) string {
	if id := baseIdent(expr); id != nil {
		return id.Name
	}
	return ""
}

// EmbeddedTypeNames returns the names of all embedded types
// in a struct.
func EmbeddedTypeNames(st *ast.StructType) []string {
	var out []string
	for _, f := range st.Fields.List {
		if len(f.Names) != 0 {
			continue
		}
		if id := baseIdent(f.Type); id != nil {
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
		if id := baseIdent(f.Type); id != nil {
			out[id.Name] = id.Pos()
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

// SubjectField describes an event field typed as a datapages subject segment.
type SubjectField struct {
	FieldName  string            // e.g. "Recipients"
	Kind       model.SubjectKind // segment type, e.g. datapages.SubjectUser
	SignalName string            // e.g. "instance_id" (from signal:"instance_id" tag, empty otherwise)
	Pos        token.Pos         // position of the field name identifier
}

// SubjectFieldResult holds the result of inspecting a struct for subject fields.
type SubjectFieldResult struct {
	// Fields are the valid subject fields found, in definition order.
	Fields []SubjectField
	// AfterPayload is non-nil when a subject field appears after a
	// non-subject (payload) field. It points to the offending field.
	AfterPayload *SubjectField
	// DuplicateSignal is non-nil when two subject fields share the
	// same signal:"..." tag value. It points to the second occurrence.
	// DuplicateSignalFirst names the first field that used the signal.
	DuplicateSignal      *SubjectField
	DuplicateSignalFirst string
	// UserWithSignal is non-nil when a user-addressed subject field has a
	// signal:"..." tag. Such a field is bound to the authenticated user
	// and must not be bound to a signal.
	UserWithSignal *SubjectField
	// InvalidSignal is non-nil when a signal:"..." tag value is malformed.
	InvalidSignal *SubjectField
	// Unexported lists unexported subject fields.
	// Generated code in another package can't reach them, hence they're rejected.
	Unexported []SubjectField
	// Prefixed lists exported fields named like a subject field
	// (prefix "Subject") that aren't typed as one. Kind is
	// model.SubjectKindNone for all of them.
	Prefixed []SubjectField
}

// SubjectFields inspects a type spec for fields typed as datapages subject
// segments ([github.com/romshark/datapages.Subject], .Subjects, .SubjectUser
// and .SubjectUsers). Returns them in definition order together with the
// violations found along the way.
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
		kind := typecheck.SubjectKindOf(info.TypeOf(f.Type))
		if !kind.IsSubject() {
			// Not a subject field, it's a payload field.
			if f.Names[0].IsExported() {
				seenPayload = true
				if strings.HasPrefix(name, "Subject") {
					result.Prefixed = append(result.Prefixed, SubjectField{
						FieldName: name,
						Pos:       f.Names[0].Pos(),
					})
				}
			}
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
			Kind:       kind,
			SignalName: signalName,
			Pos:        f.Names[0].Pos(),
		}

		if !f.Names[0].IsExported() {
			result.Unexported = append(result.Unexported, sf)
			continue
		}

		if seenPayload && result.AfterPayload == nil {
			result.AfterPayload = &sf
		}
		if kind.IsUser() && signalName != "" && result.UserWithSignal == nil {
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
