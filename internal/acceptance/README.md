# Acceptance cases

Each directory here with a `go.mod` is one case: an application,
the code generated from it, and the tests that drive it over HTTP.

What matters about a generator is what its output does. Reading the output as
text checks one version of that code. These cases send requests and read responses,
which checks the behaviour every version has to provide.

A case is a module of its own. `go build ./...` and `go test ./...` of the root
module do not descend into it, which is how a directory full of applications
can live inside this repository. The generated code is committed.
An editor resolves it, a reviewer reads it, and a case runs by hand:

```sh
cd internal/acceptance/routing && go test -race ./...
```

`mage genDatapages` regenerates every case and every example.

## The runner

[`acceptance_test.go`](acceptance_test.go) is a plain test package of the root
module. Per case it:

1. scans the module for its `datapages.NewServer` call, parses the `app` package,
   generates into a temporary directory, and compares the result with the
   committed `app/datapagesgen`.
   A difference fails with *run: mage genDatapages*.
   The case then runs what the generator writes today;
2. runs `go test -race -count=1 -coverpkg=./app/datapagesgen/... ./...` inside the
   module and merges the coverage profile.

`go test ./internal/acceptance/ -run 'TestAcceptance/routing'` runs one case.
`-short` skips all of them.

`anonstreams`, `events`, `sessions` and `wildcardsubjects` assert against both
brokers datapages ships, so they run `brokers.Main` from their `TestMain` and
need a running Docker daemon: it starts a real NATS server in a container.
Without one they fail with *starting NATS container*. Every other case runs on
the in-memory broker alone and needs nothing.

## Writing a case

```
internal/acceptance/mycase/
  go.mod  go.sum          module github.com/romshark/datapages/internal/acceptance/mycase
  app/app.go              the application, in package app
  app/datapagesgen/       generated, committed
  mycase_test.go          the tests, in package acceptance_test
  contract_case_test.go   wires the case into the shared suite
  newserver_test.go       the mustNewServer helper
  cmd/server/             generated once, committed; compiled by every run
```

Copy an existing case, rewrite `go.mod`, then run `mage genDatapages`.

## The client

[`client`](client) speaks the protocol for a case. It holds what every case
does the same way: the headers a Datastar request carries, and reading an SSE
stream in the background. A case writes only the requests that are its own.

```go
c := client.New(t, mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer)))

resp := c.Get(t, href.PageIndex())            // no content negotiation
require.Equal(t, "term=x", resp.Element(t, "echo"))
c.Action(t, http.MethodPost, "/save/", `{"x":1}`)

s := c.OpenStream(t, "/_$/", signals)         // connect-time signals, or nil
require.True(t, s.Saw(`<div id="out">`))      // waits a second for it
require.True(t, s.Never("other tab"))         // waits the window out
```

It names no generated code. A case keeps everything the model decides.

`mustNewServer` is the per-case helper in `newserver_test.go`. It wraps
`datapages.NewServer` with the case's three type arguments and fails the test
on a configuration error, which no test can carry on from.
Assertions use `testify/require`.

Two things to know when writing requests:

- Responses are compressed by default. The client asks for `Accept-Encoding: identity`.
  What a test reads is then the bytes the handler wrote.
- The in-memory broker matches subjects the way NATS does. `*` covers one
  token, `>` covers the rest. See `wildcardsubjects`.

### Configuration

A case configures nothing. What shapes generation is in the code: the app directory,
the generated package and the metrics mode are the type arguments of
the `datapages.NewServer` call in `cmd/server/main.go`, which `serverscan.Scan` reads,
and the assets come from the `embed.FS` the app package declares.
No case carries a `datapages.yaml`.

The runner reads an optional `acceptance.json` next to the `go.mod`
(`readCaseOptions` in `acceptance_test.go`) with one field, `no_race`, which
drops the race detector for that case. No case currently has one: add the file
only when a case is too slow or too noisy under `-race`, and say in the case why.

## The shared contract suite

[`contract`](contract) holds the assertions that apply to every generated server,
whatever the application: the page shell, compression, URL canonicalization,
`ListenAndServe`, `ListenAndServeTLS`, `Shutdown` closing open streams,
the `main.go` options, opening and reopening an SSE stream, an event dispatched from
an action arriving on it, and every URL `href` and `action` build addressing a real route.

The server is generated per model. The same assertion therefore runs against
different code in each case. A case joins with one test:

```go
func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			return mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
				contract.Options[datapages.ServerOption](opts)...)
		},
		// …
	})
}
```

The suite cannot name a case's generated package. It is a different package in every case.
The `Case` therefore carries the generated symbols the assertions need,
wired in by the case. `contract.Opt` adapts an option constructor.
`contract.Options` converts the values back on the way in. Fields left nil skip
the assertions that need them.

## Recording a defect

None are open, and the suite has no way to record one: a case either passes or
the build is broken. Cover a defect the framework has not fixed yet with a case
that asserts what the framework does today. Name it after that behaviour,
and say at the assertion what the correct one would be:

```go
// Fixing this turns the case red here,
// which is the signal to assert the correct behaviour instead.
require.Equal(t, http.StatusOK, resp.Status,
	"a URL no page claims is still answered with 200")
```

[`TestCompileFixtures`](../generator/compile_test.go) builds the generated code
of every parser fixture. It accepts no exceptions.
