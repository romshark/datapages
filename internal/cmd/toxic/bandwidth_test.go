package toxic

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetch reads the whole response to a request with the Cookie header cookie.
// It takes no [testing.T], which lets a goroutine call it.
func fetch(
	method, target, cookie string, body io.Reader,
) (*http.Response, string, error) {
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return nil, "", err
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp, string(b), err
}

// TestLinksReserve tests that data on a link queues behind earlier data,
// that other links run in parallel, and that an idle link starts at now.
func TestLinksReserve(t *testing.T) {
	l := newLinks()
	t0 := time.Unix(0, 0)
	ms := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Millisecond) }
	a := linkKey{profile: "a"}

	// 1 KB/s carries 1 byte per ms.
	require.Equal(t, ms(100), l.reserve(a, 100, 1, t0))
	require.Equal(t, ms(200), l.reserve(a, 100, 1, t0))
	for name, k := range map[string]linkKey{
		"profile": {profile: "b"},
		"scope":   {profile: "a", scope: "GET /{$}"},
		"stream":  {profile: "a", stream: Upstream},
	} {
		require.Equal(t, ms(100), l.reserve(k, 100, 1, t0), name)
	}
	require.Equal(t, ms(600), l.reserve(a, 100, 1, ms(500)))
	require.Equal(t, ms(610), l.reserve(a, 1000, 100, ms(500)))
}

// TestLinksSweep tests that idle links are dropped once there are more than minSweep,
// and that a busy link stays.
func TestLinksSweep(t *testing.T) {
	l := newLinks()
	t0 := time.Unix(0, 0)
	busy := linkKey{profile: "busy"}
	l.reserve(busy, 10_000, 1, t0)
	for i := range minSweep - 1 {
		l.reserve(linkKey{profile: strconv.Itoa(i)}, 1, 1, t0)
	}
	require.Len(t, l.free, minSweep)

	last := linkKey{profile: "last"}
	l.reserve(last, 1, 1, t0.Add(time.Second))
	require.Len(t, l.free, 2)
	require.Contains(t, l.free, busy)
	require.Contains(t, l.free, last)
}

// TestRemoveCookie tests that only the profile cookie leaves the Cookie header.
func TestRemoveCookie(t *testing.T) {
	for name, tc := range map[string]struct{ in, want []string }{
		"none": {},
		"only": {
			in: []string{"datapages_toxiproxy=x"},
		},
		"first": {
			in:   []string{"datapages_toxiproxy=x; a=1"},
			want: []string{"a=1"},
		},
		"middle": {
			in:   []string{"a=1; datapages_toxiproxy=x; b=2"},
			want: []string{"a=1; b=2"},
		},
		"absent": {
			in:   []string{"a=1; b=2"},
			want: []string{"a=1; b=2"},
		},
		"prefix": {
			in:   []string{"datapages_toxiproxy_2=x"},
			want: []string{"datapages_toxiproxy_2=x"},
		},
		"2 lines": {
			in:   []string{"a=1", "datapages_toxiproxy=x"},
			want: []string{"a=1"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			if tc.in != nil {
				h["Cookie"] = tc.in
			}
			removeCookie(h, profileCookie)
			require.Equal(t, tc.want, h["Cookie"])
		})
	}
}

// TestFrontToxicsScope tests that a toxic of the front has the scope of the
// endpoint that overrides its slot, else the global scope.
func TestFrontToxicsScope(t *testing.T) {
	toxics := *newFrontToxics(Settings{
		Global: Set{rateDown: toxicWith(64)},
		Endpoints: map[string]Set{
			"GET /own/{$}":   {rateDown: toxicWith(8)},
			"GET /other/{$}": {latencyDown: toxicWith(100)},
		},
	})
	global := frontToxic{active: active{slot: rateDown, toxic: toxicWith(64)}}
	for pattern, want := range map[string]frontToxic{
		"GET /own/{$}": {
			active: active{slot: rateDown, toxic: toxicWith(8)},
			scope:  "GET /own/{$}",
		},
		"GET /other/{$}": global,
		"GET /none/{$}":  global,
	} {
		got, ok := toxics.toxic(pattern, rateDown)
		require.True(t, ok, pattern)
		require.Equal(t, want, got, pattern)
	}
}

