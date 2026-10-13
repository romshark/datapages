# Counter

A real-time collaborative counter, built 2 times in 1 module.

This is what a multi-application module looks like. 2 app packages, 2 generated packages, 2 entry points:

```
app/simple/                    cmd/simple/
app/simple/datapagesgen/
app/fancy/                     cmd/fancy/
app/fancy/datapagesgen/
```

- `simple` is the bare-bones version: the smallest thing that counts.
- `fancy` is the same model with animated digits and a polished shell.

Generated code always goes into a `datapagesgen` package directly under the app package it belongs to, which is what lets the 2 live side by side.

## Run

```sh
go run ./cmd/simple   # http://localhost:8080/
go run ./cmd/fancy    # http://localhost:8081/
```

## Develop

1 dev server runs 1 application, so pass the directory of the one to run:

```sh
datapages watch --app app/simple
datapages watch --app app/fancy
```
