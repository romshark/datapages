# Vendored Lucide icons

The icons the HTML page's buttons carry, from
[lucide-static](https://github.com/lucide-icons/lucide) v0.544.0, ISC licensed.
They are inlined into the page, which loads nothing from the network.

To add one, take it from the same release:

```sh
V=0.544.0
curl -fL "https://cdn.jsdelivr.net/npm/lucide-static@$V/icons/<name>.svg" -o "<name>.svg"
```

`page.html.tmpl` reaches them by file name: `{{index .Icons "info"}}`.
