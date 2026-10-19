package httpserve

import (
	"bufio"
	"cmp"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/contentcoding"
)

// responseWindow is the zstd and brotli window of a compressed body. For list pages
// of 400 KB and 2 MB, windows of 1 MiB and 4 MiB sent no fewer bytes and held
// up to 2.7 and 4.4 times the memory of 256 KiB.
const responseWindow = 256 << 10

// stageSize is the least a compressed body collects before it passes its
// writes to the encoder. A templ component writes many short strings, and
// an encoder takes bytes: each string would be a call and a copy of its own.
const stageSize = 4 << 10

// sniffLen is how much of a body [http.DetectContentType] reads.
const sniffLen = 512

type responseCompression struct {
	codings []*contentcoding.Coding
	minSize int
	bufs    sync.Pool // *[]byte of capacity max(minSize, stageSize).
	firsts  sync.Pool // Empty http.Header for [compressingWriter.first].
}

// newResponseCompression returns nil when compression is disabled.
// conf must be nil or valid.
func newResponseCompression(
	conf *datapages.ResponseCompressionConfig,
) *responseCompression {
	if datapages.IsDevMode() {
		// datapages watch adds its reload script to HTML,
		// and it decodes only gzip and brotli.
		return nil
	}
	var c datapages.ResponseCompressionConfig
	if conf != nil {
		c = *conf
	}
	if c.Disabled {
		return nil
	}
	codings := c.Codings
	if len(codings) == 0 {
		codings = []datapages.Coding{
			datapages.CodingZstd, datapages.CodingBrotli, datapages.CodingGzip,
		}
	}
	bc := &responseCompression{
		minSize: cmp.Or(c.MinSize, datapages.DefaultResponseMinSize),
	}
	for _, name := range codings {
		level := cmp.Or(c.GzipLevel, datapages.DefaultResponseGzipLevel)
		switch name {
		case datapages.CodingZstd:
			level = cmp.Or(c.ZstdLevel, datapages.DefaultResponseZstdLevel)
		case datapages.CodingBrotli:
			level = cmp.Or(c.BrotliQuality, datapages.DefaultResponseBrotliQuality)
		}
		bc.codings = append(bc.codings,
			contentcoding.New(string(name), level, responseWindow))
	}
	size := max(bc.minSize, stageSize)
	bc.bufs.New = func() any {
		b := make([]byte, 0, size)
		return &b
	}
	bc.firsts.New = func() any { return make(http.Header) }
	return bc
}

// wrap compresses the responses of next except under assetsPrefix,
// whose handlers pass every response through.
func (bc *responseCompression) wrap(
	next http.Handler, assetsPrefix string,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if assetsPrefix != "" && strings.HasPrefix(r.URL.Path, assetsPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		// Compressing HEAD gives it the headers of GET. net/http drops the body.
		cw := &compressingWriter{
			ResponseWriter: w,
			bc:             bc,
			coding: contentcoding.Negotiate(
				bc.codings, r.Header["Accept-Encoding"],
			),
		}
		defer cw.release()
		next.ServeHTTP(cw, r)
		cw.finish()
	})
}

// compressingWriter buffers a body until the minimum size, a flush or the end
// of the handler. An ineligible response passes through immediately.
//
// Pass-through writes do not touch its state because SSE writes may race with
// [ResponseBodyWritten].
type compressingWriter struct {
	http.ResponseWriter
	bc     *responseCompression
	coding *contentcoding.Coding  // Nil when the request accepts no coding.
	enc    *contentcoding.Encoder // Non-nil while the body is compressed.

	// buf holds the body back before the decision and collects the writes
	// to enc after it. bufp is the pool entry it came from.
	bufp *[]byte
	buf  []byte

	err    error // The first failed write to enc or of the held-back body.
	status int   // The first status the handler wrote, 0 for the implicit 200.
	// threshold is the body size [compressingWriter.check] waits for.
	threshold int
	decided   bool
	// wrote includes buffered and compressed writes.
	wrote       bool
	passThrough bool
	hijacked    bool
	// first holds the headers of the first write, which net/http sends, while
	// the body is held back.
	first http.Header
}

