# Development

## Documentation pages

`mage watchDocs` serves the GitHub Pages site from the working tree. It reloads open tabs when a source file changes:

```sh
mage watchDocs
```

Open http://127.0.0.1:7340/ for the landing page or http://127.0.0.1:7340/docs/ for the documentation. Ctrl+C stops the watcher. The first run downloads Templier and templ.

`docPages` in `internal/tools/render-pages/docs.go` lists each page, its Markdown file and its URL in navigation order. To add a page, write the file in `guide/` and add it to `docPages`. Markdown links are relative to their source file, as on GitHub.

The watcher shows a placeholder for a page whose file does not exist and skips checks for links to its headings. `mage genDocs` fails when the file is missing.

The watcher reacts to these files.

| File | Effect |
| ---- | ------ |
| `guide/*.md`, `SPECIFICATION.md`, `CHANGELOG.md`, `FAQ.md` | The tab reloads. |
| `docs/*` | The tab reloads. |
| `internal/tools/render-pages/style.css` | The tab reloads. The server serves this file unminified in place of `docs/style.css`. |
| `internal/tools/render-pages/*.templ` | templ regenerates the Go code and the tab reloads. A change to the Go code of a template rebuilds the server first. |
| `internal/tools/render-pages/*.go` | The server is rebuilt and restarted, and the tab reloads. |

Compiler and Markdown errors appear in the tab. A link to a missing heading is 1 such error. The tab reloads after the error is fixed.

Restart the watcher after renaming or deleting a `.templ` file. Until then, Templier keeps showing the error for the missing file.

Run `mage genDocs` before committing. It writes `docs/style.css`, the minified stylesheet that CI checks, and it can run while the watcher runs.

### How it works

`mage watchDocs` runs [Templier](https://github.com/romshark/templier) in `internal/tools/render-pages` with `templier.yml`. Templier runs `templ generate --watch`, builds the module and starts it with `-serve 127.0.0.1:7341`. Its proxy listens on port 7340 and adds the reload script. The server renders each request from the current templates, Markdown files and `style.css`.

Templier watches only the module directory. Every 250 ms, the server checks the page sources and the files in `docs/`. It rewrites `internal/tools/render-pages/.reload` after a change. A custom watcher in `templier.yml` then reloads the tabs.

templ watch mode keeps the template strings in temporary files, which `templ generate` deletes. `mage watchDocs` sets `TEMPL_DEV_MODE_ROOT` to a directory of its own, which the `templ generate` of `mage genDocs` leaves alone.
