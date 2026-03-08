// Package paramvalidation validates handler parameter structs
// (path, query, signals) and route-to-path consistency.
package paramvalidation

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/romshark/datapages/internal/routepattern"
	"github.com/romshark/datapages/parser/internal/structtag"
	"github.com/romshark/datapages/parser/internal/typecheck"
	"github.com/romshark/datapages/parser/model"
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
	ErrPathFieldDuplicateTag = errors.New(
		"path struct field has duplicate path tag value",
	)
	ErrPathFieldEmptyTag = errors.New(
		`path struct field path tag must have a non-empty name`,
	)
)

// Query parameter errors.
var (
	ErrQueryParamNotStruct = errors.New(
		"query parameter must be a struct",
	)
	ErrQueryFieldUnexported = errors.New(
		"query struct field must be exported",
	)
	ErrQueryFieldMissingTag = errors.New(
		`query struct field must have a query:"..." tag`,
	)
	ErrQueryFieldDuplicateTag = errors.New(
		"query struct field has duplicate query tag value",
	)
	ErrQueryFieldEmptyTag = errors.New(
		`query struct field query tag must have a non-empty name`,
	)
)

// Signals parameter errors.
var (
	ErrSignalsParamNotStruct = errors.New(
		"signals parameter must be a struct",
	)
	ErrSignalsFieldUnexported = errors.New(
		"signals struct field must be exported",
	)
	ErrSignalsFieldMissingTag = errors.New(
		`signals struct field must have a json:"..." tag`,
	)
	ErrSignalsFieldDuplicateTag = errors.New(
		"signals struct field has duplicate json tag value",
	)
	ErrSignalsFieldEmptyTag = errors.New(
		`signals struct field json tag must have a non-empty name`,
	)
)

// IsSessionTokenParam reports whether the AST field is
// named "sessionToken".
func IsSessionTokenParam(f *ast.Field) bool {
	return len(f.Names) > 0 &&
		f.Names[0].Name == "sessionToken"
}

// IsSessionParam reports whether the AST field is named
// "session".
func IsSessionParam(f *ast.Field) bool {
	return len(f.Names) > 0 &&
		f.Names[0].Name == "session"
}

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

	seen := make(map[string]bool, st.NumFields())
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
			return &ErrorPathFieldMissingTag{
				FieldName: field.Name(), Recv: recv, Method: method,
			}
		}
		tagVal := structtag.PathTagValue(tag)
		if tagVal == "" {
			return &ErrorPathFieldEmptyTag{
				FieldName: field.Name(), Recv: recv, Method: method,
			}
		} else if seen[tagVal] {
			return &ErrorPathFieldDuplicateTag{
				FieldName: field.Name(), TagValue: tagVal,
				Recv: recv, Method: method,
			}
		}
		seen[tagVal] = true
	}
	return nil
}

// IsQueryParam reports whether the AST field is named
// "query".
func IsQueryParam(f *ast.Field) bool {
	return len(f.Names) > 0 && f.Names[0].Name == "query"
}

