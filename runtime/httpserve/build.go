package httpserve

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"sync"

	"github.com/romshark/datapages"
)

// buildMetaName names the meta element that carries the build ID of a document.
//
// The build script reads this element for each request. A head morph may keep
// an old script node without running the new script, but it updates the
// element's attributes with the ID for the current markup.
const buildMetaName = "datapages-build"

// buildScript sends the build ID of the document with every same-origin
// Datastar request. It reloads the page when a server of another build answers
// 205 Reset Content.
//
// It runs before the deferred Datastar module and installs the wrapper once.
// A head morph may run the script again.
//
// A stale stream GET waits for a random delay of up to 2s before reloading.
// This spreads page loads from tabs that reconnect together. A stale action
// reloads at once.
//
// Reloads without a successful request wait 1s, 2s, 4s, and up to 30s.
// sessionStorage keeps the count across reloads. This delay limits reloads
// when a rolling deployment sends one tab to different builds.
//
// A beforeunload handler can cancel a reload. The next stale response more than
// 10s after the attempt schedules another one. A shorter wait would let the
// next stream retry restart a reload that is still loading.
//
// history.replaceState changes a form POST history entry to GET so that
// location.reload does not submit the form again.
//
// The wrapper builds one Request from the fetch arguments and copies it with
// the build header. The copy sets body to undefined, which makes it reuse the
// body of the first Request. Passing init.body again encodes the FormData of a
// multipart form with a new boundary. The copied Content-Type header still
// names the first boundary, and the server cannot parse the body.
//
// The wrapper is async: like fetch, it rejects for
// invalid arguments instead of throwing.
const buildScript = `(() => {
		const g=globalThis,s=Symbol.for("datapages.build")
		if (g[s]) return
		g[s]=true
		const k="datapages-build-reloads",o=g.fetch.bind(g)
		let due=Infinity,timer,tries
		const reload=later => {
			if (tries===undefined) {
				try {
					tries=+sessionStorage.getItem(k)||0
					sessionStorage.setItem(k,tries+1)
				} catch { tries=1 }
			}
			const at=Date.now()+(tries && Math.min(1000*2**(tries-1),30000))+
				(later ? Math.random()*2000:0)
			if (at>=due) return
			due=at
			clearTimeout(timer)
			timer=setTimeout(() => {
				setTimeout(() => { due=Infinity },10000)
				try { history.replaceState(history.state,"",location.href) } catch {}
				location.reload()
			},at-Date.now())
		}
		g.fetch=async (i,init) => {
			const r=new Request(i,init)
			const b=document.querySelector('meta[name="` + buildMetaName + `"]')?.content
			if (!b || r.headers.get("Datastar-Request")!=="true" ||
				new URL(r.url,location.href).origin!==location.origin
			) return o(r)
			const h=new Headers(r.headers)
			h.set("` + datapages.HeaderBuild + `",b)
			const resp=await o(new Request(r,{...init,body:undefined,headers:h}))
			if (resp.status===205 && resp.headers.has("` + datapages.HeaderBuild + `")) {
				reload(r.method==="GET")
			} else if (resp.ok) {
				try { sessionStorage.removeItem(k) } catch {}
			}
			return resp
		}
	})()`

// BuildID returns the server's build ID. It is empty if the server could not
// create an ID. Each document sends this ID, and the server rejects Datastar
// requests with a different ID.
func (c *Core) BuildID() string { return c.buildID }

// buildHead returns the build meta element followed by the build script
// opened with scriptOpen. It returns nothing for a server without a build ID.
func (c *Core) buildHead(scriptOpen string) string {
	if c.buildID == "" {
		return ""
	}
	return c.htmlBuildMeta + scriptOpen + buildScript + "</script>"
}

// rejectStaleBuild wraps next. It answers a Datastar request from another build
// with 205 Reset Content and [datapages.HeaderBuild]. The build script then reloads
// the page. next doesn't run because its IDs, routes, or signals may differ.
//
// A request without the header passes. It comes from another client or from an
// older page without the build script. Only Datastar requests are checked.
// The service worker copies the header into its page fetch. That fetch must pass.
func (c *Core) rejectStaleBuild(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(datapages.HeaderBuild)
		if id == "" || id == c.buildID || !IsDatastarRequest(r.Header) {
			next.ServeHTTP(w, r)
			return
		}
		c.logger.Debug("request from a page of another build",
			slog.String("path", r.URL.Path))
		h := w.Header()
		h.Set(datapages.HeaderBuild, c.buildID)
		h.Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusResetContent)
	})
}

// defaultBuildID returns a hash of the executable file.
//
// If the file cannot be read, it hashes the embedded Go build information.
// This changes with module versions, the VCS revision, and build settings.
// If neither source is available, the server has no build ID.
func defaultBuildID(logger *slog.Logger) string {
	id, err := executableBuildID()
	if err == nil {
		return id
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		logger.Warn("build ID taken from the build info: "+
			"the executable is unreadable", slog.Any("err", err))
		sum := sha256.Sum256([]byte(bi.String()))
		return hex.EncodeToString(sum[:16])
	}
	logger.Warn("no build ID: the executable is unreadable and has no build info. "+
		"Open pages will not reload after a deployment", slog.Any("err", err))
	return ""
}

// executableBuildID hashes the executable once per process.
// Every server in the process runs the same file.
var executableBuildID = sync.OnceValues(func() (string, error) {
	f, err := openExecutable()
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)[:16]), nil
})

// openExecutable opens the running file. On Linux, /proc/self/exe still names
// it after a deployment replaces its path. os.Executable then names the new
// binary instead.
func openExecutable() (*os.File, error) {
	if f, err := os.Open("/proc/self/exe"); err == nil {
		return f, nil
	}
	p, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}
