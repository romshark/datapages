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

// ReadSignals decodes Datastar signals into v. GET and DELETE use the datastar
// query parameter; other methods use the body. A missing query leaves v unchanged.
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
