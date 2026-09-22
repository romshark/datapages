package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/romshark/datapages/example/file-upload/app/datapagesgen/action"
	"github.com/romshark/datapages/example/file-upload/store"
)

// UploadJob tells the uploader where the bytes of one file go,
// which byte comes next and how many to send at a time.
// It travels to the browser as the argument of a script.
type UploadJob struct {
	ID     string `json:"id"`
	Chunk  string `json:"chunk"`
	Offset int64  `json:"offset"`

	// ChunkSize is [App.ChunkSize] when the job was made. The browser does
	// not decide how much it may hand over, the server does.
	ChunkSize int64 `json:"chunkSize"`
}

// chunkPathPrefix is the chunk endpoint up to the identifier, cut out of the
// generated action expression like [chunkURL] is.
var chunkPathPrefix = strings.SplitN(chunkURL(chunkIDMark), chunkIDMark, 2)[0]

// chunkIDMark stands in for an identifier and survives url.PathEscape.
const chunkIDMark = "0id0"

func newUploadJob(f store.File, chunkSize int64) UploadJob {
	return UploadJob{
		ID: f.ID, Chunk: chunkURL(f.ID), Offset: f.Received, ChunkSize: chunkSize,
	}
}

// callScript renders a call of the uploader with arg as its only argument.
// encoding/json escapes "<" and ">": no value can
// end the script element the call travels in.
func callScript(fn string, arg any) (string, error) {
	b, err := json.Marshal(arg)
	if err != nil {
		return "", fmt.Errorf("encoding %s argument: %w", fn, err)
	}
	return fn + "(" + string(b) + ")", nil
}

// chunkURL is the chunk endpoint of id with an empty offset parameter,
// to which the uploader appends the offset of each chunk.
// The 1 is rendered and cut off again because a zero renders no parameter at all.
func chunkURL(id string) string {
	return strings.TrimSuffix(actionURL(
		action.PageIndex.Chunk.PUT(id, action.PageIndex.Chunk.PUTQuery(1)),
	), "1")
}

// actionURL returns the URL of a generated Datastar action expression.
//
// The uploader sends chunks with fetch, which needs a plain URL,
// and no generated package renders one. Cutting it out of the expression keeps
// the route declared in one place: the doc comment of its handler.
//
// url.PathEscape percent-encodes an apostrophe: the first two apostrophes of
// an expression without options delimit the whole URL.
func actionURL(expr string) string {
	open := strings.IndexByte(expr, '\'')
	if open < 0 {
		return ""
	}
	end := strings.IndexByte(expr[open+1:], '\'')
	if end < 0 {
		return ""
	}
	return expr[open+1 : open+1+end]
}
