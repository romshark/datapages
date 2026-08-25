// Wires both applications of this module into the shared contract suite.
//
// The suite runs twice, against two generated packages written in one run.
// Each has to pass on its own: what the frontend was generated with is no part
// of what the admin server has to do, and the other way round.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/contract"
	adminapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin"
	admingen "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen"
	adminaction "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen/action"
	adminhref "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen/href"
	frontendapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend"
	frontendgen "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen"
	frontendaction "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen/action"
	frontendhref "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen/href"
)

// TestContractFrontend must not use t.Parallel() because the generated Init sets the
// package-level logger of the href package, which contract.Run's ExternalHref test reads.
func TestContractFrontend(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			return mustNewFrontend(t, &frontendapp.App{}, broker(),
				contract.Options[datapages.ServerOption](opts)...)
		},
		WithMiddleware: contract.OptVariadic(datapages.WithMiddleware),
		WithDatastarJS: contract.Opt(datapages.WithDatastarJS),
		WithHTTPServer: contract.Opt(datapages.WithHTTPServer),
		WithLogger:     contract.Opt(datapages.WithLogger),
		StreamSubjects: frontendgen.MessageBrokerStreamSubjects,
		HrefExternal:   frontendhref.External,
		HrefSetLogger:  frontendhref.SetLogger,
		Links:          []string{frontendhref.PageIndex()},
		StreamPath:     "/_$/",
		DispatchAction: frontendaction.POSTPageIndexNotice(),
		DispatchBody:   `{"text":"hello"}`,
		Actions: []string{
			frontendaction.POSTPageIndexSignIn(),
			frontendaction.POSTPageIndexNotice(),
		},
		SignalActions: []string{
			frontendaction.POSTPageIndexSignIn(),
			frontendaction.POSTPageIndexNotice(),
		},
		// optionedAction carries every option at once.
		// The keys and their order are asserted by the contract suite.
		OptionedAction: frontendaction.POSTPageIndexNotice(
			frontendaction.WithBefore("$busy = true"),
			frontendaction.WithContentType(frontendaction.ContentTypeForm),
			frontendaction.WithSelector("#it's"),
			frontendaction.WithHeaders(map[string]string{
				"X-Trace": "abc",
				// A value with a quote in it must not close the string it sits in,
				// and a second header must be separated from the first.
				"X-Note": "it's here",
			}),
			frontendaction.WithFilterSignals("name", "secret"),
			frontendaction.WithOpenWhenHidden(true),
			frontendaction.WithPayload("{id: $id}"),
			frontendaction.WithRetry(frontendaction.RetryAlways),
			frontendaction.WithRetryInterval(500),
			frontendaction.WithRetryScaler(1.5),
			frontendaction.WithRetryMaxWaitMs(30000),
			frontendaction.WithRetryMaxCount(3),
			frontendaction.WithRequestCancellation(frontendaction.RequestCancellationDisabled),
			frontendaction.WithAfter("$busy = false"),
		),
	})
}

// TestContractAdmin must not use t.Parallel() because the generated Init sets the
// package-level logger of the href package, which contract.Run's ExternalHref test reads.
func TestContractAdmin(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			return mustNewAdmin(t, &adminapp.App{}, broker(),
				contract.Options[datapages.ServerOption](opts)...)
		},
		WithMiddleware: contract.OptVariadic(datapages.WithMiddleware),
		WithDatastarJS: contract.Opt(datapages.WithDatastarJS),
		WithHTTPServer: contract.Opt(datapages.WithHTTPServer),
		WithLogger:     contract.Opt(datapages.WithLogger),
		StreamSubjects: admingen.MessageBrokerStreamSubjects,
		HrefExternal:   adminhref.External,
		HrefSetLogger:  adminhref.SetLogger,
		Links:          []string{adminhref.PageIndex()},
		StreamPath:     "/_$/",
		DispatchAction: adminaction.POSTPageIndexReport(),
		DispatchBody:   `{"n":1}`,
		Actions:        []string{adminaction.POSTPageIndexReport()},
		SignalActions:  []string{adminaction.POSTPageIndexReport()},
		// optionedAction carries every option at once.
		// The keys and their order are asserted by the contract suite.
		OptionedAction: adminaction.POSTPageIndexReport(
			adminaction.WithBefore("$busy = true"),
			adminaction.WithContentType(adminaction.ContentTypeForm),
			adminaction.WithSelector("#it's"),
			adminaction.WithHeaders(map[string]string{
				"X-Trace": "abc",
				// A value with a quote in it must not close the string it sits in,
				// and a second header must be separated from the first.
				"X-Note": "it's here",
			}),
			adminaction.WithFilterSignals("name", "secret"),
			adminaction.WithOpenWhenHidden(true),
			adminaction.WithPayload("{id: $id}"),
			adminaction.WithRetry(adminaction.RetryAlways),
			adminaction.WithRetryInterval(500),
			adminaction.WithRetryScaler(1.5),
			adminaction.WithRetryMaxWaitMs(30000),
			adminaction.WithRetryMaxCount(3),
			adminaction.WithRequestCancellation(adminaction.RequestCancellationDisabled),
			adminaction.WithAfter("$busy = false"),
		),
	})
}
