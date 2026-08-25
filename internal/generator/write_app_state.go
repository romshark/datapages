package generator

import (
	"github.com/romshark/datapages/internal/parser/model"
)

// stateSuffix is the suffix used to form per-page state symbol names.
// Since state types may carry any exported name, the full type name is
// used verbatim — e.g. "StateIndex" gives "allocateStateIndex" and
// "TabContext" gives "allocateTabContext".
func stateSuffix(st *model.StateType) string {
	return st.TypeName
}

// stateTypeRef returns the *model.StateType for a given type name;
// the parser guarantees the name is registered when used by a handler.
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
	w.Line(1, "// The page carries an identifier that stands for one tab's state.")
	w.Line(1, "// A cache that hands it to a second visitor hands over the state.")
	w.Line(1, `w.Header().Set("Cache-Control", "no-store")`)
}

// writeStateFetchWrapper emits the inline script that replaces globalThis.fetch
// on a stateful page. The wrapper adds the instance id to every same-origin
// Datastar request, reloads once when the server rejects the id,
// and reloads on a back/forward-cache restore. Nothing else on the page sees the id.
//
// It is written before the Datastar bundle and runs at parse time.
// A module script would be deferred and would miss requests made before it installs.
//
// The id is read back from the response header set by the page GET.
// It is verified again here: the value reaches the page inside a
// JavaScript string literal, and only a value the server signed may get there.
//
// The script drops its own node once __dpInstance holds the id. The closure is
// the copy the wrapper needs, which makes the node's text a second copy of
// a bearer credential, sitting where every later reader of the DOM finds it.
// Removing it does not clear the id from the response body.
func (w *Writer) writeStateFetchWrapper() {
	if !w.usage.stateRuntime {
		return
	}
	w.Line(1, "// The instance id is a bearer credential: presenting it claims a tab's state.")
	w.Line(1, "// The script below closes over the id and then drops its own node.")
	w.Line(1, "// The closure is what the fetch wrapper reads, and no copy is")
	w.Line(1, "// left in the DOM for a later reader to lift, a session replay")
	w.Line(1, "// recorder or an error reporter included. The response body still")
	w.Line(1, "// carries the id, which is what the page's Cache-Control: no-store is for.")
	w.Raw("\tif id := w.Header().Get(stateInstanceIDHeader); s.verifyStateInstanceID(id) {\n")
	w.Raw("\t\tif _, err := io.WriteString(w, `<script>(() => {\n")
	w.Raw("\t\tlet __dpInstance=\"`); err != nil { return err }\n")
	w.Raw("\t\tif _, err := io.WriteString(w, id); err != nil { return err }\n")
	w.Raw("\t\tif _, err := io.WriteString(w, `\"\n")
	w.Raw(`		document.currentScript?.remove()
		const k="datapages-reloaded:"+location.pathname
		const mark=v => { try { v ? sessionStorage.setItem(k,"1"):sessionStorage.removeItem(k) } catch {} }
		const marked=() => { try { return !!sessionStorage.getItem(k) } catch { return false } }
		const o2 = globalThis.fetch.bind(globalThis)
		globalThis.fetch=(i,init={}) => {
			const isReq=i instanceof Request
			const r=isReq ? i:new Request(i,init)
			if (r.headers.get("Datastar-Request")!=="true" ||
				new URL(r.url,location.href).origin!==location.origin
			) return isReq ? o2(r,init):o2(r)
			const h=new Headers(r.headers)
			if (__dpInstance) h.set("Datapages-Instance",__dpInstance)
			return o2(new Request(r,{...init,headers:h})).then(resp => {
				if (resp.status===409 && resp.headers.get("Datapages-Retry")==="reconnect") {
					__dpInstance=""
					if (!marked()) {
						mark(true)
						location.reload()
					}
				} else if (resp.ok) {
					mark(false)
				}
				return resp
			})
		}
		globalThis.addEventListener("pageshow", e => {
			if (e.persisted) location.reload()
		})
	})()</script>`)
	w.Raw("`); err != nil { return err }\n")
	w.Raw("\t}\n")
}

