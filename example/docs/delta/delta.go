package delta

import (
	"errors"
	"math"
	"slices"
)

// ErrSurrogateSplit reports a position inside a UTF-16 surrogate pair.
// Valid in JavaScript, not in a Go string.
var ErrSurrogateSplit = errors.New("position splits a surrogate pair")

// Delta is a list of operations. A document is a Delta of inserts only.
// A change is a Delta that retains, deletes and inserts.
type Delta struct {
	Ops []Op `json:"ops"`
}

// Op is one operation of a [Delta].
// Exactly one of Insert, Embed, Retain and Delete is nonzero.
type Op struct {
	Insert string
	Embed  Embed // Inserted object of length 1, such as an image.
	Retain int   // UTF-16 code units to keep.
	Delete int   // UTF-16 code units to remove.
	// Attributes are the formats of inserted text or the format changes of
	// retained text. A nil value removes a format. Values are string, bool,
	// float64 or nil. Operations share maps. Never modify one.
	Attributes Attributes
}

// Embed is an object Quill inserts as one unit, {"Type":"Value"} in JSON,
// such as {"image":"/images/a.png"}. The zero Embed is none.
type Embed struct{ Type, Value string }

// Attributes maps format names to values.
type Attributes map[string]any

type kind uint8

const (
	kindInsert kind = iota
	kindRetain
	kindDelete
)

// infinity is the length of an exhausted iterator, Infinity in quill-delta.
const infinity = math.MaxInt

func (op Op) kind() kind {
	switch {
	case op.Delete > 0:
		return kindDelete
	case op.Retain > 0:
		return kindRetain
	}
	return kindInsert
}

// IsInsert reports whether op inserts text or an embed.
func (op Op) IsInsert() bool { return op.kind() == kindInsert }

// IsEmbed reports whether op inserts an embed.
func (op Op) IsEmbed() bool { return op.Embed != (Embed{}) }

// IsRetain reports whether op keeps text.
func (op Op) IsRetain() bool { return op.kind() == kindRetain }

// IsDelete reports whether op removes text.
func (op Op) IsDelete() bool { return op.kind() == kindDelete }

// Len is the number of UTF-16 code units op inserts, keeps or removes.
func (op Op) Len() int {
	switch op.kind() {
	case kindDelete:
		return op.Delete
	case kindRetain:
		return op.Retain
	}
	if op.IsEmbed() {
		return 1
	}
	return utf16Len(op.Insert)
}

func (op Op) equal(o Op) bool {
	return op.Insert == o.Insert && op.Embed == o.Embed && op.Retain == o.Retain &&
		op.Delete == o.Delete && op.Attributes.Equal(o.Attributes)
}

// Insert appends an insert of text with attrs. No-op for empty text.
func (d *Delta) Insert(text string, attrs Attributes) *Delta {
	if text == "" {
		return d
	}
	return d.Push(Op{Insert: text, Attributes: attrs.orNil()})
}

// InsertEmbed appends an insert of e with attrs.
func (d *Delta) InsertEmbed(e Embed, attrs Attributes) *Delta {
	return d.Push(Op{Embed: e, Attributes: attrs.orNil()})
}

// Retain appends a retain of n code units with attrs. No-op for n <= 0.
func (d *Delta) Retain(n int, attrs Attributes) *Delta {
	if n <= 0 {
		return d
	}
	return d.Push(Op{Retain: n, Attributes: attrs.orNil()})
}

// Push appends op, merged into the last operation if both have the same kind
// and attributes. Embeds never merge. Like quill-delta, it puts an insert
// before a trailing delete. Both orders give the same document.
func (d *Delta) Push(op Op) *Delta {
	index := len(d.Ops)
	if index > 0 {
		last := d.Ops[index-1]
		if op.kind() == kindDelete && last.kind() == kindDelete {
			d.Ops[index-1] = Op{Delete: last.Delete + op.Delete}
			return d
		}
		if last.kind() == kindDelete && op.kind() == kindInsert {
			index--
			if index == 0 {
				d.Ops = slices.Insert(d.Ops, 0, op)
				return d
			}
			last = d.Ops[index-1]
		}
		if op.Attributes.Equal(last.Attributes) {
			switch {
			case op.kind() == kindInsert && last.kind() == kindInsert &&
				!op.IsEmbed() && !last.IsEmbed():
				d.Ops[index-1] = Op{
					Insert: last.Insert + op.Insert, Attributes: op.Attributes,
				}
				return d
			case op.kind() == kindRetain && last.kind() == kindRetain:
				d.Ops[index-1] = Op{
					Retain: last.Retain + op.Retain, Attributes: op.Attributes,
				}
				return d
			}
		}
	}
	d.Ops = slices.Insert(d.Ops, index, op)
	return d
}

// chop removes a trailing retain without attributes, a no-op.
func (d *Delta) chop() {
	if n := len(d.Ops); n > 0 {
		last := d.Ops[n-1]
		if last.kind() == kindRetain && len(last.Attributes) == 0 {
			d.Ops = d.Ops[:n-1]
		}
	}
	if len(d.Ops) == 0 {
		d.Ops = nil
	}
}

