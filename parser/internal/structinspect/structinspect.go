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

// HasTargetUserIDs reports whether a type spec has a
// TargetUserIDs []string field with a `json:"-"` tag.
func HasTargetUserIDs(
	ts *ast.TypeSpec, info *types.Info,
) bool {
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return false
	}
	for _, f := range st.Fields.List {
		if len(f.Names) != 1 ||
			f.Names[0].Name != "TargetUserIDs" {
			continue
		}
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
		if f.Tag == nil {
			return false
		}
		tag, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			return false
		}
		return strings.Contains(tag, `json:"-"`)
	}
	return false
}
