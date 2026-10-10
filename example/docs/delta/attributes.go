package delta

import "maps"

// Equal reports whether a and b hold the same formats.
// A nil map equals an empty one.
func (a Attributes) Equal(b Attributes) bool { return maps.Equal(a, b) }

// orNil returns nil for an empty map, as quill-delta omits empty attributes.
func (a Attributes) orNil() Attributes {
	if len(a) == 0 {
		return nil
	}
	return a
}

// composeAttributes returns the formats of text formatted with a, then b.
// keepNull keeps the nils of b, which a change needs and a document doesn't.
// Returns a or b itself if the other adds nothing.
func composeAttributes(a, b Attributes, keepNull bool) Attributes {
	if len(b) == 0 {
		return a.orNil()
	}
	if len(a) == 0 && (keepNull || !hasNil(b)) {
		return b
	}
	var out Attributes
	set := func(k string, v any) {
		if out == nil {
			out = make(Attributes, len(a)+len(b))
		}
		out[k] = v
	}
	for k, v := range b {
		if v != nil || keepNull {
			set(k, v)
		}
	}
	for k, v := range a {
		if _, ok := b[k]; !ok {
			set(k, v)
		}
	}
	return out
}

func hasNil(a Attributes) bool {
	for _, v := range a {
		if v == nil {
			return true
		}
	}
	return false
}

// transformAttributes returns format changes b, concurrent with a,
// rewritten to apply after a. With priority, it drops the formats a also sets.
func transformAttributes(a, b Attributes, priority bool) Attributes {
	if a == nil {
		return b
	}
	if b == nil {
		return nil
	}
	if !priority {
		return b
	}
	var out Attributes
	for k, v := range b {
		if _, ok := a[k]; ok {
			continue
		}
		if out == nil {
			out = make(Attributes, len(b))
		}
		out[k] = v
	}
	return out
}
