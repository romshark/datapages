// Package structinspect provides AST helpers for inspecting
// Go struct types and method receivers.
package structinspect

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"
)

// ReceiverTypeName extracts the type name from a method
// receiver expression, handling both T and *T forms.
func ReceiverTypeName(expr ast.Expr) string {
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

// EmbeddedTypeNames returns the names of all embedded types
// in a struct.
func EmbeddedTypeNames(st *ast.StructType) []string {
	var out []string
	for _, f := range st.Fields.List {
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
		switch t := f.Type.(type) {
		case *ast.Ident:
			out[t.Name] = t.Pos()
		case *ast.StarExpr:
			if id, ok := t.X.(*ast.Ident); ok {
				out[id.Name] = id.Pos()
			}
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
	FieldName string    // e.g. "SubjectUser"
	Name      string    // e.g. "User"
	Pos       token.Pos // position of the field name identifier
}

// SubjectFieldResult holds the result of inspecting a struct for Subject fields.
type SubjectFieldResult struct {
	// Fields are the valid Subject fields found, in definition order.
	Fields []SubjectField
	// AfterPayload is non-nil when a Subject field appears after a
	// non-Subject (payload) field. It points to the offending field.
	AfterPayload *SubjectField
}

// SubjectFields inspects a type spec for Subject-prefixed fields.
// A valid Subject field has type []string and struct tag json:"-".
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

		// Validate type is []string.
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

		// Validate json:"-" tag.
		if f.Tag == nil {
			continue
		}
		tag, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			continue
		}
		if !strings.Contains(tag, `json:"-"`) {
			continue
		}

		sf := SubjectField{
			FieldName: name,
			Name:      suffix,
			Pos:       f.Names[0].Pos(),
		}

		if seenPayload && result.AfterPayload == nil {
			result.AfterPayload = &sf
		}

		result.Fields = append(result.Fields, sf)
	}

	return result
}