// TestFrontProfileCookie tests that the front starts a profile for a browser
// without the cookie, and that the app does not get the cookie.
func TestFrontProfileCookie(t *testing.T) {
	_, base := startFrontWith(t, Config{Endpoints: endpoints("GET /")},
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Cookie", r.Header.Get("Cookie"))
		})

	resp, _, err := fetch(http.MethodGet, base+"/", "", nil)
	require.NoError(t, err)
	cookies := resp.Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, profileCookie, cookies[0].Name)
	require.NotEmpty(t, cookies[0].Value)
	require.True(t, cookies[0].HttpOnly)

	resp, _, err = fetch(http.MethodGet, base+"/",
		"a=1; "+profileCookie+"="+cookies[0].Value, nil)
	require.NoError(t, err)
	require.Empty(t, resp.Cookies(), "the profile exists")
	require.Equal(t, "a=1", resp.Header.Get("X-Cookie"))
}

// TestFrontBandwidthShared tests that the requests of 1 browser profile share
// the rate of a bandwidth toxic, and that those of 2 profiles do not.
// At 8 KB/s, 1 response of bodySize bytes takes 512ms, 2 of 1 profile 1024ms.
func TestFrontBandwidthShared(t *testing.T) {
	_, base := startFront(t, Config{
		Endpoints: endpoints("GET /"),
		Settings:  Settings{Global: Set{rateDown: toxicWith(8)}},
	})
	// both fetches 1 response for each of p1 and p2 at once and returns the time it took.
	both := func(p1, p2 string) time.Duration {
		start := time.Now()
		var wg sync.WaitGroup
		for _, p := range []string{p1, p2} {
			wg.Go(func() {
				_, body, err := fetch(http.MethodGet, base+"/", profileCookie+"="+p, nil)
				assert.NoError(t, err)
				assert.Len(t, body, bodySize)
			})
		}
		wg.Wait()
		return time.Since(start)
	}
	require.GreaterOrEqual(t, both("a", "a"), 1024*time.Millisecond)
	require.Less(t, both("b", "c"), 1024*time.Millisecond)
}

// TestFrontBandwidthUpstream tests that an upstream bandwidth toxic paces the
// request body the app reads. At 8 KB/s, bodySize bytes take 512ms.
func TestFrontBandwidthUpstream(t *testing.T) {
	var got atomic.Int64
	_, base := startFrontWith(t, Config{
		Endpoints: endpoints("POST /"),
		Settings:  Settings{Global: Set{rateUp: toxicWith(8)}},
	}, func(_ http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		got.Store(n)
	})

	start := time.Now()
	resp, _, err := fetch(http.MethodPost, base+"/", "",
		strings.NewReader(strings.Repeat("x", bodySize)))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int64(bodySize), got.Load())
	require.GreaterOrEqual(t, time.Since(start), 512*time.Millisecond)
}

// TestFrontBandwidthChange tests that a change of the toxics applies to a
// response in progress. At 1 KB/s, the response would take 4s.
func TestFrontBandwidthChange(t *testing.T) {
	f, base := startFront(t, Config{
		Endpoints: endpoints("GET /"),
		Settings:  Settings{Global: Set{rateDown: toxicWith(1)}},
	})
	updated := make(chan error, 1)
	time.AfterFunc(200*time.Millisecond, func() {
		updated <- f.Update(func(s *Settings) error {
			delete(s.Global, rateDown)
			return nil
		})
	})

	start := time.Now()
	_, body, err := fetch(http.MethodGet, base+"/", "", nil)
	require.NoError(t, err)
	require.Len(t, body, bodySize)
	require.Less(t, time.Since(start), 2*time.Second)
	require.NoError(t, <-updated)
}
