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

	"github.com/romshark/datapages/internal/parser/internal/typecheck"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/routepattern"
	"github.com/romshark/datapages/internal/structtag"
)

// Path parameter errors.
var (
	ErrPathParamNotStruct = errors.New(
		"path parameter must be a struct",
	)
	ErrPathFieldUnexported = errors.New(
		"path struct field must be exported",
	)
	ErrPathFieldMissingTag = errors.New(
		`path struct field must have a path:"..." tag`,
	)
	ErrPathFieldUnsupportedType = errors.New(
		"path struct field has unsupported type",
	)
	ErrPathFieldNotInRoute = errors.New(
		"path struct field tag does not match any route variable",
	)
	ErrPathMissingRouteVar = errors.New(
		"route variable has no matching path struct field",
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
	ErrQueryFieldUnsupportedType = errors.New(
		"query struct field has unsupported type",
	)
	ErrQueryReflectSignalNotInSignals = errors.New(
		"query reflectsignal tag references signal not in signals parameter",
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

// fieldPosError wraps an error with the AST position of a struct field.
type fieldPosError struct {
	pos token.Pos
	err error
}

func (e *fieldPosError) Error() string     { return e.err.Error() }
func (e *fieldPosError) Unwrap() error     { return e.err }
func (e *fieldPosError) ASTPos() token.Pos { return e.pos }

// IsSessionParam reports whether the AST field is typed datapages.Session[Data].
func IsSessionParam(f *ast.Field, info *types.Info) bool {
	return typecheck.IsSessionType(f.Type, info)
}

// IsStateParam reports whether the AST field is typed datapages.State[Values].
// The type is what makes it a state parameter. The name may be anything.
func IsStateParam(f *ast.Field, info *types.Info) bool {
	_, ok := typecheck.StateValuesType(f.Type, info)
	return ok
}

// IsStateIDParam reports whether the AST field is named "stateID".
func IsStateIDParam(f *ast.Field) bool {
	return len(f.Names) > 0 && f.Names[0].Name == "stateID"
}

// StateParamElementName returns the state type name referenced by a
// datapages.State[T] parameter. Returns "" when the type argument is not a
// plain identifier, which is what a pointer, an anonymous struct or a
// qualified type gives.
func StateParamElementName(f *ast.Field) string {
	id, ok := typecheck.TypeArgExpr(f.Type).(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

// IsPathParam reports whether the AST field is typed datapages.Path[Values].
func IsPathParam(f *ast.Field, info *types.Info) bool {
	_, ok := typecheck.PathValuesType(f.Type, info)
	return ok
}

// ValidatePathStruct validates that the Values type argument of a datapages.Path
// parameter is a struct with exported fields of supported types (string, bool, integers,
// floats, or encoding.TextUnmarshaler) each carrying a `path:"..."` tag.
func ValidatePathStruct(
	values ast.Expr, info *types.Info, recv, method string,
) error {
	t := info.TypeOf(values)
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
		fpos := field.Pos()

		if !field.Exported() {
			return &fieldPosError{pos: fpos, err: fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrPathFieldUnexported,
				field.Name(), recv, method,
			)}
		}
		if !typecheck.IsInputFieldType(field.Type()) {
			return &fieldPosError{pos: fpos, err: fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrPathFieldUnsupportedType,
				field.Name(), recv, method,
			)}
		}
		if !strings.Contains(tag, `path:"`) {
			return &ErrorPathFieldMissingTag{
				FieldName: field.Name(), Recv: recv, Method: method,
				Pos: fpos,
			}
		}
		tagVal := structtag.PathTagValue(tag)
		if tagVal == "" {
			return &ErrorPathFieldEmptyTag{
				FieldName: field.Name(), Recv: recv, Method: method,
				Pos: fpos,
			}
		} else if seen[tagVal] {
			return &ErrorPathFieldDuplicateTag{
				FieldName: field.Name(), TagValue: tagVal,
				Recv: recv, Method: method,
				Pos: fpos,
			}
		}
		seen[tagVal] = true
	}
	return nil
}

// IsQueryParam reports whether the AST field is typed datapages.Query[Values].
func IsQueryParam(f *ast.Field, info *types.Info) bool {
	_, ok := typecheck.QueryValuesType(f.Type, info)
	return ok
}

// ValidateQueryStruct validates that the Values type argument of a
// datapages.Query parameter is a struct with exported fields each
// carrying a `query:"..."` tag.
func ValidateQueryStruct(
	values ast.Expr, info *types.Info, recv, method string,
) error {
	t := info.TypeOf(values)
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
		fpos := field.Pos()

		if !field.Exported() {
			return &fieldPosError{pos: fpos, err: fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrQueryFieldUnexported,
				field.Name(), recv, method,
			)}
		}
		if !typecheck.IsInputFieldType(field.Type()) {
			return &fieldPosError{pos: fpos, err: fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrQueryFieldUnsupportedType,
				field.Name(), recv, method,
			)}
		}
		if !strings.Contains(tag, `query:"`) {
			return &ErrorQueryFieldMissingTag{
				FieldName: field.Name(), Recv: recv, Method: method,
				Pos: fpos,
			}
		}
		tagVal := structtag.QueryTagValue(tag)
		if tagVal == "" {
			return &ErrorQueryFieldEmptyTag{
				FieldName: field.Name(), Recv: recv, Method: method,
				Pos: fpos,
			}
		} else if seen[tagVal] {
			return &ErrorQueryFieldDuplicateTag{
				FieldName: field.Name(), TagValue: tagVal,
				Recv: recv, Method: method,
				Pos: fpos,
			}
		}
		seen[tagVal] = true
	}
	return nil
}

