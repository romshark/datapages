# Development

## Documentation pages

`mage watchDocs` serves the GitHub Pages site from the working tree. It reloads open tabs when a source file changes:

```sh
mage watchDocs
```

The documentation is under `/docs/` of the printed address. The first run downloads Templier and templ.

To add a page, write the file in `guide/` and add it to `docPages` in `internal/tools/render-pages/docs.go`.

The watcher shows a placeholder for a page whose file does not exist and skips checks for links to its headings. `mage genDocs` fails when the file is missing.

The watcher reacts to these files.

| File | Effect |
| ---- | ------ |
| `guide/*.md`, `SPECIFICATION.md`, `CHANGELOG.md`, `FAQ.md` | The tab reloads. |
| `docs/*` | The tab reloads. |
| `internal/tools/render-pages/style.css` | The tab reloads. The server serves this file unminified in place of `docs/style.css`. |
| `internal/tools/render-pages/*.templ` | templ regenerates the Go code and the tab reloads. A change to the Go code of a template rebuilds the server first. |
| `internal/tools/render-pages/*.go`, except `*_test.go` | The server is rebuilt and restarted, and the tab reloads. |

Compiler and Markdown errors appear in the tab. A link to a missing heading is 1 such error. The tab reloads after the error is fixed.

Restart the watcher after renaming or deleting a `.templ` file. Until then, Templier keeps showing the error for the missing file.

Run `mage genDocs` before committing. It writes `docs/style.css`, the minified stylesheet that CI checks, and it can run while the watcher runs.
