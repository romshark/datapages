package generator

import (
	"github.com/romshark/datapages/parser/model"
)

// stateSuffix is the suffix used to form per-page state symbol names.
// Since state types may carry any exported name, the full type name is
// used verbatim — e.g. "StateIndex" gives "allocateStateIndex" and
// "TabContext" gives "allocateTabContext".
func stateSuffix(st *model.StateType) string {
	return st.TypeName
}

// stateTypeRef returns the *model.StateType for a given type name; the
// parser guarantees the name is registered when used by a handler.
func stateTypeRef(m *model.App, typeName string) *model.StateType {
	return m.States[typeName]
}

// writeMintInstanceIDOnGET emits the code that signs a new Datapages-Instance
// identifier and sets it on the response header on the GET of a stateful page.
// Placed before the user's GET method is invoked so the header is present on
// the response even when the handler writes body content early.
func (w *Writer) writeMintInstanceIDOnGET() {
	w.Line(0, "")
	w.Line(1, "// Mint the per-instance identifier for this page load.")
	w.Line(1, "// The client echoes this value on action requests and on the SSE")
	w.Line(1, "// stream connect via the Datapages-Instance header.")
	w.Line(1, "instanceID, err := s.signStateInstanceID()")
	w.Line(1, "if err != nil {")
	w.Line(2, `s.httpErrIntern(w, r, nil, "minting state instance", err)`)
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "w.Header().Set(stateInstanceIDHeader, instanceID)")
}

// writeVerifyInstanceIDHeader emits the header-read + HMAC-verify preamble
// shared by stateful stream handlers and stateful action handlers. On
// failure it writes 409 Conflict + Datapages-Retry: reconnect and returns.
func (w *Writer) writeVerifyInstanceIDHeader() {
	w.Line(1, "instanceID := r.Header.Get(stateInstanceIDHeader)")
	w.Line(1, "if !s.verifyStateInstanceID(instanceID) {")
	w.Line(2, "w.Header().Set(stateRetryHeader, stateRetryReconnect)")
	w.Line(2, "http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)")
	w.Line(2, "return")
	w.Line(1, "}")
}

