package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/file-upload/store"
)

const maxFileSize = 1 << 30 // 1 GiB

// POSTStage is /stage
//
// It opens the upload dialog on the picked files.
// Nothing reaches the store until the visitor confirms the names.
func (p PageIndex) POSTStage(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateIndex],
	signals datapages.Signals[struct {
		Files []PickedFile `json:"files"`
	}],
) error {
	unfinished := p.unfinished()
	pending := make([]PendingFile, 0, len(signals.Values.Files))
	for _, f := range signals.Values.Files {
		name, err := store.CleanName(f.Name)
		if err != nil {
			return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
		}
		if f.Size < 0 || f.Size > maxFileSize {
			return fmt.Errorf("%w: %s is %s, the limit is %s",
				datapages.ErrBadRequest, name, humanSize(f.Size), humanSize(maxFileSize))
		}
		row := PendingFile{PickedFile: PickedFile{
			Name: name, Size: f.Size, ContentType: f.ContentType,
		}}
		if match, ok := unfinished[key(name, f.Size)]; ok {
			// The name offered is the one that upload already carries,
			// so that confirming the dialog unchanged renames nothing.
			row.Name = match.Name
			row.MatchID, row.MatchPercent = match.ID, match.Percent()
			// One unfinished upload takes one of the picked files,
			// which stops the same file picked twice from continuing it twice.
			delete(unfinished, key(name, f.Size))
		}
		pending = append(pending, row)
	}
	state.Values.Pending = pending
	return sse.PatchElement(uploadDialog(pending))
}

// unfinished are the uploads a picked file could continue, by name and size.
// A complete file is not one of them: its bytes are all there, and uploading
// it again is a second file rather than the rest of this one.
func (p PageIndex) unfinished() map[string]store.File {
	out := make(map[string]store.File)
	for _, f := range p.App.files.List() {
		if f.Complete() {
			continue
		}
		if _, taken := out[key(f.OriginalName, f.Size)]; !taken {
			out[key(f.OriginalName, f.Size)] = f
		}
	}
	return out
}

func key(name string, size int64) string {
	return strconv.FormatInt(size, 10) + ":" + name
}

// POSTCancel is /cancel
func (p PageIndex) POSTCancel(
	r *http.Request, sse datapages.SSE, state datapages.State[StateIndex],
) error {
	state.Values.Pending = nil
	return sse.PatchElement(uploadDialog(nil))
}

// POSTStart is /start
//
// The dialog answers as form fields because it holds one name and one choice
// per file, which no fixed signal struct describes. The jobs travel back on a
// tab-scoped event rather than on this response, since a handler that opens a
// stream cannot read the request body afterwards.
func (p PageIndex) POSTStart(
	r *http.Request,
	state datapages.State[StateIndex],
	stateID string,
	filesChanged datapages.Dispatcher[EventFilesChanged],
	uploadStarted datapages.Dispatcher[EventUploadStarted],
) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("%w: reading form: %w", datapages.ErrBadRequest, err)
	}
	pending := state.Values.Pending
	jobs := make([]UploadJob, 0, len(pending))
	for i, f := range pending {
		// One job per picked file and in the order they were picked:
		// that is how the uploader pairs them with the File objects it holds.
		if f.MatchID != "" && r.PostForm.Get(dupField(i)) != dupSeparate {
			job, err := p.continueUpload(f.MatchID, r.PostForm.Get(nameField(i)), stateID)
			if err == nil {
				jobs = append(jobs, job)
				continue
			}
			// The upload it would have continued was finished or deleted
			// while the dialog stood open, which leaves a new file to create.
			if !errors.Is(err, store.ErrNotFound) &&
				!errors.Is(err, store.ErrNotUploading) {
				return err
			}
		}
		created, err := p.App.files.Create(
			r.PostForm.Get(nameField(i)), f.Name, f.ContentType, f.Size,
		)
		if err != nil {
			return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
		}
		// A file of no bytes is complete on creation and needs no owner.
		if _, err := p.App.files.SetOwner(created.ID, stateID); err != nil &&
			!errors.Is(err, store.ErrNotUploading) {
			return err
		}
		jobs = append(jobs, newUploadJob(created, p.App.ChunkSize()))
	}
	state.Values.Pending = nil

	return errors.Join(
		uploadStarted.Dispatch(EventUploadStarted{
			Tab: datapages.SubjectStateID(stateID), Jobs: jobs,
		}),
		filesChanged.Dispatch(EventFilesChanged{}),
	)
}

// continueUpload hands an unfinished upload to the tab named by stateID under
// the name the dialog carries, and reports where its bytes continue.
func (p PageIndex) continueUpload(id, name, stateID string) (UploadJob, error) {
	if _, err := p.App.files.SetOwner(id, stateID); err != nil {
		return UploadJob{}, err
	}
	if _, err := p.App.files.Rename(id, name); err != nil {
		return UploadJob{}, err
	}
	f, err := p.App.files.SetStatus(id, store.StatusUploading)
	if err != nil {
		return UploadJob{}, err
	}
	return newUploadJob(f, p.App.ChunkSize()), nil
}

// OnUploadStarted closes the upload dialog of
// the tab that confirmed it and starts its transfers.
func (p PageIndex) OnUploadStarted(
	event EventUploadStarted, sse datapages.SSE,
) error {
	if err := sse.PatchElement(uploadDialog(nil)); err != nil {
		return err
	}
	script, err := callScript("dpUpload.start", event.Jobs)
	if err != nil {
		return err
	}
	return sse.ExecuteScript(script)
}