// ValidateQueryStruct validates that a query parameter is a
// struct with exported fields each carrying a `query:"..."` tag.
func ValidateQueryStruct(
	f *ast.Field, info *types.Info, recv, method string,
) error {
	t := info.TypeOf(f.Type)
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrQueryParamNotStruct, recv, method,
		)
	}

	seen := make(map[string]bool, st.NumFields())
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
			return &ErrorQueryFieldMissingTag{
				FieldName: field.Name(), Recv: recv, Method: method,
			}
		}
		tagVal := structtag.QueryTagValue(tag)
		if tagVal == "" {
			return &ErrorQueryFieldEmptyTag{
				FieldName: field.Name(), Recv: recv, Method: method,
			}
		} else if seen[tagVal] {
			return &ErrorQueryFieldDuplicateTag{
				FieldName: field.Name(), TagValue: tagVal,
				Recv: recv, Method: method,
			}
		}
		seen[tagVal] = true
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
// is a struct with exported fields each carrying a `json:"..."` tag.
func ValidateSignalsStruct(
	f *ast.Field, info *types.Info, recv, method string,
) error {
	t := info.TypeOf(f.Type)
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf(
			"%w in %s.%s",
			ErrSignalsParamNotStruct, recv, method,
		)
	}

	seen := make(map[string]bool, st.NumFields())
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
			return &ErrorSignalsFieldMissingTag{
				FieldName: field.Name(), Recv: recv, Method: method,
			}
		}
		tagVal := structtag.JSONTagValue(tag)
		if tagVal == "" {
			return &ErrorSignalsFieldEmptyTag{
				FieldName: field.Name(), Recv: recv, Method: method,
			}
		} else if seen[tagVal] {
			return &ErrorSignalsFieldDuplicateTag{
				FieldName: field.Name(), TagValue: tagVal,
				Recv: recv, Method: method,
			}
		}
		seen[tagVal] = true
	}
	return nil
}

// ErrorPathFieldMissingTag is ErrPathFieldMissingTag with suggestion context.
type ErrorPathFieldMissingTag struct {
	FieldName string
	Recv      string
	Method    string
}

func (e *ErrorPathFieldMissingTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrPathFieldMissingTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorPathFieldMissingTag) Unwrap() error { return ErrPathFieldMissingTag }

// ErrorPathFieldEmptyTag is ErrPathFieldEmptyTag with suggestion context.
type ErrorPathFieldEmptyTag struct {
	FieldName string
	Recv      string
	Method    string
}

func (e *ErrorPathFieldEmptyTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrPathFieldEmptyTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorPathFieldEmptyTag) Unwrap() error { return ErrPathFieldEmptyTag }

// ErrorPathFieldDuplicateTag is ErrPathFieldDuplicateTag with suggestion context.
type ErrorPathFieldDuplicateTag struct {
	FieldName string
	TagValue  string
	Recv      string
	Method    string
}

func (e *ErrorPathFieldDuplicateTag) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s.%s",
		ErrPathFieldDuplicateTag, e.TagValue, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorPathFieldDuplicateTag) Unwrap() error { return ErrPathFieldDuplicateTag }

// ErrorQueryFieldMissingTag is ErrQueryFieldMissingTag with suggestion context.
type ErrorQueryFieldMissingTag struct {
	FieldName string
	Recv      string
	Method    string
}

func (e *ErrorQueryFieldMissingTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s", ErrQueryFieldMissingTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorQueryFieldMissingTag) Unwrap() error { return ErrQueryFieldMissingTag }

// ErrorQueryFieldEmptyTag is ErrQueryFieldEmptyTag with suggestion context.
type ErrorQueryFieldEmptyTag struct {
	FieldName string
	Recv      string
	Method    string
}

func (e *ErrorQueryFieldEmptyTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s", ErrQueryFieldEmptyTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorQueryFieldEmptyTag) Unwrap() error { return ErrQueryFieldEmptyTag }

// ErrorQueryFieldDuplicateTag is ErrQueryFieldDuplicateTag with suggestion context.
type ErrorQueryFieldDuplicateTag struct {
	FieldName string
	TagValue  string
	Recv      string
	Method    string
}

func (e *ErrorQueryFieldDuplicateTag) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s.%s",
		ErrQueryFieldDuplicateTag, e.TagValue, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorQueryFieldDuplicateTag) Unwrap() error { return ErrQueryFieldDuplicateTag }

// ErrorSignalsFieldMissingTag is ErrSignalsFieldMissingTag with suggestion context.
type ErrorSignalsFieldMissingTag struct {
	FieldName string
	Recv      string
	Method    string
}

func (e *ErrorSignalsFieldMissingTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrSignalsFieldMissingTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorSignalsFieldMissingTag) Unwrap() error { return ErrSignalsFieldMissingTag }

// ErrorSignalsFieldEmptyTag is ErrSignalsFieldEmptyTag with suggestion context.
type ErrorSignalsFieldEmptyTag struct {
	FieldName string
	Recv      string
	Method    string
}

func (e *ErrorSignalsFieldEmptyTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrSignalsFieldEmptyTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorSignalsFieldEmptyTag) Unwrap() error { return ErrSignalsFieldEmptyTag }

// ErrorSignalsFieldDuplicateTag is ErrSignalsFieldDuplicateTag with suggestion context.
type ErrorSignalsFieldDuplicateTag struct {
	FieldName string
	TagValue  string
	Recv      string
	Method    string
}

func (e *ErrorSignalsFieldDuplicateTag) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s.%s",
		ErrSignalsFieldDuplicateTag, e.TagValue, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorSignalsFieldDuplicateTag) Unwrap() error {
	return ErrSignalsFieldDuplicateTag
}

