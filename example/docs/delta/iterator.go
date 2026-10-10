package delta

// iterator splits the operations of a delta into pieces of any length,
// like OpIterator in quill-delta.
type iterator struct {
	ops   []Op
	index int

	// offset counts the code units of ops[index] already returned,
	// byteOff the bytes of its insert.
	offset, byteOff int

	// opLen caches the length of ops[index], negative until computed.
	// Computing it reads the whole insert.
	opLen int

	// err is the first [ErrSurrogateSplit] of next. next then continues with
	// whole runes, and the caller finishes its loop.
	err error
}

func newIterator(ops []Op) *iterator { return &iterator{ops: ops, opLen: -1} }

func (it *iterator) hasNext() bool { return it.peekLength() < infinity }

func (it *iterator) currentLen() int {
	if it.opLen < 0 {
		it.opLen = it.ops[it.index].Len()
	}
	return it.opLen
}

func (it *iterator) peekLength() int {
	if it.index >= len(it.ops) {
		return infinity
	}
	return it.currentLen() - it.offset
}

// peekType returns the kind of the next operation, [kindRetain] after the last.
func (it *iterator) peekType() kind {
	if it.index >= len(it.ops) {
		return kindRetain
	}
	return it.ops[it.index].kind()
}

// next returns the next length code units of the current operation,
// or its rest if length is not positive or exceeds the rest.
func (it *iterator) next(length int) Op {
	if it.index >= len(it.ops) {
		return Op{Retain: infinity}
	}
	op := it.ops[it.index]
	offset, byteOff := it.offset, it.byteOff
	rest := it.currentLen() - offset
	whole := length <= 0 || length >= rest
	if whole {
		length = rest
		it.index++
		it.offset, it.byteOff, it.opLen = 0, 0, -1
	} else {
		it.offset += length
	}
	switch op.kind() {
	case kindDelete:
		return Op{Delete: length}
	case kindRetain:
		return Op{Retain: length, Attributes: op.Attributes}
	}
	if op.IsEmbed() {
		return Op{Embed: op.Embed, Attributes: op.Attributes}
	}
	if whole {
		return Op{Insert: op.Insert[byteOff:], Attributes: op.Attributes}
	}
	end, ok := advance(op.Insert, byteOff, length)
	if !ok && it.err == nil {
		it.err = ErrSurrogateSplit
	}
	it.byteOff = end
	return Op{Insert: op.Insert[byteOff:end], Attributes: op.Attributes}
}

// rest returns the remaining operations, the current one cut at the offset.
func (it *iterator) rest() []Op {
	if !it.hasNext() {
		return nil
	}
	if it.offset == 0 {
		return it.ops[it.index:]
	}
	saved := *it
	first := it.next(0)
	rest := append([]Op{first}, it.ops[it.index:]...)
	*it = saved
	return rest
}
