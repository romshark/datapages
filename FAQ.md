# FAQ

Frequently asked questions about Datapages.

## Can and *should* I use Datapages for (local-first) Go GUI apps?

**Yes.** More specifically - the best way to write local-first Datapages apps is to make them a hybrid web app.

Datapages optimizes for the thin client architecture, where the served HTML/CSS/JS is just a thin presentation and intent capturing layer, because this reduces the complexity and amount of code, as well as overall performance characteristics by avoiding a lot of JavaScript. Frameworks like [Wails v3](https://v3.wails.io/) can then be used to run the Go server on the `localhost` loopback and display this thin shell in a webview (the system's native browser without the browser window shell). 

[example/calculator](https://github.com/romshark/datapages/tree/main/example/calculator) shows how this can be achieved.

Since this will not require you to ship a whole Chromium (like Electron; 100-150 MB compressed, typically ~200-300+ MB installed) or a whole JavaScript runtime (like `deno compile`; 60-70 MB) in the installer - the distributable and its installation size will be rather small. The [Calculator](https://github.com/romshark/datapages/tree/main/example/calculator) example is just ~22 MB on `darwin/arm64`.

This approach is more efficient, and overall better than the [Wails v2](https://wails.io/) architecture, where JavaScript still plays a big role.

P.S.
Even though [Wails v3](https://v3.wails.io/) is still in Beta - it is already successfully used in production. Due to technical limitations, [Wails v2](https://wails.io/) cannot be used with Datastar and Datapages.

## When to do a JavaScript SPA instead?

Go for a JavaScript single page application if:
1. the server is not your source of truth.
2. you're building a local-first standalone CDN-hostable website(-app).

For server-centric applications that are mostly useless when offline with the server being the inevitable source of truth, or even local-first installed GUI apps ([see previous question](#can-and-should-i-use-datapages-for-local-first-go-gui-apps)) - Datapages is a better choice for several reasons:

- **No `npm` supply chain**: All you need is Go and HTML/CSS with tiny pieces of JavaScript inside, not the entire JavaScript zoo. This reduces the attack surface of your code base.
- **Optimal agentic engineering**: Datapages ships with all AI skills and CLI tools necessary for coding agents to be utmost efficient. This avoids wasting tokens on huge piles of React and Go API boilerplate.
- **Less code**: You don't need to maintain a JavaScript code base + a JSON API server, in fact, you need no API at all. The amount of code is substantially lower.
- **More efficient SSR**: you don't need to run a JavaScript runtime like with Next.js for SSR. Rendering HTML with Go is significantly more resource-efficient and faster.
- **Lighter bundle**: Datastar (v1.0.4) is the entire runtime at just ~13KB gzipped, plus a short inline script on pages that use per-tab state. React, Vue, Angular, or even HTMX + Alpine.js all usually end up being larger.
- **Real-Time by default**: Making your SPA a real-time multiplayer UI is usually considerably more extra work and code. With Datapages you get real-time web UIs out of the box.

If the only reason you're going for a JavaScript SPA is a larger ecosystem and from that you only really need a UI kit - consider these alternatives instead that work great with Datapages:

- [Morpheus](https://romshark.github.io/morpheus/) (works best with fat-morph)
- [Basecoat](https://basecoatui.com/components/button/)
- [WebAwesome](https://webawesome.com/)
- [Datastar Rocket](https://data-star.dev/reference/rocket)
- ..others? (PRs welcome!)

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
BenchmarkTemplatingStd-14                3781242               302.1 ns/op           256 B/op          8 allocs/op
BenchmarkTemplatingTempl-14             13310950                90.89 ns/op          165 B/op          5 allocs/op
BenchmarkTemplatingQuicktemplate-14     53826442                22.41 ns/op            0 B/op          0 allocs/op
BenchmarkTemplatingGomponents-14        19847491                60.69 ns/op            0 B/op          0 allocs/op
BenchmarkTemplatingJet-14               16934940                70.05 ns/op           24 B/op          1 allocs/op
PASS
ok      github.com/romshark/datapages/internal/templatingbench  6.173s
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
