# FAQ

Frequently asked questions about Datapages.

## Why templ instead of `html/template`?

[templ](https://templ.guide/) provides:

- **Compile-time type safety.** Template errors are caught at build time,
  not at runtime. For example,
  [this program](https://go.dev/play/p/UnQxq6OjHlV) compiles and runs
  but fails during rendering. With templ, the equivalent would fail the build.
- **Components are Go functions.** They're composable, testable,
  and refactorable with standard Go tooling.
- **IDE support.** The templ LSP gives autocomplete, go-to-definition,
  and inline diagnostics. Standard Go templates are opaque strings
  to the IDE - at best you get basic syntax highlighting.
- **Higher performance.** Templ utilizes code generation to produce efficient
  rendering code ahead of time, which is more efficient than `html/template` rendering
  (see benchmark results below).

Templating benchmark source: [`internal/bench/`](internal/bench/)

```
goos: darwin
goarch: arm64
pkg: github.com/romshark/datapages/internal/bench
cpu: Apple M4 Pro
BenchmarkTemplatingStd-14      	 3560989	       321.1 ns/op	     256 B/op	       8 allocs/op
BenchmarkTemplatingTempl-14    	12684994	        94.81 ns/op	     117 B/op	       4 allocs/op
PASS
coverage: 79.3% of statements
```

Shoutout to the templ developers and contributors, who are doing an awesome job and
without whom Datapages would be only half as awesome! ❤️
