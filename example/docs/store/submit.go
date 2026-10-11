package store

import (
	"fmt"
	"time"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/richtext"
)

// Submission is a change an editor sends.
type Submission struct {
	// Client identifies the editor: 1 load of the document in 1 tab.
	Client string
	// Seq numbers the changes of Client from 1.
	Seq int
	// Rev is the revision the change was made at.
	Rev   int
	Delta delta.Delta
}

// Submit transforms a change made at an earlier revision against the changes
// since and applies it as the next revision. If concurrent changes leave
// something Quill cannot show, a [richtext.Repair] follows as the next one.
//
// A change already applied, resent after a lost response, is a no-op.
func (s *Store) Submit(id string, sub Submission) error {
	if !ValidClient(sub.Client) || sub.Seq <= 0 {
		return fmt.Errorf("%w: client %q, sequence number %d", ErrInvalid, sub.Client, sub.Seq)
	}
	if err := richtext.ValidateChange(sub.Delta); err != nil {
		return err
	}
	return s.with(id, func(d *doc) error { return d.submit(sub) })
}

func (d *doc) submit(sub Submission) error {
	missed, err := d.since(sub.Rev)
	if err != nil {
		return err
	}
	for _, c := range missed {
		if c.Client == sub.Client && c.Seq == sub.Seq {
			return nil
		}
	}
	change := sub.Delta
	for _, c := range missed {
		// The change applied first takes priority.
		change = c.Delta.Transform(change, true)
	}
	if change.BaseLength() > d.content.Length() {
		return fmt.Errorf("%w: change reaches past the end of the document", ErrInvalid)
	}
	next, err := d.content.Compose(change)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := richtext.CheckDocument(next); err != nil {
		return err
	}
	if next.Length() > MaxLength {
		return fmt.Errorf("%w: document longer than %d characters", ErrInvalid, MaxLength)
	}

	now := time.Now()
	changes := []Change{{
		Rev: d.rev + 1, Delta: change, Client: sub.Client, Seq: sub.Seq, At: now,
	}}
	if fix := richtext.Repair(next, change); len(fix.Ops) > 0 {
		if next, err = next.Compose(fix); err != nil {
			return err
		}
		changes = append(changes, Change{Rev: d.rev + 2, Delta: fix, At: now})
	}

	// Compact first. On failure the state is untouched and the change is
	// rejected, never applied without a log.
	if d.logged >= compactAfter {
		if err := d.compact(); err != nil {
			return err
		}
	}
	if err := d.appendLog(changes...); err != nil {
		return err
	}
	d.content, d.updated = next, now
	for _, c := range changes {
		d.rev = c.Rev
		d.remember(c)
		d.moveCursors(c)
	}
	return nil
}

// since returns the changes after rev.
func (d *doc) since(rev int) ([]Change, error) {
	first := d.rev - len(d.history) + 1
	switch {
	case rev == d.rev:
		return nil, nil
	case rev > d.rev || rev < first-1:
		return nil, fmt.Errorf("%w: revision %d, the document has %d to %d",
			ErrStale, rev, first-1, d.rev)
	}
	return d.history[rev+1-first:], nil
}