// Dispatch parameter errors.
var (
	ErrDispatchParamNotFunc = errors.New(
		"dispatch parameter must be a function type",
	)
	ErrDispatchMustReturnError error = &ErrorDispatchMustReturnError{}
	ErrDispatchNoParams        error = &ErrorDispatchNoParams{}
	ErrDispatchParamNotEvent   error = &ErrorDispatchParamNotEvent{}
)

// ErrorDispatchMustReturnError is returned when a dispatch function's
// return type is not exactly `error`.
type ErrorDispatchMustReturnError struct {
	Recv       string    // e.g. "PageFoo"
	MethodName string    // e.g. "GET"
	ParamTypes string    // e.g. "EventFoo, EventBar"
	Pos        token.Pos // position of the problematic return type or func keyword
}

func (e *ErrorDispatchMustReturnError) Error() string {
	if e.Recv == "" {
		return "dispatch function must return exactly one value of type error"
	}
	return fmt.Sprintf(
		"dispatch function must return exactly one value of type error in %s.%s",
		e.Recv, e.MethodName,
	)
}

func (e *ErrorDispatchMustReturnError) Is(target error) bool {
	_, ok := target.(*ErrorDispatchMustReturnError)
	return ok
}

func (e *ErrorDispatchMustReturnError) ASTPos() token.Pos { return e.Pos }

// ErrorDispatchParamNotEvent is returned when a dispatch function
// parameter is not an event type.
type ErrorDispatchParamNotEvent struct {
	Recv       string    // e.g. "PageFoo"
	MethodName string    // e.g. "GET"
	Pos        token.Pos // position of the non-event parameter type
}

func (e *ErrorDispatchParamNotEvent) Error() string {
	return fmt.Sprintf(
		"dispatch function parameter must be an event type in %s.%s",
		e.Recv, e.MethodName,
	)
}

func (e *ErrorDispatchParamNotEvent) Is(target error) bool {
	_, ok := target.(*ErrorDispatchParamNotEvent)
	return ok
}

func (e *ErrorDispatchParamNotEvent) ASTPos() token.Pos { return e.Pos }

// ErrorDispatchNoParams is returned when a dispatch function has no parameters.
type ErrorDispatchNoParams struct {
	Recv       string    // e.g. "PageFoo"
	MethodName string    // e.g. "GET"
	Pos        token.Pos // position of the empty param list
}

func (e *ErrorDispatchNoParams) Error() string {
	return fmt.Sprintf(
		"dispatch function must have at least one event parameter in %s.%s",
		e.Recv, e.MethodName,
	)
}

func (e *ErrorDispatchNoParams) Is(target error) bool {
	_, ok := target.(*ErrorDispatchNoParams)
	return ok
}

func (e *ErrorDispatchNoParams) ASTPos() token.Pos { return e.Pos }

