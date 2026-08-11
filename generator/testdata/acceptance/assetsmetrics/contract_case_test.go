// Wires the assets and metrics case into the shared contract suite.

package acceptance_test

import (
	"testing"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"
	"dpacceptance/datapagesgen/action"
	"dpacceptance/datapagesgen/href"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

var contractCase = contract{
	newServer: func(
		t *testing.T, opts ...datapagesgen.ServerOption,
	) *datapagesgen.Server {
		t.Helper()
		opts = append(opts,
			datapagesgen.WithAssets(app.StaticFS),
			datapagesgen.WithPrometheus(datapagesgen.PrometheusConfig{
				Host:       "127.0.0.1:0",
				Registerer: registry,
				Gatherer:   registry,
			}))
		return datapagesgen.NewServer(&app.App{}, inmem.New(8), opts...)
	},
	links: []string{href.PageIndex(), href.Asset("style.css")},
	actions: []string{
		action.POSTPageIndexFail(),
		action.POSTPageIndexAnnounce(),
	},
	signalActions: []string{action.POSTPageIndexAnnounce()},
	// optionedAction carries every option at once. The keys and their order
	// are asserted by the contract suite.
	optionedAction: action.POSTPageIndexFail(
		action.WithBefore("$busy = true"),
		action.WithContentType(action.ContentTypeForm),
		action.WithSelector("#it's"),
		action.WithHeaders(map[string]string{
			"X-Trace": "abc",
			// A value with a quote in it must not close the string it
			// sits in, and a second header must be separated from the
			// first.
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
	streamPath:     "/_$/",
	hasAssets:      true,
	dispatchAction: action.POSTPageIndexAnnounce(),
	dispatchBody:   `{"text":"hello"}`,
}
