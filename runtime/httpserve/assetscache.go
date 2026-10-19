package httpserve

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"path"
	"strconv"
	"sync"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/exactpath"
)

// defaultCacheControl is the Cache-Control of assets without an
// [datapages.AssetsCacheConfig], the one of its zero value. Without any, a
// browser reuses a file that has Last-Modified without asking, by heuristic
// freshness. no-cache would make the offline worker fetch every asset from
// the network before it answers from its copy.
const defaultCacheControl = "public, max-age=0"

// revalidated sends the responses of next with [defaultCacheControl].
func revalidated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", defaultCacheControl)
		next.ServeHTTP(w, r)
	})
}

// assetsCache adds the headers configured by [datapages.AssetsCacheConfig]
// before next serves the file. [http.ServeContent] handles If-None-Match when
// an ETag is present.
//
// Missing files and directory listings receive no added cache headers.
type assetsCache struct {
	next         http.Handler
	fsys         http.FileSystem
	cacheControl []string // Shared by every response.
	etag         bool

	// files holds the ETag value of each file by [exactpath.Clean] path, nil
	// without ETags, which every request reads without a lock.
	//
	// Missing paths leave no entry, and [exactSpellings] passes only paths
	// spelled as listed, which bounds the map by the files in fsys instead of
	// the paths clients request.
	files sync.Map
}

// newAssetsCache adds the headers in conf to files next serves from fsys.
func newAssetsCache(
	fsys http.FileSystem, conf datapages.AssetsCacheConfig, next http.Handler,
) *assetsCache {
	cacheControl := conf.CacheControl
	if cacheControl == "" {
		cacheControl = "public, max-age=" +
			strconv.FormatInt(int64(conf.MaxAge.Seconds()), 10)
		if conf.Immutable {
			cacheControl += ", immutable"
		}
	}
	return &assetsCache{
		next:         next,
		fsys:         fsys,
		cacheControl: []string{cacheControl},
		etag:         !conf.DisableETag,
	}
}

func (c *assetsCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if tag, ok := c.lookup(r.URL.Path); ok {
		h := w.Header()
		h["Cache-Control"] = c.cacheControl
		if tag != nil {
			h["Etag"] = tag
		}
	}
	c.next.ServeHTTP(w, r)
}

// lookup reports whether p resolves to a file and returns its ETag value.
// The value is nil when ETags are disabled or the file cannot be read.
func (c *assetsCache) lookup(p string) (tag []string, isFile bool) {
	rel := exactpath.Clean(p)
	if v, ok := c.files.Load(rel); ok {
		return v.([]string), true
	}

	f, opened, ok := c.openFile(rel)
	if !ok {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	if c.etag {
		sum, ok := c.digest(opened)
		if !ok {
			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				return nil, true
			}
			sum = [sha256.Size]byte(h.Sum(nil))
		}
		// Truncate the digest to 128 bits to keep the header short.
		// Raw URL encoding has no character that needs escaping inside a quoted ETag.
		tag = []string{`"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`}
	}
	c.files.Store(rel, tag)
	return tag, true
}

// digester is a handler that hashes the files it serves, as the handler of
// [assetcompress.Compressor.Handler] hashes the files it compresses.
type digester interface {
	// Digest returns the SHA-256 of the file at rel, an [exactpath.Clean] path,
	// or false when the handler does not hash the file.
	Digest(rel string) ([sha256.Size]byte, bool)
}

// digest returns the SHA-256 that next takes of the file at rel, if any.
func (c *assetsCache) digest(rel string) (sum [sha256.Size]byte, ok bool) {
	if d, isDigester := c.next.(digester); isDigester {
		return d.Digest(rel)
	}
	return sum, false
}

// openFile opens the regular file [http.FileServer] serves for rel, an
// [exactpath.Clean] path, and returns its path. For a directory, it opens index.html.
func (c *assetsCache) openFile(rel string) (http.File, string, bool) {
	f, err := c.fsys.Open("/" + rel)
	if err != nil {
		return nil, "", false
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, "", false
	}
	if !stat.IsDir() {
		if !stat.Mode().IsRegular() {
			_ = f.Close()
			return nil, "", false
		}
		return f, rel, true
	}
	_ = f.Close()

	rel = path.Join(rel, "index.html")
	f, err = c.fsys.Open("/" + rel)
	if err != nil {
		return nil, "", false
	}
	stat, err = f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		_ = f.Close()
		return nil, "", false
	}
	return f, rel, true
}