// IsDispatchParam reports whether the AST field is named
// "dispatch".
func IsDispatchParam(f *ast.Field) bool {
	return len(f.Names) > 0 &&
		f.Names[0].Name == "dispatch"
}

// funcParamTypes returns a comma-separated list of parameter type
// expressions from a function type AST node (e.g. "EventFoo, EventBar").
func funcParamTypes(ft *ast.FuncType) string {
	if ft.Params == nil {
		return ""
	}
	var parts []string
	for _, p := range ft.Params.List {
		t := types.ExprString(p.Type)
		n := len(p.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, ", ")
}

// ValidateDispatchFunc validates that a dispatch parameter
// is a function type with EventXXX parameters and a single
// error return. Returns the list of event type names.
func ValidateDispatchFunc(
	f *ast.Field,
	info *types.Info,
	eventTypeNames map[string]struct{},
	recv, method string,
) ([]string, error) {
	ft, ok := f.Type.(*ast.FuncType)
	if !ok {
		return nil, fmt.Errorf(
			"%w in %s.%s",
			ErrDispatchParamNotFunc, recv, method,
		)
	}

	var errs []error

	// Validate return: exactly one value of type error.
	retOK := true
	if ft.Results == nil || len(ft.Results.List) == 0 {
		retOK = false
	} else {
		retCount := 0
		for _, r := range ft.Results.List {
			n := len(r.Names)
			if n == 0 {
				n = 1
			}
			retCount += n
		}
		if retCount != 1 {
			retOK = false
		} else if retType := info.TypeOf(ft.Results.List[0].Type); !typecheck.IsError(retType) {
			retOK = false
		}
	}
	if !retOK {
		retPos := ft.Pos() // fallback: func keyword
		if ft.Results != nil && len(ft.Results.List) > 0 {
			retPos = ft.Results.List[0].Type.Pos()
		}
		errs = append(errs, &ErrorDispatchMustReturnError{
			Recv:       recv,
			MethodName: method,
			ParamTypes: funcParamTypes(ft),
			Pos:        retPos,
		})
	}

	// Validate parameters: at least one, all EventXXX.
	var eventNames []string
	if ft.Params == nil || len(ft.Params.List) == 0 {
		errs = append(errs, &ErrorDispatchNoParams{
			Recv:       recv,
			MethodName: method,
			Pos:        ft.Pos(),
		})
	} else {
		for _, p := range ft.Params.List {
			name, ok := typecheck.EventTypeNameOf(
				p.Type, info, eventTypeNames,
			)
			if !ok {
				errs = append(errs, &ErrorDispatchParamNotEvent{
					Recv:       recv,
					MethodName: method,
					Pos:        p.Type.Pos(),
				})
				break
			}
			// Account for grouped names (a, b EventFoo).
			n := len(p.Names)
			if n == 0 {
				n = 1
			}
			for range n {
				eventNames = append(eventNames, name)
			}
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return eventNames, nil
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
		var errs []error
		for v := range varSet {
			errs = append(errs, fmt.Errorf(
				"%w: {%s} in %s.%s",
				ErrPathMissingRouteVar, v, recv, method,
			))
		}
		return errors.Join(errs...)
	}

	st, ok := h.InputPath.Type.Resolved.Underlying().(*types.Struct)
	if !ok {
		return nil
	}

	var errs []error
	for i := range st.NumFields() {
		tagVal := structtag.PathTagValue(st.Tag(i))
		if tagVal == "" {
			continue
		}
		if !varSet[tagVal] {
			errs = append(errs, fmt.Errorf(
				"%w: %q in %s.%s",
				ErrPathFieldNotInRoute,
				tagVal, recv, method,
			))
		} else {
			delete(varSet, tagVal)
		}
	}
	for v := range varSet {
		errs = append(errs, fmt.Errorf(
			"%w: {%s} in %s.%s",
			ErrPathMissingRouteVar, v, recv, method,
		))
	}
	return errors.Join(errs...)
}
