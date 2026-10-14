# Performance of sse-writer against main

Measured 2026-10-13 on main 96dbce08 and origin/sse-writer a58537ed. KB and MB are decimal units.

sse-writer is faster and allocates less in every case measured except 2. Chrome responses are a few hundred bytes larger, and a 99 KB event to Safari before 26.3 takes 3% longer.

Most of the gain is per response, not per event. On main, every SSE response calls `datastar.NewSSE(w, r, datastar.WithCompression())`. It builds a brotli, a zstd, a gzip and a deflate encoder and test-writes each one, whether or not the client accepts compression. That costs 6.63 MB and about 100 allocations before the first byte. sse-writer builds its codings once in `httpserve.NewCore` and takes an encoder from a pool on the first event.

## Per SSE response

Measured in `runtime/sse` without handler work:

| | main | sse-writer |
|---|---|---|
| open and close, no event | 0.54 ms, 6.63 MB, 100 allocs | 0.28 us, 176 B, 6 allocs |
| open, 1 small patch, close, uncompressed | 0.57 ms, 6.63 MB, 114 | 0.42 us, 176 B, 6 |
| same, Chrome (brotli q6 -> zstd 1) | 1.03 ms, 8.79 MB, 139 | 4.7 us, 238 B, 9 |
| same, Safari < 26.3 (brotli q6 -> q4) | 1.03 ms, 8.79 MB, 138 | 21.9 us, 209 B, 7 |
| same, gzip only (Go client) | 0.99 ms, 7.72 MB, 140 | 5.3 us, 208 B, 7 |

About half of main's time is GC work caused by that garbage. With `GOGC=400`, main's end-to-end calculator action drops from 1.18 ms to 0.64 ms. The allocated bytes stay the same.

## End to end

The calculator example's `QUERY /input/`, sent to `ServeHTTP`. Each request goes through routing, signal reading, the handler and 1 `PatchElement` of the 2.3 KB calculator.

| client | main | sse-writer | parallel, 24 threads (req/s) |
|---|---|---|---|
| uncompressed | 1175 us, 6.73 MB, 354 allocs | 17.9 us, 8.4 KB, 191 | 5.6k -> 246k |
| Chrome | 1887 us, 8.93 MB, 385 | 30.2 us, 13.4 KB, 194 | 4.4k -> 537k |
| Safari < 26.3 | 1903 us, 8.93 MB, 382 | 80.3 us, 9.9 KB, 192 | 4.3k -> 180k |
| gzip | 1528 us, 7.81 MB, 374 | 41.2 us, 12.3 KB, 192 | 4.2k -> 394k |

On sse-writer, parallel uncompressed requests are slower than zstd ones. The cause is Go runtime heap-lock contention (`runtime.lock2`, 4% of CPU) among 24 goroutines that only render and allocate. The SSE writer takes under 1% of that profile's CPU.

## Per event on an open stream, uncompressed

| event | main | sse-writer |
|---|---|---|
| PatchElement 175 B | 549 ns, 530 B, 7 allocs | 72 ns, 0 B, 0 |
| PatchElement 8 KB, 1 line | 6.70 us, 19.5 KB, 7 | 1.86 us, 0, 0 |
| PatchElement 8 KB, 227 lines | 28.1 us, 60 KB, 243 | 4.06 us, 0, 0 |
| PatchElement 99 KB | 170 us, 806 KB, 20 | 117 us, 425 KB, 14 |
| PatchElementAt | 720 ns, 570 B, 9 | 119 ns, 0, 0 |
| RemoveElement | 252 ns, 120 B, 3 | 60 ns, 0, 0 |
| ExecuteScript | 749 ns, 707 B, 16 | 97 ns, 0, 0 |
| PatchSignals, struct | 799 ns, 737 B, 6 | 392 ns, 80 B, 2 |
| PatchSignals, RawMessage | 576 ns, 681 B, 5 | 173 ns, 24 B, 1 |
| Redirect | 1123 ns, 1036 B, 21 | 313 ns, 48 B, 3 |
| Prefetch, 3 URLs | 1007 ns, 1201 B, 21 | 298 ns, 144 B, 2 |

