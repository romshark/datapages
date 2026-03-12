## Done

The linter catches situations like `<a href="/login">Login</a>` where href is hard-coded and would silently break if the URL changes. It insists you use the generated href builder: `<a href={ href.PageLogin() }>`.

For runtime values like `<a href={ dynamicValue }>`, the linter requires wrapping with `href.External`:
```
<a href={ href.External("http://www.google.com") }>
<a href={ href.External(dynamicValue) }>
```
`External` checks at runtime whether the link is internal and writes an error log.

`<a href={ href.External("/login") }>` also fails at lint time — the linter resolves the string literal and detects it's internal.

```
// ✅ These are OK
href="https://data-star.dev"
href="ftp://..."
href="#foo-bar"
href="//cdn.example.com"                          // protocol-relative
href="mailto:test@example.com"
href="tel:+1234567890"
href="sms:+1234567890"
href="data:text/plain,hello"
href={ href.PageFooBar() }
href={ SomeConstant }                              // checked that constant value is non-relative
href={ href.External("https://data-star.dev") }
href={ href.External(variable) }
href={ "https://data-star.dev" }
href={ "mailto:test@example.com" }
href={ "tel:+1234567890" }
href={ "sms:+1234567890" }

// ⚠️ ERROR; gen/lint will fail
href=""
href=" "                  // Still empty
href="?tab=bazz"
href="relative"
href=" relative "         // Spaces are stripped
href="/relative"
href="/relative#foo"
href="./relative"
href="../relative"
href={ "/relative" }
href={ variable }
href={ fmt.Sprint("...") }
href={ someFunc() }
href="javascript:void(0)"
```

**Limitation:** Cannot detect absolute links to the same domain (`https://mydomain.com/login`). Document in README.

The implemented algorithm (in `parser/internal/templcheck/templcheck.go`):
```go
func IsAllowedNonRelativeHref(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" { return false }
	if strings.HasPrefix(s, "#") { return true }       // fragment
	if strings.HasPrefix(s, "?") { return false }      // query-only
	if strings.HasPrefix(s, "//") { return true }      // protocol-relative
	if strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") { return false }   // internal paths
	u, err := url.Parse(s)
	if err == nil && u.Scheme != "" {
		if strings.EqualFold(u.Scheme, "javascript") { return false }
		return true
	}
	return false                                        // bare relative
}
```

Since `href.External` is in the package namespace, all page hrefs are prefixed: `PageIndex` instead of `Index`.

## All done