// IsSignalsParam reports whether the AST field is typed datapages.Signals[Values].
func IsSignalsParam(f *ast.Field, info *types.Info) bool {
	_, ok := typecheck.SignalsValuesType(f.Type, info)
	return ok
}

// ValidateSignalsStruct validates that the Values type argument of a
// datapages.Signals parameter is a struct with exported fields each
// carrying a `json:"..."` tag.
func ValidateSignalsStruct(
	values ast.Expr, info *types.Info, recv, method string,
) error {
	t := info.TypeOf(values)
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
		fpos := field.Pos()

		if !field.Exported() {
			return &fieldPosError{pos: fpos, err: fmt.Errorf(
				"%w: field %s in %s.%s",
				ErrSignalsFieldUnexported,
				field.Name(), recv, method,
			)}
		}
		if !strings.Contains(tag, `json:"`) {
			return &ErrorSignalsFieldMissingTag{
				FieldName: field.Name(), Recv: recv, Method: method,
				Pos: fpos,
			}
		}
		tagVal := structtag.JSONTagValue(tag)
		if tagVal == "" {
			return &ErrorSignalsFieldEmptyTag{
				FieldName: field.Name(), Recv: recv, Method: method,
				Pos: fpos,
			}
		} else if seen[tagVal] {
			return &ErrorSignalsFieldDuplicateTag{
				FieldName: field.Name(), TagValue: tagVal,
				Recv: recv, Method: method,
				Pos: fpos,
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
	Pos       token.Pos
}

func (e *ErrorPathFieldMissingTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrPathFieldMissingTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorPathFieldMissingTag) Unwrap() error     { return ErrPathFieldMissingTag }
func (e *ErrorPathFieldMissingTag) ASTPos() token.Pos { return e.Pos }

// ErrorPathFieldEmptyTag is ErrPathFieldEmptyTag with suggestion context.
type ErrorPathFieldEmptyTag struct {
	FieldName string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorPathFieldEmptyTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrPathFieldEmptyTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorPathFieldEmptyTag) Unwrap() error     { return ErrPathFieldEmptyTag }
func (e *ErrorPathFieldEmptyTag) ASTPos() token.Pos { return e.Pos }

// ErrorPathFieldDuplicateTag is ErrPathFieldDuplicateTag with suggestion context.
type ErrorPathFieldDuplicateTag struct {
	FieldName string
	TagValue  string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorPathFieldDuplicateTag) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s.%s",
		ErrPathFieldDuplicateTag, e.TagValue, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorPathFieldDuplicateTag) Unwrap() error     { return ErrPathFieldDuplicateTag }
func (e *ErrorPathFieldDuplicateTag) ASTPos() token.Pos { return e.Pos }

// ErrorQueryFieldMissingTag is ErrQueryFieldMissingTag with suggestion context.
type ErrorQueryFieldMissingTag struct {
	FieldName string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorQueryFieldMissingTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s", ErrQueryFieldMissingTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorQueryFieldMissingTag) Unwrap() error     { return ErrQueryFieldMissingTag }
func (e *ErrorQueryFieldMissingTag) ASTPos() token.Pos { return e.Pos }

// ErrorQueryFieldEmptyTag is ErrQueryFieldEmptyTag with suggestion context.
type ErrorQueryFieldEmptyTag struct {
	FieldName string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorQueryFieldEmptyTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s", ErrQueryFieldEmptyTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorQueryFieldEmptyTag) Unwrap() error     { return ErrQueryFieldEmptyTag }
func (e *ErrorQueryFieldEmptyTag) ASTPos() token.Pos { return e.Pos }

// ErrorQueryFieldDuplicateTag is ErrQueryFieldDuplicateTag with suggestion context.
type ErrorQueryFieldDuplicateTag struct {
	FieldName string
	TagValue  string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorQueryFieldDuplicateTag) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s.%s",
		ErrQueryFieldDuplicateTag, e.TagValue, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorQueryFieldDuplicateTag) Unwrap() error     { return ErrQueryFieldDuplicateTag }
func (e *ErrorQueryFieldDuplicateTag) ASTPos() token.Pos { return e.Pos }

// ErrorSignalsFieldMissingTag is ErrSignalsFieldMissingTag with suggestion context.
type ErrorSignalsFieldMissingTag struct {
	FieldName string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorSignalsFieldMissingTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrSignalsFieldMissingTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorSignalsFieldMissingTag) Unwrap() error     { return ErrSignalsFieldMissingTag }
func (e *ErrorSignalsFieldMissingTag) ASTPos() token.Pos { return e.Pos }

// ErrorSignalsFieldEmptyTag is ErrSignalsFieldEmptyTag with suggestion context.
type ErrorSignalsFieldEmptyTag struct {
	FieldName string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorSignalsFieldEmptyTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s.%s",
		ErrSignalsFieldEmptyTag, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorSignalsFieldEmptyTag) Unwrap() error     { return ErrSignalsFieldEmptyTag }
func (e *ErrorSignalsFieldEmptyTag) ASTPos() token.Pos { return e.Pos }

// ErrorSignalsFieldDuplicateTag is ErrSignalsFieldDuplicateTag with suggestion context.
type ErrorSignalsFieldDuplicateTag struct {
	FieldName string
	TagValue  string
	Recv      string
	Method    string
	Pos       token.Pos
}

func (e *ErrorSignalsFieldDuplicateTag) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s.%s",
		ErrSignalsFieldDuplicateTag, e.TagValue, e.FieldName, e.Recv, e.Method)
}

