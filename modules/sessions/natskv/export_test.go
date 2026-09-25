package natskv

import "github.com/nats-io/nats.go"

// WrapKV replaces sm's KV handle so tests can
// interleave bucket operations within a manager call.
func WrapKV[Data any](sm *SessionManager[Data], wrap func(nats.KeyValue) nats.KeyValue) {
	sm.kv = wrap(sm.kv)
}
