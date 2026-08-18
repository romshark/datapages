package gotypes_test

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/gotypes"
)

// named wraps a basic type in a named type, as an app's own `type UserID string` does.
func named(name string, underlying types.Type) types.Type {
	obj := types.NewTypeName(0, types.NewPackage("example.com/app", "app"), name, nil)
	return types.NewNamed(obj, underlying, nil)
}

func TestPredicates(t *testing.T) {
	t.Parallel()
	for name, td := range map[string]struct {
		typ    types.Type
		string bool
		named  bool
		bool   bool
		uint64 bool
		int    bool
		float  bool
	}{
		"string": {typ: types.Typ[types.String], string: true},
		"named string": {
			typ: named("UserID", types.Typ[types.String]), string: true, named: true,
		},
		"bool":       {typ: types.Typ[types.Bool], bool: true},
		"named bool": {typ: named("Flag", types.Typ[types.Bool]), bool: true},
		"int":        {typ: types.Typ[types.Int], int: true},
		"int8":       {typ: types.Typ[types.Int8], int: true},
		"uint64":     {typ: types.Typ[types.Uint64], uint64: true, int: true},
		"named uint64": {
			typ: named("Seq", types.Typ[types.Uint64]), uint64: true, int: true,
		},
		"float32": {typ: types.Typ[types.Float32], float: true},
		"float64": {typ: types.Typ[types.Float64], float: true},
		"uintptr": {typ: types.Typ[types.Uintptr]},
		"slice":   {typ: types.NewSlice(types.Typ[types.Byte])},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, td.string, gotypes.IsString(td.typ), "IsString")
			require.Equal(t, td.named, gotypes.IsNamedString(td.typ), "IsNamedString")
			require.Equal(t, td.bool, gotypes.IsBool(td.typ), "IsBool")
			require.Equal(t, td.uint64, gotypes.IsUint64(td.typ), "IsUint64")
			require.Equal(t, td.int, gotypes.IsInt(td.typ), "IsInt")
			require.Equal(t, td.float, gotypes.IsFloat(td.typ), "IsFloat")
		})
	}
}

func TestIntTypeName(t *testing.T) {
	t.Parallel()
	for name, td := range map[string]struct {
		typ      types.Type
		expect   string
		bits     int
		unsigned bool
	}{
		"int":   {typ: types.Typ[types.Int], expect: "int", bits: 0},
		"int8":  {typ: types.Typ[types.Int8], expect: "int8", bits: 8},
		"int16": {typ: types.Typ[types.Int16], expect: "int16", bits: 16},
		"int32": {typ: types.Typ[types.Int32], expect: "int32", bits: 32},
		"int64": {typ: types.Typ[types.Int64], expect: "int64", bits: 64},
		"uint":  {typ: types.Typ[types.Uint], expect: "uint", bits: 0, unsigned: true},
		"uint8": {
			typ: types.Typ[types.Uint8], expect: "uint8", bits: 8, unsigned: true,
		},
		"uint16": {
			typ: types.Typ[types.Uint16], expect: "uint16", bits: 16, unsigned: true,
		},
		"uint32": {
			typ: types.Typ[types.Uint32], expect: "uint32", bits: 32, unsigned: true,
		},
		"uint64": {
			typ: types.Typ[types.Uint64], expect: "uint64", bits: 64, unsigned: true,
		},
		"named int32": {
			typ: named("Age", types.Typ[types.Int32]), expect: "int32", bits: 32,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, td.expect, gotypes.IntTypeName(td.typ))
			bits, unsigned := gotypes.IntParseInfo(td.typ)
			require.Equal(t, td.bits, bits)
			require.Equal(t, td.unsigned, unsigned)
		})
	}
}

func TestFloat(t *testing.T) {
	t.Parallel()
	require.Equal(t, "float32", gotypes.FloatTypeName(types.Typ[types.Float32]))
	require.Equal(t, "float64", gotypes.FloatTypeName(types.Typ[types.Float64]))
	require.Equal(t, 32, gotypes.FloatBits(types.Typ[types.Float32]))
	require.Equal(t, 64, gotypes.FloatBits(types.Typ[types.Float64]))
	require.Equal(t, 32, gotypes.FloatBits(named("Ratio", types.Typ[types.Float32])))
}

func TestQualifiedTypeName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "string", gotypes.QualifiedTypeName(types.Typ[types.String]))
	require.Equal(t, "app.UserID",
		gotypes.QualifiedTypeName(named("UserID", types.Typ[types.String])))
}

func TestImplementsTextUnmarshaler(t *testing.T) {
	t.Parallel()

	pkg := types.NewPackage("example.com/app", "app")
	obj := types.NewTypeName(0, pkg, "Date", nil)
	nt := types.NewNamed(obj, types.Typ[types.String], nil)
	sig := types.NewSignatureType(
		types.NewVar(0, pkg, "d", types.NewPointer(nt)), nil, nil,
		types.NewTuple(types.NewVar(
			0, nil, "text", types.NewSlice(types.Typ[types.Byte]),
		)),
		types.NewTuple(types.NewVar(
			0, nil, "", types.Universe.Lookup("error").Type(),
		)),
		false,
	)
	nt.AddMethod(types.NewFunc(0, pkg, "UnmarshalText", sig))

	// The method is on the pointer receiver, which the value type must satisfy too.
	require.True(t, gotypes.ImplementsTextUnmarshaler(nt))
	require.True(t, gotypes.ImplementsTextUnmarshaler(types.NewPointer(nt)))

	require.False(t, gotypes.ImplementsTextUnmarshaler(types.Typ[types.String]))
	require.False(t, gotypes.ImplementsTextUnmarshaler(nil))
}
