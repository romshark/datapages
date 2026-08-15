// Wires the anonstreams case into the shared contract suite.
//
// The app declares a Session type and a stateful page, so NewServer needs a
// CSRF token manager and a state config: it panics without either.

package acceptance_test

import (
	"crypto/sha256"
	"testing"

	"github.com/romshark/datapages/internal/acceptance/anonstreams/app"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/datapagesgen/href"
	"github.com/romshark/datapages/internal/acceptance/contract"
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
			stateKey := sha256.Sum256([]byte("acceptance-state"))
			sessions := sessinmem.New[app.Session](
				sesstokgen.Generator{Length: sesstokgen.DefaultLength},
			)
			opts = append(opts,
				datapagesgen.WithCSRFProtection(
					datapagesgen.CSRFConfig{TokenManager: tm},
				),
				datapagesgen.WithStateConfig(datapagesgen.StateConfig{
					HMACKey: stateKey[:],
				}))
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
		Links: []string{
			href.PageIndex(),
			href.PageRooms(),
			href.PageTabs(),
		},
		Actions: []string{
			action.POSTPageRoomsPost(),
			action.POSTPageRoomsNotice(),
			action.POSTPageTabsBump(),
		},
		SignalActions: []string{
			action.POSTPageRoomsPost(),
			action.POSTPageRoomsNotice(),
		},
		// The stateful page,
		// whose stream the suite opens and whose state it watches expire.
		Index:           href.PageTabs(),
		StreamPath:      "/tabs/_$/",
		DispatchAction:  action.POSTPageTabsBump(),
		StateAction:     action.POSTPageTabsBump(),
		StateActionBody: "",
		OptionedAction: action.POSTPageTabsBump(
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
