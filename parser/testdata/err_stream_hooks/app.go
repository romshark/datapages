package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureMissingReq */

func (PageIndex) OnStreamOpen(streamID uint64) error {
	return nil
}

// PageMissingStreamID is /missing-stream-id
type PageMissingStreamID struct{ App *App }

func (PageMissingStreamID) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureMissingStreamID */

func (PageMissingStreamID) OnStreamOpen(r *http.Request) error {
	return nil
}

// PageWrongStreamID is /wrong-stream-id
type PageWrongStreamID struct{ App *App }

func (PageWrongStreamID) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrStreamIDParamNotUint64 */

func (PageWrongStreamID) OnStreamOpen(
	r *http.Request, streamID string,
) error {
	return nil
}

// PageClosedSignals is /closed-signals
type PageClosedSignals struct{ App *App }

func (PageClosedSignals) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: signals not allowed on OnStreamClosed */

func (PageClosedSignals) OnStreamClosed(
	r *http.Request,
	streamID uint64,
	signals struct {
		Foo string `json:"foo"`
	},
) error {
	return nil
}

// PageClosedBadReturn is /closed-bad-return
type PageClosedBadReturn struct{ App *App }

func (PageClosedBadReturn) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureStreamHookReturnMustBeError */

func (PageClosedBadReturn) OnStreamClosed(
	r *http.Request, streamID uint64,
) {
}

// PageClosedSSE is /closed-sse
type PageClosedSSE struct{ App *App }

func (PageClosedSSE) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: sse not allowed on OnStreamClosed */

func (PageClosedSSE) OnStreamClosed(
	r *http.Request,
	streamID uint64,
	sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// PageOpenPath is /open-path
type PageOpenPath struct{ App *App }

func (PageOpenPath) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: path not allowed on OnStreamOpen */

func (PageOpenPath) OnStreamOpen(
	r *http.Request,
	streamID uint64,
	path struct {
		ID string `path:"id"`
	},
) error {
	return nil
}

// PageClosedPath is /closed-path
type PageClosedPath struct{ App *App }

func (PageClosedPath) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: path not allowed on OnStreamClosed */

func (PageClosedPath) OnStreamClosed(
	r *http.Request,
	streamID uint64,
	path struct {
		ID string `path:"id"`
	},
) error {
	return nil
}

// PageOpenQuery is /open-query
type PageOpenQuery struct{ App *App }

func (PageOpenQuery) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: query not allowed on OnStreamOpen */

func (PageOpenQuery) OnStreamOpen(
	r *http.Request,
	streamID uint64,
	query struct {
		Search string `query:"search"`
	},
) error {
	return nil
}

// PageClosedQuery is /closed-query
type PageClosedQuery struct{ App *App }

func (PageClosedQuery) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: query not allowed on OnStreamClosed */

func (PageClosedQuery) OnStreamClosed(
	r *http.Request,
	streamID uint64,
	query struct {
		Search string `query:"search"`
	},
) error {
	return nil
}

// PageActionStreamID is /action-stream-id
type PageActionStreamID struct{ App *App }

func (PageActionStreamID) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: streamID not allowed on action handler */

// POSTDoSomething is /action-stream-id/do-something
func (PageActionStreamID) POSTDoSomething(
	r *http.Request,
	streamID uint64,
) error {
	return nil
}

// PageWrongStreamIDType is /wrong-stream-id-type
type PageWrongStreamIDType struct{ App *App }

func (PageWrongStreamIDType) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return nil, nil
}

/* ErrStreamIDParamNotUint64: streamID int instead of uint64 */

func (PageWrongStreamIDType) OnStreamOpen(
	r *http.Request, streamID int,
) error {
	return nil
}
