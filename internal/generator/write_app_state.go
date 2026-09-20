package generator

import (
	"github.com/romshark/datapages/internal/parser/model"
)

// stateSuffix keeps the full type name because
// each bound state type has a separate runtime.
func stateSuffix(st *model.StateType) string {
	return st.TypeName
}

// stateTypeRef needs no presence check: the parser registers every state type
// used by a handler.
func stateTypeRef(m *model.App, typeName string) *model.StateType {
	return m.States[typeName]
}

// writeMintInstanceIDOnGET emits code that mints a new Datapages-Instance id
// for a stateful page GET. It runs before the user's GET method. The response
// therefore contains the header even when the handler writes body content early.
func (w *Writer) writeMintInstanceIDOnGET() {
	w.Line(0, "")
	w.Line(1, "instanceID, err := newStateInstanceID()")
	w.Line(1, "if err != nil {")
	w.Line(2, `s.httpErrIntern(w, r, nil, "minting state instance", err)`)
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "w.Header().Set(stateInstanceIDHeader, instanceID)")
	w.Line(1, "// A shared cache would expose this bearer id to another visitor.")
	w.Line(1, `w.Header().Set("Cache-Control", "no-store")`)
}

// writeStateFetchWrapper emits the inline script that replaces globalThis.fetch
// on a stateful page. The wrapper adds the instance id to every same-origin
// Datastar request, reloads once when the server rejects the id,
// and reloads on a back/forward-cache restore.
//
// It is written before the Datastar bundle and runs at parse time.
// A module script would be deferred and would miss requests made before it installs.
//
// The shape check makes writing the id verbatim into a JavaScript string safe.
// The script removes its node after copying the id into the wrapper closure.
// The response body still contains the id and relies on Cache-Control: no-store.
func (w *Writer) writeStateFetchWrapper() {
	if !w.usage.stateRuntime {
		return
	}
	w.Line(1, "// The id authorizes access to one tab's state. The fetch wrapper keeps")
	w.Line(1, "// it in a closure. Removing the script node prevents later DOM readers,")
	w.Line(1, "// including replay and error-reporting tools, from recording it.")
	w.Line(1, "// Cache-Control: no-store prevents caches from retaining the response body.")
	w.Raw("\tif id := w.Header().Get(stateInstanceIDHeader); wellFormedStateInstanceID(id) {\n")
	w.Raw("\t\tif _, err := io.WriteString(w, s.ScriptTagOpen(r)); err != nil { return err }\n")
	w.Raw("\t\tif _, err := io.WriteString(w, `(() => {\n")
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

func (w *Writer) writeVerifyInstanceIDHeader() {
	w.Line(1, "instanceID := r.Header.Get(stateInstanceIDHeader)")
	w.Line(1, "if !wellFormedStateInstanceID(instanceID) {")
	w.Line(2, "w.Header().Set(stateRetryHeader, stateRetryReconnect)")
	w.Line(2, "http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)")
	w.Line(2, "return")
	w.Line(1, "}")
}

// writeStateAllocateOrReject emits allocation before handleStreamRequest
// commits the SSE status. This lets a full server return 503 and ensures an
// open stream already has registered state.
//
// The deferred release also covers a stream that fails before its close hook.
// Release is idempotent because an opened stream calls it from both paths.
func (w *Writer) writeStateAllocateOrReject(p *model.Page) {
	suffix := stateSuffix(p.State)
	w.Linef(1, "slot := s.allocate%s(instanceID)", suffix)
	w.Line(1, "if slot == nil {")
	w.Line(2, `w.Header().Set("Retry-After", "5")`)
	w.Line(2, "http.Error(w,")
	w.Line(3, "http.StatusText(http.StatusServiceUnavailable),")
	w.Line(3, "http.StatusServiceUnavailable)")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Linef(1, "defer s.release%s(instanceID, slot)", suffix)
}

func (w *Writer) writeStateRouteKeyVar() {
	w.Line(1, "stateID := stateRouteKey(instanceID)")
}

// writeLookupSlotOrReject emits the stateful action lookup and its
// 409+reconnect response for a missing slot. Streams allocate instead.
//
// Request parsing stays between lookup and lock.
// A slow request body therefore cannot hold the tab mutex.
func (w *Writer) writeLookupSlotOrReject(st *model.StateType) {
	suffix := stateSuffix(st)
	w.Linef(1, "slot, ok := s.lookup%s(instanceID)", suffix)
	w.Line(1, "if !ok {")
	w.Line(2, "w.Header().Set(stateRetryHeader, stateRetryReconnect)")
	w.Line(2, "http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)")
	w.Line(2, "return")
	w.Line(1, "}")
}

// writeLockSlotOrReject emits the slot lock after the request body is read.
// The mutex serializes all handlers of one tab, including the SSE event loop.
// Locking before the read would let a slow request stall the tab and fill the
// event buffer.
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

// stateSlotTypeName uses the full state type name because pages bound to the
// same type share its slot type and instance map.
func stateSlotTypeName(st *model.StateType) string {
	return "stateSlot" + st.TypeName
}

func stateMapName(st *model.StateType) string {
	return "stateInstances" + st.TypeName
}

func stateStoreTypeRef(st *model.StateType) string {
	return "stateStore[" + stateSlotTypeName(st) + "]"
}

func statefulPages(m *model.App) []*model.Page {
	var out []*model.Page
	for _, p := range m.Pages {
		if p.State != nil {
			out = append(out, p)
		}
	}
	return out
}

// boundStateTypes preserves page order instead of ranging over m.States,
// whose map order is random.
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

func (w *Writer) writeStateRuntime(m *model.App, appPkg string) {
	if !w.usage.stateRuntime {
		return
	}

	w.writeStateIDHelpers()
	w.writeStateStoreType()

	for _, st := range boundStateTypes(m) {
		w.writeStateSlot(st, appPkg)
	}
}

func (w *Writer) writeStateStoreType() {
	w.Raw(`
const stateStoreShards = 32

// stateStoreSeed prevents clients from choosing a shard for an id.
var stateStoreSeed = maphash.MakeSeed()

type stateStoreShard[S any] struct {
	mu sync.RWMutex
	m  map[string]*S
}

// stateStore initializes shards lazily, which makes its zero value ready to use.
type stateStore[S any] struct {
	shards [stateStoreShards]stateStoreShard[S]
}

func (s *stateStore[S]) shard(id string) *stateStoreShard[S] {
	return &s.shards[maphash.String(stateStoreSeed, id)%stateStoreShards]
}

func (s *stateStore[S]) Load(id string) (*S, bool) {
	sh := s.shard(id)
	sh.mu.RLock()
	slot, ok := sh.m[id]
	sh.mu.RUnlock()
	return slot, ok
}

// Store registers slot, replacing an older stream under the same id.
func (s *stateStore[S]) Store(id string, slot *S) {
	sh := s.shard(id)
	sh.mu.Lock()
	if sh.m == nil {
		sh.m = make(map[string]*S)
	}
	sh.m[id] = slot
	sh.mu.Unlock()
}

// CompareAndDelete prevents a closing stream from deleting a replacement slot
// registered under the same id.
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

func (w *Writer) writeStateIDHelpers() {
	w.Raw(`
const stateInstanceIDHeader = "Datapages-Instance"

// stateRetryHeader tells the generated client that no state slot exists and
// that it must reconnect.
const stateRetryHeader = "Datapages-Retry"

const stateRetryReconnect = "reconnect"

// stateInstanceIDLen is the unpadded base64url length of 16 bytes.
const stateInstanceIDLen = 22

// newStateInstanceID returns 128 random bits as unpadded base64url.
//
// The id is an unsigned bearer credential. Signing would prove only that a
// server issued it. It would not make another tab's random id harder to guess
// or stop clients from opening streams to consume the instance limit.
// State is created only when a stream presents an id.
//
// Unsigned ids require no key shared between servers. A load balancer may
// route the page GET and stream to different servers. The stream's server
// allocates the state.
func newStateInstanceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// wellFormedStateInstanceID accepts exactly [stateInstanceIDLen] base64url characters.
// This bounds client-chosen map keys and permits verbatim use in
// the page's JavaScript string.
func wellFormedStateInstanceID(id string) bool {
	if len(id) != stateInstanceIDLen {
		return false
	}
	for _, c := range []byte(id) {
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		case c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// stateRouteKey keeps the bearer instance id out of broker subjects, which may
// appear in logs, stream storage, traces and metrics. It returns the first 16
// bytes of SHA-256 as unpadded base64url. The result can address the tab but
// cannot authorize state access. Its keyless derivation is stable across
// servers and process restarts.
func stateRouteKey(id string) string {
	sum := sha256.Sum256([]byte(id))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}
`)
}

func (w *Writer) writeStateSlot(st *model.StateType, appPkg string) {
	slot := stateSlotTypeName(st)
	stateType := st.TypeName

	w.Raw("\n")
	w.Linef(0, "// %s belongs to one stream and is never reused.", slot)
	w.Line(0, "// A reconnect allocates a new slot and state value.")
	w.Linef(0, "type %s struct {", slot)
	w.Linef(1, "state *%s.%s", appPkg, stateType)
	w.Line(1, "mu    sync.Mutex // serializes all stateful handler calls on this instance")
	w.Line(1, "dead  bool")
	w.Line(0, "}")

	w.writeStateMethods(st, appPkg)
}

func (w *Writer) writeStateMethods(st *model.StateType, appPkg string) {
	slot := stateSlotTypeName(st)
	instances := stateMapName(st)
	stateType := st.TypeName
	suffix := st.TypeName

	w.Raw("\n")
	w.Linef(0,
		"// allocate%s reserves capacity and registers state before the stream opens.",
		suffix)
	w.Line(0, "// It returns nil at the instance limit. id must pass [wellFormedStateInstanceID].")
	w.Linef(0, "func (s *Server) allocate%s(id string) *%s {", suffix, slot)
	w.Line(1, "if !s.ReserveStateInstance() {")
	w.Line(2, "return nil")
	w.Line(1, "}")
	w.Linef(1, "slot := &%s{state: new(%s.%s)}", slot, appPkg, stateType)
	w.Linef(1, "s.%s.Store(id, slot)", instances)
	w.Line(1, "return slot")
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "func (s *Server) lookup%s(id string) (*%s, bool) {", suffix, slot)
	w.Linef(1, "return s.%s.Load(id)", instances)
	w.Line(0, "}")

	w.Raw("\n")
	w.Linef(0, "// release%s drops state and capacity exactly once.", suffix)
	w.Line(0, "// Passing slot preserves a replacement registered")
	w.Line(0, "// under the same id while an older stream closes.")
	w.Linef(0, "func (s *Server) release%s(id string, slot *%s) {", suffix, slot)
	w.Line(1, "slot.mu.Lock()")
	w.Line(1, "if slot.dead {")
	w.Line(2, "slot.mu.Unlock()")
	w.Line(2, "return")
	w.Line(1, "}")
	w.Line(1, "slot.dead = true")
	w.Line(1, "slot.state = nil")
	w.Line(1, "slot.mu.Unlock()")
	w.Linef(1, "s.%s.CompareAndDelete(id, slot)", instances)
	w.Line(1, "s.ReleaseStateInstance()")
	w.Line(0, "}")
}
