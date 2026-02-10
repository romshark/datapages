// Package paramvalidation validates handler parameter structs
// (path, query, signals) and route-to-path consistency.
package paramvalidation

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"datapages/parser/internal/routepattern"
	"datapages/parser/internal/structtag"
	"datapages/parser/internal/typecheck"
	"datapages/parser/model"
)

// Path parameter errors.
var (
	ErrPathParamNotStruct = errors.New(
		"path parameter must be an anonymous struct",
	)
	ErrPathFieldUnexported = errors.New(
		"path struct field must be exported",
	)
	ErrPathFieldMissingTag = errors.New(
		`path struct field must have a path:"..." tag`,
	)
	ErrPathFieldNotString = errors.New(
		"path struct field must be of type string",
	)
	ErrPathFieldNotInRoute = errors.New(
		"path struct field tag does not match " +
			"any route variable",
	)
	ErrPathMissingRouteVar = errors.New(
		"route variable has no matching " +
			"path struct field",
	)
)

// Query parameter errors.
var (
	ErrQueryParamNotStruct = errors.New(
		"query parameter must be an anonymous struct",
	)
	ErrQueryFieldUnexported = errors.New(
		"query struct field must be exported",
	)
	ErrQueryFieldMissingTag = errors.New(
		`query struct field must have a query:"..." tag`,
	)
)

// Signals parameter errors.
var (
	ErrSignalsParamNotStruct = errors.New(
		"signals parameter must be an anonymous struct",
	)
	ErrSignalsFieldUnexported = errors.New(
		"signals struct field must be exported",
	)
	ErrSignalsFieldMissingTag = errors.New(
		`signals struct field must have a json:"..." tag`,
	)
)

// IsPathParam reports whether the AST field is named "path".
func IsPathParam(f *ast.Field) bool {
	return len(f.Names) > 0 && f.Names[0].Name == "path"
}

// ValidatePathStruct validates that a path parameter is an
// anonymous struct with exported string fields each carrying
// a `path:"..."` tag.
func ValidatePathStruct(
	f *ast.Field, info *types.Info, recv, method string,
) error {
	if _, ok := f.Type.(*ast.StructType); !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrPathParamNotStruct, recv, method,
		)
	}

	t := info.TypeOf(f.Type)
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrPathParamNotStruct, recv, method,
		)
	}

	for i := range st.NumFields() {
		field := st.Field(i)
		tag := st.Tag(i)

		if !field.Exported() {
			return fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrPathFieldUnexported,
				field.Name(), recv, method,
			)
		}
		if !typecheck.IsString(field.Type()) {
			return fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrPathFieldNotString,
				field.Name(), recv, method,
			)
		}
		if !strings.Contains(tag, `path:"`) {
			return fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrPathFieldMissingTag,
				field.Name(), recv, method,
			)
		}
	}
	return nil
}

// IsQueryParam reports whether the AST field is named
// "query".
func IsQueryParam(f *ast.Field) bool {
	return len(f.Names) > 0 && f.Names[0].Name == "query"
}

// ValidateQueryStruct validates that a query parameter is an
// anonymous struct with exported fields each carrying a
// `query:"..."` tag.
func ValidateQueryStruct(
	f *ast.Field, info *types.Info, recv, method string,
) error {
	if _, ok := f.Type.(*ast.StructType); !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrQueryParamNotStruct, recv, method,
		)
	}

	t := info.TypeOf(f.Type)
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrQueryParamNotStruct, recv, method,
		)
	}

	for i := range st.NumFields() {
		field := st.Field(i)
		tag := st.Tag(i)

		if !field.Exported() {
			return fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrQueryFieldUnexported,
				field.Name(), recv, method,
			)
		}
		if !strings.Contains(tag, `query:"`) {
			return fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrQueryFieldMissingTag,
				field.Name(), recv, method,
			)
		}
	}
	return nil
}

// IsSignalsParam reports whether the AST field is named
// "signals".
func IsSignalsParam(f *ast.Field) bool {
	return len(f.Names) > 0 &&
		f.Names[0].Name == "signals"
}

// ValidateSignalsStruct validates that a signals parameter
// is an anonymous struct with exported fields each carrying
// a `json:"..."` tag.
func ValidateSignalsStruct(
	f *ast.Field, info *types.Info, recv, method string,
) error {
	if _, ok := f.Type.(*ast.StructType); !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrSignalsParamNotStruct, recv, method,
		)
	}

	t := info.TypeOf(f.Type)
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrSignalsParamNotStruct, recv, method,
		)
	}

	for i := range st.NumFields() {
		field := st.Field(i)
		tag := st.Tag(i)

		if !field.Exported() {
			return fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrSignalsFieldUnexported,
				field.Name(), recv, method,
			)
		}
		if !strings.Contains(tag, `json:"`) {
			return fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrSignalsFieldMissingTag,
				field.Name(), recv, method,
			)
		}
	}
	return nil
}

// ValidatePathAgainstRoute checks that every path struct
// field tag matches a route variable and vice versa.
func ValidatePathAgainstRoute(
	h *model.Handler, recv, method string,
) error {
	varSet := make(map[string]bool)
	for v := range routepattern.Vars(h.Route) {
		varSet[v] = true
	}

	if h.InputPath == nil {
		for v := range varSet {
			return fmt.Errorf(
				"%w: {%s} in %s.%s",
				ErrPathMissingRouteVar, v, recv, method,
			)
		}
		return nil
	}

	st, ok := h.InputPath.Type.Resolved.Underlying().(*types.Struct)
	if !ok {
		return nil
	}

	for i := range st.NumFields() {
		tagVal := structtag.PathTagValue(st.Tag(i))
		if tagVal == "" {
			continue
		}
		if !varSet[tagVal] {
			return fmt.Errorf(
				"%w: %q in %s.%s",
				ErrPathFieldNotInRoute,
				tagVal, recv, method,
			)
		}
		delete(varSet, tagVal)
	}

	for v := range varSet {
		return fmt.Errorf(
			"%w: {%s} in %s.%s",
			ErrPathMissingRouteVar, v, recv, method,
		)
	}
	return nil
}
