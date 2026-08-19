package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureMissingReq */

func (PageIndex) StreamOpen(streamID uint64) error {
	return nil
}

// PageMissingStreamID is /missing-stream-id
type PageMissingStreamID struct{ App *App }

func (PageMissingStreamID) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureMissingStreamID */

func (PageMissingStreamID) StreamOpen(r *http.Request) error {
	return nil
}

// PageWrongStreamID is /wrong-stream-id
type PageWrongStreamID struct{ App *App }

func (PageWrongStreamID) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrStreamIDParamNotUint64 */

func (PageWrongStreamID) StreamOpen(
	r *http.Request, streamID string,
) error {
	return nil
}

// PageClosedSignals is /closed-signals
type PageClosedSignals struct{ App *App }

func (PageClosedSignals) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: signals not allowed on StreamClose */

func (PageClosedSignals) StreamClose(
	r *http.Request,
	streamID uint64,
	signals datapages.Signals[struct {
		Foo string `json:"foo"`
	}],
) error {
	return nil
}

// PageClosedBadReturn is /closed-bad-return
type PageClosedBadReturn struct{ App *App }

func (PageClosedBadReturn) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureStreamHookReturnMustBeError */

func (PageClosedBadReturn) StreamClose(
	r *http.Request, streamID uint64,
) int {
	return 0
}

// PageClosedSSE is /closed-sse
type PageClosedSSE struct{ App *App }

func (PageClosedSSE) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: sse not allowed on StreamClose */

func (PageClosedSSE) StreamClose(
	r *http.Request,
	streamID uint64,
	sse datapages.SSE,
) error {
	return nil
}

// PageOpenPath is /open-path
type PageOpenPath struct{ App *App }

func (PageOpenPath) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: path not allowed on StreamOpen */

func (PageOpenPath) StreamOpen(
	r *http.Request,
	streamID uint64,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) error {
	return nil
}

// PageClosedPath is /closed-path
type PageClosedPath struct{ App *App }

func (PageClosedPath) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: path not allowed on StreamClose */

func (PageClosedPath) StreamClose(
	r *http.Request,
	streamID uint64,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) error {
	return nil
}

// PageOpenQuery is /open-query
type PageOpenQuery struct{ App *App }

func (PageOpenQuery) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: query not allowed on StreamOpen */

func (PageOpenQuery) StreamOpen(
	r *http.Request,
	streamID uint64,
	query datapages.Query[struct {
		Search string `query:"search"`
	}],
) error {
	return nil
}

// PageClosedQuery is /closed-query
type PageClosedQuery struct{ App *App }

func (PageClosedQuery) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrSignatureUnsupportedInput: query not allowed on StreamClose */

func (PageClosedQuery) StreamClose(
	r *http.Request,
	streamID uint64,
	query datapages.Query[struct {
		Search string `query:"search"`
	}],
) error {
	return nil
}

// PageActionStreamID is /action-stream-id
type PageActionStreamID struct{ App *App }

func (PageActionStreamID) GET(
	r *http.Request,
) (body datapages.Component, err error) {
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
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrStreamIDParamNotUint64: streamID int instead of uint64 */

func (PageWrongStreamIDType) StreamOpen(
	r *http.Request, streamID int,
) error {
	return nil
}

// PageHookFuncDispatch is /hook-func-dispatch
type PageHookFuncDispatch struct{ App *App }

func (PageHookFuncDispatch) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

// EventPing is "ping"
type EventPing struct {
	Data string `json:"data"`
}

/* ErrSignatureUnsupportedInput: stream hook with an untyped dispatcher */

func (PageHookFuncDispatch) StreamOpen(
	r *http.Request,
	streamID uint64,
	dispatch func(EventPing) error,
) error {
	return nil
}

// PageHookDuplicateDispatch is /hook-duplicate-dispatch
type PageHookDuplicateDispatch struct{ App *App }

func (PageHookDuplicateDispatch) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrDispatchDuplicate: stream hook with two dispatchers of one event type */

func (PageHookDuplicateDispatch) StreamClose(
	r *http.Request,
	streamID uint64,
	dispatchPing datapages.Dispatcher[EventPing],
	dispatchPingAgain datapages.Dispatcher[EventPing],
) error {
	return nil
}
