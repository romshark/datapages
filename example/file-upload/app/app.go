package app

import (
	"embed"
	"net/http"
	"sync"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/file-upload/store"
	"github.com/romshark/datapages/example/file-upload/throttle"
)

// The limit inputs accept a rate between these, in KiB/s. Zero is not one of them:
// no limit is the Unlimited switch of [Limit], not a rate of nothing.
const (
	MinLimitKiB = 1
	MaxLimitKiB = 1 << 20 // 1 GiB/s
)

// DefaultLimitKiB is the rate a direction starts with, so that switching its
// limit on offers a number rather than an empty field.
const DefaultLimitKiB = 1024

// LimitStepKiB is how far one step of the rate field moves it.
// The field declares it and the arrow keys apply it, hence one constant for both.
const LimitStepKiB = 64

// chunkBudget is how much of the upload limit one chunk may carry, hence how
// long a transfer that died goes on moving the progress bar. The limit is enforced by
// reading the body slowly, which leaves the bytes the browser handed over sitting in
// the socket buffers: the server commits them whatever the browser does.
// A fixed size bounds that by nothing: 1 MiB at 1 KiB/s is 17 minutes.
const chunkBudget = 10 * time.Second

// The floor keeps a slow transfer from paying a request per few hundred bytes.
// The ceiling is what an unlimited transfer sends.
const (
	minChunkSize = 16 << 10
	maxChunkSize = 1 << 20
)

// morpheusCDN serves the three static assets of the Morpheus UI kit.
// Self-hosting takes the same files from the min directory of its repository.
const morpheusCDN = "https://cdn.jsdelivr.net/gh/romshark/morpheus@v0.1.0/min"

// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS

// EventFilesChanged is "files.changed"
//
// It carries nothing: every subscriber renders the list from the store.
type EventFilesChanged struct{}

// EventUploadStarted is "upload.started"
//
// It reaches the one tab that holds the files the jobs describe.
type EventUploadStarted struct {
	Tab datapages.SubjectStateID `json:"tab"`

	Jobs []UploadJob `json:"jobs"`
}

// EventLimitsChanged is "limits.changed"
//
// The limits belong to the server, not to the tab that typed them.
type EventLimitsChanged struct{}

// StateIndex is the per-tab state of [PageIndex]:
// the files a tab picked but has not confirmed in the upload dialog yet.
//
// Which tab is sending a file is not part of it. Two tabs must not send the same one,
// which no tab can decide on its own: [store.File.Owner] holds it
// and the handlers name their tab with stateID.
type StateIndex struct {
	Pending []PendingFile

	// Origin is the scheme and host the tab reached this server on,
	// kept because a file list rendered from an event has no request to
	// read it from and the copy button needs an absolute URL.
	Origin string
}

// PickedFile is one file as the browser described it to [PageIndex.POSTStage].
type PickedFile struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"type"`
}

// PendingFile is one picked file as the upload dialog shows it.
type PendingFile struct {
	PickedFile

	// MatchID is an unfinished upload of the same original name and size,
	// which this file can continue instead of starting a second one.
	// The dialog offers the choice, since a name and a size are all a browser
	// reveals and two different files can share both.
	MatchID      string
	MatchPercent int
}

// Limit is one direction's transfer limit. KiB outlives Unlimited being
// switched on, so that switching it off again restores the rate the visitor
// had chosen instead of an empty field.
type Limit struct {
	KiB       int64
	Unlimited bool
}

// Rate is what the limiter enforces, in bytes per second. Zero is unlimited.
func (l Limit) Rate() int64 {
	if l.Unlimited {
		return 0
	}
	return l.KiB * 1024
}

// Limits are both directions.
type Limits struct{ Upload, Download Limit }

type App struct {
	files *store.Store

	// One budget per direction, shared by every transfer in it:
	// a limit of this application is a limit of the server.
	upload, download *throttle.Limiter

	// The limiters keep only the rate in force. The rate a switched-off limit
	// would return to is not a rate a limiter can hold, hence these fields.
	limitsMu sync.Mutex
	limits   Limits
}

func NewApp(files *store.Store) *App {
	unlimited := Limit{KiB: DefaultLimitKiB, Unlimited: true}
	return &App{
		files:    files,
		upload:   throttle.NewLimiter(0),
		download: throttle.NewLimiter(0),
		limits:   Limits{Upload: unlimited, Download: unlimited},
	}
}

// Limits are the transfer limits as the page holds them.
func (a *App) Limits() Limits {
	a.limitsMu.Lock()
	defer a.limitsMu.Unlock()
	return a.limits
}

// SetLimits changes both transfer limits and returns what they are now.
// A rate out of range is clamped rather than refused,
// and the tab shows what took effect.
func (a *App) SetLimits(l Limits) Limits {
	l.Upload.KiB = clampLimit(l.Upload.KiB)
	l.Download.KiB = clampLimit(l.Download.KiB)

	a.limitsMu.Lock()
	a.limits = l
	a.limitsMu.Unlock()

	a.upload.SetRate(l.Upload.Rate())
	a.download.SetRate(l.Download.Rate())
	return l
}

func clampLimit(kib int64) int64 { return min(max(kib, MinLimitKiB), MaxLimitKiB) }

// ChunkSize is how many bytes the uploader puts in one chunk:
// [chunkBudget] of the upload limit, within the size bounds.
func (a *App) ChunkSize() int64 {
	rate := a.upload.Rate()
	if rate <= 0 {
		return maxChunkSize
	}
	return min(max(rate*int64(chunkBudget/time.Second), minChunkSize), maxChunkSize)
}

func (*App) Head(r *http.Request) datapages.Head { return head() }

// RecoverError shows what a rejected action complained about.
// Without it a Datastar request reports the error to the browser console only.
//
// The message is appended into the stack rather than morphed over it:
// a morph of the host would strip the attributes neo-toaster writes on itself,
// and every message the visitor has not read yet would go with it.
func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElementAt(errorToast(err), "#toast", datapages.PatchModeAppend)
}
