# Development

## Documentation pages

[docpages](https://github.com/romshark/docpages) renders the Markdown files of `guide/` as the docs of the GitHub Pages site. `mage watchDocs` serves them from the working tree and reloads open tabs when a file changes:

```sh
mage watchDocs
```

The docs are at http://127.0.0.1:7340/. An error, such as a link to a missing heading, appears in the tab until it is fixed.

The folders and file names of `guide/` are the navigation, as [Layout](https://github.com/romshark/docpages/blob/v0.4.0/docs/2-guide/1-layout.md) of docpages describes. To add a page, add a Markdown file with the next number to a group folder, as `guide/4-guides/17-caching.md`. `SPECIFICATION.md`, `CHANGELOG.md` and `FAQ.md` stay at the repository root. The pages of `guide/5-reference/` and `guide/6-more/` show them with `source` in their front matter.

| File | Contents |
| ---- | -------- |
| `guide/README.md` | The first page and, in its front matter, the settings of the site |
| `guide/template.templ` | The links of the header and the footer and the license |
| `guide/style.css` | The beta badge after the logo |
| `guide/logo.svg`, `guide/favicon.svg` | The logo of the header and the icon of the browser tab |
| `guide/go.mod` | The docs module, which requires docpages for the templates. `go tool docpages` in `guide/` runs that version. |

`mage genDocs` writes the docs into `docs/docs/` and the landing page into `docs/index.html`, which [render-landing](internal/tools/render-landing/README.md) renders. Git ignores these files. The Pages workflow runs `mage genDocs` before each deployment.

## Versions

The site serves the docs of main and of each feature version from v0.11 on, which `releases` in the front matter of `guide/README.md` selects. docpages builds the docs of a feature version from the tag of its latest patch, as v0.11.5 for v0.11, and `docs/docs/index.html` redirects to the latest release. See [Versions](https://github.com/romshark/docpages/blob/v0.4.0/docs/2-guide/7-versions.md) of docpages.
