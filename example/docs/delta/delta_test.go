package delta_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/docs/delta"
)

func ins(text string, attrs delta.Attributes) delta.Op {
	return delta.Op{Insert: text, Attributes: attrs}
}

func ret(n int, attrs delta.Attributes) delta.Op {
	return delta.Op{Retain: n, Attributes: attrs}
}

func del(n int) delta.Op { return delta.Op{Delete: n} }

func emb(attrs delta.Attributes) delta.Op {
	return delta.Op{Embed: image, Attributes: attrs}
}

func ops(o ...delta.Op) delta.Delta { return delta.Delta{Ops: o} }

var bold = delta.Attributes{"bold": true}

// TestPush tests how [delta.Delta.Push] merges and reorders operations,
// with cases from the quill-delta test suite.
func TestPush(t *testing.T) {
	for name, tc := range map[string]struct {
		in   []delta.Op
		want delta.Delta
	}{
		"inserts merge": {
			in:   []delta.Op{ins("a", nil), ins("b", nil)},
			want: ops(ins("ab", nil)),
		},
		"inserts with different attributes stay": {
			in:   []delta.Op{ins("a", bold), ins("b", nil)},
			want: ops(ins("a", bold), ins("b", nil)),
		},
		"retains merge": {
			in:   []delta.Op{ret(1, bold), ret(2, delta.Attributes{"bold": true})},
			want: ops(ret(3, bold)),
		},
		"deletes merge": {
			in:   []delta.Op{del(1), del(2)},
			want: ops(del(3)),
		},
		"insert moves before delete": {
			in:   []delta.Op{ret(1, nil), del(1), ins("a", nil)},
			want: ops(ret(1, nil), ins("a", nil), del(1)),
		},
		"insert before leading delete": {
			in:   []delta.Op{del(1), ins("a", nil)},
			want: ops(ins("a", nil), del(1)),
		},
		"insert moves before delete and merges": {
			in:   []delta.Op{ins("a", nil), del(1), ins("b", nil)},
			want: ops(ins("ab", nil), del(1)),
		},
		"embeds stay apart": {
			in:   []delta.Op{emb(nil), emb(nil)},
			want: ops(emb(nil), emb(nil)),
		},
		"text and embed stay apart": {
			in:   []delta.Op{ins("a", nil), emb(nil), ins("b", nil)},
			want: ops(ins("a", nil), emb(nil), ins("b", nil)),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var d delta.Delta
			for _, op := range tc.in {
				d.Push(op)
			}
			require.Equal(t, tc.want, d)
		})
	}
}

