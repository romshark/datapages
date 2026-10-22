# render-pages

render-pages generates the GitHub Pages site of Datapages from the Markdown files of the repository and the templ templates of this directory. It writes static files into `docs/`. It is a separate Go module, which keeps goldmark, Chroma and Morpheus out of the root `go.mod`.

## Commands

| Command | Effect |
| ------- | ------ |
| `mage genDocs` | Writes the site into `docs/` and minifies `style.css` into `docs/style.css`. |
| `mage watchDocs` | Serves the site on http://127.0.0.1:7340 and reloads it on changes. See [DEV.md](../../../DEV.md). |
| `go test ./...` | Runs the tests. Run it in this directory. |

The [Pages](../../../.github/workflows/pages.yml) and [release](../../../.github/workflows/release.yml) workflows render and deploy the site.

## Output

- `index.html`, the landing page
- 1 HTML page and 1 Markdown copy for each page in `docPages` in [docs.go](docs.go)
- `search.json`, the search index
- `llms.txt` and `llms-full.txt`, which list and concatenate the Markdown copies for language models

Git ignores these files. `docs/style.css` is committed, and `mage checkGen` fails when it is stale.

## Pages

`docPages` lists the pages in navigation order. For each page:

1. goldmark parses the file as GitHub Flavored Markdown.
2. Links are resolved relative to the file, as on GitHub. A link to the file of a page leads to the page, a link to another file of the repository leads to GitHub, and a link to a missing heading fails the build.
3. Chroma highlights code blocks. `title="..."` after the language adds a title bar. `mermaid` blocks become diagrams.
4. GitHub alerts such as `> [!NOTE]` become callouts.
5. The templates add the header, the navigation and the contents of the page.

## Search

The build splits each page into sections at its headings and writes 1 entry per section into `search.json`, with the page title, the heading, the URL and the text. `docs.js` downloads the file on first use and searches it in the browser. A result contains every word of the query, and matches in headings rank above matches in the text.

## Browser

- `docs.js` runs the navigation overlay, the highlight of the current section, the search and the Mermaid diagrams. It loads Mermaid only on pages with a diagram.
- A script in `<head>` applies the stored theme before the first paint.
- The sidebar, the search dialog, the copy buttons and the theme menu are Morpheus components. The pages load Morpheus from jsDelivr at the version in `go.mod`, which matches its Go components. The landing page does not load it.

## Files

| File | Contents |
| ---- | -------- |
| `main.go` | Flags and the build |
| `serve.go` | The server of `mage watchDocs` |
| `docs.go` | `docPages`, Markdown conversion and links |
| `highlight.go`, `blocks.go` | Code blocks, callouts and heading links |
| `search.go`, `llms.go` | `search.json`, `llms.txt` and `llms-full.txt` |
| `morpheus.go` | The Morpheus URL |
| `*.templ` | The pages, the theme switch and the icons |
| `style.css` | The stylesheet |
| `templier.yml` | The watcher configuration |
