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
  (see benchmark results below). Faster engines exist, but templ offers
  the best balance of performance, type safety, and developer experience.
  Additional engines may be supported in the future if requested.

Templating benchmark source: [`internal/templatingbench/`](internal/templatingbench/)

```
goos: darwin
goarch: arm64
pkg: github.com/romshark/datapages/internal/templatingbench
cpu: Apple M4 Pro
BenchmarkTemplatingStd-14                3509767               329.4 ns/op           256 B/op          8 allocs/op
BenchmarkTemplatingTempl-14             12522609                95.28 ns/op          117 B/op          4 allocs/op
BenchmarkTemplatingQuicktemplate-14     55614774                21.76 ns/op            0 B/op          0 allocs/op
BenchmarkTemplatingGomponents-14        10426274               115.2 ns/op            16 B/op          1 allocs/op
BenchmarkTemplatingJet-14               16843957                69.83 ns/op           24 B/op          1 allocs/op
PASS
ok      github.com/romshark/datapages/internal/templatingbench  6.216s
```

Shoutout to the
[templ developers and contributors](https://github.com/a-h/templ/graphs/contributors),
who are doing an awesome job and without whom Datapages would be only half as awesome! ❤️

## Why NATS Core over JetStream?

The Datapages message broker dispatches events for live UI updates over SSE,
not data changes. A lost event just means a stale UI until the next refresh
or page reload. [JetStream](https://docs.nats.io/nats-concepts/jetstream)'s
durability, ack-based delivery, and replay guarantees add overhead and
complexity with no real payoff for this use case.

[Core NATS](https://docs.nats.io/nats-concepts/core-nats) is sufficient because:

- **Partial writes are unlikely and harmless.** Core NATS
  [`Publish` buffers outgoing data locally](https://docs.nats.io/using-nats/developer/sending/caches),
  so each call is a fast in-memory append. A dispatch loop failing
  mid-way would require the connection to drop between two appends.
  Even then, the result is just a missed UI update - not data loss.
- **Lower latency.** No ack round-trip per publish.
- **Simpler deployment.** No stream/consumer configuration needed.
