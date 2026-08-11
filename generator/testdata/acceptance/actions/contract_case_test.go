// Wires the actions case into the shared contract suite.

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
		return datapagesgen.NewServer(&app.App{}, inmem.New(8), opts...)
	},
	links: []string{
		href.PageIndex(),
		href.PageForm(),
		href.PageLog(),
	},
	actions: []string{
		action.POSTPageFormSubmit(),
		action.PUTPageFormReplace(),
		action.PATCHPageFormTouch(),
		action.DELETEPageFormRemove(),
		action.POSTPageFormBump(7, action.QueryPOSTPageFormBump{By: 3}),
		action.POSTPageFormRender(),
		action.POSTPageFormGo(),
		action.POSTPageFormPatch(),
		action.POSTAppPing(),
		action.DELETEAppAll(),
	},
	signalActions: []string{
		action.POSTPageFormSubmit(),
		action.POSTPageFormPatch(),
	},
	// optionedAction carries every option at once. The keys and their order
	// are asserted by the contract suite.
	optionedAction: action.POSTPageFormSubmit(
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
}