// BodyWritten reports buffered and compressed writes that the underlying
// writer may not have seen.
func (cw *compressingWriter) BodyWritten() bool { return cw.wrote }

// PassThrough sends the response uncompressed. [ServeFile] calls it for a file.
func (cw *compressingWriter) PassThrough() { cw.passThrough = true }

// Unwrap exposes the writer underneath to [http.ResponseController].
func (cw *compressingWriter) Unwrap() http.ResponseWriter { return cw.ResponseWriter }

func (cw *compressingWriter) WriteHeader(code int) {
	switch {
	case code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols:
		// An informational status precedes the final one and sends no body.
		cw.ResponseWriter.WriteHeader(code)
		return
	case cw.decided:
		cw.ResponseWriter.WriteHeader(code)
		return
	case cw.status != 0, cw.wrote:
		// net/http sends the first status, or 200 for a write before one,
		// and drops every later one.
		return
	}
	cw.status = code
	cw.check()
}

func (cw *compressingWriter) Write(p []byte) (int, error) {
	if cw.decided && cw.enc == nil {
		return cw.ResponseWriter.Write(p)
	}
	cw.wrote = true
	n := 0
	if !cw.decided {
		if cw.check() {
			if cw.err != nil {
				return 0, cw.err
			}
			return cw.ResponseWriter.Write(p)
		}
		n = cw.hold(len(p))
		cw.buf = append(cw.buf, p[:n]...)
		if len(cw.buf) < cw.threshold {
			return n, nil
		}
		cw.decide(false)
		if cw.enc == nil {
			if cw.err != nil || n == len(p) {
				return n, cw.err
			}
			m, err := cw.ResponseWriter.Write(p[n:])
			return n + m, err
		}
	}
	m, err := cw.compress(p[n:])
	return n + m, err
}

// WriteString keeps io.WriteString from copying s into a new byte slice,
// as Write does for the writer underneath.
func (cw *compressingWriter) WriteString(s string) (int, error) {
	if cw.decided && cw.enc == nil {
		return io.WriteString(cw.ResponseWriter, s)
	}
	cw.wrote = true
	n := 0
	if !cw.decided {
		if cw.check() {
			if cw.err != nil {
				return 0, cw.err
			}
			return io.WriteString(cw.ResponseWriter, s)
		}
		n = cw.hold(len(s))
		cw.buf = append(cw.buf, s[:n]...)
		if len(cw.buf) < cw.threshold {
			return n, nil
		}
		cw.decide(false)
		if cw.enc == nil {
			if cw.err != nil || n == len(s) {
				return n, cw.err
			}
			m, err := io.WriteString(cw.ResponseWriter, s[n:])
			return n + m, err
		}
	}
	m, err := cw.compressString(s[n:])
	return n + m, err
}

// ReadFrom preserves the underlying ReaderFrom and sendfile paths for
// pass-through responses. Other responses go through Write.
func (cw *compressingWriter) ReadFrom(src io.Reader) (int64, error) {
	if !cw.decided && cw.check() && cw.err != nil {
		return 0, cw.err
	}
	if cw.decided && cw.enc == nil {
		if rf, ok := cw.ResponseWriter.(io.ReaderFrom); ok {
			return rf.ReadFrom(src)
		}
		return io.Copy(cw.ResponseWriter, src)
	}
	return io.Copy(compressingWriteOnly{cw}, src)
}

// compressingWriteOnly prevents io.Copy from calling ReadFrom recursively.
type compressingWriteOnly struct{ cw *compressingWriter }

func (w compressingWriteOnly) Write(p []byte) (int, error) { return w.cw.Write(p) }

func (cw *compressingWriter) Flush() { _ = cw.FlushError() }

// FlushError commits the compression decision and flushes the response.
func (cw *compressingWriter) FlushError() error {
	if !cw.decided && !cw.check() {
		cw.decide(false)
	}
	if cw.enc != nil {
		if err := cw.flushStage(); err != nil {
			return err
		}
		if err := cw.enc.Flush(); err != nil {
			cw.err = err
			return err
		}
	} else if cw.err != nil {
		return cw.err
	}
	return http.NewResponseController(cw.ResponseWriter).Flush()
}

