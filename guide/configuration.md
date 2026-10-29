# datapages.yaml

`datapages.yaml` names the entry point that `datapages gen` writes for a new project. It also configures the `datapages watch` development server. The file has 36 supported keys.

## The scaffold file

`datapages init` and `datapages watch` write this file into a module that has none:

```yaml title="datapages.yaml"
cmd: cmd/server
watch:
  exclude:
    - ".git/**" # git internals
    - ".*"      # hidden files/directories
    - "*~"      # editor backup files
```

[`cmd`](#cmd) names the directory where `datapages gen` writes the entry point while the module has no `datapages.NewServer` call. The [`exclude`](#exclude) patterns keep changes to these files from rebuilding the app. `.*` and `*~` match only names in the module root, because `*` does not match `/`.

With `--web-components typescript`, `init` adds a watcher that bundles the TypeScript sources, and keeps `web/node_modules/` away from the file watcher:

```yaml title="datapages.yaml"
cmd: cmd/server
watch:
  exclude:
    - ".git/**" # git internals
    - ".*"      # hidden files/directories
    - "*~"      # editor backup files
  watcher-ignore:
    # The tree is large and would use up watch descriptors.
    - "web/node_modules/**"
  custom-watchers:
    - name: "Bundle the Web Components (esbuild)"
      include:
        - "web/src/**/*.ts"
      cmd: "cd web && node build.mjs"
      fail-on-error: true
      requires: reload
```

## Location and loading

- The file is in the module root, next to `go.mod`, and is named `datapages.yaml` or `datapages.yml`. Both at once are an error.
- `datapages gen`, `datapages watch` and `datapages init` read it. `datapages lint` and `datapages version` do not.
- Without the file, every key has its default. `watch` and `init` then write the scaffold file, and `gen` does not. A file that `watch` writes applies from its next start.
- `watch` reads the file once, when it starts. A change to the file makes `watch` run `datapages gen`, which reads `cmd` again. The `watch` settings apply from the next start.

The parser accepts a strict subset of YAML:

- A key this page does not list is an error, such as `field app not found in type config.Config`.
- A boolean is `true` or `false`. `yes`, `on` and `True` are errors.
- A duration is a Go duration string, such as `50ms`, `2s` or `1m30s`. A bare number is an error.
- A list item without a value is an error.
- `null` is accepted for `watch`, `tls`, `compiler`, the lists and `env`, and is an error for every other key, `log` included. An empty value, as `cmd:` or `debounce:`, gives the default.
- An empty file is an error, and so is a file with only comments.

[Validation](#validation) lists the checks that run when `watch` starts.

## Keys

| Key | Type | Default |
| --- | ---- | ------- |
| [`cmd`](#cmd) | string | `cmd/server` |
| [`watch`](#watch) | mapping | none |
| [`watch.app-host`](#app-host) | URL | `http://localhost:8080` |
| [`watch.proxy-timeout`](#proxy-timeout) | duration | `2s`, no effect |
| [`watch.flags`](#flags) | string | `""` |
| [`watch.dir-work`](#dir-work) | path | the module root |
| [`watch.debounce`](#debounce) | duration | `50ms` |
| [`watch.exclude`](#exclude) | list of globs | none |
| [`watch.watcher-ignore`](#watcher-ignore) | list of globs | none |
| [`watch.format`](#format) | boolean | `false` |
| [`watch.lint`](#lint) | boolean | `false` |
| [`watch.log`](#watchlog) | mapping | the defaults of its keys |
| [`watch.log.level`](#level) | `erronly`, `verbose` or `debug` | `erronly` |
| [`watch.log.clear-on`](#clear-on) | `restart` or `file-change` | `""`, no effect |
| [`watch.log.print-js-debug-logs`](#print-js-debug-logs) | boolean | `false` |
| [`watch.tls`](#watchtls) | mapping | none, plain HTTP |
| [`watch.tls.cert`](#cert) | path | none |
| [`watch.tls.key`](#key) | path | none |
| [`watch.compiler`](#watchcompiler) | mapping | none |
| [`watch.compiler.gcflags`](#watchcompiler) | string | `""` |
| [`watch.compiler.ldflags`](#watchcompiler) | string | `""` |
| [`watch.compiler.asmflags`](#watchcompiler) | string | `""` |
| [`watch.compiler.tags`](#watchcompiler) | list of strings | none |
| [`watch.compiler.race`](#watchcompiler) | boolean | `false` |
| [`watch.compiler.trimpath`](#watchcompiler) | boolean | `false` |
| [`watch.compiler.msan`](#watchcompiler) | boolean | `false` |
| [`watch.compiler.p`](#watchcompiler) | integer | `0` |
| [`watch.compiler.env`](#watchcompiler) | map of strings | none |
| [`watch.custom-watchers`](#watchcustom-watchers) | list of mappings | none |
| [`watch.custom-watchers[].name`](#name) | string | required |
| [`watch.custom-watchers[].include`](#include) | list of globs | none |
| [`watch.custom-watchers[].exclude`](#exclude-1) | list of globs | none |
| [`watch.custom-watchers[].cmd`](#cmd-1) | shell command | `""` |
| [`watch.custom-watchers[].fail-on-error`](#fail-on-error) | boolean | `false` |
| [`watch.custom-watchers[].debounce`](#debounce-1) | duration | `50ms` |
| [`watch.custom-watchers[].requires`](#requires) | `reload`, `restart` or `rebuild` | `""` |

## `cmd`

Type string. Default `cmd/server`.

The directory of the main package, relative to the module root. An absolute path, or a path that leaves the module through `..`, is an error, as `invalid cmd "../server": must be a relative path inside the module`.

`cmd` matters while no main package of the module calls `datapages.NewServer`:

- `datapages gen` writes `<cmd>/main.go` with a call for the app package `app/` in a module without a call. It writes nothing when the directory holds Go files of package `main`, and fails when they belong to another package.
- `datapages watch` builds `<cmd>` when no main package calls `datapages.NewServer` for the app package it runs, as when the only call is in a test.
- `datapages init` names `<cmd>/` as the server command in `AGENTS.md` of a module with 1 app package and no main package with a call.

The first `datapages gen` writes `main.go` with the call. From then on `gen` ignores `cmd`, and `watch` builds the main package that holds the call. To move the entry point later, move its directory.

Set `cmd` before the first `datapages gen` of a new module to generate the entry point into another directory:

```yaml title="datapages.yaml"
cmd: cmd/web
```

## `watch`

Type mapping. Default none, which gives every key below its default.

The settings of `datapages watch`, which runs [templier](https://github.com/romshark/templier) v0.13.4. [`datapages watch`](cli.md#datapages-watch) describes what it does on startup and on a file change.

### Dev server and app

#### `app-host`

Type string, an `http` or `https` URL with a host. Default `http://localhost:8080`.

The address of the app. The dev server forwards every request to it. After it starts the app, `watch` sends `OPTIONS` requests to this URL every 100 ms until any response arrives, for about 10 seconds at most, then reloads the open tabs. Make the app listen here. The `cmd/server/main.go` of the scaffold listens on `HOST` and `PORT` from the environment or `.env`, `localhost:8080` by default.

A value without a scheme, such as `localhost:8080`, stops `watch` at startup with `watch.app-host "localhost:8080" must be an http or https URL with a host, such as http://localhost:8080`.

For an app that serves HTTPS, use an `https` URL. The startup requests accept any certificate. The forwarded requests check the certificate of the app against the root certificates of the system.

The address of the dev server itself is the `--host` flag of `datapages watch`, `localhost:7331` by default.

#### `proxy-timeout`

Type duration. Default `2s`.

No effect. templier v0.13.4 accepts the value and uses it nowhere. A forwarded request waits for the app without a time limit. A forwarded request that fails before a response arrives, such as while the app restarts, is tried up to 20 times, with pauses that grow from 100 ms.

#### `flags`

Type string. Default `""`.

The arguments of the app process. `watch` splits the string at runs of white space. It interprets no quotes, which turns `-name "a b"` into the 3 arguments `-name`, `"a` and `b"`. The `main.go` of the scaffold reads no flags. It takes its settings from environment variables.

```yaml title="datapages.yaml"
watch:
  flags: -verbose -addr localhost:8080
```

#### `dir-work`

Type string. Default the module root.

The working directory, relative to the module root. `watch` joins the value to the module root, a leading `/` included, and `..` can leave the module. The directory must exist. `watch` uses it as:

- the working directory of the app, which resolves relative paths such as the `.env` that the scaffold reads, and the directory of static files in dev mode, as `./app/static`
- the working directory of the custom watcher commands, `datapages gen` included, of `templ fmt` for [`format`](#format) and of `golangci-lint` for [`lint`](#lint)
- the base of the [`exclude`](#exclude) patterns and of the `include` and `exclude` patterns of custom watchers

`go build` runs in the directory that `watch` was started in, and `templ generate --watch` in the module root, whatever `dir-work` says. [`watcher-ignore`](#watcher-ignore) patterns are relative to the module root.

> [!WARNING]
> Leave `dir-work` at the module root. The patterns of the built-in `datapages gen` watcher are relative to the module root and match no file relative to another directory, which stops a changed `.go` file of an app package from regenerating the code. In dev mode the app also serves no static files, because it reads them from `./<app package>/<assets directory>` relative to its working directory.

### Rebuilds

#### `debounce`

Type duration. Default `50ms`, also for `0`.

The time `watch` waits after the last change before rebuilding the app. Each change restarts the wait. A burst of changes, such as a `git checkout`, therefore causes 1 build. This value applies to rebuilds requested by file changes and templ. Custom watchers have their own [`debounce`](#debounce-1). A negative value makes `watch` panic at startup.

#### `exclude`

Type list of glob patterns. Default none. The scaffold sets 3.

A changed file whose path matches a pattern does not rebuild the app. Custom watchers that include the file still run. `watch` matches the path relative to [`dir-work`](#dir-work), with `/` as the separator on every system.

The patterns use the syntax of [doublestar](https://github.com/bmatcuk/doublestar). `*` matches within 1 path element, `**` matches any number of elements, and `?`, `[...]` and `{a,b}` work as in shell globs:

| Pattern | Matches | Does not match |
| ------- | ------- | -------------- |
| `.*` | `.env`, `.git` | `app/.hidden`, `.vscode/settings.json` |
| `**/.*` | `.env`, `app/.hidden` | `.vscode/settings.json` |
| `**/.*/**` | `.env`, `.vscode/settings.json` | `app/app.go` |
| `*~` | `notes.txt~` | `app/app.go~` |
| `**/*~` | `notes.txt~`, `app/app.go~` | |
| `.git/**` | `.git`, `.git/index` | `.gitignore` |

#### `watcher-ignore`

Type list of glob patterns. Default none.

Paths that the file watcher leaves out. Unlike [`exclude`](#exclude), a matching file produces no event at all, for custom watchers too, and a matching directory gets no watch. `watch` matches the path relative to the module root, with the syntax of `exclude`.

Use it for large trees that would use up the file watches of the system, such as `node_modules`. Write the pattern to match everything below the directory, as `web/node_modules/**` does. `web/node_modules` alone leaves the directories below it watched.

### Formatting and linting

#### `format`

Type boolean. Default `false`.

`true` runs `templ fmt -fail <file>` in [`dir-work`](#dir-work) on each `.templ` file that is written or created, which rewrites the file in the format of templ. After a file it changed, templ exits with status 1, and `watch` logs the error `templ formatting error`. The rebuild continues.

#### `lint`

Type boolean. Default `false`.

`true` runs `golangci-lint run <module root>/...` in [`dir-work`](#dir-work) with every build, in parallel with `go build`. When golangci-lint exits with status 1, the error page shows its output and `watch` does not start the new build. Another exit status is only logged. `watch` stops at startup when `golangci-lint` is not on the PATH.

### `watch.log`

Type mapping. `log: null` is an error.

#### `level`

Type string, `erronly`, `verbose` or `debug`. Default `erronly`, also for `""`.

What `watch` logs to stderr, in the text format of Go's `log/slog`:

| Value | Logs |
| ----- | ---- |
| `erronly` | Errors |
| `verbose` | Errors, warnings and progress, such as `templier started` with the dev server URL, the build times and the restarts of the app |
| `debug` | Also every command `watch` runs, every file event and the output of templ |

Warnings, such as `unsupported templ version`, show only at `verbose` and `debug`. The output of the app shows at every level.

#### `clear-on`

Type string, `restart` or `file-change`. Default `""`.

No effect. templier clears the terminal through a function that its caller passes, and `watch` passes none. The value is still checked. Another one fails the loading of the file with `invalid clear-on "always", use: restart, file-change`.

#### `print-js-debug-logs`

Type boolean. Default `false`.

`true` makes the script that the dev server adds to every page log the state of its WebSocket connection to the browser console with `console.debug`. The messages start with `templier:`.

### `watch.tls`

Type mapping. Default none, which serves the dev server over plain HTTP.

With the section, the dev server on `--host` serves HTTPS with this certificate, and the address to open becomes `https://localhost:7331`. Both keys are required. `tls: {}` stops `watch` at startup with `engine: TLS.Cert is required when TLS is set`.

#### `cert`

Type string. Default none.

The path of the PEM file with the certificate. A relative path is relative to the directory that `watch` was started in, which is the module root.

#### `key`

Type string. Default none.

The path of the PEM file with the private key of the certificate, relative like `cert`.

A file that cannot be read stops `watch` after startup with the error of the TLS setup of Go. The section configures the dev server only. The `main.go` of the scaffold serves HTTPS when `PATH_TLS_CERT` and `PATH_TLS_KEY` name the files. Set [`app-host`](#app-host) to an `https` URL then.

```yaml title="datapages.yaml"
watch:
  tls:
    cert: .dev/localhost.pem
    key: .dev/localhost-key.pem
```

### `watch.compiler`

Type mapping. Default none, which builds without extra flags.

The flags of the `go build` that `watch` runs, as `go build <flags> -o <temporary file> ./<main package>/`. `watch` adds the flags in the order of this table:

| Key | Type | Default | Adds |
| --- | ---- | ------- | ---- |
| `gcflags` | string | `""` | `-gcflags <value>` |
| `ldflags` | string | `""` | `-ldflags <value>` |
| `asmflags` | string | `""` | `-asmflags <value>` |
| `tags` | list of strings | none | `-tags <tag>,<tag>` |
| `race` | boolean | `false` | `-race` |
| `trimpath` | boolean | `false` | `-trimpath` |
| `msan` | boolean | `false` | `-msan` |
| `p` | integer from 0 to 4294967295 | `0` | `-p <n>`, nothing for `0` |
| `env` | map of strings | none | `<NAME>=<value>` in the environment of `go build` |

An empty string, an empty list and `false` add nothing. Each string goes to `go build` as 1 argument, without shell quoting, which passes `all=-N -l` whole to `-gcflags`. `env` applies to `go build` only, not to the app, and overrides a variable of the same name in the environment of `watch`. The flags apply only to the builds of `watch`.

```yaml title="datapages.yaml"
watch:
  compiler:
    gcflags: all=-N -l
    tags:
      - dev
    env:
      CGO_ENABLED: "1"
```

### `watch.custom-watchers`

Type list of mappings. Default none.

Each entry runs a command when a file it includes changes, then takes an action, such as reloading the tabs. `watch` runs the command of every watcher once at startup, in list order, before the first build. A built-in watcher comes before the configured ones:

| Key | Value of the built-in watcher |
| --- | ----------------------------- |
| `name` | `datapages gen` |
| `include` | `<app>/**/*.go` for every app package, `<dir>/*.go` for every package with a `datapages.NewServer` call, `datapages.yaml`, `datapages.yml` |
| `exclude` | `<app>/datapagesgen/**` for every app package |
| `cmd` | The running `datapages` binary with the argument `gen` |
| `fail-on-error` | `true` |
| `requires` | `rebuild` |

When a change triggers several watchers, each runs its command. `watch` then takes the strongest action they require, in the order `rebuild`, `restart`, `reload`, none. A change that triggers a watcher gets that action instead of the default rebuild. With `rebuild`, a file that [`exclude`](#exclude) matches, or whose name ends in `_templ.go`, still does not rebuild.

#### `name`

Type string. Required.

The name of the watcher in the log and above its output on the error page. A watcher without a name stops `watch` at startup with `engine: CustomWatchers[1] has no name` for the first watcher of the list, because the watcher that runs `datapages gen` comes first.

#### `include`

Type list of glob patterns. Default none.

The files that trigger the watcher, in the syntax of [`exclude`](#exclude) and relative to [`dir-work`](#dir-work). A watcher without `include` runs only at startup.

#### `exclude`

Type list of glob patterns. Default none.

Files that `include` matches and that do not trigger the watcher.

#### `cmd`

Type string. Default `""`.

The shell command, run with `sh -c`, or `cmd /C` on Windows, in [`dir-work`](#dir-work). Its environment is that of `watch` plus `FORCE_COLOR=1` and `CLICOLOR_FORCE=1`. A watcher may leave out `cmd` when it sets `requires`. A watcher with neither stops `watch` at startup. When the first word of the command is no program on the PATH, `watch` logs a warning, which shows at the `verbose` and `debug` levels, and runs the command anyway, because it may be a shell builtin.

#### `fail-on-error`

Type boolean. Default `false`.

With `true`, a command that exits with status 1 puts its output on the error page, which replaces the app until a later run of the command succeeds. The action of `requires` does not run. With `false`, `watch` logs status 1 as an error and skips the action. With either value, `watch` logs another non-zero status, or a command that cannot start, and takes the action anyway.

#### `debounce`

Type duration. Default `50ms`, also for `0`.

The time the watcher waits after the last change of a file it includes before it runs its command. A negative value makes `watch` panic at startup.

#### `requires`

Type string, `reload`, `restart` or `rebuild`. Default `""`.

The action after the command succeeds:

| Value | Action |
| ----- | ------ |
| `""` | None. The command runs and nothing else happens |
| `reload` | Reload the open tabs |
| `restart` | Stop the app, start the same binary again, then reload the tabs |
| `rebuild` | Build the app again, with [`lint`](#lint) when it is on, restart it and reload the tabs |

## Validation

`gen`, `watch` and `init` check the file when they load it. They reject a YAML syntax error, an unknown key, a value of the wrong type, a `level`, `clear-on` or `requires` value they do not know, and a `cmd` outside the module.

`watch` checks the rest when it starts and stops on the first error:

- `app-host` is an `http` or `https` URL with a host.
- Each pattern in `exclude`, `watcher-ignore` and the `include` and `exclude` of the custom watchers is a valid glob. `[` is not.
- Each custom watcher has a `name`, and a `cmd` or a `requires`.
- A `tls` section has `cert` and `key`.
- `templ` is on the PATH, and so is `golangci-lint` when `lint` is `true`.

Except for the `app-host` check, these errors start with `initializing watch engine:`. Nothing checks that `dir-work` and the files of `tls` exist before `watch` uses them.

## Complete example

This file sets every key. `proxy-timeout` and `clear-on` have no effect, and `asmflags`, `trimpath` and `msan` keep their defaults:

```yaml title="datapages.yaml"
cmd: cmd/server
watch:
  app-host: http://localhost:8080
  proxy-timeout: 2s # no effect
  flags: -verbose
  dir-work: . # the module root
  debounce: 100ms
  exclude:
    - ".git/**"
    - "**/.*"
    - "**/*~"
    - "docs/**"
  watcher-ignore:
    - "web/node_modules/**"
  format: true
  lint: true
  log:
    level: verbose
    clear-on: restart # no effect
    print-js-debug-logs: true
  tls:
    cert: .dev/localhost.pem
    key: .dev/localhost-key.pem
  compiler:
    gcflags: all=-N -l
    ldflags: -X main.version=dev
    asmflags: ""
    tags:
      - dev
    race: true
    trimpath: false
    msan: false
    p: 4
    env:
      CGO_ENABLED: "1"
  custom-watchers:
    - name: Bundle the Web Components (esbuild)
      include:
        - "web/src/**/*.ts"
      exclude:
        - "web/src/**/*.test.ts"
      cmd: cd web && node build.mjs
      fail-on-error: true
      debounce: 200ms
      requires: reload
    - name: Restart on a change of .env
      include:
        - ".env"
      requires: restart
```

The second watcher restarts the app when `.env` changes. The `main.go` of the scaffold reads `.env` when it starts. `**/.*` in `exclude`, like `.*` in the scaffold file, keeps a change of `.env` from rebuilding the app. The watcher still runs, because `exclude` does not apply to custom watchers.

## See also

- [`datapages watch`](cli.md#datapages-watch): what the dev server does on startup and on a file change, and its flags.
- [App packages](cli.md#app-packages): how the commands find the app packages and the main package to build.
- [Static assets](assets.md): the static files that dev mode reads from the source tree.
- [Dev Mode](../SPECIFICATION.md#dev-mode): what `TEMPL_DEV_MODE`, which `watch` sets for the app, changes in the server.
- [Troubleshooting](troubleshooting.md): dev server problems.
