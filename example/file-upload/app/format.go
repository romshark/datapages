package app

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/file-upload/store"
)

// humanSize renders n as a byte count with binary units.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// nameField and dupField name the form fields of row i of the upload dialog.
// The index is in the name because a row that continues an upload carries a
// different control than one that starts a new file, and reading the values
// back by position would depend on which.
func nameField(i int) string { return "name-" + strconv.Itoa(i) }
func dupField(i int) string  { return "dup-" + strconv.Itoa(i) }

// dupSeparate is the value of a dup field that asks for a second upload
// instead of continuing the one that is already there.
const dupSeparate = "separate"

// heldBy reports whether tab is the one sending f. An empty tab holds nothing:
// GET renders before the stream that would give it state exists, and a file whose
// sender is gone has no owner either. Comparing the two directly would make them match.
func heldBy(f store.File, tab string) bool { return tab != "" && f.Owner == tab }

// transferState is where a transfer stands as the file list shows it.
// The store statuses do not cover it: a file nobody holds the missing bytes of is
// not being uploaded, whatever the store still expects.
type transferState string

const (
	transferComplete    transferState = "complete"
	transferPaused      transferState = "paused"
	transferInterrupted transferState = "interrupted"
	transferUploading   transferState = "uploading"
)

// transferStateOf is what the badge of f says.
//
// An owner before a status: a file nobody holds the missing bytes of cannot go
// on whatever the store still expects of it, and the row offers to take it
// over rather than to resume it. Reading the status first would badge a file
// paused in a tab that has since gone as merely paused, which is the one state
// its owner could undo.
func transferStateOf(f store.File) transferState {
	switch {
	case f.Complete():
		return transferComplete
	case f.Owner == "":
		return transferInterrupted
	case f.Status == store.StatusPaused:
		return transferPaused
	}
	return transferUploading
}

// transferExplanation says what is happening to f, for a visitor who sees a
// badge and a control that do not obviously belong together.
func transferExplanation(f store.File, tab string) string {
	switch transferStateOf(f) {
	case transferInterrupted:
		return "No tab is sending this file. Its " + humanSize(f.Received) +
			" are kept. Pick the same file again to carry on from there."
	case transferPaused:
		if heldBy(f, tab) {
			return "You paused this upload. Resume it to send the rest."
		}
		return "Another tab paused this upload and still holds the file."
	case transferUploading:
		if heldBy(f, tab) {
			return "This tab is sending the rest of this file."
		}
		return "Another tab is sending the rest of this file."
	}
	return ""
}

// stagedMeta is what the upload dialog says about a file besides its name.
// It is one string because Templ puts a space between the parts it renders.
func stagedMeta(f PendingFile) string {
	if f.ContentType == "" {
		return humanSize(f.Size)
	}
	return humanSize(f.Size) + " \u00b7 " + f.ContentType
}

// toastID is unique per message: neo-toaster reads the HTML id of an appended
// toast as its identity and folds a repeated one into the toast already
// standing instead of stacking a second card.
func toastID() string {
	return "toast-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// errorMessage is what a rejected action tells the visitor. A server-side
// failure carries nothing beyond its status: its message names the request
// that produced it and belongs in the log.
func errorMessage(err error) string {
	for _, e := range []error{
		datapages.ErrBadRequest, datapages.ErrConflict,
		datapages.ErrForbidden, datapages.ErrNotFound,
	} {
		if errors.Is(err, e) {
			msg := strings.TrimSpace(strings.TrimPrefix(err.Error(), e.Error()+":"))
			if msg == "" {
				return e.Error()
			}
			return msg
		}
	}
	return "The server could not handle that request."
}

// attrs merges attribute sets so that a Morpheus helper such as
// [neo.DialogClose] can be spread alongside the app's own attributes,
// which the components take one set at a time. A later set wins.
func attrs(sets ...templ.Attributes) templ.Attributes {
	out := templ.Attributes{}
	for _, s := range sets {
		maps.Copy(out, s)
	}
	return out
}
