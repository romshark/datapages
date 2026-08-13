// Wires the per-tab state case into the shared contract suite.
//
// A stateful app must be given an HMAC key: NewServer panics without one.

package acceptance_test

import (
	"crypto/sha256"
	"testing"

	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/statefulpages/app"
	"github.com/romshark/datapages/internal/acceptance/statefulpages/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/statefulpages/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/statefulpages/datapagesgen/href"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			key := sha256.Sum256([]byte("acceptance"))
			opts = append(opts, datapagesgen.WithStateConfig(
				datapagesgen.StateConfig{
					HMACKey: key[:],
					// Short enough for the suite to watch a tab's state expire.
					GracePeriod: contract.StateGrace,
				},
			))
			return datapagesgen.NewServer(&app.App{}, inmem.New(8),
				contract.Options[datapagesgen.ServerOption](opts)...)
		},
		WithAssets:     contract.Opt(datapagesgen.WithAssets),
		WithMiddleware: contract.Opt(datapagesgen.WithMiddleware),
		WithDatastarJS: contract.Opt(datapagesgen.WithDatastarJS),
		WithHTTPServer: contract.Opt(datapagesgen.WithHTTPServer),
		WithLogger:     contract.Opt(datapagesgen.WithLogger),
		StreamSubjects: datapagesgen.MessageBrokerStreamSubjects,
		HrefExternal:   href.External,
		HrefSetLogger:  href.SetLogger,
		Links:          []string{href.PageIndex(), href.PageFailOpen()},
		Actions:        []string{action.POSTPageIndexUpdate()},
		SignalActions:  []string{action.POSTPageIndexUpdate()},
		// optionedAction carries every option at once.
		// The keys and their order are asserted by the contract suite.
		OptionedAction: action.POSTPageIndexUpdate(
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
		StreamPath:      "/_$/",
		DispatchAction:  action.POSTPageIndexUpdate(),
		DispatchBody:    `{"filter":"x"}`,
		StateAction:     action.POSTPageIndexUpdate(),
		StateActionBody: `{"filter":"x"}`,
		StateGrace:      contract.StateGrace,
	})
}