- datastar-go's `PatchElements` allocates 1 string per HTML line, which is why the 227-line case has 243 allocations on main.
- What sse-writer still allocates for signals, Redirect and Prefetch is JSON encoding and argument boxing, which main also pays.

## Per event, compressed

Time and wire bytes per event, main -> sse-writer. Each render changes 1 list item. 4096 distinct renders cycle, so none repeats within a compression window.

| event | Chrome (br q6 -> zstd 1) | Safari < 26.3 (br q6 -> br q4) | gzip -> gzip |
|---|---|---|---|
| 175 B | 4.43 -> 1.49 us, 11 -> 13 B | 4.39 -> 3.62 us, 11 -> 11 B | 2.40 -> 1.52 us |
| 8 KB, 1 line | 39.2 -> 7.8 us, 23 -> 69 B | 38.9 -> 31.9 us, 23 -> 38 B | 25.4 -> 14.4 us |
| 8 KB, 227 lines | 66.0 -> 12.2 us, 23 -> 76 B | 66.5 -> 43.5 us, 23 -> 39 B | 49.1 -> 21.0 us |
| 99 KB | 461 -> 197 us, 22 -> 326 B | 473 -> 487 us (+3%), 22 -> 69 B | 540 -> 488 us |

gzip sends the same bytes on both branches.

## Memory held per open stream

Heap after GC, averaged over 20 streams. 0.6 KB of each figure is the benchmark's own request and writer.

| state | Chrome | Safari < 26.3 | gzip |
|---|---|---|---|
| opened, no event | 10.9 KB -> 0.6 KB | 10.9 KB -> 0.6 KB | 0.8 -> 0.6 KB |
| after 1 small event | 2.14 -> 1.55 MB | 2.14 -> 0.55 MB | 1.08 -> 1.08 MB |
| after 20 x 99 KB | 11.6 -> 1.61 MB | 11.6 -> 3.71 MB | 1.08 -> 1.08 MB |

For 500 Chrome tabs past 2 MB of updates, that is 5.8 GB on main and 0.8 GB on sse-writer.

## Other differences

- **`ReadSignals` GET:** 1163 -> 767 ns, 577 -> 144 B, 7 -> 3 allocs. POST is unchanged at 439 -> 424 ns, 48 B and 1 alloc.
- **Binary size:** the classifieds server goes from 19.60 to 19.50 MB and todolist from 16.54 to 16.46 MB. 3 fewer modules are linked: datastar-go, CAFxX/httpcompression and bytebufferpool.

## Where sse-writer is worse

- **Bytes on the wire.** zstd level 1 against main's brotli q6 makes the calculator response 650 B instead of 536 B with Chrome (+21%). A 99 KB re-render sends 326 B instead of 22 B. With Safari the response grows 7%. With gzip it grows 9 B, which is the end of the gzip stream that main never writes. All of these stay under 0.4% of the raw HTML. `DefaultSSEZstdLevel` in `options.go` chooses this trade-off, and `datapages.WithSSECompression` changes it.
- **Events over 64 KiB.** `event.free` in `runtime/sse/event.go` drops buffers above `maxPooledBuf`, so each such event regrows its buffer from 0. That costs 14 allocations and 425 KB per 99 KB event, still less than main's 806 KB. A copy with `maxPooledBuf = 1 << 20` made 0 allocations, and the 99 KB event dropped from 116 to 22 us uncompressed, 189 to 84 us with zstd, 477 to 234 us with gzip and 481 to 278 us with brotli. The cap exists because of [go.dev/issue/23199](https://go.dev/issue/23199), which the comment cites. Small events would reuse 1 MiB buffers and keep them alive.
- **Brotli action responses** take 21.9 us against 4.7 us with zstd. Clearing brotli's hash table takes 35% of that and building its entropy codes 46%, both on each response's first flush. That is still 1/47 of main's time.

## CHANGELOG figures

The branch's `CHANGELOG.md` states these figures:

