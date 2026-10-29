# Toxiproxy settings page

The settings page of the Toxiproxy front of `datapages watch`, a Datapages application on `watch.toxiproxy.ui-host`, by default on a port the OS picks. It reads and changes `toxic.Front` in [`internal/cmd/toxic`](..). [`internal/cmd/toxiproxy.go`](../../toxiproxy.go) builds its server.

| File | Holds |
| ---- | ----- |
| `app.go` | the page and its actions |
| `view.go` | what the templates show, including the endpoint search |
| `app.templ` | the templates, with [Morpheus](https://github.com/romshark/morpheus) components |
| `presets.templ` | the preset combobox and its dialogs |
| `static/ui.css` | the styles and the Morpheus tokens of the theme |
| `static/morpheus/` | the Morpheus bundle, generated |
| `static/icons/` | the [Lucide](https://lucide.dev) icons and the Datapages logo |
| `datapagesgen/` | generated code, never edit |

## Change it

After a change to the pages or actions in `app.go`, regenerate `datapagesgen`:

```sh
mage genDatapages # or: go run ./internal/tools/genapp internal/cmd/toxic/ui
```

`genapp` exists because `datapages gen` refuses this module, whose runtime packages import each other.

After a change to a `.templ` file, run `mage genTempl`, never while `datapages watch` runs.

Before using another Morpheus component, add it to `morpheusComponents` in `magefiles/mage.go` and run `mage genMorpheus`.

Copy a new icon from `lucide-static`, at the version in the license comment of the others, into `static/icons/`. `datapages.svg` is a copy of [`docs/logo.svg`](../../../../docs/logo.svg). `github.svg` is the GitHub mark of `simple-icons`, since Lucide has no brand icons.

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
