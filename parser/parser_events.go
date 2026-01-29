package parser

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
)

func (p *Parser) validateEvents(ctx *parseCtx, errs *Errors) {
	for name := range ctx.eventTypeNames {
		ts := ctx.typeSpecByName[name]
		p.validateEventType(ctx, errs, ts.Pos(), name, ctx.pkg.TypesInfo.TypeOf(ts.Type), map[types.Type]bool{})
	}
}

func (p *Parser) validateEventType(ctx *parseCtx, errs *Errors, pos token.Pos, name string, t types.Type, visited map[types.Type]bool) {
	if t == nil {
		return
	}
	if visited[t] {
		return
	}
	visited[t] = true

	// If pointer, unwrap
	if ptr, ok := t.(*types.Pointer); ok {
		p.validateEventType(ctx, errs, pos, name, ptr.Elem(), visited)
		return
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

	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := st.Tag(i)

		// 1. Must be exported
		if !f.Exported() {
			errs.ErrAt(ctx.pkg.Fset.Position(pos), fmt.Errorf("%w: field %s in %s", ErrEventFieldUnexported, f.Name(), name))
		}

		// 2. Must have json tag
		// Minimal check: verify `json:"..."` exists.
		if !strings.Contains(tag, "json:\"") {
			errs.ErrAt(ctx.pkg.Fset.Position(pos), fmt.Errorf("%w: field %s in %s", ErrEventFieldMissingTag, f.Name(), name))
		}

		// Recurse
		p.validateEventType(ctx, errs, pos, name, f.Type(), visited)
	}
}