// TestCompose tests [delta.Delta.Compose] with cases from the quill-delta test suite.
func TestCompose(t *testing.T) {
	for name, tc := range map[string]struct {
		a, b, want delta.Delta
	}{
		"insert insert": {
			a:    ops(ins("A", nil)),
			b:    ops(ins("B", nil)),
			want: ops(ins("BA", nil)),
		},
		"insert retain": {
			a: ops(ins("A", nil)),
			b: ops(ret(1, delta.Attributes{
				"bold":  true,
				"color": "red",
				"font":  nil,
			})),
			want: ops(ins("A", delta.Attributes{
				"bold":  true,
				"color": "red",
			})),
		},
		"insert delete": {
			a:    ops(ins("A", nil)),
			b:    ops(del(1)),
			want: ops(),
		},
		"delete insert": {
			a:    ops(del(1)),
			b:    ops(ins("B", nil)),
			want: ops(ins("B", nil), del(1)),
		},
		"delete retain": {
			a: ops(del(1)),
			b: ops(ret(1, delta.Attributes{
				"bold":  true,
				"color": "red",
			})),
			want: ops(del(1), ret(1, delta.Attributes{
				"bold":  true,
				"color": "red",
			})),
		},
		"delete delete": {
			a:    ops(del(1)),
			b:    ops(del(1)),
			want: ops(del(2)),
		},
		"retain insert": {
			a:    ops(ret(1, delta.Attributes{"color": "blue"})),
			b:    ops(ins("B", nil)),
			want: ops(ins("B", nil), ret(1, delta.Attributes{"color": "blue"})),
		},
		"retain retain": {
			a: ops(ret(1, delta.Attributes{"color": "blue"})),
			b: ops(ret(1, delta.Attributes{
				"bold":  true,
				"color": "red",
				"font":  nil,
			})),
			want: ops(ret(1, delta.Attributes{
				"bold":  true,
				"color": "red",
				"font":  nil,
			})),
		},
		"retain delete": {
			a:    ops(ret(1, delta.Attributes{"color": "blue"})),
			b:    ops(del(1)),
			want: ops(del(1)),
		},
		"insert in middle of text": {
			a:    ops(ins("Hello", nil)),
			b:    ops(ret(3, nil), ins("X", nil)),
			want: ops(ins("HelXlo", nil)),
		},
		"insert and delete ordering": {
			a:    ops(ins("Hello", nil)),
			b:    ops(ins("Hello", nil)),
			want: ops(ins("HelloHello", nil)),
		},
		"insert and delete at one position": {
			a:    ops(ins("Hello", nil)),
			b:    ops(ret(3, nil), ins("X", nil), del(1)),
			want: ops(ins("HelXo", nil)),
		},
		"remove all attributes": {
			a:    ops(ins("A", delta.Attributes{"bold": true})),
			b:    ops(ret(1, delta.Attributes{"bold": nil})),
			want: ops(ins("A", nil)),
		},
		"retain past the end": {
			a:    ops(ins("A", nil)),
			b:    ops(ret(2, bold)),
			want: ops(ins("A", bold), ret(1, bold)),
		},
		"retain start optimization": {
			a: ops(ins("A", bold), ins("B", nil), ins("C", bold), del(1)),
			b: ops(ret(3, nil), ins("D", nil)),
			want: ops(
				ins("A", bold), ins("B", nil), ins("C", bold), ins("D", nil), del(1),
			),
		},
		"retain start optimization split": {
			a: ops(ins("A", bold), ins("B", nil), ins("C", bold), ret(5, nil), del(1)),
			b: ops(ret(4, nil), ins("D", nil)),
			want: ops(
				ins("A", bold), ins("B", nil), ins("C", bold),
				ret(1, nil), ins("D", nil), ret(4, nil), del(1),
			),
		},
		"retain end optimization": {
			a:    ops(ins("A", bold), ins("B", nil), ins("C", bold)),
			b:    ops(del(1)),
			want: ops(ins("B", nil), ins("C", bold)),
		},
		"retain end optimization join": {
			a: ops(
				ins("A", bold), ins("B", nil), ins("C", bold),
				ins("D", nil), ins("E", bold), ins("F", nil),
			),
			b: ops(ret(1, nil), del(1)),
			want: ops(
				ins("AC", bold), ins("D", nil), ins("E", bold), ins("F", nil),
			),
		},
		"embed retain": {
			a:    ops(emb(nil), ins("\n", nil)),
			b:    ops(ret(1, bold)),
			want: ops(emb(bold), ins("\n", nil)),
		},
		"embed delete": {
			a:    ops(ins("a", nil), emb(nil), ins("b", nil)),
			b:    ops(ret(1, nil), del(1)),
			want: ops(ins("ab", nil)),
		},
		"insert before embed": {
			a:    ops(emb(nil)),
			b:    ops(ins("x", nil)),
			want: ops(ins("x", nil), emb(nil)),
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := tc.a.Compose(tc.b)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestTransform tests [delta.Delta.Transform] with cases from the quill-delta test suite.
func TestTransform(t *testing.T) {
	for name, tc := range map[string]struct {
		a, b     delta.Delta
		priority bool
		want     delta.Delta
	}{
		"insert insert with priority": {
			a: ops(ins("A", nil)), b: ops(ins("B", nil)), priority: true,
			want: ops(ret(1, nil), ins("B", nil)),
		},
		"insert insert without priority": {
			a: ops(ins("A", nil)), b: ops(ins("B", nil)),
			want: ops(ins("B", nil)),
		},
		"insert retain": {
			a: ops(ins("A", nil)), b: ops(ret(1, delta.Attributes{"bold": true})),
			want: ops(ret(1, nil), ret(1, delta.Attributes{"bold": true})),
		},
		"insert delete": {
			a: ops(ins("A", nil)), b: ops(del(1)),
			want: ops(ret(1, nil), del(1)),
		},
		"delete insert": {
			a: ops(del(1)), b: ops(ins("B", nil)),
			want: ops(ins("B", nil)),
		},
		"delete retain": {
			a: ops(del(1)), b: ops(ret(1, bold)),
			want: ops(),
		},
		"delete delete": {
			a: ops(del(1)), b: ops(del(1)),
			want: ops(),
		},
		"retain insert": {
			a: ops(ret(1, delta.Attributes{"color": "blue"})), b: ops(ins("B", nil)),
			want: ops(ins("B", nil)),
		},
		"retain retain with priority": {
			a:        ops(ret(1, delta.Attributes{"color": "blue"})),
			b:        ops(ret(1, delta.Attributes{"bold": true, "color": "red"})),
			priority: true,
			want:     ops(ret(1, delta.Attributes{"bold": true})),
		},
		"retain retain without priority": {
			a:    ops(ret(1, delta.Attributes{"color": "blue"})),
			b:    ops(ret(1, delta.Attributes{"bold": true, "color": "red"})),
			want: ops(ret(1, delta.Attributes{"bold": true, "color": "red"})),
		},
		"retain delete": {
			a: ops(ret(1, delta.Attributes{"color": "blue"})), b: ops(del(1)),
			want: ops(del(1)),
		},
		"alternating edits": {
			a:    ops(ret(2, nil), ins("si", nil), del(5)),
			b:    ops(ret(1, nil), ins("e", nil), del(5), ret(1, nil), ins("ow", nil)),
			want: ops(ret(1, nil), ins("e", nil), del(1), ret(2, nil), ins("ow", nil)),
		},
		"conflicting appends with priority": {
			a: ops(ret(3, nil), ins("aa", nil)), b: ops(ret(3, nil), ins("bb", nil)),
			priority: true,
			want:     ops(ret(5, nil), ins("bb", nil)),
		},
		"conflicting appends without priority": {
			a: ops(ret(3, nil), ins("aa", nil)), b: ops(ret(3, nil), ins("bb", nil)),
			want: ops(ret(3, nil), ins("bb", nil)),
		},
		"prepend and append": {
			a: ops(ins("aa", nil)), b: ops(ret(3, nil), ins("bb", nil)),
			want: ops(ret(5, nil), ins("bb", nil)),
		},
		"trailing deletes with differing lengths": {
			a: ops(ret(2, nil), del(1)), b: ops(del(3)),
			want: ops(del(2)),
		},
		"embed insert": {
			a: ops(emb(nil)), b: ops(ins("B", nil)), priority: true,
			want: ops(ret(1, nil), ins("B", nil)),
		},
		"insert embed": {
			a: ops(ins("AB", nil)), b: ops(ret(1, nil), emb(nil)),
			want: ops(ret(3, nil), emb(nil)),
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.a.Transform(tc.b, tc.priority))
		})
	}
}

// TestTransformPosition tests [delta.Delta.TransformPosition] with cases from the
// quill-delta test suite.
func TestTransformPosition(t *testing.T) {
	for name, tc := range map[string]struct {
		d        delta.Delta
		index    int
		priority bool
		want     int
	}{
		"insert before position": {d: ops(ins("A", nil)), index: 2, want: 3},
		"insert after position": {
			d: ops(ret(2, nil), ins("A", nil)), index: 1, want: 1,
		},
		"insert at position": {
			d: ops(ret(2, nil), ins("A", nil)), index: 2, want: 3,
		},
		"insert at position with priority": {
			d: ops(ret(2, nil), ins("A", nil)), index: 2, priority: true, want: 2,
		},
		"delete before position": {d: ops(del(2)), index: 4, want: 2},
		"delete after position": {
			d: ops(ret(4, nil), del(2)), index: 2, want: 2,
		},
		"delete across position": {
			d: ops(ret(1, nil), del(4)), index: 2, want: 1,
		},
		"insert and delete before position": {
			d: ops(ret(2, nil), ins("A", nil), del(2)), index: 4, want: 3,
		},
		"insert before and delete across position": {
			d: ops(ret(2, nil), ins("A", nil), del(4)), index: 4, want: 3,
		},
		"delete before and delete across position": {
			d: ops(del(1), ret(1, nil), del(4)), index: 4, want: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.d.TransformPosition(tc.index, tc.priority))
		})
	}
}

