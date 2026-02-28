package parser

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"github.com/romshark/datapages/parser/internal/structtag"
)

// _jsonUnmarshalerIface and _textUnmarshalerIface are synthetic interface types
// used to check whether a type implements json.Unmarshaler or
// encoding.TextUnmarshaler via types.Implements.
var _jsonUnmarshalerIface, _textUnmarshalerIface *types.Interface

func init() {
	byteSlice := types.NewSlice(types.Typ[types.Byte])
	errType := types.Universe.Lookup("error").Type()
	makeIface := func(methodName string) *types.Interface {
		sig := types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, nil, "", byteSlice)),
			types.NewTuple(types.NewVar(token.NoPos, nil, "", errType)),
			false,
		)
		iface := types.NewInterfaceType(
			[]*types.Func{types.NewFunc(token.NoPos, nil, methodName, sig)}, nil,
		)
		iface.Complete()
		return iface
	}
	_jsonUnmarshalerIface = makeIface("UnmarshalJSON")
	_textUnmarshalerIface = makeIface("UnmarshalText")
}

// implementsUnmarshaler reports whether t (or *t) implements json.Unmarshaler
// or encoding.TextUnmarshaler.
func implementsUnmarshaler(t *types.Named) bool {
	for _, typ := range []types.Type{t, types.NewPointer(t)} {
		if types.Implements(typ, _jsonUnmarshalerIface) ||
			types.Implements(typ, _textUnmarshalerIface) {
			return true
		}
	}
	return false
}

func validateEvents(ctx *parseCtx, errs *Errors) {
	for name := range ctx.eventTypeNames {
		ts := ctx.typeSpecByName[name]
		validateEventType(
			ctx, errs, ts.Name.Pos(), name,
			ctx.pkg.TypesInfo.TypeOf(ts.Type), map[types.Type]bool{},
		)

	}
}

func validateEventType(
	ctx *parseCtx, errs *Errors, pos token.Pos, name string,
	t types.Type, visited map[types.Type]bool,
) {
	if t == nil {
		return
	}
	if visited[t] {
		return
	}
	visited[t] = true

	// If pointer, unwrap
	if ptr, ok := t.(*types.Pointer); ok {
		validateEventType(ctx, errs, pos, name, ptr.Elem(), visited)
		return
	}

	// Don't recurse into named types that implement json.Unmarshaler or
	// encoding.TextUnmarshaler — they handle their own JSON encoding
	// (e.g. time.Time). Check both value and pointer receiver method sets.
	if named, ok := t.(*types.Named); ok {
		if implementsUnmarshaler(named) {
			return
		}
	}

	// We only care about structs.
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		// Named types wrapping basics? strictly speaking events are structs in this framework.
		// But if it's a field deep down, it might be int/string.
		// The top level event IS a struct per firstPassTypes logic (maybe? let's check).
		// For now we only deeply validate structs.
		return
	}

	seenTags := make(map[string]bool, st.NumFields())
	for i := range st.NumFields() {
		f := st.Field(i)
		tag := st.Tag(i)

		// Fields marked json:"-" are intentionally excluded from JSON; skip all checks.
		if structtag.JSONTagExcluded(tag) {
			continue
		}

		// 1. Must be exported
		if !f.Exported() {
			errs.ErrAt(ctx.pkg.Fset.Position(pos),
				fmt.Errorf("%w: field %s in %s", ErrEventFieldUnexported, f.Name(), name))
		}

		// 2. Must have json tag
		// Minimal check: verify `json:"..."` exists.
		if !strings.Contains(tag, "json:\"") {
			errs.ErrAt(ctx.pkg.Fset.Position(pos),
				&ErrorEventFieldMissingTag{FieldName: f.Name(), TypeName: name})
		} else {
			// 3. Must not duplicate a json tag value already seen at this level.
			tagVal := structtag.JSONTagValue(tag)
			if seenTags[tagVal] {
				errs.ErrAt(ctx.pkg.Fset.Position(pos),
					&ErrorEventFieldDuplicateTag{
						FieldName: f.Name(), TagValue: tagVal, TypeName: name,
					})
			} else {
				seenTags[tagVal] = true
			}
		}

		// Recurse
		validateEventType(ctx, errs, pos, name, f.Type(), visited)
	}
}
