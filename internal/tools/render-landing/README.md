# render-landing

render-landing writes `docs/index.html`, the landing page of the GitHub Pages site of Datapages, from the templ templates of this directory.

The docs below the landing page are the Markdown files of [guide/](../../../guide/README.md), which [docpages](https://github.com/romshark/docpages) renders into `docs/docs/`. render-landing runs after docpages. The page links the version of the docs that `docs/docs/index.html` redirects to, `main` or the latest release, and the build fails if that version lacks a page that the landing page links.

## Commands

| Command | Effect |
| ------- | ------ |
| `mage genDocs` | Writes the docs into `docs/docs/` with docpages, minifies `style.css` into `docs/style.css` and writes `docs/index.html`. |
| `go test ./...` | Runs the tests. Run it in this directory. |

Git ignores the files that `mage genDocs` writes. The [Pages workflow](../../../.github/workflows/pages.yml) runs it before each deployment.

## Files

| File | Contents |
| ---- | -------- |
| `main.go` | The version of the docs, the paths that the page links, and the build |
| `index.templ` | The page, its header and its footer |
| `theme.templ` | The theme switch and the script that applies the theme before the first paint |
| `icons.templ` | The icons |
| `style.css` | The stylesheet |

The page links the docs by their paths in docpages, which follow the folders and file names of `guide/`. Renaming a linked page of `guide/` requires changing its path in `main.go`.
