# Acceptance cases

Each directory here holding a `go.mod` is one case: an application, the code
generated from it, and the tests that drive it over HTTP.

What matters about a generator is what its output does. Reading the output as
text checks one version of the code instead of the behaviour every version has
to provide. These cases send requests and read responses.

A case is a module of its own, which is why a directory full of applications
can live inside the repository: `go build ./...` and `go test ./...` of the
root module do not descend into a nested module. The generated code is
committed, which is what lets an editor resolve it, a reviewer read it, and a
case be run by hand:

```sh
cd internal/acceptance/routing && go test -race ./...
```

Regenerate every case, and every example, with `mage genDatapages`.

## The runner

[`acceptance_test.go`](acceptance_test.go) is a plain test package of the root
module. Per case it:

1. reads `datapages.yaml`, parses the `app` package and generates into a
   temporary directory, then compares the result with the committed
   `datapagesgen`. A difference fails with *run: mage genDatapages*. What the
   case runs is therefore what the generator writes today;
2. runs `go test -race -count=1 -coverpkg=./datapagesgen/... ./...` inside the
   module and merges the coverage profile.

`go test ./internal/acceptance/ -run 'TestAcceptance/routing'` runs one case.
`-short` skips all of them.

## Writing a case

```
internal/acceptance/mycase/
  go.mod  go.sum          module github.com/romshark/datapages/internal/acceptance/mycase
  datapages.yaml          what "datapages gen" reads
  app/app.go              the application, in package app
  mycase_test.go          the tests, in package acceptance_test
  contract_case_test.go   wires the case into the shared suite
  datapagesgen/           generated, committed
  cmd/server/             generated once, committed; compiled by every run
  acceptance.json         optional, see below
```

Copy an existing case, rewrite `go.mod`, then run `mage genDatapages`.

## The client

[`client`](client) speaks the protocol so that a case does not have to. It
holds what every case does the same way — the headers a Datastar request
carries, reading an SSE stream in the background, the page load that mints a
tab's instance id — and leaves a case with the requests that are its own.

```go
c := client.New(t, datapagesgen.NewServer(&app.App{}, inmem.New(8)))

resp := c.Get(t, href.PageIndex())            // no content negotiation
require.Equal(t, "term=x", resp.Element(t, "echo"))
c.Action(t, http.MethodPost, "/save/", `{"x":1}`)

s := c.OpenStream(t, "/_$/", signals)         // connect-time signals, or nil
require.True(t, s.Saw(`<div id="out">`))      // waits a second for it
require.True(t, s.Never("other tab"))         // waits the window out

tab := c.OpenTab(t, "/", "")                  // page load -> instance id -> stream
tab.Act(t, http.MethodPost, "/update/", body) // carries Datapages-Instance
tab.Close(); tab.Reopen(t)                    // drop and reconnect
```

It names no generated code, so a case keeps everything the model decides.
Assertions use `testify/require`.

Two things to know when writing requests:

- Responses are compressed by default. The client asks for
  `Accept-Encoding: identity`, which keeps the bytes a test reads the bytes
  the handler wrote.
- The in-memory broker matches subjects the way NATS does: `*` covers one
  token and `>` covers the rest. See `wildcardsubjects`.

### `acceptance.json`

| field | meaning |
| ----- | ------- |
| `no_race` | run the case without the race detector |

Everything that shapes generation — the app directory, the generated package
name, Prometheus, assets — is in `datapages.yaml`, the same file an
application of a user has.

## The shared contract suite

[`contract`](contract) holds the assertions that apply to every generated
server regardless of the application: the page shell, compression, URL
canonicalization, `ListenAndServe`, `ListenAndServeTLS`, `Shutdown` closing
open streams, the `main.go` options, opening and reopening an SSE stream, an
event dispatched from an action arriving on it, per-tab state released after
its grace period, and every URL `href` and `action` build addressing a real
route.

The server is generated per model, so the same assertion runs against
different code in each case. A case joins with one test:

```go
func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			return datapagesgen.NewServer(&app.App{}, inmem.New(8),
				contract.Options[datapagesgen.ServerOption](opts)...)
		},
		WithAssets: contract.Opt(datapagesgen.WithAssets),
		// …
	})
}
```

The suite cannot name a case's generated package: it is a different package in
every case. The `Case` therefore carries the generated symbols the assertions
need, wired in by the case. `contract.Opt` adapts an option constructor and
`contract.Options` converts the values back on the way in. Fields left nil skip
the assertions that need them.

## Recording a defect

There are none open, and the suite has no way to record one: a case either
passes or the build is broken. A defect the framework has not fixed yet is
covered by a case that asserts what the framework does today, named after that
behaviour and saying at the assertion what the correct one would be:

```go
// Fixing this turns the case red here, which is the signal to assert the
// correct behaviour instead.
require.Equal(t, http.StatusOK, resp.Status,
	"a URL no page claims is still answered with 200")
```

[`TestCompileFixtures`](../../generator/compile_test.go) builds the generated
code of every parser fixture and accepts no exceptions.
