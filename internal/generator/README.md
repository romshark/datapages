# Generator tests

Three tests cover this package. Each answers a different question.

| test | question |
| ---- | -------- |
| [`TestCompileFixtures`](compile_test.go) | does the generator emit code that builds, for every model shape the parser accepts? |
| [`TestGeneratePartialModels`](partial_test.go) | does the generator refuse an application the parser rejected, without writing anything? |
| [`TestExamplesAreUpToDate`](generator_test.go) | is the generated code committed under `example/` still what the generator produces? |

A fourth question — does the generated code behave correctly when it runs? — is answered outside this package, by the acceptance cases under [../internal/acceptance](../internal/acceptance). They are applications with their generated code committed next to them, driven over HTTP.

Nothing except `TestExamplesAreUpToDate` reads generated source. Assertions are requests, responses, calls to generated functions, or values the application recorded while a generated handler ran.

## TestCompileFixtures

Every acceptance case builds its module too. This looks redundant. Three reasons it is not:

- A fixture whose generated code does not compile cannot be an acceptance case: a module that does not build never reaches an assertion. `knownBroken` is where such a fixture is recorded; it is empty.
- A `go build` per fixture is cheap and reuses fixtures the parser maintains. A fixture added by parser work is covered the day it lands.
- It builds the app package in a directory named `pages` while the package is named `app`. Both are free choices in `datapages.yaml`. No acceptance case uses that combination.

The acceptance cases cover the behaviour of every shape the parser accepts, a stateful page serving anonymous streams included (the `anonstreams` case).

## TestGeneratePartialModels

The parser returns a partial model next to its errors, and any caller can pass it to `Generate`. That model describes an application nobody wrote. `Generate` validates it and returns an error. It also writes nothing unless every file renders. On error the destination is left as it was.

`datapages gen` stops before generating when the app package does not parse. Existing generated code keeps what the last successful run produced. A package that was never generated is written as stubs, which hold no application code and let the import resolve. See `TestGenFailure*` in [../internal/cmd/cmd_test.go](../internal/cmd/cmd_test.go).

## Coverage

`mage coverage` reports how much of this package the suite runs, next to how much of the code it writes the acceptance cases run.

```
generator (./generator/...):  89.5% of statements
code it generates:            78.3% of statements (run by the acceptance cases)
```
