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
)

// assetsCache adds the headers configured by [datapages.AssetsCacheConfig]
// before next serves the file. [http.ServeContent] handles If-None-Match when
// an ETag is present.
//
// Missing files and directory listings receive no added cache headers.
type assetsCache struct {
	next         http.Handler
	fsys         http.FileSystem
	cacheControl string

	// etags holds file digests by cleaned path. It is nil when ETags are disabled.
	//
	// Missing paths leave no entry, which bounds the map by the files in fsys
	// instead of the paths clients request.
	lockETags sync.RWMutex
	etags     map[string]string
}

// newAssetsCache adds the headers in conf to files next serves from fsys.
func newAssetsCache(
	fsys http.FileSystem, conf datapages.AssetsCacheConfig, next http.Handler,
) *assetsCache {
	c := &assetsCache{
		next:         next,
		fsys:         fsys,
		cacheControl: conf.CacheControl,
	}
	if conf.CacheControl == "" {
		c.cacheControl = "public, max-age=" +
			strconv.FormatInt(int64(conf.MaxAge.Seconds()), 10)
		if conf.Immutable {
			c.cacheControl += ", immutable"
		}
	}
	if !conf.DisableETag {
		c.etags = make(map[string]string)
	}
	return c
}

func (c *assetsCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if tag, ok := c.lookup(r.URL.Path); ok {
		w.Header().Set("Cache-Control", c.cacheControl)
		if tag != "" {
			w.Header().Set("ETag", tag)
		}
	}
	c.next.ServeHTTP(w, r)
}

// lookup reports whether name resolves to a file and returns its ETag.
// The tag is empty when ETags are disabled or the file cannot be read.
func (c *assetsCache) lookup(name string) (tag string, isFile bool) {
	name = path.Clean("/" + name)
	if c.etags != nil {
		c.lockETags.RLock()
		tag, isFile = c.etags[name]
		c.lockETags.RUnlock()
		if isFile {
			return tag, true
		}
	}

	f, ok := c.openFile(name)
	if !ok {
		return "", false
	}
	defer func() { _ = f.Close() }()
	if c.etags == nil {
		return "", true
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", true
	}
	// Truncate the digest to 128 bits to keep the header short. Raw URL encoding
	// contains no character that needs escaping inside a quoted ETag.
	tag = `"` + base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:16]) + `"`

	c.lockETags.Lock()
	c.etags[name] = tag
	c.lockETags.Unlock()
	return tag, true
}

// openFile opens the regular file [http.FileServer] serves for name.
// For a directory, it opens index.html.
func (c *assetsCache) openFile(name string) (http.File, bool) {
	f, err := c.fsys.Open(name)
	if err != nil {
		return nil, false
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, false
	}
	if !stat.IsDir() {
		if !stat.Mode().IsRegular() {
			_ = f.Close()
			return nil, false
		}
		return f, true
	}
	_ = f.Close()

	f, err = c.fsys.Open(path.Join(name, "index.html"))
	if err != nil {
		return nil, false
	}
	stat, err = f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		_ = f.Close()
		return nil, false
	}
	return f, true
}
