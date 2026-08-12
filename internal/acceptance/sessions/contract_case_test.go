// Wires the sessions case into the shared contract suite.
//
// An app with a Session type must be given a session manager and a CSRF token manager.
// This constructor supplies both.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/sessions/app"
	"github.com/romshark/datapages/internal/acceptance/sessions/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/sessions/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/sessions/datapagesgen/href"
	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
	sessinmem "github.com/romshark/datapages/modules/sessmanager/inmem"
	"github.com/romshark/datapages/modules/sesstokgen"
)

func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			tm, err := csrfhmac.New(csrfSecret)
			if err != nil {
				t.Fatalf("building CSRF token manager: %v", err)
			}
			sessions := sessinmem.New[app.Session](
				sesstokgen.Generator{Length: sesstokgen.DefaultLength},
			)
			opts = append(opts, datapagesgen.WithCSRFProtection(
				datapagesgen.CSRFConfig{TokenManager: tm},
			))
			return datapagesgen.NewServer(
				&app.App{}, inmem.New(8), sessions,
				contract.Options[datapagesgen.ServerOption](opts)...,
			)
		},
		WithAssets:     contract.Opt(datapagesgen.WithAssets),
		WithMiddleware: contract.Opt(datapagesgen.WithMiddleware),
		WithDatastarJS: contract.Opt(datapagesgen.WithDatastarJS),
		WithHTTPServer: contract.Opt(datapagesgen.WithHTTPServer),
		WithLogger:     contract.Opt(datapagesgen.WithLogger),
		StreamSubjects: datapagesgen.MessageBrokerStreamSubjects,
		HrefExternal:   href.External,
		HrefSetLogger:  href.SetLogger,
		Links: []string{
			href.PageIndex(),
			href.PageLogin(),
			href.PageLog(),
			href.PageToken(),
			href.PageSecret(),
		},
		Actions: []string{
			action.POSTPageLoginSubmit(),
			action.POSTPageLoginBroadcast(),
			action.POSTPageLoginNotify(),
			action.POSTPageLoginRename(),
			action.POSTAppSignOut(),
		},
		SignalActions: []string{
			action.POSTPageLoginSubmit(),
			action.POSTPageLoginBroadcast(),
			action.POSTPageLoginNotify(),
		},
		// optionedAction carries every option at once.
		// The keys and their order are asserted by the contract suite.
		OptionedAction: action.POSTPageLoginSubmit(
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
