# Acceptance cases

Each directory here with a `go.mod` is one case: an application,
the code generated from it, and the tests that send it requests over HTTP.

Reading generated code as text checks one version of it. These cases send
requests and read responses, which checks the behaviour every version must provide.

A case is a module of its own, which keeps it out of `go build ./...` and
`go test ./...` of the root module. The generated code is committed,
and a case runs by hand:

```sh
(cd internal/acceptance/routing && go test -race ./...)
```

`mage genDatapages` regenerates every case and every example.

## The runner

[`acceptance_test.go`](acceptance_test.go) is a plain test package of the root module.
Per case it:

1. scans the module for its `datapages.NewServer` calls, parses each `app` package
   they name, generates into a temporary directory, and compares the result with the
   committed `app/datapagesgen`.
   A difference fails with *run: mage genDatapages*.
   The case then runs what the generator writes today;
2. runs `go test -race -count=1 -coverpkg=<generated packages> ./...` inside the
   module and merges the coverage profile.

`go test ./internal/acceptance/ -run 'TestAcceptance/routing'` runs one case.
`-short` skips all of them.

Don't pass `-race` to the runner. It only shells out `go test` per case, and each
case is already run with the race detector. What the outer flag instruments is
the parser and the generator inside `requireGeneratedIsCurrent`, which triples
the wall time of the run and covers no shipped code.

The flag `-parallel` bounds how many cases run in parallel. It defaults to `GOMAXPROCS`.

Two rules a new test has to keep:

- Anything reaching the NATS server has to go through `brokers.Conn`,
  which serializes it against the other tests of the case.
  One server serves the whole case and its subject space is shared.
- Building a server sets the package-level logger of the generated `href` package.
  `contract.Run` runs its `ExternalHref` test before it starts the parallel ones,
  and the `TestContract` of a case therefore does not call `t.Parallel`.

`anonstreams`, `events`, `sessions` and `wildcardsubjects` assert against both
brokers datapages ships. Their `TestMain` runs `brokers.Main`, which starts a
real NATS server in a container and needs a running Docker daemon.
Without one they fail with *starting NATS container*.
Every other case runs on the in-memory broker and needs nothing.

## Writing a case

Copy an existing case for the layout, rewrite `go.mod`, then run
`mage genDatapages`. The `cmd/server` a case carries is
generated once and committed; every run compiles it.

`mustNewServer` in `newserver_test.go` wraps `datapages.NewServer` with the
case's four type arguments and fails the test on a configuration error, which
no test can carry on from.

## The client

[`client`](client) speaks the protocol for a case: the headers a Datastar
request carries, and reading an SSE stream in the background.
A case writes only the requests that are its own.

It names no generated code. A case keeps everything the model decides.
[`multiapp/instances_test.go`](multiapp/instances_test.go) uses a page load,
an action, a stream and a cookie jar.

Responses are compressed by default. The client asks for
`Accept-Encoding: identity`, which makes what a test reads the bytes the
handler wrote. The in-memory broker matches subjects the way NATS does: `*`
covers one token, `>` covers the rest. See `wildcardsubjects`.

## Where the calls live

A case usually keeps one `datapages.NewServer` call in `cmd/server/main.go` and
one in `newserver_test.go`. A module is not held to that:

- `multicall` builds one application from four packages: the command, a library
  package beside it, one under `internal/`, and the test package. Every call
  names the same app and the same four type arguments, which every call naming
  one app package has to do. The case builds a server from each constructor and
  asserts they serve the same routes.
- `multiapp` builds two applications whose type arguments are the opposite of
  each other: `app/frontend` names a session type and
  `datapages.DisablePrometheus`, `app/admin` names `datapages.DisableSessions`
  and `datapages.EnablePrometheus`. Neither is a property of the module. One run
  generates the session handling into one package and the metrics
  instrumentation into the other, and each generated server refuses the option
  it was not generated for. Both `NewServer` calls sit in `serve/` subpackages;
  the command picks one at startup. `instances_test.go` then runs four servers
  at once, two per app, which is where package-level state in generated code
  would show.

Neither case asserts what the scan read. A call the scan misses generates
nothing, and the case stops compiling.

### Configuration

A case configures nothing. What shapes generation is in the code: the app directory,
the generated package and the metrics mode are the type arguments of the
`datapages.NewServer` calls the module holds, wherever they are written,
which `serverscan.Scan` reads, and the assets come from the `embed.FS` the app package
declares. No case carries a `datapages.yaml`.

The runner reads an optional `acceptance.json` next to the `go.mod`
(`readCaseOptions` in `acceptance_test.go`) with one field, `no_race`, which
drops the race detector for that case. Add the file only when a case is too
slow or too noisy under `-race`, and say in the case why.

## The shared contract suite

[`contract`](contract) holds the assertions that apply to every generated server,
whatever the application: the page shell, compression, URL canonicalization,
`ListenAndServe`, `ListenAndServeTLS`, `Shutdown` closing open streams,
the `main.go` options, opening and reopening an SSE stream, an event dispatched from
an action arriving on it, and every URL `href` and `action` build addressing a real route.

The server is generated per model. The same assertion therefore runs against
different code in each case. A case joins with one `TestContract` that fills in
a `contract.Case`; every `contract_case_test.go` here is an example. See
[`minimal`](minimal/contract_case_test.go).

The generated package differs per case, and the suite cannot name it. `Case`
therefore carries the generated symbols the assertions need. `contract.Opt`
adapts an option constructor and `contract.Options` converts the values back on
the way in. Fields left nil skip the assertions that need them.

## Recording a defect

The suite has no way to record an open defect: a case either passes or the
build is broken. Cover a defect the framework has not fixed yet with a case
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