- **"An event allocates nothing and takes 1/4 to 1/5 of the time":** `PatchElement`, `PatchElementAt`, `RemoveElement` and `ExecuteScript` allocate nothing up to 64 KiB. Measured time is 1/2 to 1/7.7 of main's, and 1/1.45 at 99 KB.
- **"A compressed event takes 1/5 to 1/10 of the time":** not reproduced per event. Measured 1/2.3 to 1/5.4 with Chrome, 1/1.2 to +3% with Safari and 1/1.1 to 1/2.3 with gzip. A whole action response takes 1/47 to 1/219.
- **"6.7 MB, before its first byte":** measured 6.63 MB.
- **"about 1.5 MB" with zstd:** measured 1.55 to 1.61 MB.
- **"up to 10 MB with brotli":** measured 11.6 MB after 2 MB of updates.
- **"500 open tabs held about 5 GB":** measured 5.8 GB after 2 MB of updates per tab and 1.07 GB after 1 small event.

## Action items

1. Pool event buffers above 64 KiB in a second `sync.Pool`. That keeps the measured 2x to 5x gain for large events and keeps large buffers out of small events.
2. Replace the CHANGELOG ratios with the measured ranges, or cite the benchmark they come from.
3. Re-run against the current origin/sse-writer. The fetch for this run failed with an access-rights error, which means the measured ref, last updated 2026-10-12 16:36, may be stale.

## Method

- **Revisions.** main 96dbce08 against the local ref origin/sse-writer a58537ed. Main's 6 commits since the merge base 3311e956 touch neither `runtime/` nor the datastar-go version. The calculator's generated code differs between the 2 trees in 13 lines, all SSE-related.
- **Benchmarks.** `runtime/sse/cmp_bench_test.go` and `example/calculator/app/cmp_e2e_bench_test.go`, identical on both trees and not committed. A per-branch adapter opens a stream the way that branch's generated code does.

  main:

  ```go
  func openStream(w http.ResponseWriter, r *http.Request) (datapages.SSE, func()) {
  	return sse.New(datastar.NewSSE(w, r, datastar.WithCompression())), func() {}
  }
  ```

  sse-writer:

  ```go
  var benchCompression = sse.NewCompression(nil)

  func openStream(w http.ResponseWriter, r *http.Request) (datapages.SSE, func()) {
  	s := sse.Open(w, r, benchCompression)
  	return s, s.Close
  }
  ```

- **Clients.** `Accept-Encoding` values:
  - uncompressed: none
  - Chrome 123+ and Firefox 126+: `gzip, deflate, br, zstd`
  - Safari < 26.3: `gzip, deflate, br`
  - gzip only, as Go's `http.Client` sends: `gzip`
- **Workloads.** A 175 B counter, an 8 KB list on 1 line, the same list with 1 item per line (227 lines) and a 99 KB list on 1 line. Components render like templ-generated code, through templ's pooled 4 KiB buffer, from precomputed strings. Rendering allocates nothing, which leaves only the SSE path in the counts.
- **Response writer.** Counts the bytes written and discards them. It implements `http.Flusher`.
- **End to end.** A server from `datapages.NewServer` with a discarding logger. Each request reuses 1 `*http.Request` with the body `{"input":"12+3","fresh":false,"num":""}`. The parallel benchmark runs `b.RunParallel` with 1 client per goroutine.
- **Memory per stream.** Open 20 streams, send the state's events, run `runtime.GC` twice, divide the `HeapAlloc` delta by 20.
- **Environment.** Go 1.27.2, linux/amd64, Intel Xeon w5-2455X (12 cores, 24 threads), GOMAXPROCS 24, idle machine, `powersave` CPU governor.
- **Statistics.** 10 rounds alternating the main and sse-writer binaries, 1 run each per round, compared with benchstat. Every time difference reported has p <= 0.023 with n=10, most p=0.000. Memory per stream comes from 5 runs of 5 iterations each. The `maxPooledBuf` experiment ran 10 separate rounds alternating sse-writer and the modified copy.
