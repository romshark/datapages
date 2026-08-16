# Generator tests

Three tests cover this package. Each answers a different question.

| test | question |
| ---- | -------- |
| [`TestCompileFixtures`](compile_test.go) | does the generator emit code that builds, for every model shape the parser accepts? |
| [`TestGeneratePartialModels`](partial_test.go) | does the generator refuse an application the parser rejected, without writing anything? |
| [`TestExamplesAreUpToDate`](generator_test.go) | is the generated code committed under `example/` still what the generator produces? |

A fourth question is what the generated code does when it runs. That one is
answered outside this package, by the acceptance cases under
[../acceptance](../acceptance). Each is an application with its generated code
committed next to it, driven over HTTP.

Only `TestExamplesAreUpToDate` reads generated source. Every other assertion is
a request, a response, a call to a generated function, or a value the
application recorded while a generated handler ran.

## TestCompileFixtures

Every acceptance case builds its module too. This looks redundant. It is not,
for three reasons:

- A fixture whose generated code does not compile cannot be an acceptance case.
  A module that does not build never reaches an assertion. This test accepts no
  exceptions.
- A `go build` per fixture is cheap and reuses fixtures the parser maintains. A
  fixture added by parser work is covered the day it lands.
- It builds the app package in a directory named `pages` while the package is
  named `app`. Both are free choices in `datapages.yaml`. No acceptance case
  uses that combination.

## TestGeneratePartialModels

The parser returns a partial model next to its errors, and any caller can pass
that model to `Generate`. It describes an application nobody wrote. `Generate`
validates it and returns an error. It also writes nothing unless every file
renders. On error the destination is left as it was.

`datapages gen` stops before it generates when the app package does not parse.
Generated code keeps what the last successful run produced. A package that was
never generated is written as stubs. Stubs hold no application code and let the
import resolve. See `TestGenFailure*` in [../cmd/cmd_test.go](../cmd/cmd_test.go).

## Coverage

`mage coverage` reports how much of this package the suite runs, next to how
much of the code it writes the acceptance cases run.

```
generator (./internal/generator/...):  89.9% of statements
code it generates:           77.6% of statements (run by the acceptance cases)
```
