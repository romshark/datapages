# render-pages

render-pages generates the GitHub Pages site of Datapages from the Markdown files of the repository and the templ templates of this directory. It writes static files into `docs/`. It is a separate Go module, which keeps goldmark, Chroma and Morpheus out of the root `go.mod`.

## Commands

| Command | Effect |
| ------- | ------ |
| `mage genDocs` | Builds the Morpheus bundle into `docs/morpheus/`, minifies `style.css` into `docs/style.css` and writes the site into `docs/`. |
| `mage watchDocs` | Serves the site on http://127.0.0.1:7340 and reloads it on changes. See [DEV.md](../../../DEV.md). |
| `go test ./...` | Runs the tests. Run it in this directory. |

The [Pages workflow](../../../.github/workflows/pages.yml) renders and deploys the site. The [release workflow](../../../.github/workflows/release.yml) runs it after it publishes a release.

## Versions

The site serves the pages of the latest release under `/docs/<tag>/` and the pages of the main branch under `/docs/main/`. Each version renders with the render-pages of its own commit. Until a release has render-pages, the site serves main alone.

| Flags | Pages |
| ----- | ----- |
| `-version v1.2.0` | The release. The version menu of the header offers the same page of main. |
| `-version main -release v1.2.0` | main, next to the release. The version menu and a note offer the same page of the release. `noindex` keeps search engines on the release. The landing page, `/docs/` and `/llms.txt` lead to the release. |
| `-version main` | main alone, as `mage genDocs` and `mage watchDocs` render it. The version menu offers main alone, a note says that it is unreleased, and the landing page leads to it. |

Links to files of the repository name the tag of a release and `main` for main. When the other version lacks a page, such as one added after the release, `docs.js` opens the first page of that version instead.

## Output

Each version is a directory of `docs/docs/`, named after it, with:

- 1 HTML page and 1 Markdown copy for each page in `docPages` in [docs.go](docs.go)
- `search.json`, the search index
- `llms.txt` and `llms-full.txt`, which list and concatenate the Markdown copies for language models
- copies of `style.css`, `docs.js`, `favicon.svg` and `morpheus/` of `docs/`, which keep the pages of a release on the stylesheet and scripts of their release

The run for main also writes `docs/index.html`, the landing page, `docs/docs/index.html`, which opens the default version, and `docs/llms.txt`, which links the `llms.txt` of each version.

Git ignores these files. `docs/style.css` and `docs/morpheus/` are committed, and `mage checkGen` fails when they are stale.

## Pages

`docPages` lists the pages in navigation order. For each page:

1. goldmark parses the file as GitHub Flavored Markdown.
2. Links are resolved relative to the file, as on GitHub. A link to the file of a page leads to the page, a link to another file of the repository leads to GitHub, and a link to a missing heading fails the build.
3. Chroma highlights code blocks. `title="..."` after the language adds a title bar. `mermaid` blocks become diagrams.
4. GitHub alerts such as `> [!NOTE]` become callouts.
5. The templates add the header, the navigation and the contents of the page.

## Search

The build splits each page into sections at the headings that its contents list and writes 1 entry per section into `search.json`, with the page title, the heading, the URL and the text. `docs.js` downloads the file on first use and searches it in the browser. A result contains every word of the query, and matches in headings rank above matches in the text.

## Browser

- `docs.js` runs the navigation overlay, the highlight of the current section, the search, the version menu and the Mermaid diagrams. It loads Mermaid only on pages with a diagram.
- A script in `<head>` applies the stored theme before the first paint.
- The sidebar, the search dialog, the copy buttons, the theme menu and the version menu are Morpheus components. `mage genDocs` bundles only these with the `morpheus bundle` command, at the version in `go.mod`, which matches its Go components, and copies the caret icon of the version menu. The bundle has no theme: `style.css` sets the theme tokens of Morpheus to the colors of the site. The landing page does not load Morpheus.

## Files

| File | Contents |
| ---- | -------- |
| `main.go` | Flags and the build |
| `edition.go` | The version that a build documents and the links to the other version |
| `serve.go` | The server of `mage watchDocs` |
| `docs.go` | `docPages`, Markdown conversion and links |
| `highlight.go`, `blocks.go` | Code blocks, callouts and heading links |
| `search.go`, `llms.go` | `search.json`, `llms.txt` and `llms-full.txt` |
| `*.templ` | The pages, the theme switch and the icons |
| `style.css` | The stylesheet |
| `templier.yml` | The watcher configuration |
