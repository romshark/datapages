package delta_test

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/docs/delta"
)

var (
	chars   = []string{"a", "b", " ", "\n", "é", "😀"}
	formats = []delta.Attributes{
		nil, nil,
		{"bold": true},
		{"italic": true},
		{"color": "red"},
		{"header": float64(1)},
	}
	formatChanges = []delta.Attributes{
		nil,
		{"bold": true},
		{"bold": nil},
		{"color": "blue"},
		{"header": nil, "italic": true},
	}
)

func randomText(r *rand.Rand, n int) string {
	s := ""
	for range n {
		s += chars[r.IntN(len(chars))]
	}
	return s
}

var image = delta.Embed{Type: "image", Value: "a.png"}

// insertRandom appends up to maxLen random runes, or 1 time in 5 an image.
func insertRandom(r *rand.Rand, d *delta.Delta, maxLen int) {
	format := formats[r.IntN(len(formats))]
	if r.IntN(5) == 0 {
		d.InsertEmbed(image, format)
		return
	}
	d.Insert(randomText(r, 1+r.IntN(maxLen)), format)
}

func randomDocument(r *rand.Rand) delta.Delta {
	var d delta.Delta
	for range 1 + r.IntN(6) {
		insertRandom(r, &d, 8)
	}
	d.Insert("\n", nil)
	return d
}

// cuts returns the positions between the runes and embeds of doc,
// the only places a change may cut it.
func cuts(doc delta.Delta) []int {
	at := []int{0}
	for _, op := range doc.Ops {
		if op.IsEmbed() {
			at = append(at, at[len(at)-1]+1)
			continue
		}
		for _, r := range op.Insert {
			at = append(at, at[len(at)-1]+delta.UTF16Len(string(r)))
		}
	}
	return at
}

func randomChange(r *rand.Rand, doc delta.Delta) delta.Delta {
	at := cuts(doc)
	var d delta.Delta
	i := 0
	for step := 0; step < 10 && i < len(at)-1; step++ {
		switch r.IntN(3) {
		case 0:
			insertRandom(r, &d, 4)
			continue
		case 1:
			j := min(len(at)-1, i+1+r.IntN(5))
			d.Retain(at[j]-at[i], formatChanges[r.IntN(len(formatChanges))])
			i = j
		default:
			j := min(len(at)-1, i+1+r.IntN(5))
			d.Push(delta.Op{Delete: at[j] - at[i]})
			i = j
		}
	}
	return d
}

func compose(t *testing.T, a, b delta.Delta) delta.Delta {
	t.Helper()
	c, err := a.Compose(b)
	require.NoError(t, err)
	return c
}

// TestConvergence tests on seeded random cases that 2 concurrent changes,
// each transformed against the other with opposite priority, give the same
// document in either order. Server and editors depend on it.
func TestConvergence(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		doc := randomDocument(r)
		a, b := randomChange(r, doc), randomChange(r, doc)

		viaA := compose(t, compose(t, doc, a), a.Transform(b, true))
		viaB := compose(t, compose(t, doc, b), b.Transform(a, false))
		require.Equal(t, viaA, viaB, "doc %v\na %v\nb %v", doc, a, b)
		for _, op := range viaA.Ops {
			require.True(t, op.IsInsert(), "the result is no document: %v", viaA)
		}

		// Composing the changes first gives the same document.
		require.Equal(t, viaA, compose(t, doc, compose(t, a, a.Transform(b, true))))
	}
}

// TestTransformPositionFollowsText tests on seeded random cases that
// the position before a kept character moves with it.
// Without priority, it also moves past inserts in front of the character.
func TestTransformPositionFollowsText(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for range 2000 {
		doc := randomDocument(r)
		change := randomChange(r, doc)
		text := plain(doc)
		afterText := plain(compose(t, doc, change))
		deleted := deletedPositions(change)
		for p, unit := range text {
			if deleted[p] {
				continue
			}
			q := change.TransformPosition(p, false)
			require.Equal(t, unit, afterText[q], "position %d", p)
			require.LessOrEqual(t, change.TransformPosition(p, true), q)
		}
	}
}

// deletedPositions returns the positions that change deletes.
func deletedPositions(change delta.Delta) map[int]bool {
	deleted := make(map[int]bool)
	at := 0
	for _, op := range change.Ops {
		switch {
		case op.IsDelete():
			for i := range op.Delete {
				deleted[at+i] = true
			}
			at += op.Delete
		case op.IsRetain():
			at += op.Retain
		}
	}
	return deleted
}

// plain returns the UTF-16 code units of a document, U+FFFC for an embed.
func plain(doc delta.Delta) []uint16 {
	var units []uint16
	for _, op := range doc.Ops {
		if op.IsEmbed() {
			units = append(units, 0xfffc)
			continue
		}
		for _, r := range op.Insert {
			if r >= 0x10000 {
				r -= 0x10000
				units = append(units, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff)))
				continue
			}
			units = append(units, uint16(r))
		}
	}
	return units
}
