# Vendored Morpheus assets

[Morpheus](https://github.com/romshark/morpheus) v0.1.0, the web component kit
the HTML page in `page.html.tmpl` is built from. The files are the minified
release artifacts, embedded into the page so that it loads nothing from the
network. Do not edit them.

To move to another release, replace the tag and re-download:

```sh
V=v0.1.0
for f in morpheus.css theme-default.css bundle.js; do
  curl -fL "https://cdn.jsdelivr.net/gh/romshark/morpheus@$V/min/$f" -o "$f"
done
```

`bundle.js` reads the icons of some components over HTTP from
`/static/icons/<name>.svg`. The page has nowhere to fetch from, so the one
component that needs an icon, the tree, is given its chevron through the
`slot="icon"` the kit provides for it, and the `<neo-icon>` the item keeps in
its shadow root as the slot fallback is removed: it requests its SVG even when
the slot is filled.