func (e *ErrorSignalsFieldDuplicateTag) Unwrap() error     { return ErrSignalsFieldDuplicateTag }
func (e *ErrorSignalsFieldDuplicateTag) ASTPos() token.Pos { return e.Pos }

// ErrDispatchParamNotEvent is reported when the type argument of
// datapages.Dispatcher is not an event type.
var ErrDispatchParamNotEvent error = &ErrorDispatchParamNotEvent{}

// ErrorDispatchParamNotEvent is returned when the type argument of
// datapages.Dispatcher is not an event type.
type ErrorDispatchParamNotEvent struct {
	Recv       string    // e.g. "PageFoo"
	MethodName string    // e.g. "GET"
	TypeName   string    // e.g. "string"
	Pos        token.Pos // position of the type argument
}

func (e *ErrorDispatchParamNotEvent) Error() string {
	if e.TypeName == "" {
		return fmt.Sprintf(
			"datapages.Dispatcher type argument must be an event type in %s.%s",
			e.Recv, e.MethodName,
		)
	}
	return fmt.Sprintf(
		"datapages.Dispatcher type argument must be an event type in %s.%s: %s",
		e.Recv, e.MethodName, e.TypeName,
	)
}

func (e *ErrorDispatchParamNotEvent) Is(target error) bool {
	_, ok := target.(*ErrorDispatchParamNotEvent)
	return ok
}

func (e *ErrorDispatchParamNotEvent) ASTPos() token.Pos { return e.Pos }

// IsDispatchParam reports whether the AST field is typed datapages.Dispatcher[EventXXX].
func IsDispatchParam(f *ast.Field, info *types.Info) bool {
	return typecheck.IsDispatchType(f.Type, info)
}

// ValidateDispatch validates that the type argument of a datapages.Dispatcher[EventXXX]
// parameter is a declared event type. Returns the event type name.
func ValidateDispatch(
	f *ast.Field,
	info *types.Info,
	eventTypeNames map[string]struct{},
	recv, method string,
) (string, error) {
	name, ok := typecheck.DispatchEventTypeName(f.Type, info)
	if !ok {
		// The caller checks IsDispatchParam first, hence unreachable.
		return "", &ErrorDispatchParamNotEvent{
			Recv: recv, MethodName: method, Pos: f.Type.Pos(),
		}
	}
	if _, isEvent := eventTypeNames[name]; !isEvent {
		return "", &ErrorDispatchParamNotEvent{
			Recv:       recv,
			MethodName: method,
			TypeName:   name,
			Pos:        f.Type.Pos(),
		}
	}
	return name, nil
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
			errs = append(errs, &fieldPosError{
				pos: st.Field(i).Pos(),
				err: fmt.Errorf(
					"%w: %q in %s.%s",
					ErrPathFieldNotInRoute,
					tagVal, recv, method,
				),
			})
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

// ValidateReflectSignal checks that every reflectsignal tag
// on a query field references a json tag value in the signals struct.
func ValidateReflectSignal(
	h *model.Handler, recv, method string,
) error {
	if h.InputQuery == nil || h.InputSignals == nil {
		return nil
	}

	querySt, ok := h.InputQuery.Type.Resolved.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	sigSt, ok := h.InputSignals.Type.Resolved.Underlying().(*types.Struct)
	if !ok {
		return nil
	}

	sigNames := make(map[string]bool, sigSt.NumFields())
	for i := range sigSt.NumFields() {
		if v := structtag.JSONTagValue(sigSt.Tag(i)); v != "" {
			sigNames[v] = true
		}
	}

	for i := range querySt.NumFields() {
		rs := structtag.ReflectSignalTagValue(querySt.Tag(i))
		if rs == "" {
			continue
		}
		if !sigNames[rs] {
			return fmt.Errorf(
				"%w: %q in %s.%s",
				ErrQueryReflectSignalNotInSignals,
				rs, recv, method,
			)
		}
	}
	return nil
}