// writeVerifyInstanceIDHeader emits the header-read + HMAC-verify preamble
// shared by stateful stream handlers and stateful action handlers.
// On failure it writes 409 Conflict + Datapages-Retry: reconnect and returns.
func (w *Writer) writeVerifyInstanceIDHeader() {
	w.Line(1, "instanceID := r.Header.Get(stateInstanceIDHeader)")
	w.Line(1, "if !s.verifyStateInstanceID(instanceID) {")
	w.Line(2, "w.Header().Set(stateRetryHeader, stateRetryReconnect)")
	w.Line(2, "http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)")
	w.Line(2, "return")
	w.Line(1, "}")
}

// writeStateCapacityCheck emits the capacity check of a stateful stream handler.
// The stream request commits its status line before the open hook runs,
// which leaves this as the last point where a full server can say so.
// A reconnect reuses its instance and passes.
func (w *Writer) writeStateCapacityCheck(st *model.StateType) {
	w.Linef(1, "if _, live := s.lookup%s(instanceID); !live && !s.stateHasCapacity() {",
		stateSuffix(st))
	w.Line(2, `w.Header().Set("Retry-After", "5")`)
	w.Line(2, "http.Error(w,")
	w.Line(3, "http.StatusText(http.StatusServiceUnavailable),")
	w.Line(3, "http.StatusServiceUnavailable)")
	w.Line(2, "return")
	w.Line(1, "}")
}

// writeStateRouteKeyVar emits the local that names this tab in message broker subjects.
// Handlers receive it as their `stateID` parameter and
// dispatch tab-scoped events with it.
func (w *Writer) writeStateRouteKeyVar() {
	w.Line(1, "stateID := s.stateRouteKey(instanceID)")
}

// writeLookupSlotOrReject emits the code that looks up an allocated slot by instanceID,
// responding 409+reconnect if missing. Used by stateful action handlers;
// the stream handler uses allocate/reconnect instead.
//
// It only finds the slot. writeLockSlotOrReject is what claims it, and the two sit apart:
// everything a request reads from the network belongs between them,
// so that no client holds a tab's mutex by sending a body slowly.
func (w *Writer) writeLookupSlotOrReject(st *model.StateType) {
	suffix := stateSuffix(st)
	w.Linef(1, "slot, ok := s.lookup%s(instanceID)", suffix)
	w.Line(1, "if !ok {")
	w.Line(2, "w.Header().Set(stateRetryHeader, stateRetryReconnect)")
	w.Line(2, "http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)")
	w.Line(2, "return")
	w.Line(1, "}")
}