// concat returns a copy of d with other appended.
// Only the first operation of other can merge.
func (d Delta) concat(other Delta) Delta {
	result := Delta{Ops: slices.Clone(d.Ops)}
	if len(other.Ops) > 0 {
		result.Push(other.Ops[0])
		result.Ops = append(result.Ops, other.Ops[1:]...)
	}
	return result
}

// Length returns the number of code units d inserts, keeps and removes,
// the text length for a document.
func (d Delta) Length() int {
	n := 0
	for _, op := range d.Ops {
		n += op.Len()
	}
	return n
}

// BaseLength returns the number of code units d keeps and removes:
// the length of the shortest document d applies to.
func (d Delta) BaseLength() int {
	n := 0
	for _, op := range d.Ops {
		if op.kind() != kindInsert {
			n += op.Len()
		}
	}
	return n
}

// Compose returns the change that applies d, then other. For a document d,
// that is the document after other. It returns [ErrSurrogateSplit] if a
// retain or delete of other ends inside a surrogate pair of d.
func (d Delta) Compose(other Delta) (Delta, error) {
	thisIter := newIterator(d.Ops)
	otherIter := newIterator(other.Ops)
	var ops []Op
	if len(other.Ops) > 0 {
		first := other.Ops[0]
		if first.kind() == kindRetain && first.Attributes == nil {
			// Copy the inserts of d that the leading retain keeps whole.
			firstLeft := first.Retain
			for thisIter.peekType() == kindInsert && thisIter.peekLength() <= firstLeft {
				firstLeft -= thisIter.peekLength()
				ops = append(ops, thisIter.next(0))
			}
			if first.Retain-firstLeft > 0 {
				otherIter.next(first.Retain - firstLeft)
			}
		}
	}
	result := Delta{Ops: ops}
	for thisIter.hasNext() || otherIter.hasNext() {
		switch {
		case otherIter.peekType() == kindInsert:
			result.Push(otherIter.next(0))
		case thisIter.peekType() == kindDelete:
			result.Push(thisIter.next(0))
		default:
			length := min(thisIter.peekLength(), otherIter.peekLength())
			thisOp := thisIter.next(length)
			otherOp := otherIter.next(length)
			switch {
			case otherOp.kind() == kindRetain:
				newOp := Op{Insert: thisOp.Insert, Embed: thisOp.Embed}
				if thisOp.kind() == kindRetain {
					newOp = Op{Retain: length}
				}
				// A retain keeps format removals. An insert has none to keep.
				newOp.Attributes = composeAttributes(
					thisOp.Attributes, otherOp.Attributes,
					thisOp.kind() == kindRetain,
				)
				result.Push(newOp)
				if !otherIter.hasNext() && result.Ops[len(result.Ops)-1].equal(newOp) {
					// The rest of d is unchanged.
					result = result.concat(Delta{Ops: thisIter.rest()})
					result.chop()
					return result, errors.Join(thisIter.err, otherIter.err)
				}
			case otherOp.kind() == kindDelete && thisOp.kind() == kindRetain:
				result.Push(otherOp)
				// Deleting text that d inserts cancels both.
			}
		}
	}
	result.chop()
	return result, errors.Join(thisIter.err, otherIter.err)
}

// Transform returns other, a change concurrent with d, rewritten to apply after d.
// For inserts at one position, priority puts the insert of d first.
func (d Delta) Transform(other Delta, priority bool) Delta {
	thisIter := newIterator(d.Ops)
	otherIter := newIterator(other.Ops)
	var result Delta
	for thisIter.hasNext() || otherIter.hasNext() {
		switch {
		case thisIter.peekType() == kindInsert &&
			(priority || otherIter.peekType() != kindInsert):
			n := thisIter.peekLength()
			thisIter.next(0)
			result.Retain(n, nil)
		case otherIter.peekType() == kindInsert:
			result.Push(otherIter.next(0))
		default:
			length := min(thisIter.peekLength(), otherIter.peekLength())
			thisOp := thisIter.next(length)
			otherOp := otherIter.next(length)
			switch {
			case thisOp.kind() == kindDelete:
				// d deleted the text: nothing left to delete or format.
			case otherOp.kind() == kindDelete:
				result.Push(otherOp)
			default:
				result.Retain(length, transformAttributes(
					thisOp.Attributes, otherOp.Attributes, priority,
				))
			}
		}
	}
	result.chop()
	return result
}

// TransformPosition returns index moved by d.
// An insert at index moves it, unless priority is set.
func (d Delta) TransformPosition(index int, priority bool) int {
	it := newIterator(d.Ops)
	offset := 0
	for it.hasNext() && offset <= index {
		length := it.peekLength()
		k := it.peekType()
		it.next(0)
		switch {
		case k == kindDelete:
			index -= min(length, index-offset)
			continue
		case k == kindInsert && (offset < index || !priority):
			index += length
		}
		offset += length
	}
	return index
}
