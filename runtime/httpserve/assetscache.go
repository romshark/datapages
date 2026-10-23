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

// assetsCache adds the headers configured by [datapages.AssetsCacheConfig] before next
// serves the file. [http.ServeContent] handles If-None-Match when an ETag is present.
//
// Missing files and directory listings receive no added cache headers.
type assetsCache struct {
	next         http.Handler
	fsys         http.FileSystem
	tags         *assetTags
	cacheControl []string // Shared by every response.
	etag         bool
}

// newAssetsCache adds the headers in conf to the files of fsys that next serves.
// It keeps their ETags in tags.
func newAssetsCache(
	fsys http.FileSystem, tags *assetTags, conf datapages.AssetsCacheConfig,
	next http.Handler,
) *assetsCache {
	cacheControl := conf.CacheControl
	if cacheControl == "" {
		// no-cache instead of max-age=0 would make the offline worker fetch
		// every asset from the network before it answers from its copy.
		cacheControl = "public, max-age=" +
			strconv.FormatInt(int64(conf.MaxAge.Seconds()), 10)
		if conf.Immutable {
			cacheControl += ", immutable"
		}
	}
	return &assetsCache{
		next:         next,
		fsys:         fsys,
		tags:         tags,
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
	if v, ok := c.tags.Load(rel); ok {
		return v.(*assetTag).value, true
	}

	f, opened, ok := c.openFile(rel)
	if !ok {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	t := untagged
	if c.etag {
		// A directory and its index.html resolve to 1 file.
		if v, ok := c.tags.Load(opened); ok {
			t = v.(*assetTag)
		} else if t = newAssetTag(f); t == nil {
			return nil, true
		}
	}
	c.tags.Store(rel, t)
	if opened != rel {
		// The compressor asks by the name of the file.
		c.tags.Store(opened, t)
	}
	return t.value, true
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

// assetTags holds the *assetTag of each file by [exactpath.Clean] path,
// which every request reads without a lock.
//
// Missing paths leave no entry, and [exactSpellings] passes only paths spelled as listed,
// which bounds the map by the files in fsys instead of the paths clients request.
type assetTags struct{ sync.Map }

// sum returns the SHA-256 of the file at name that its ETag comes from.
// The compressor takes it instead of reading the file again.
func (t *assetTags) sum(name string) ([sha256.Size]byte, bool) {
	if v, ok := t.Load(name); ok {
		if tag := v.(*assetTag); tag.value != nil {
			return tag.sum, true
		}
	}
	return [sha256.Size]byte{}, false
}

// assetTag is the ETag value of a file and the SHA-256 it comes from.
type assetTag struct {
	value []string // Shared by every response. Nil without ETags.
	sum   [sha256.Size]byte
}

// untagged is the entry of every file without an ETag.
var untagged = new(assetTag)

// newAssetTag hashes f for its ETag, or returns nil when f cannot be read.
func newAssetTag(f io.Reader) *assetTag {
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil
	}
	t := &assetTag{sum: [sha256.Size]byte(h.Sum(nil))}
	// Truncate the digest to 128 bits to keep the header short.
	// Raw URL encoding has no character that needs escaping inside a quoted ETag.
	t.value = []string{`"` + base64.RawURLEncoding.EncodeToString(t.sum[:16]) + `"`}
	return t
}