// writeLockSlotOrReject emits the code that takes the slot mutex for the rest
// of the handler. It is emitted once the request has been read in full,
// since the mutex serializes every handler of one tab, including the SSE event loop:
// a client that trickles its body would otherwise stall its own tab,
// and events published to a stalled stream are dropped once its buffer fills.
//
// The stream can close between the lookup and the lock and drop the state,
// which is why liveness is re-checked here.
func (w *Writer) writeLockSlotOrReject() {
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
// its slot type and instance map. App-level action handlers,
// which belong to no page, resolve the same symbols from their `state *T` parameter.

func stateSlotTypeName(st *model.StateType) string {
	return "stateSlot" + st.TypeName
}

func stateMapName(st *model.StateType) string {
	return "stateInstances" + st.TypeName
}

// pageStateSlotTypeName returns the slot type name for the page's bound state.
// The page must have State != nil.
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
//   - The sharded instance store, shared by all state types
//   - Per state type: slot type + store of live instances
//   - HMAC sign/verify helpers for Datapages-Instance header
//   - Mint/lookup/release/reconnect helpers
//
// This runtime is only emitted when at least one page declares a state type.
func (w *Writer) writeStateRuntime(m *model.App, appPkg string) {
	if !w.usage.stateRuntime {
		return
	}

	w.writeStateHMACHelpers()
	w.writeStateStoreType()

	for _, st := range boundStateTypes(m) {
		w.writeStateSlot(st, appPkg)
	}
}

// writeStateStoreType emits the map that holds live instances.
// One type serves every state type, instantiated per slot type.
func (w *Writer) writeStateStoreType() {
	w.Raw(`
// stateStoreShards is how many independent maps a stateStore spreads its keys over.
// Instance ids are random, so they distribute evenly,
// and each shard carries its own lock.
const stateStoreShards = 32

// stateStoreSeed randomizes shard selection per process.
var stateStoreSeed = maphash.MakeSeed()

// stateStoreShard is one lock and the keys that belong to it.
type stateStoreShard[S any] struct {
	mu sync.RWMutex
	m  map[string]*S
}

// stateStore maps a verified Datapages-Instance id to the live slot it names.
//
// Every key is written once and deleted once, and none is read after its deletion.
// That is the opposite of what sync.Map is tuned for,
// which is a read-mostly set of stable keys. Sharded plain maps read without a write
// barrier and delete without leaving a tombstone behind.
//
// The zero value is ready to use.
type stateStore[S any] struct {
	shards [stateStoreShards]stateStoreShard[S]
}

func (s *stateStore[S]) shard(id string) *stateStoreShard[S] {
	return &s.shards[maphash.String(stateStoreSeed, id)%stateStoreShards]
}

// Load returns the slot registered under id, or (nil, false).
func (s *stateStore[S]) Load(id string) (*S, bool) {
	sh := s.shard(id)
	sh.mu.RLock()
	slot, ok := sh.m[id]
	sh.mu.RUnlock()
	return slot, ok
}

// Store registers slot under id, replacing whatever was there.
func (s *stateStore[S]) Store(id string, slot *S) {
	sh := s.shard(id)
	sh.mu.Lock()
	if sh.m == nil {
		sh.m = make(map[string]*S)
	}
	sh.m[id] = slot
	sh.mu.Unlock()
}

// CompareAndDelete removes id only while it still names slot.
// A stream can allocate a fresh slot under an id whose predecessor is on its way out,
// and the one leaving must not take the new one with it.
func (s *stateStore[S]) CompareAndDelete(id string, slot *S) bool {
	sh := s.shard(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.m[id] != slot {
		return false
	}
	delete(sh.m, id)
	return true
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

// stateInstanceIDTag and the tag stateRouteKey writes name what each MAC is for.
// One key, two derivations: without a tag of its own on each,
// a value computed for one of them is a value the other would accept.
var stateInstanceIDTag = []byte("datapages-instance\x00")

// signStateInstanceID produces an HMAC-signed Datapages-Instance value.
// The payload is 16 random bytes; the output is:
// base64url(payload) + "~" + base64url(hmacSHA256(tag, payload)).
func (s *Server) signStateInstanceID() (string, error) {
	if s.stateConf == nil {
		return "", errors.New("state runtime not configured")
	}
	var payload [16]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.stateConf.HMACKey)
	mac.Write(stateInstanceIDTag)
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
	mac.Write(stateInstanceIDTag)
	mac.Write(payload)
	return hmac.Equal(mac.Sum(nil), sig)
}

// stateRouteKey derives the value that names a tab in message broker subjects.
// Subjects travel further than a request does: into broker logs,
// stream storage, traces and metrics. The Datapages-Instance id itself
// stays out of them, since presenting it is what claims a tab's state.
// Knowing the routing key only lets a dispatcher address that tab.
//
// Callers must verify the id first.
func (s *Server) stateRouteKey(id string) string {
	mac := hmac.New(sha256.New, s.stateConf.HMACKey)
	mac.Write([]byte("datapages-route\x00"))
	mac.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

// stateLiveInstances counts the instances of every state type.
// Memory is shared between them,
// and so is the budget in datapages.StateConfig.MaxConcurrentInstances.
var stateLiveInstances atomic.Int64

// stateNoInstanceLimit is what a negative MaxConcurrentInstances means:
// no budget at all. The counter is still kept, since releases decrement it
// unconditionally and a configuration is free to change between processes.
func (s *Server) stateNoInstanceLimit() bool {
	return s.stateConf.MaxConcurrentInstances < 0
}

// stateReserveInstance takes one slot out of the budget. It reports false
// when the server is full, which fails the stream open that asked for it.
// An unlimited server serves every caller.
func (s *Server) stateReserveInstance() bool {
	n := stateLiveInstances.Add(1)
	if s.stateNoInstanceLimit() {
		return true
	}
	if n > int64(s.stateConf.MaxConcurrentInstances) {
		stateLiveInstances.Add(-1)
		return false
	}
	return true
}

// stateHasCapacity reports whether the budget has room right now.
// It is a look, not a claim, and callers use it before a stream commits its status line.
// stateReserveInstance is what actually holds the bound.
func (s *Server) stateHasCapacity() bool {
	if s.stateNoInstanceLimit() {
		return true
	}
	return stateLiveInstances.Load() < int64(s.stateConf.MaxConcurrentInstances)
}

// errStateAtCapacity fails a stream open that finds no free instance.
var errStateAtCapacity = errors.New("state instance limit reached")
`)
}

func (w *Writer) writeStateSlot(st *model.StateType, appPkg string) {
	slot := stateSlotTypeName(st)
	instances := stateMapName(st)
	stateType := st.TypeName

	w.Raw("\n")
	w.Linef(0, "// %s holds one instance of %s.", slot, stateType)
	w.Line(0, "// It is allocated on StreamOpen and dropped on StreamClose.")
	w.Line(0, "// An instance lives exactly as long as the stream that created it and")
	w.Line(0, "// is never reused: a client that reconnects gets a new one.")
	w.Linef(0, "type %s struct {", slot)
	w.Linef(1, "state *%s.%s", appPkg, stateType)
	w.Line(1, "mu    sync.Mutex // serializes all stateful handler calls on this instance")
	w.Line(1, "dead  bool       // true once the stream closed and the state was dropped")
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// %s maps a verified Datapages-Instance id to the live slot.", instances)
	w.Linef(0, "var %s stateStore[%s]", instances, slot)

	w.writeStateMethods(st, appPkg)
}

// writeStateMethods emits the allocate/lookup/release methods of a state type.
// - allocate<T>: called by StreamOpen; allocates the state, registers slot.
// - lookup<T>:   called by actions/OnXXX; returns the slot or (nil, false).
// - release<T>:  called by StreamClose; drops the state at once.
func (w *Writer) writeStateMethods(st *model.StateType, appPkg string) {
	slot := stateSlotTypeName(st)
	instances := stateMapName(st)
	stateType := st.TypeName
	suffix := st.TypeName

	w.Raw("\n")
	w.Linef(0,
		"// allocate%s allocates a zeroed state value, registers it under id",
		suffix)
	w.Line(0, "// and hands it to the SSE stream that asked for it.")
	w.Line(0, "// Returns the slot so callers (StreamOpen) can pass the state to the user,")
	w.Line(0, "// or nil when the server holds as many instances as it may.")
	w.Linef(0, "func (s *Server) allocate%s(id string) *%s {", suffix, slot)
	w.Line(1, "if !s.stateReserveInstance() {")
	w.Line(2, "return nil")
	w.Line(1, "}")
	w.Linef(1, "slot := &%s{state: new(%s.%s)}", slot, appPkg, stateType)
	w.Linef(1, "%s.Store(id, slot)", instances)
	w.Line(1, "return slot")
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// lookup%s returns the registered slot for id, or (nil, false)", suffix)
	w.Line(0, "// when no live instance matches. The id is not HMAC-verified here;")
	w.Line(0, "// call verifyStateInstanceID first.")
	w.Linef(0, "func (s *Server) lookup%s(id string) (*%s, bool) {", suffix, slot)
	w.Linef(1, "return %s.Load(id)", instances)
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// release%s drops the slot's state the moment its stream closes.", suffix)
	w.Line(0, "// Nothing of the instance outlives the stream: a client that")
	w.Line(0, "// reconnects opens a new stream and gets a freshly allocated state.")
	w.Line(0, "//")
	w.Line(0, "// The caller passes the slot it allocated rather than the id alone.")
	w.Line(0, "// A tab can hold two streams at once while the server still tears the")
	w.Line(0, "// older one down, and the second registers its own slot under the same id.")
	w.Line(0, "// Releasing by id would give back whichever slot the map holds now.")
	w.Line(0, "//")
	w.Line(0, "// Calling this twice on one slot releases it once.")
	w.Linef(0, "func (s *Server) release%s(id string, slot *%s) {", suffix, slot)
	w.Line(1, "slot.mu.Lock()")
	w.Line(1, "if slot.dead {")
	w.Line(2, "slot.mu.Unlock()")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "slot.dead = true")
	w.Line(1, "slot.state = nil")
	w.Line(1, "slot.mu.Unlock()")
	w.Line(1, "// A stream can allocate a fresh slot under this id while this")
	w.Line(1, "// one is on its way out. Drop only the slot this call owns.")
	w.Linef(1, "%s.CompareAndDelete(id, slot)", instances)
	w.Line(1, "stateLiveInstances.Add(-1)")
	w.Line(0, "}")
}
