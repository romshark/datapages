package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/richtext"
)

// doc is 1 document. mu guards every field but dir and id.
type doc struct {
	dir, id string

	mu      sync.Mutex
	deleted bool
	title   string
	updated time.Time
	content delta.Delta
	rev     int

	// history holds the last changes, the last one at rev.
	history []Change

	log     *os.File // Open <id>.log.
	logged  int      // Changes in log.
	logSize int64    // Bytes in log.
	// logBuf and logEnc encode log lines, reused for every change.
	logBuf bytes.Buffer
	logEnc *json.Encoder

	// preview caches [richtext.Preview] of content at revision previewRev.
	preview    string
	previewRev int

	editors map[string]*editor
	// joined numbers the editors in the order they opened the document.
	joined int
	// presence and cursors count changes of the editors and their cursors.
	// A tab compares them with the values it last got.
	presence, cursors int
}

// Change is 1 revision of a document.
type Change struct {
	Rev   int         `json:"rev"`
	Delta delta.Delta `json:"delta"`
	// Client is the editor that sent the change, empty for a repair by the
	// store.
	Client string `json:"client,omitempty"`
	// Seq numbers the changes of 1 editor. A retry repeats it.
	Seq int       `json:"seq,omitempty"`
	At  time.Time `json:"at"`
}

// snapshot is the content of <id>.json.
type snapshot struct {
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	Updated time.Time   `json:"updated"`
	Rev     int         `json:"rev"`
	Content delta.Delta `json:"content"`
}

func (d *doc) path(ext string) string { return filepath.Join(d.dir, d.id+ext) }

// load reads the snapshot of a document and replays its log.
func load(dir, id string) (*doc, error) {
	d := &doc{dir: dir, id: id, editors: make(map[string]*editor), previewRev: -1}
	b, err := os.ReadFile(d.path(".json"))
	if err != nil {
		return nil, err
	}
	var snap snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, err
	}
	if snap.ID != id {
		return nil, fmt.Errorf("snapshot of %q in the file of %q", snap.ID, id)
	}
	if err := richtext.CheckDocument(snap.Content); err != nil {
		return nil, err
	}
	d.title, d.updated = snap.Title, snap.Updated
	d.rev, d.content = snap.Rev, snap.Content
	if err := d.replay(); err != nil {
		return nil, err
	}
	return d, d.openLog()
}

// replay applies the logged changes after the snapshot. All logged changes go
// to the history, for changes made at earlier revisions. It truncates the log
// after the last valid line, dropping a partial line left by a crash.
func (d *doc) replay() error {
	f, err := os.OpenFile(d.path(".log"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	r := bufio.NewReader(f)
	good := int64(0)
read:
	for {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		var c Change
		if json.Unmarshal(line, &c) != nil {
			break
		}
		switch {
		case c.Rev <= d.rev:
			// Already in the snapshot.
		case c.Rev == d.rev+1:
			next, err := d.content.Compose(c.Delta)
			if err != nil || richtext.CheckDocument(next) != nil {
				break read
			}
			d.content, d.rev, d.updated = next, c.Rev, c.At
		default:
			break read
		}
		if n := len(d.history); n > 0 && d.history[n-1].Rev != c.Rev-1 {
			d.history = nil
		}
		d.remember(c)
		d.logged++
		good += int64(len(line))
	}
	if n := len(d.history); n > 0 && d.history[n-1].Rev != d.rev {
		d.history = nil
	}
	d.logSize = good
	return f.Truncate(good)
}

func (d *doc) openLog() error {
	f, err := os.OpenFile(d.path(".log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	d.log = f
	return nil
}

// remember appends c to the history, dropping changes beyond [historyLen].
func (d *doc) remember(c Change) {
	d.history = append(d.history, c)
	if extra := len(d.history) - historyLen; extra > 0 {
		d.history = append(d.history[:0], d.history[extra:]...)
	}
}

// appendLog writes all changes to the log or none. A partial line would join
// the next one and stop replay there.
func (d *doc) appendLog(changes ...Change) error {
	if err := d.encodeLog(changes); err != nil {
		return err
	}
	if _, err := d.log.Write(d.logBuf.Bytes()); err != nil {
		return fmt.Errorf("logging changes: %w", errors.Join(err, d.log.Truncate(d.logSize)))
	}
	d.logSize += int64(d.logBuf.Len())
	d.logged += len(changes)
	return nil
}

// encodeLog encodes changes into logBuf, 1 line each.
func (d *doc) encodeLog(changes []Change) error {
	if d.logEnc == nil {
		d.logEnc = json.NewEncoder(&d.logBuf)
	}
	d.logBuf.Reset()
	for _, c := range changes {
		if err := d.logEnc.Encode(c); err != nil {
			return err
		}
	}
	return nil
}

// compact writes a snapshot and rewrites the log with the last [keepLogged]
// changes, for the history after a restart. Each file is written next to its
// target and renamed over it: a crash leaves the old file or the new one.
func (d *doc) compact() error {
	b, err := json.Marshal(snapshot{
		ID: d.id, Title: d.title, Updated: d.updated,
		Rev: d.rev, Content: d.content,
	})
	if err != nil {
		return err
	}
	if err := replaceFile(d.path(".json"), b); err != nil {
		return fmt.Errorf("writing snapshot: %w", err)
	}
	if d.log == nil {
		return nil
	}
	keep := d.history[max(0, len(d.history)-keepLogged):]
	if err := d.encodeLog(keep); err != nil {
		return err
	}
	// Windows refuses to rename over an open file.
	if err := d.log.Close(); err != nil {
		return err
	}
	if err := replaceFile(d.path(".log"), d.logBuf.Bytes()); err != nil {
		return errors.Join(fmt.Errorf("rewriting log: %w", err), d.openLog())
	}
	d.logged, d.logSize = len(keep), int64(d.logBuf.Len())
	return d.openLog()
}

// replaceFile writes b to path through a file next to it.
func replaceFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
