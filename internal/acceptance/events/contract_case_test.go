// Wires the events case into the shared contract suite.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/events/app"
	"github.com/romshark/datapages/internal/acceptance/events/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/events/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/events/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// TestContract must not use t.Parallel() because the generated Init sets the
// package-level logger of the href package, which contract.Run's ExternalHref test reads.
func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			return mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
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
			href.PageLog(),
			href.PageOther(),
			href.PageRoom(),
		},
		Actions: []string{
			action.POSTPageIndexTick(),
			action.POSTPageRoomSay(),
			action.POSTPageRoomBroadcast(),
		},
		SignalActions: []string{
			action.POSTPageIndexTick(),
			action.POSTPageRoomSay(),
			action.POSTPageRoomBroadcast(),
		},
		// optionedAction carries every option at once.
		// The keys and their order are asserted by the contract suite.
		OptionedAction: action.POSTPageIndexTick(
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
		StreamPath:     "/_$/",
		DispatchAction: action.POSTPageIndexTick(),
		DispatchBody:   `{"n":1}`,
	})
}
