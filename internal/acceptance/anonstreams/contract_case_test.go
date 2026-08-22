// Wires the anonstreams case into the shared contract suite.
//
// The app declares a Session type, so NewServer needs a CSRF token manager:
// datapages.NewServer fails without one.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/app"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/app/datapagesgen/href"
	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			sessions := sessinmem.New[struct{}](
				sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
			)
			return mustNewServer(t, &app.App{},
				inmem.New(messaging.DefaultBrokerChanBuffer), sessions,
				contract.Options[datapages.ServerOption](opts)...)
		},
		WithMiddleware: contract.OptVariadic(datapages.WithMiddleware),
		WithDatastarJS: contract.Opt(datapages.WithDatastarJS),
		WithHTTPServer: contract.Opt(datapages.WithHTTPServer),
		WithLogger:     contract.Opt(datapages.WithLogger),
		StreamSubjects: datapagesgen.MessageBrokerStreamSubjects,
		HrefExternal:   href.External,
		HrefSetLogger:  href.SetLogger,
		Links: []string{
			href.PageIndex(),
			href.PageRooms(),
			href.PageFeed(),
		},
		Actions: []string{
			action.POSTPageRoomsPost(),
			action.POSTPageRoomsNotice(),
			action.POSTPageFeedTick(),
		},
		SignalActions: []string{
			action.POSTPageRoomsPost(),
			action.POSTPageRoomsNotice(),
			action.POSTPageFeedTick(),
		},
		// The page whose stream subscribes by nothing,
		// which is the one the suite can open without supplying a signal.
		Index:          href.PageFeed(),
		StreamPath:     "/feed/_$/",
		DispatchAction: action.POSTPageFeedTick(),
		DispatchBody:   `{"n":1}`,
		OptionedAction: action.POSTPageFeedTick(
			action.WithBefore("$busy = true"),
			action.WithContentType(action.ContentTypeForm),
			action.WithSelector("#it's"),
			action.WithHeaders(map[string]string{
				"X-Trace": "abc",
				// A value with a quote in it must not close the string it sits in,
				// and a second header must be separated from the first.
				"X-Note": "it's here",
			}),
			action.WithFilterSignals("name", "secret"),
			action.WithOpenWhenHidden(true),
			action.WithPayload("{id: $id}"),
			action.WithRetry(action.RetryAlways),
			action.WithRetryInterval(500),
			action.WithRetryScaler(1.5),
			action.WithRetryMaxWaitMs(30000),
			action.WithRetryMaxCount(3),
			action.WithRequestCancellation(action.RequestCancellationDisabled),
			action.WithAfter("$busy = false"),
		),
	})
}
