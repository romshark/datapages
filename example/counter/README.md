# Counter

A real-time collaborative counter, built twice in one module.

This is what a multi-application module looks like. Two app packages, two
generated packages, two entry points:

```
app/simple/                    cmd/simple/
app/simple/datapagesgen/
app/fancy/                     cmd/fancy/
app/fancy/datapagesgen/
```

- `simple` is the bare-bones version: the smallest thing that counts.
- `fancy` is the same model with animated digits and a polished shell.

Generated code always goes into a `datapagesgen` package directly under the app
package it belongs to, which is what lets the two live side by side.

## Run

```sh
go run ./cmd/simple   # http://localhost:8080/
go run ./cmd/fancy    # http://localhost:8081/
```

## Develop

One dev server runs one application, so name the one to run:

```sh
datapages watch --app simple
datapages watch --app fancy
```