// TestUTF16 tests that lengths and positions count UTF-16 code units,
// 2 per rune above U+FFFF.
func TestUTF16(t *testing.T) {
	doc := ops(ins("a😀é\n", nil))
	require.Equal(t, 5, doc.Length())

	got, err := doc.Compose(ops(ret(3, nil), ins("x", nil)))
	require.NoError(t, err)
	require.Equal(t, ops(ins("a😀xé\n", nil)), got)

	got, err = doc.Compose(ops(ret(1, nil), del(2)))
	require.NoError(t, err)
	require.Equal(t, ops(ins("aé\n", nil)), got)

	got, err = doc.Compose(ops(ret(1, nil), ret(2, bold)))
	require.NoError(t, err)
	require.Equal(t, ops(ins("a", nil), ins("😀", bold), ins("é\n", nil)), got)
}

// TestComposeSurrogateSplit tests that [delta.Delta.Compose] rejects a change ending inside
// a surrogate pair instead of producing half a rune.
func TestComposeSurrogateSplit(t *testing.T) {
	doc := ops(ins("a😀b\n", nil))
	for name, change := range map[string]delta.Delta{
		"insert":       ops(ret(2, nil), ins("x", nil)),
		"delete":       ops(ret(1, nil), del(1)),
		"delete after": ops(ret(2, nil), del(1)),
		"format":       ops(ret(2, bold)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := doc.Compose(change)
			require.ErrorIs(t, err, delta.ErrSurrogateSplit)
		})
	}
}

