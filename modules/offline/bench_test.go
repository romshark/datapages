package offline_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romshark/datapages/modules/offline"
)

// BenchmarkMiddleware benchmarks [offline.Middleware]
// with HTML heads of different sizes and tag densities.
func BenchmarkMiddleware(b *testing.B) {
	var items strings.Builder
	for i := range 50 {
		fmt.Fprintf(&items, `<li class="item"><a href="/items/%d/">Item %d</a></li>`,
			i, i)
	}
	body := "</head><body><ul>" + items.String() + "</ul></body></html>"
	head := `<!DOCTYPE html><html><head><meta charset="UTF-8"/><title>Items</title>`

	for name, page := range map[string]string{
		"4KB page": head + body,
		"80KB head": head +
			strings.Repeat(
				`<link rel="preload" href="/static/font.woff2" as="font"/>`, 300,
			) +
			"<style>" +
			strings.Repeat(
				".card{display:flex;padding:8px 12px}\n", 1500,
			) +
			"</style>" + body,
		"140KB of meta tags": head + strings.Repeat("<meta/>", 20_000) + body,
		"100KB of <":         head + strings.Repeat("<", 100_000) + body,
	} {
		h := offline.Middleware("/offline/", offline.Config{WorkerVersion: 1})(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, page)
			}),
		)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				h.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
}
