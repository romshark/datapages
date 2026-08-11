// Wires the sessions case into the shared contract suite.
//
// An app with a Session type must be given a session manager and a CSRF token
// manager. This constructor supplies both.

package acceptance_test

import (
	"testing"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"
	"dpacceptance/datapagesgen/action"
	"dpacceptance/datapagesgen/href"

	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
	sessinmem "github.com/romshark/datapages/modules/sessmanager/inmem"
	"github.com/romshark/datapages/modules/sesstokgen"
)

var contractCase = contract{
	newServer: func(
		t *testing.T, opts ...datapagesgen.ServerOption,
	) *datapagesgen.Server {
		t.Helper()
		tm, err := csrfhmac.New(csrfSecret)
		if err != nil {
			t.Fatalf("building CSRF token manager: %v", err)
		}
		sessions := sessinmem.New[app.Session](
			sesstokgen.Generator{Length: sesstokgen.DefaultLength})
		opts = append(opts, datapagesgen.WithCSRFProtection(
			datapagesgen.CSRFConfig{TokenManager: tm}))
		return datapagesgen.NewServer(
			&app.App{}, inmem.New(8), sessions, opts...)
	},
	links: []string{
		href.PageIndex(),
		href.PageLogin(),
		href.PageLog(),
		href.PageToken(),
		href.PageSecret(),
	},
	actions: []string{
		action.POSTPageLoginSubmit(),
		action.POSTPageLoginBroadcast(),
		action.POSTPageLoginNotify(),
		action.POSTPageLoginRename(),
		action.POSTAppSignOut(),
	},
	signalActions: []string{
		action.POSTPageLoginSubmit(),
		action.POSTPageLoginBroadcast(),
		action.POSTPageLoginNotify(),
	},
	// optionedAction carries every option at once. The keys and their order
	// are asserted by the contract suite.
	optionedAction: action.POSTPageLoginSubmit(
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