// TestLengths tests [delta.Delta.Length] and [delta.Delta.BaseLength] of a change.
func TestLengths(t *testing.T) {
	d := ops(ret(2, nil), ins("ab", nil), del(3), ret(1, bold))
	require.Equal(t, 8, d.Length())
	require.Equal(t, 6, d.BaseLength())
}

// TestJSON tests that operations round-trip through quill-delta JSON.
func TestJSON(t *testing.T) {
	const in = `{"ops":[{"insert":"Hi","attributes":{"bold":true,"color":"#ff0000"}},` +
		`{"retain":3,"attributes":{"header":1,"italic":null}},` +
		`{"delete":2},{"insert":{"image":"a.png"},"attributes":{"bold":true}},` +
		`{"insert":"\n"}]}`
	var d delta.Delta
	require.NoError(t, json.Unmarshal([]byte(in), &d))
	require.Equal(t, ops(
		ins("Hi", delta.Attributes{"bold": true, "color": "#ff0000"}),
		ret(3, delta.Attributes{"header": float64(1), "italic": nil}),
		del(2),
		emb(bold),
		ins("\n", nil),
	), d)
	out, err := json.Marshal(d)
	require.NoError(t, err)
	require.JSONEq(t, in, string(out))
}

// TestJSONRejects tests that [delta.Op.UnmarshalJSON] rejects operations quill-delta
// never produces for text and images.
func TestJSONRejects(t *testing.T) {
	for name, in := range map[string]string{
		"embed with two keys":     `{"insert":{"image":"a.png","video":"b.mp4"}}`,
		"embed without key":       `{"insert":{}}`,
		"embed without type":      `{"insert":{"":"a.png"}}`,
		"embed number":            `{"insert":{"image":1}}`,
		"empty insert":            `{"insert":""}`,
		"null insert":             `{"insert":null}`,
		"no action":               `{"attributes":{"bold":true}}`,
		"two actions":             `{"insert":"a","retain":1}`,
		"zero retain":             `{"retain":0}`,
		"negative delete":         `{"delete":-1}`,
		"fractional retain":       `{"retain":1.5}`,
		"embed retain":            `{"retain":{"image":"x.png"}}`,
		"huge retain":             `{"retain":2147483648}`,
		"delete with attributes":  `{"delete":1,"attributes":{"bold":true}}`,
		"object attribute":        `{"insert":"a","attributes":{"x":{"y":1}}}`,
		"array attribute":         `{"insert":"a","attributes":{"x":[1]}}`,
		"unknown field":           `{"insert":"a","foo":1}`,
		"attributes not object":   `{"insert":"a","attributes":"bold"}`,
		"insert number":           `{"insert":1}`,
		"retain string":           `{"retain":"1"}`,
		"delete leading zero int": `{"delete":01}`,
	} {
		t.Run(name, func(t *testing.T) {
			var op delta.Op
			require.Error(t, json.Unmarshal([]byte(in), &op))
		})
	}
}

// TestJSONEmptyAttributes tests that an empty attributes object becomes nil.
func TestJSONEmptyAttributes(t *testing.T) {
	var d delta.Delta
	require.NoError(t, json.Unmarshal(
		[]byte(`{"ops":[{"retain":1,"attributes":{}}]}`), &d,
	))
	require.Equal(t, ops(ret(1, nil)), d)
}
