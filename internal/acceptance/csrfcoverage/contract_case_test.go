// Wires the csrfcoverage case into the shared contract suite.
//
// An app that declares a Session type must be given a CSRF token manager:
// NewServer panics without one.

package acceptance_test

import (
	"crypto/sha256"
	"testing"

	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/app"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/datapagesgen/href"
	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
	sessinmem "github.com/romshark/datapages/modules/sessmanager/inmem"
	"github.com/romshark/datapages/modules/sesstokgen"
)

func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			key := sha256.Sum256([]byte("acceptance-csrf"))
			tm, err := csrfhmac.New(key[:])
			if err != nil {
				t.Fatalf("building CSRF token manager: %v", err)
			}
			sessions := sessinmem.New[struct{}](
				sesstokgen.Generator{Length: sesstokgen.DefaultLength},
			)
			opts = append(opts, datapagesgen.WithCSRFProtection(
				datapagesgen.CSRFConfig{TokenManager: tm},
			))
			return datapagesgen.NewServer(&app.App{}, inmem.New(8), sessions,
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
		Links:          []string{href.PageIndex()},
		Actions: []string{
			action.POSTPageIndexSignIn(),
			action.POSTPageIndexDelete(),
		},
		SignalActions: []string{action.POSTPageIndexSignIn()},
		OptionedAction: action.POSTPageIndexSignIn(
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