// writeLookupSlotOrReject emits the code that looks up an allocated slot
// by instanceID, responding 409+reconnect if missing. Used by stateful
// action handlers; the stream handler uses allocate/reconnect instead.
// The grace timer can expire between the lookup and the lock and return
// the state to the pool, which is why liveness is re-checked under the lock.
func (w *Writer) writeLookupSlotOrReject(st *model.StateType) {
	suffix := stateSuffix(st)
	w.Linef(1, "slot, ok := s.lookup%s(instanceID)", suffix)
	w.Line(1, "if !ok {")
	w.Line(2, "w.Header().Set(stateRetryHeader, stateRetryReconnect)")
	w.Line(2, "http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "slot.mu.Lock()")
	w.Line(1, "defer slot.mu.Unlock()")
	w.Line(1, "if slot.dead {")
	w.Line(2, "w.Header().Set(stateRetryHeader, stateRetryReconnect)")
	w.Line(2, "http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)")
	w.Line(2, "return")
	w.Line(1, "}")
}

// State runtime symbols are derived from the state type's full Go name.
// The runtime belongs to the state type. Pages that share a state type share
// its slot type, pool and instance map. App-level action handlers,
// which belong to no page, resolve the same symbols from their `state *T` parameter.

func stateSlotTypeName(st *model.StateType) string {
	return "stateSlot" + st.TypeName
}

func statePoolName(st *model.StateType) string {
	return "statePool" + st.TypeName
}

func stateMapName(st *model.StateType) string {
	return "stateInstances" + st.TypeName
}

// pageStateSlotTypeName returns the slot type name for the page's bound
// state. The page must have State != nil.
func pageStateSlotTypeName(p *model.Page) string {
	return stateSlotTypeName(p.State)
}

// statefulPages returns the subset of pages that bind a state type.
func statefulPages(m *model.App) []*model.Page {
	var out []*model.Page
	for _, p := range m.Pages {
		if p.State != nil {
			out = append(out, p)
		}
	}
	return out
}

// boundStateTypes returns every state type bound by a page, in page order,
// each one once. Several pages may share a state type.
func boundStateTypes(m *model.App) []*model.StateType {
	var out []*model.StateType
	seen := map[string]bool{}
	for _, p := range statefulPages(m) {
		if seen[p.State.TypeName] {
			continue
		}
		seen[p.State.TypeName] = true
		out = append(out, p.State)
	}
	return out
}

// writeStateRuntime emits the server-side state runtime:
//
//   - StateConfig + WithStateConfig option
//   - Per state type: slot type + sync.Pool + sync.Map of live instances
//   - HMAC sign/verify helpers for Datapages-Instance header
//   - Mint/lookup/release/reconnect helpers
//
// This runtime is only emitted when at least one page declares a state type.
func (w *Writer) writeStateRuntime(m *model.App, appPkg string) {
	if !w.usage.stateRuntime {
		return
	}

	w.writeStateConfigType()
	w.writeStateConfigOption()
	w.writeStateHMACHelpers()

	for _, st := range boundStateTypes(m) {
		w.writeStateSlot(st, appPkg)
	}
}

func (w *Writer) writeStateConfigType() {
	w.Raw(`
// StateConfig configures the per-page-instance server-side state runtime.
// Pass via WithStateConfig when at least one page declares a StateXXX type.
type StateConfig struct {
	// HMACKey signs the Datapages-Instance identifier that rides on
	// request/response headers. Required; must be non-empty.
	// Rotating the key invalidates all live instances; clients recover
	// by reloading the page.
	HMACKey []byte

	// GracePeriod is how long a stateful instance survives after its SSE
	// stream closes, waiting for the client to reconnect (e.g. after a
	// transient network blip). Default 30s.
	GracePeriod time.Duration
}
`)
}

func (w *Writer) writeStateConfigOption() {
	w.Raw(`
// WithStateConfig enables the per-page-instance server-side state runtime.
// Required when at least one page declares a StateXXX type.
//
// On multi-server deployments the load balancer MUST route requests for a
// given client consistently to the same backend (sticky sessions), since
// state lives in process memory.
func WithStateConfig(conf StateConfig) ServerOption {
	return func(s *Server) error {
		if len(conf.HMACKey) == 0 {
			return errors.New("WithStateConfig: HMACKey is required")
		}
		if conf.GracePeriod <= 0 {
			conf.GracePeriod = 30 * time.Second
		}
		s.stateConf = &conf
		return nil
	}
}
`)
}

func (w *Writer) writeStateHMACHelpers() {
	w.Raw(`
// stateInstanceIDHeader is the request/response header used to carry the
// server-issued per-page-instance identifier.
const stateInstanceIDHeader = "Datapages-Instance"

// stateRetryHeader signals the client to reconnect before retrying an
// action that was rejected because no live instance was found.
const stateRetryHeader = "Datapages-Retry"

// stateRetryReconnect is the only value currently emitted on stateRetryHeader.
const stateRetryReconnect = "reconnect"

// stateInstanceIDSep separates payload and signature in a Datapages-Instance value.
// It must not be "." and must not appear in the base64url alphabet:
// the identifier is used verbatim as a single message-broker subject token
// (see the SubjectStateID event routing), and "." would split it in two,
// breaking single-token wildcard matching.
const stateInstanceIDSep = '~'

// signStateInstanceID produces an HMAC-signed Datapages-Instance value.
// The payload is 16 random bytes; the output is:
// base64url(payload) + "~" + base64url(hmacSHA256(payload)).
func (s *Server) signStateInstanceID() (string, error) {
	if s.stateConf == nil {
		return "", errors.New("state runtime not configured")
	}
	var payload [16]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.stateConf.HMACKey)
	mac.Write(payload[:])
	return base64.RawURLEncoding.EncodeToString(payload[:]) +
		string(stateInstanceIDSep) +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// verifyStateInstanceID returns true when the supplied Datapages-Instance
// value is well-formed and signed by the server's HMAC key.
func (s *Server) verifyStateInstanceID(id string) bool {
	if s.stateConf == nil || id == "" {
		return false
	}
	sep := strings.IndexByte(id, stateInstanceIDSep)
	if sep <= 0 || sep == len(id)-1 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(id[:sep])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(id[sep+1:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, s.stateConf.HMACKey)
	mac.Write(payload)
	return hmac.Equal(mac.Sum(nil), sig)
}
`)
}

func (w *Writer) writeStateSlot(st *model.StateType, appPkg string) {
	slot := stateSlotTypeName(st)
	pool := statePoolName(st)
	instances := stateMapName(st)
	stateType := st.TypeName

	w.Raw("\n")
	w.Linef(0, "// %s holds one instance of %s.", slot, stateType)
	w.Linef(0, "// It is checked out of %s on StreamOpen and returned", pool)
	w.Linef(0, "// when StreamClose elapses without a reconnect within GracePeriod.")
	w.Linef(0, "type %s struct {", slot)
	w.Linef(1, "state    *%s.%s", appPkg, stateType)
	w.Line(1, "mu       sync.Mutex // serializes all stateful handler calls on this instance")
	w.Line(1, "streamID uint64     // currently-associated SSE stream (0 when detached)")
	w.Line(1, "timer    *time.Timer // grace-period timer; nil while a stream is attached")
	w.Line(1, "dead     bool        // true once the state went back to the pool")
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// %s pools %s values across instance checkouts.", pool, stateType)
	w.Linef(0, "// Generated Reset-on-checkout zeroes the struct before handing it out.")
	w.Linef(0, "var %s = sync.Pool{", pool)
	w.Linef(1, "New: func() any { return new(%s.%s) },", appPkg, stateType)
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// %s maps a verified Datapages-Instance id to the live slot.", instances)
	w.Linef(0, "var %s sync.Map // key: string (instance id), value: *%s", instances, slot)

	w.writeStateMethods(st, appPkg)
}

// writeStateMethods emits the allocate/lookup/close methods of a state type.
// - allocate<T>: called by StreamOpen; zeroes pooled state, registers slot.
// - lookup<T>:   called by actions/OnXXX; returns the slot or (nil, false).
// - closeStream<T>: called by StreamClose; starts grace timer, detaches streamID.
// - reconnect<T>: called by StreamOpen on repeat id; cancels timer, attaches streamID.
func (w *Writer) writeStateMethods(st *model.StateType, appPkg string) {
	slot := stateSlotTypeName(st)
	pool := statePoolName(st)
	instances := stateMapName(st)
	stateType := st.TypeName
	suffix := st.TypeName

	w.Raw("\n")
	w.Linef(0,
		"// allocate%s checks a state value out of %s, zeroes it, registers it",
		suffix, pool)
	w.Line(0, "// under id, and attaches it to the given SSE streamID.")
	w.Line(0, "// Returns the slot so callers (StreamOpen) can pass the state to the user.")
	w.Linef(0, "func (s *Server) allocate%s(id string, streamID uint64) *%s {",
		suffix, slot)
	w.Linef(1, "st := %s.Get().(*%s.%s)", pool, appPkg, stateType)
	w.Linef(1, "*st = %s.%s{} // total reset; safe reuse across tenants", appPkg, stateType)
	w.Linef(1, "slot := &%s{state: st, streamID: streamID}", slot)
	w.Linef(1, "%s.Store(id, slot)", instances)
	w.Line(1, "return slot")
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// lookup%s returns the registered slot for id, or (nil, false)", suffix)
	w.Line(0, "// when no live instance matches. The id is not HMAC-verified here;")
	w.Line(0, "// call verifyStateInstanceID first.")
	w.Linef(0, "func (s *Server) lookup%s(id string) (*%s, bool) {", suffix, slot)
	w.Linef(1, "v, ok := %s.Load(id)", instances)
	w.Line(1, "if !ok {")
	w.Line(2, "return nil, false")
	w.Line(1, "}")
	w.Linef(1, "return v.(*%s), true", slot)
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// reconnect%s tries to reattach an existing slot (grace-period reconnect)", suffix)
	w.Line(0, "// to a new SSE streamID. Returns (slot, true) on success; (nil, false)")
	w.Line(0, "// when the id is unknown (e.g. grace period already elapsed).")
	w.Line(0, "// A slot whose state already went back to the pool is refused.")
	w.Linef(0, "func (s *Server) reconnect%s(id string, streamID uint64) (*%s, bool) {",
		suffix, slot)
	w.Linef(1, "slot, ok := s.lookup%s(id)", suffix)
	w.Line(1, "if !ok {")
	w.Line(2, "return nil, false")
	w.Line(1, "}")
	w.Line(1, "slot.mu.Lock()")
	w.Line(1, "defer slot.mu.Unlock()")
	w.Line(1, "if slot.dead {")
	w.Line(2, "return nil, false")
	w.Line(1, "}")
	w.Line(1, "if slot.timer != nil {")
	w.Line(2, "slot.timer.Stop()")
	w.Line(2, "slot.timer = nil")
	w.Line(1, "}")
	w.Line(1, "slot.streamID = streamID")
	w.Line(1, "return slot, true")
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// closeStream%s marks id as detached from streamID and starts", suffix)
	w.Line(0, "// the grace timer. If the client reconnects with the same id")
	w.Line(0, "// before the timer fires, the slot is reused as-is.")
	w.Line(0, "//")
	w.Line(0, "// A tab can hold two streams at once while the server still")
	w.Line(0, "// tears the older one down. Only the attached stream may detach.")
	w.Linef(0, "func (s *Server) closeStream%s(id string, streamID uint64) {", suffix)
	w.Linef(1, "slot, ok := s.lookup%s(id)", suffix)
	w.Line(1, "if !ok {")
	w.Line(1, "\treturn")
	w.Line(1, "}")
	w.Line(1, "slot.mu.Lock()")
	w.Line(1, "if slot.streamID != streamID {")
	w.Line(2, "slot.mu.Unlock()")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "slot.streamID = 0")
	w.Line(1, "if slot.timer != nil {")
	w.Line(2, "slot.timer.Stop()")
	w.Line(1, "}")
	w.Line(1, "slot.timer = time.AfterFunc(s.stateConf.GracePeriod, func() {")
	w.Line(2, "slot.mu.Lock()")
	w.Line(2, "// A stream reattached while this timer was already running,")
	w.Line(2, "// or another timer got here first.")
	w.Line(2, "if slot.streamID != 0 || slot.dead {")
	w.Line(3, "slot.mu.Unlock()")
	w.Line(3, "return")
	w.Line(2, "}")
	w.Line(2, "slot.dead = true")
	w.Line(2, "st := slot.state")
	w.Line(2, "slot.state = nil")
	w.Line(2, "slot.mu.Unlock()")
	w.Linef(2, "%s.Delete(id)", instances)
	w.Line(2, "if st != nil {")
	w.Linef(3, "%s.Put(st)", pool)
	w.Line(2, "}")
	w.Line(1, "})")
	w.Line(1, "slot.mu.Unlock()")
	w.Line(0, "}")
}
