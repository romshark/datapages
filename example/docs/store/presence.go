package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

// Collaborator is an editor as the others see it, without the client id,
// which authorizes its changes.
type Collaborator struct {
	Name  string
	Color string
	// You marks the editor a [View] is for.
	You bool
	// Cursor is nil until the editor reports one.
	Cursor *Cursor
}

// Cursor is a selection, or a caret when Length is zero.
type Cursor struct {
	Index  int `json:"index"`
	Length int `json:"length"`
}

// editor is a tab that has the document open.
type editor struct {
	Collaborator
	order int
	// streams counts the open streams of the tab. A reconnect can open one
	// before the old one closes.
	streams   int
	cursorSeq int
}

// NewClient returns a new client id: 16 random bytes as unpadded base64url.
func NewClient() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// ValidClient reports whether client has the form of a [NewClient] id.
func ValidClient(client string) bool {
	var b [16]byte
	if len(client) != base64.RawURLEncoding.EncodedLen(len(b)) {
		return false
	}
	n, err := base64.RawURLEncoding.Decode(b[:], []byte(client))
	return err == nil && n == len(b)
}

var (
	animals = []string{
		"Axolotl", "Badger", "Capybara", "Chameleon", "Dingo", "Dolphin",
		"Fox", "Hedgehog", "Ibex", "Koala", "Lemur", "Lynx", "Manatee",
		"Narwhal", "Ocelot", "Otter", "Panda", "Penguin", "Quokka",
		"Raccoon", "Squirrel", "Tapir", "Turtle", "Wombat",
	}
	// colors are dark enough for white initials.
	colors = []string{
		"#1a73e8", "#d93025", "#188038", "#e37400", "#9334e6", "#007b83",
		"#c5221f", "#b06000", "#3949ab", "#00897b", "#d81b60", "#5f6368",
	}
)

// Identity returns how an editor appears to the others, derived from the
// client id to stay the same across reconnects.
func Identity(client string) Collaborator {
	sum := sha256.Sum256([]byte(client))
	n := binary.BigEndian.Uint64(sum[:8])
	return Collaborator{
		Name:  "Anonymous " + animals[n%uint64(len(animals))],
		Color: colors[n/uint64(len(animals))%uint64(len(colors))],
	}
}

// Join records that client opened the document.
func (s *Store) Join(id, client string) error {
	if !ValidClient(client) {
		return fmt.Errorf("%w: client %q", ErrInvalid, client)
	}
	return s.with(id, func(d *doc) error {
		e := d.editors[client]
		if e == nil {
			d.joined++
			e = &editor{Collaborator: Identity(client), order: d.joined}
			d.editors[client] = e
			d.presence++
			d.cursors++
		}
		e.streams++
		return nil
	})
}

// Leave records that client closed the document.
func (s *Store) Leave(id, client string) error {
	return s.with(id, func(d *doc) error {
		e := d.editors[client]
		if e == nil {
			return nil
		}
		if e.streams--; e.streams > 0 {
			return nil
		}
		delete(d.editors, client)
		d.presence++
		d.cursors++
		return nil
	})
}

// MoveCursor records the cursor of client at revision rev and reports whether
// it moved. It drops a cursor older than the last one reported or at a
// revision outside the history.
func (s *Store) MoveCursor(id, client string, seq, rev int, c Cursor) (bool, error) {
	if !ValidClient(client) || c.Index < 0 || c.Length < 0 {
		return false, fmt.Errorf("%w: cursor %+v of client %q", ErrInvalid, c, client)
	}
	moved := false
	err := s.with(id, func(d *doc) error {
		e := d.editors[client]
		if e == nil || seq <= e.cursorSeq {
			return nil
		}
		missed, err := d.since(rev)
		if errors.Is(err, ErrStale) {
			return nil
		}
		start, end := c.Index, c.Index+c.Length
		for _, ch := range missed {
			own := ch.Client == client
			start = ch.Delta.TransformPosition(start, !own)
			end = ch.Delta.TransformPosition(end, !own)
		}
		// A cursor stays before the final "\n".
		last := d.content.Length() - 1
		start = min(start, last)
		end = min(max(end, start), last)
		e.Cursor = &Cursor{Index: start, Length: end - start}
		e.cursorSeq = seq
		d.cursors++
		moved = true
		return nil
	})
	return moved, err
}

// moveCursors moves every cursor through c: past an insert of its own editor
// at the cursor, before one of another editor. Quill does the same in each
// editor.
func (d *doc) moveCursors(c Change) {
	for client, e := range d.editors {
		if e.Cursor == nil {
			continue
		}
		own := client == c.Client
		start := c.Delta.TransformPosition(e.Cursor.Index, !own)
		end := c.Delta.TransformPosition(e.Cursor.Index+e.Cursor.Length, !own)
		e.Cursor = &Cursor{Index: start, Length: max(0, end-start)}
	}
}

// View is what a tab needs to catch up with a document.
type View struct {
	Rev   int
	Title string
	// Changes are the changes after the revision the tab has.
	Changes []Change
	// Collaborators are the editors in the order they opened the document.
	Collaborators []Collaborator
	// Presence and Cursors change when Collaborators or their cursors do.
	Presence, Cursors int
}

// View returns the state of a document for client at revision rev.
func (s *Store) View(id, client string, rev int) (View, error) {
	var v View
	err := s.with(id, func(d *doc) error {
		missed, err := d.since(rev)
		if err != nil {
			return err
		}
		editors := make([]*editor, 0, len(d.editors))
		for _, e := range d.editors {
			editors = append(editors, e)
		}
		slices.SortFunc(editors, func(a, b *editor) int { return a.order - b.order })
		v = View{
			Rev:           d.rev,
			Title:         d.title,
			Changes:       slices.Clone(missed),
			Collaborators: make([]Collaborator, len(editors)),
			Presence:      d.presence,
			Cursors:       d.cursors,
		}
		for i, e := range editors {
			v.Collaborators[i] = e.Collaborator
			v.Collaborators[i].You = e == d.editors[client]
		}
		return nil
	})
	return v, err
}
