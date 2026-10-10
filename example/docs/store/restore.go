package store

import (
	"fmt"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/richtext"
)

// Pending is what an editor kept in the browser and the server never confirmed:
// Sent, the change it submitted with sequence number Seq at revision Rev, and
// Buffer, made after Sent and never submitted. Buffer may be empty.
type Pending struct {
	Client string
	Seq    int
	Rev    int
	Sent   delta.Delta
	Buffer delta.Delta
}

// Restore applies the pending changes of an editor that closed before the
// server confirmed them. The store may have applied Sent already. Restore goes
// through the changes since Rev as the editor would have received them, which
// shows whether it did, and applies what remains as the editor would have
// submitted it. Restoring the same changes again is a no-op.
func (s *Store) Restore(id string, p Pending) error {
	if !ValidClient(p.Client) || p.Seq <= 0 {
		return fmt.Errorf("%w: client %q, sequence number %d", ErrInvalid, p.Client, p.Seq)
	}
	for _, change := range []delta.Delta{p.Sent, p.Buffer} {
		if err := richtext.ValidateChange(change); err != nil {
			return err
		}
	}
	return s.with(id, func(d *doc) error { return d.restore(p) })
}

func (d *doc) restore(p Pending) error {
	missed, err := d.since(p.Rev)
	if err != nil {
		return err
	}
	pending := []Submission{{Client: p.Client, Seq: p.Seq, Delta: p.Sent}}
	if len(p.Buffer.Ops) > 0 {
		pending = append(pending, Submission{Client: p.Client, Seq: p.Seq + 1, Delta: p.Buffer})
	}
	for _, c := range missed {
		if len(pending) > 0 && c.Client == p.Client && c.Seq == pending[0].Seq {
			// The acknowledgment the editor never received.
			pending = pending[1:]
			continue
		}
		// Sync.receive in web/src/sync.ts transforms sent and buffer the same way.
		change := c.Delta
		for i := range pending {
			pending[i].Delta, change = change.Transform(pending[i].Delta, true),
				pending[i].Delta.Transform(change, false)
		}
	}
	rev := d.rev
	for _, sub := range pending {
		sub.Rev = rev
		if err := d.submit(sub); err != nil {
			return err
		}
		// The next change follows this one, at rev+1, and was made before
		// the repair that may follow it. Submit transforms it against the repair.
		rev++
	}
	return nil
}
