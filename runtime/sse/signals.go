package sse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/romshark/datapages/runtime/httpread"
)

var bodyBufs = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// maxPooledBuf matches fmt's limit, which keeps a rare large body from
// staying in bodyBufs. See [Go issue 23199].
//
// [Go issue 23199]: https://go.dev/issue/23199
const maxPooledBuf = 64 << 10

// ReadSignals decodes Datastar signals into v. GET and DELETE use the datastar
// query parameter; other methods use the body, as the [SDK specification] lists.
// A missing query leaves v unchanged.
//
// [SDK specification]: https://github.com/starfederation/datastar/blob/v1.0.4/sdk/ADR.md#readsignals
func ReadSignals(r *http.Request, v any) error {
	if r.Method == http.MethodGet || r.Method == http.MethodDelete {
		q := httpread.QueryValue(r.URL.RawQuery, "datastar")
		if q == "" {
			return nil
		}
		return json.Unmarshal([]byte(q), v)
	}
	b := bodyBufs.Get().(*bytes.Buffer)
	defer func() {
		if b.Cap() <= maxPooledBuf {
			b.Reset()
			bodyBufs.Put(b)
		}
	}()
	if _, err := b.ReadFrom(r.Body); err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	return json.Unmarshal(b.Bytes(), v)
}