// Hijack sends buffered bytes uncompressed before handing over the connection.
func (cw *compressingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if !cw.decided {
		cw.sendHeld()
	}
	conn, rw, err := http.NewResponseController(cw.ResponseWriter).Hijack()
	if err == nil {
		cw.hijacked = true
	}
	return conn, rw, err
}

func (cw *compressingWriter) hold(n int) int {
	if cw.bufp == nil {
		cw.bufp = cw.bc.bufs.Get().(*[]byte)
		cw.buf = (*cw.bufp)[:0]
	}
	return min(n, cap(cw.buf)-len(cw.buf))
}

// check passes ineligible responses through. Otherwise it sets the body size
// needed for a decision. It runs when net/http would send the headers.
//
// Without a Content-Type, net/http sniffs the type from the first 512 bytes,
// and the decision waits for as many.
func (cw *compressingWriter) check() bool {
	if cw.threshold != 0 {
		return false
	}
	ct, declared := cw.Header()["Content-Type"]
	if cw.refuses() || declared && (len(ct) == 0 || !contentcoding.Compressible(ct[0])) {
		cw.sendHeld()
		return true
	}
	cw.threshold = cw.bc.minSize
	if !declared {
		cw.threshold = max(cw.threshold, sniffLen)
	}
	// A shallow copy keeps them, since Header methods replace or extend value
	// slices instead of editing them.
	cw.first = cw.bc.firsts.Get().(http.Header)
	maps.Copy(cw.first, cw.Header())
	return false
}

// restoreFirst resets the headers to those of the first write, except for
// trailers, which net/http reads at the end of the handler.
func (cw *compressingWriter) restoreFirst() {
	if cw.first == nil {
		return
	}
	h := cw.Header()
	for k := range h {
		if _, ok := cw.first[k]; !ok && !isTrailer(k, cw.first) {
			delete(h, k)
		}
	}
	for k, v := range cw.first {
		if !isTrailer(k, cw.first) {
			h[k] = v
		}
	}
	cw.releaseFirst()
}

func (cw *compressingWriter) releaseFirst() {
	if cw.first == nil {
		return
	}
	clear(cw.first)
	cw.bc.firsts.Put(cw.first)
	cw.first = nil
}

// isTrailer reports whether net/http sends k as a trailer of a response
// with the headers h.
func isTrailer(k string, h http.Header) bool {
	if strings.HasPrefix(k, http.TrailerPrefix) {
		return true
	}
	for _, v := range h["Trailer"] {
		for name := range strings.SplitSeq(v, ",") {
			if http.CanonicalHeaderKey(strings.TrimSpace(name)) == k {
				return true
			}
		}
	}
	return false
}

// refuses reports whether the status or headers forbid compression.
func (cw *compressingWriter) refuses() bool {
	if cw.passThrough {
		return true
	}
	switch cw.status {
	case http.StatusSwitchingProtocols, http.StatusNoContent, http.StatusResetContent,
		http.StatusPartialContent, http.StatusNotModified:
		return true
	}
	h := cw.Header()
	if h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" {
		return true
	}
	if ar := strings.TrimSpace(h.Get("Accept-Ranges")); ar != "" &&
		!strings.EqualFold(ar, "none") {
		return true
	}
	return contentcoding.NoTransform(h)
}

