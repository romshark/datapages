# Generator tests

Four tests cover this package. Each answers a different question.

| test | question |
| ---- | -------- |
| [`TestAcceptance`](acceptance_test.go) | does the generated code behave correctly when it runs? |
| [`TestCompileFixtures`](compile_test.go) | does the generator emit code that builds, for every model shape the parser accepts? |
| [`TestGeneratePartialModels`](partial_test.go) | does the generator refuse an application the parser rejected, without writing anything? |
| [`TestExamplesAreUpToDate`](generator_test.go) | is the generated code committed under `example/` still what the generator produces? |

Nothing except `TestExamplesAreUpToDate` reads generated source. Assertions are requests, responses, calls to generated functions, or values the application recorded while a generated handler ran.

## TestAcceptance

Each directory under [testdata/acceptance](testdata/acceptance) is one application plus its own tests. The harness copies a case into a throwaway module, parses its `app` package, runs the generator, then runs `go test -race` inside the module.

Steps per case, from `runAcceptanceCase`. Directories starting with `_` are not cases.

1. Read `options.json`, if present. Create the module directory, a `t.TempDir()` unless `-keep` is set.
2. Copy the case in without `options.json`. Write `go.mod` and `go.sum` naming the module `dpacceptance`, with versions from `example/classifieds` and `github.com/romshark/datapages` replaced by the working tree.
3. Copy in `_contract/contract_test.go`, unless the case records a bug.
4. Write the `httperr` subpackage, which an app package may import.
5. Parse the app package. Any parser error fails the case. Generate, plus `cmd/server` when `cmd` is set.
6. Run `go test -race -count=1 -coverpkg=./datapagesgen/... ./...` with `GOPROXY=off` and `GOFLAGS=-mod=mod`. Merge the coverage profile, or check the recorded failure for a `bug_` case.

### Running one case

```sh
go test ./generator/ -run 'TestAcceptance/routing' -count=1
go test ./generator/ -run TestAcceptance -count=1 -v   # with the coverage table
go test ./generator/ -short                            # skip every case
```

`-short` also skips `TestCompileFixtures`. Both build a module per case. `TestExamplesAreUpToDate` still runs.

`TestAcceptance` registers three flags. `go test` forwards them after `-args`.

| flag | effect |
| ---- | ------ |
| `-keep=DIR` | write each case module under `DIR` and leave it there |
| `-cover.out=FILE` | write the merged coverage profile of the generated code |
| `-cover.min=N` | fail when total coverage of the generated code is below N percent |

`-keep` leaves a complete Go module to read, edit and rerun:

```sh
go test ./generator/ -run 'TestAcceptance/routing' -count=1 -args -keep=/tmp/cases
cd /tmp/cases/routing && go test ./... -v && go tool cover -func=cover.out
```

### Writing a case

```
testdata/acceptance/mycase/
  app/app.go                the application, in package app
  mycase_test.go            the tests, in package acceptance_test
  contract_case_test.go     wires the case into the shared suite
  options.json              optional
```

The module is always `dpacceptance`. A case imports itself as
`dpacceptance/app` and `dpacceptance/datapagesgen`, and may import
`dpacceptance/datapagesgen/httperr`.

Two things to know when writing requests:

- Responses are compressed by default. `Accept-Encoding: identity` keeps the bytes a test reads the bytes the handler wrote.
- The in-memory broker matches subjects as exact map keys. A subscription with a NATS wildcard never receives anything. See `bug_inmem_wildcard_subjects`.

#### `options.json`

| field | meaning |
| ----- | ------- |
| `prometheus` | generate metrics instrumentation |
| `assets_url_prefix`, `assets_dir` | generate the assets subpackage and serve static files |
| `app_dir`, `gen_pkg` | override the default `app` and `datapagesgen` |
| `cmd` | also generate `cmd/server`, which the module compiles |
| `no_race` | run the case without the race detector |
| `known_bug`, `known_bug_reason` | see [bug_ cases](#bug_-cases) |

#### The shared contract suite

[`_contract/contract_test.go`](testdata/acceptance/_contract/contract_test.go) is copied into every case that does not record a bug. The server is generated per model. The same assertion therefore runs against different code in each case. It covers the page shell, compression, URL canonicalization, `ListenAndServe`, `ListenAndServeTLS`, `Shutdown` closing open streams, the `main.go` options, opening and reopening an SSE stream, an event dispatched from an action arriving on it, per-tab state released after its grace period, and every URL `href` and `action` build addressing a real route.

A case joins by declaring `contractCase` in `contract_case_test.go`. Fields are documented on the `contract` type. Only `newServer` is required, because the argument list of `NewServer` depends on the model.

### `bug_*` cases

A case whose `options.json` carries `known_bug` must **fail**, with that substring in its output. Each reproduces a defect in the generator or in a module the generated code uses. The reasoning is in the app package doc comment.

Recording a bug instead of deleting the test keeps it described in terms of what an application observes. Fixing it turns the case red until the entry is removed. `knownBroken` in [compile_test.go](compile_test.go) does the same for fixtures whose generated code does not compile.

## TestCompileFixtures

Every acceptance case compiles its module too. This looks redundant. Three reasons it is not:

- The four `knownBroken` fixtures cannot be acceptance cases. Their generated code does not compile, and a module that does not build never reaches an assertion.
- A `go build` per fixture is cheap and reuses fixtures the parser maintains. A fixture added by parser work is covered the day it lands.
- It builds the app package in a directory named `pages` while the package is named `app`. Both are free choices in `datapages.yaml`. No acceptance case uses that combination.

The acceptance cases cover the behaviour of every shape the parser accepts except one: a stateful page that also serves anonymous streams.

## TestGeneratePartialModels

The parser returns a partial model next to its errors, and any caller can pass it to `Generate`. That model describes an application nobody wrote. `Generate` validates it and returns an error. It also writes nothing unless every file renders. On error the destination is left as it was.

`datapages gen` stops before generating when the app package does not parse. Existing generated code keeps what the last successful run produced. A package that was never generated is written as stubs, which hold no application code and let the import resolve. See `TestGenFailure*` in [../internal/cmd/cmd_test.go](../internal/cmd/cmd_test.go).

## Coverage

Each case runs with coverage over the packages generated for it. `TestAcceptance` prints a per-case table and a total. `mage coverage` reports it with the coverage of the generator itself.

```
generator (./generator/...):  90.6% of statements
code it generates:            79.2% of statements (run by the acceptance suites)
```
