# Toxiproxy settings page

The settings page of the Toxiproxy front of `datapages watch`, a Datapages application. `datapages watch` serves it on `watch.toxiproxy.ui-host`, by default the port after the proxy, as `http://localhost:7333/`.

The settings live in `toxic.Front` in [`internal/cmd/toxic`](..), which the page reads and changes. [`internal/cmd/toxiproxy.go`](../../toxiproxy.go) builds the server.

| File | Holds |
| ---- | ----- |
| `app.go` | the page and its actions |
| `view.go` | what the templates show, including the search of the endpoint list |
| `app.templ` | the templates, with [Morpheus](https://github.com/romshark/morpheus) components |
| `presets.templ` | the preset combobox of the header and its dialogs |
| `static/ui.css` | the styles, including the Morpheus tokens mapped to the theme |
| `static/morpheus/` | the Morpheus bundle, generated |
| `static/icons/` | the [Lucide](https://lucide.dev) icons of the page and of the Morpheus components it uses, and the logo of Datapages |
| `datapagesgen/` | generated code, never edit |

## Change it

After a change to the pages or actions in `app.go`, regenerate `datapagesgen`:

```sh
mage genDatapages # or: go run ./internal/tools/genapp internal/cmd/toxic/ui
```

`datapages gen` refuses this module because its runtime packages import each other, which is why `genapp` exists.

After a change to `app.templ`, run `mage genTempl`. Never run it while `datapages watch` runs.

Before using another Morpheus component, add it to `morpheusComponents` in `magefiles/mage.go` and run `mage genMorpheus`.

To draw another icon, copy its SVG from `lucide-static`, at the version in the license comment of the others, into `static/icons/`. `static/icons/datapages.svg` is a copy of the logo in [`docs/logo.svg`](../../../../docs/logo.svg).

## Try it

```sh
go install ./cmd/datapages
cd example/todolist && datapages watch
```

Open the settings address watch prints. The CLI embeds the page, so a change needs a new `go install` and a restart of watch.

## Test

```sh
go test ./internal/cmd/toxic/...
go test ./internal/generator -run TestToxicUIIsUpToDate
```

The second test fails when `datapagesgen` is stale.