// decide compresses an eligible body. final means the buffer holds the full body.
func (cw *compressingWriter) decide(final bool) {
	cw.restoreFirst()
	h := cw.Header()
	ctValues, declared := h["Content-Type"]
	switch {
	case cw.refuses(), len(cw.buf) == 0, final && len(cw.buf) < cw.bc.minSize:
		cw.sendHeld()
		return
	}
	var ct string
	switch {
	case !declared:
		ct = http.DetectContentType(cw.buf)
	case len(ctValues) > 0:
		ct = ctValues[0]
	}
	if !contentcoding.Compressible(ct) {
		cw.sendHeld()
		return
	}
	// Other requests to the same URL may get a coding this one doesn't.
	contentcoding.AddVaryAcceptEncoding(h)
	if cw.coding == nil {
		cw.sendHeld()
		return
	}
	cw.decided = true
	if !declared {
		// net/http would sniff the encoded bytes.
		h["Content-Type"] = sniffedType(ct)
	}
	cw.coding.SetContentEncoding(h)
	h.Del("Content-Length")
	if tag := h.Get("Etag"); strings.HasPrefix(tag, `"`) {
		// A strong tag would claim the encoded bytes equal the uncompressed ones.
		// See RFC 9110 section 8.8.1.
		h.Set("Etag", "W/"+tag)
	}
	// Commit the headers now, as the first write would. The encoder may hold
	// its output back for long after the handler wrote it.
	cw.ResponseWriter.WriteHeader(cmp.Or(cw.status, http.StatusOK))
	cw.enc = cw.coding.Get(cw.ResponseWriter)
}

// sniffedTypes holds a shared value slice per type that
// [http.DetectContentType] returns, a fixed set.
var sniffedTypes sync.Map // Type to []string.

func sniffedType(ct string) []string {
	if v, ok := sniffedTypes.Load(ct); ok {
		return v.([]string)
	}
	v, _ := sniffedTypes.LoadOrStore(ct, []string{ct})
	return v.([]string)
}

// sendHeld writes the buffer uncompressed and passes later writes through.
func (cw *compressingWriter) sendHeld() {
	cw.restoreFirst()
	cw.decided = true
	if cw.status != 0 {
		cw.ResponseWriter.WriteHeader(cw.status)
	}
	if len(cw.buf) > 0 {
		_, cw.err = cw.ResponseWriter.Write(cw.buf)
	}
	cw.releaseBuf()
}

// compress batches short writes and sends large writes directly to the encoder.
func (cw *compressingWriter) compress(p []byte) (int, error) {
	if cw.err != nil {
		return 0, cw.err
	}
	if len(p) > cap(cw.buf)-len(cw.buf) {
		if err := cw.flushStage(); err != nil {
			return 0, err
		}
		if len(p) >= cap(cw.buf) {
			if _, err := cw.enc.Write(p); err != nil {
				cw.err = err
				return 0, err
			}
			return len(p), nil
		}
	}
	cw.buf = append(cw.buf, p...)
	return len(p), nil
}

// compressString avoids converting s to []byte.
func (cw *compressingWriter) compressString(s string) (int, error) {
	n := 0
	for {
		if cw.err != nil {
			return n, cw.err
		}
		m := min(len(s)-n, cap(cw.buf)-len(cw.buf))
		cw.buf = append(cw.buf, s[n:n+m]...)
		if n += m; n == len(s) {
			return n, nil
		}
		_ = cw.flushStage()
	}
}

func (cw *compressingWriter) flushStage() error {
	if cw.err != nil || len(cw.buf) == 0 {
		return cw.err
	}
	_, cw.err = cw.enc.Write(cw.buf)
	cw.buf = cw.buf[:0]
	return cw.err
}

// finish commits a buffered body and closes its encoder.
func (cw *compressingWriter) finish() {
	if cw.hijacked {
		return
	}
	if !cw.decided {
		if !cw.wrote && cw.status == 0 {
			return
		}
		cw.decide(true)
	}
	if cw.enc == nil {
		return
	}
	if cw.flushStage() == nil {
		cw.err = cw.enc.Close()
	}
	cw.coding.Put(cw.enc)
	cw.enc = nil
}

// release returns resources after a handler panic.
func (cw *compressingWriter) release() {
	if cw.enc != nil {
		cw.coding.Put(cw.enc)
		cw.enc = nil
	}
	cw.releaseFirst()
	cw.releaseBuf()
}

func (cw *compressingWriter) releaseBuf() {
	if cw.bufp == nil {
		return
	}
	*cw.bufp = cw.buf[:0]
	cw.bc.bufs.Put(cw.bufp)
	cw.bufp, cw.buf = nil, nil
}
