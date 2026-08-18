// Package gotypes provides predicates and renderings for plain Go types.
// Nothing here knows about Datapages.
// Both the parser and the generator inspect types with it.
package gotypes

import "go/types"

// IsString reports whether the underlying type of t is string.
func IsString(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// IsNamedString reports whether t is a string type carrying a name of its own.
// A string assigned to one, or one assigned to a string, needs a conversion.
func IsNamedString(t types.Type) bool {
	if !IsString(t) {
		return false
	}
	_, unnamed := t.(*types.Basic)
	return !unnamed
}

// IsBool reports whether the underlying type of t is bool.
func IsBool(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Bool
}

// IsUint64 reports whether the underlying type of t is uint64.
func IsUint64(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint64
}

// IsInt reports whether the underlying type of t is a signed or unsigned integer
// of any width. Untyped constants and uintptr are not integers here.
func IsInt(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch b.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
		return true
	}
	return false
}

// IsFloat reports whether the underlying type of t is float32 or float64.
func IsFloat(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	return b.Kind() == types.Float32 || b.Kind() == types.Float64
}

// IntTypeName returns the Go identifier for an integer type (e.g. "int", "uint32").
// Precondition: IsInt(t) must be true.
func IntTypeName(t types.Type) string {
	switch t.Underlying().(*types.Basic).Kind() {
	case types.Int:
		return "int"
	case types.Int8:
		return "int8"
	case types.Int16:
		return "int16"
	case types.Int32:
		return "int32"
	case types.Int64:
		return "int64"
	case types.Uint:
		return "uint"
	case types.Uint8:
		return "uint8"
	case types.Uint16:
		return "uint16"
	case types.Uint32:
		return "uint32"
	default: // Uint64
		return "uint64"
	}
}

// IntParseInfo returns the strconv bit-size argument and whether the type is unsigned,
// for use with strconv.ParseInt / strconv.ParseUint.
// Precondition: IsInt(t) must be true.
func IntParseInfo(t types.Type) (bits int, unsigned bool) {
	switch t.Underlying().(*types.Basic).Kind() {
	case types.Int:
		return 0, false
	case types.Int8:
		return 8, false
	case types.Int16:
		return 16, false
	case types.Int32:
		return 32, false
	case types.Int64:
		return 64, false
	case types.Uint:
		return 0, true
	case types.Uint8:
		return 8, true
	case types.Uint16:
		return 16, true
	case types.Uint32:
		return 32, true
	default: // Uint64
		return 64, true
	}
}

// FloatTypeName returns "float32" or "float64".
// Precondition: IsFloat(t) must be true.
func FloatTypeName(t types.Type) string {
	if t.Underlying().(*types.Basic).Kind() == types.Float32 {
		return "float32"
	}
	return "float64"
}

// FloatBits returns the strconv bit-size for ParseFloat.
// Precondition: IsFloat(t) must be true.
func FloatBits(t types.Type) int {
	if t.Underlying().(*types.Basic).Kind() == types.Float32 {
		return 32
	}
	return 64
}

// QualifiedTypeName renders a type with every package named.
func QualifiedTypeName(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string { return p.Name() })
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

// ImplementsTextUnmarshaler reports whether t or *t implements encoding.TextUnmarshaler.
func ImplementsTextUnmarshaler(t types.Type) bool {
	if t == nil {
		return false
	}
	if types.Implements(t, textUnmarshaler) {
		return true
	}
	return types.Implements(types.NewPointer(t), textUnmarshaler)
}
