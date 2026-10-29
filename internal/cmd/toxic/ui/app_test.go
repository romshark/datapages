package ui_test

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/cmd/toxic"
	"github.com/romshark/datapages/internal/cmd/toxic/ui"
	"github.com/romshark/datapages/internal/cmd/toxic/ui/datapagesgen"
	"github.com/romshark/datapages/internal/cmd/toxic/ui/datapagesgen/assets"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// limited is the pattern of an endpoint, limitedKey its endpoint query parameter.
const (
	limited    = "GET /limited/{$}"
	limitedKey = "PageLimited.GET"
)

var (
	latencyDown = toxic.Slot{Type: toxic.Latency, Stream: toxic.Downstream}
	limitDown   = toxic.Slot{Type: toxic.LimitData, Stream: toxic.Downstream}
	rateDown    = toxic.Slot{Type: toxic.ClientBandwidth, Stream: toxic.Downstream}
)

// rootPath and limitedPath match the 2 entries of the endpoint list,
// limitedHeading the scope heading of the second.
const (
	rootPath    = `<code class="tp-path">/</code>`
	limitedPath = `<code class="tp-path">/limited/` +
		`<span class="tp-route-end">{$}</span></code>`
	limitedHeading = `<code><span class="tp-route-method">GET</span> ` +
		`/limited/<span class="tp-route-end">{$}</span></code>`
)

// settings is the page at url of a front with the endpoints "GET /" and [limited],
// for the module in dir.
type settings struct {
	front  *toxic.Front
	broker messaging.Broker
	url    string
	dir    string
}

// configFile is the datapages.yaml of startSettings, which has the preset
// Slow. The front starts without it, which leaves its presets unsaved.
const configFile = "cmd: cmd/server\nwatch:\n  toxiproxy:\n    presets:\n" +
	"      - name: Slow\n        toxics:\n          latency:\n" +
	"            downstream:\n              latency: 100ms\n"

func startSettings(t *testing.T) *settings {
	t.Helper()
	upstream := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(upstream.Close)
	u, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	dir := t.TempDir()
	file := filepath.Join(dir, "datapages.yaml")
	require.NoError(t, os.WriteFile(file, []byte(configFile), 0o644))

	front, err := toxic.New(toxic.Config{
		Host:       "localhost:7332",
		UIHost:     "localhost:7333",
		Upstream:   u,
		AppName:    "shop",
		Version:    "v1.2.3",
		ConfigPath: file,
		Settings:   toxic.SettingsFromConfig(nil),
		Endpoints: []toxic.Endpoint{
			{Pattern: "GET /", Owner: "PageIndex", Handler: "GET"},
			{Pattern: limited, Owner: "PageLimited", Handler: "GET"},
		},
		ReloadSettings: func() (toxic.Settings, error) {
			c, _, err := config.Load(dir)
			if err != nil {
				return toxic.Settings{}, err
			}
			return toxic.SettingsFromConfig(c.Watch.Toxiproxy), nil
		},
		WriteConfig: func(section string) error {
			return config.SetToxiproxy(dir, section)
		},
	})
	require.NoError(t, err)
	t.Cleanup(front.Close)

	static, err := fs.Sub(ui.StaticFS, "static")
	require.NoError(t, err)
	broker := inmem.New(messaging.DefaultBrokerChanBuffer)
	srv, err := datapages.NewServer[
		ui.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		&ui.App{Front: front},
		broker,
		datapages.WithAssetsFS(http.FS(static), false),
		datapages.WithDatastarJS(assets.Path("datastar-1.0.4.js")),
		datapages.WithLogger(slog.New(slog.DiscardHandler)),
	)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &settings{front: front, broker: broker, url: ts.URL, dir: dir}
}

// do sends a request and returns the response status and body.
func do(
	t *testing.T, method, target, body string, header http.Header,
) (int, string) {
	t.Helper()
	code, b, _ := doHeader(t, method, target, body, header)
	return code, b
}

// doHeader is [do] that also returns the response header.
func doHeader(
	t *testing.T, method, target, body string, header http.Header,
) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(method, target, strings.NewReader(body))
	require.NoError(t, err)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, err)
	return resp.StatusCode, string(b), resp.Header
}

// tab is 1 browser tab with the instance of its page load and the output of its stream.
type tab struct {
	base, instance string

	mu     sync.Mutex
	stream strings.Builder
}

// openTab loads the page without query parameters and connects its stream,
// which renders the tab once.
func openTab(t *testing.T, base string) *tab {
	t.Helper()
	code, _, header := doHeader(t, http.MethodGet, base+"/", "", nil)
	require.Equal(t, http.StatusOK, code)
	tb := &tab{base: base, instance: header.Get("Datapages-Instance")}
	require.NotEmpty(t, tb.instance)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	signals := url.QueryEscape(`{"endpoint":"","search":"","kinds":""}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/_$/?datastar="+signals, nil)
	require.NoError(t, err)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tb.instance)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	t.Cleanup(func() { _ = resp.Body.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			tb.mu.Lock()
			tb.stream.Write(buf[:n])
			tb.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	tb.waitFrom(t, 0, `id="tp-main"`)
	return tb
}

// command sends a Datastar request of the tab and returns its status.
func (tb *tab) command(t *testing.T, target, signals string) int {
	t.Helper()
	code, _ := do(t, http.MethodPost, tb.base+target, signals, http.Header{
		"Datastar-Request":   {"true"},
		"Datapages-Instance": {tb.instance},
	})
	return code
}

// at returns the length of the stream output so far.
func (tb *tab) at() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.stream.Len()
}

// waitFrom waits for want in the stream output after from and returns that
// output through the end of the event with want.
func (tb *tab) waitFrom(t *testing.T, from int, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		tb.mu.Lock()
		got := tb.stream.String()[from:]
		tb.mu.Unlock()
		if i := strings.Index(got, want); i >= 0 {
			if end := strings.Index(got[i:], "\n\n"); end >= 0 {
				return got[:i+end]
			}
		}
		if time.Now().After(deadline) {
			require.Contains(t, got, want, "the stream never delivered it")
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

var (
	// pageTitle matches the h1 of the header.
	pageTitle = regexp.MustCompile(`(?s)<h1>.*?</h1>`)
	// aboutDialog matches the dialog the title opens.
	aboutDialog = regexp.MustCompile(`(?s)<neo-dialog[^>]* id="tp-about".*?</neo-dialog>`)
)

// TestPage tests loading the global scope with the about dialog of the title,
// an endpoint scope and a filtered endpoint list, and the static files the
// head loads.
func TestPage(t *testing.T) {
	s := startSettings(t)
	base := s.url

	code, body := do(t, http.MethodGet, base+"/", "", nil)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "<title>shop - Toxiproxy</title>")
	// The title is a button named by the logo and the app, which opens the about dialog.
	title := pageTitle.FindString(body)
	require.Contains(t, title, `aria-haspopup="dialog"`)
	require.Contains(t, title, `aria-controls="tp-about"`)
	require.Regexp(t, `<neo-icon [^>]*aria-label="Datapages"`, title)
	require.Regexp(t, `</neo-icon> shop</neo-button></h1>$`, title)
	about := aboutDialog.FindString(body)
	require.Contains(t, about, `>shop - Toxiproxy</h2>`)
	require.Regexp(t,
		`<dt>Toxic</dt><dd><neo-layout[^>]*><a [^>]*href="http://localhost:7332"[^>]*>`+
			`<code>localhost:7332</code>`, about)
	require.Regexp(t,
		`<dt>Non-toxic</dt><dd><neo-layout[^>]*><a [^>]*href="http://127\.0\.0\.1:\d+"`,
		about)
	require.Contains(t, about, `aria-label="About the toxic address"`)
	require.Contains(t, about, `aria-label="About the non-toxic address"`)
	require.Regexp(t,
		`<dt>Config</dt><dd><neo-layout[^>]*><code>datapages.yaml</code>`,
		about)
	// The copy button copies the absolute path of the file.
	copyConfig := regexp.MustCompile(
		`(?s)<neo-clipcopy [^>]*>\s*<neo-button [^>]*aria-label="Copy path to datapages.yaml"`,
	).FindString(about)
	require.Contains(t, copyConfig,
		`value="`+filepath.Join(s.dir, "datapages.yaml")+`"`)
	require.Contains(t, about, `<dt>Datapages</dt><dd>v1.2.3</dd>`)
	require.Regexp(t, `<dt>Toxiproxy</dt><dd><neo-layout[^>]*>v2\.\d+\.\d+<a `, about)
	repo := regexp.MustCompile(
		`<a [^>]*aria-label="Toxiproxy on GitHub"[^>]*>`,
	).FindString(about)
	require.Contains(t, repo, `href="https://github.com/Shopify/toxiproxy"`)
	require.Contains(t, about, `<dialog id="tp-about-dialog">`)
	// The theme select applies its mode through the script of the head and
	// takes the saved mode on init.
	require.Contains(t, body, "tpTheme.apply(tpTheme.mode())")
	theme := regexp.MustCompile(
		`(?s)<neo-select [^>]*id="tp-theme".*?</neo-select>`,
	).FindString(about)
	require.Contains(t,
		theme,
		`data-on:neo-select-change="tpTheme.set(evt.detail.value)"`)
	for _, mode := range []string{"system", "light", "dark"} {
		require.Contains(t, theme, `<neo-option role="option" tabindex="-1" `+
			`aria-selected="false" value="`+mode+`"`)
	}
	require.Regexp(t, `<a [^>]*href="http://localhost:7332"[^>]*>`+
		`toxic <code>localhost:7332</code>`, body)
	require.Regexp(t, `<a [^>]*href="http://127\.0\.0\.1:\d+"[^>]*>`+
		`non-toxic <code>127\.0\.0\.1:\d+</code>`, body)
	require.Contains(t, body, "<h2>Global toxics</h2>")
	require.Contains(t, body, rootPath)
	require.Contains(t, body, limitedPath)
	require.Contains(t, body, `src="/static/datastar-1.0.4.js"`)
	// The kind icon of "GET /", inlined from static/icons.
	icon := regexp.MustCompile(
		`<neo-icon [^>]*aria-label="Page"[^>]*><svg`,
	).FindString(body)
	require.Contains(t, icon, `role="img"`)
	require.Contains(t, body, "Response (downstream)")
	require.Contains(t, body, "Request (upstream)")
	// The scope description is only in the popover of its info button.
	require.Contains(t, body, `aria-label="About these toxics"`)
	require.Equal(t,
		1,
		strings.Count(body, "Each request opens a connection of its own."))
	// The header and the scope have a menu button. ui.css shows the one of the
	// scope while the scope sticks.
	require.Equal(t, 2, strings.Count(body, `aria-controls="tp-sidebar"`))
	// The sidebar sits in a neo-resizable with a handle on its right edge,
	// which has no grip. ui.css draws it as a divider.
	require.Regexp(t,
		`<neo-resizable [^>]*class="tp-sidebar-size"[^>]*>\s*<neo-sidebar `,
		body)
	require.Contains(t, body, `<span data-neo-resizable-handle="right"></span>`)

	scoped := base + "/?endpoint=" + limitedKey + "&q=lim"
	code, body = do(t, http.MethodGet, scoped, "", nil)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, limitedHeading)
	require.Contains(t, body, `href="/?endpoint=PageLimited.GET&amp;q=lim"`)
	require.Contains(t, body, `data-signals:search="'lim'"`)
	// The kind icon leads the heading.
	require.Regexp(t, `<h2[^>]*><neo-layout[^>]*><neo-icon [^>]*aria-label="Page"`, body)
	require.Equal(t,
		1,
		strings.Count(body, "<p>Served by <code>PageLimited.GET</code>.</p>"))
	require.Contains(t, body, limitedPath)
	require.NotContains(t, body, rootPath)

	code, body = do(t, http.MethodGet, base+"/?kinds=none", "", nil)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "No endpoints match.")
	require.NotContains(t, body, limitedPath)

	for _, file := range []string{
		"ui.css", "favicon.svg", "datastar-1.0.4.js", "morpheus/morpheus.min.js",
	} {
		code, _ := do(t, http.MethodGet, base+"/static/"+file, "", nil)
		require.Equal(t, http.StatusOK, code, file)
	}
}

// TestCommands tests the commands and the renders they cause.
// Show and filter render 1 tab, a settings change every tab in its own view.
// A rejected value shows only in the tab that sent it. It also covers bad requests,
// a cross-site request, clearing overrides and reloading the file.
func TestCommands(t *testing.T) {
	s := startSettings(t)
	tb, other := openTab(t, s.url), openTab(t, s.url)
	limitBytes := func() int64 {
		return s.front.Settings().Endpoints[limited][limitDown].Values[0]
	}

	at := tb.at()
	code := tb.command(t, "/show/?endpoint="+limitedKey, "{}")
	require.Equal(t, http.StatusOK, code)
	got := tb.waitFrom(t, at, limitedHeading)
	require.Contains(t, got,
		`data: signals {"endpoint":"PageLimited.GET","search":"","kinds":""}`)
	// A render morphs the sidebar, not its neo-resizable, which keeps the
	// width a drag sets.
	require.Contains(t, got, `id="tp-sidebar"`)
	require.NotContains(t, got, "<neo-resizable")

	// Every kind is no filter, which the render writes back as empty.
	at = tb.at()
	code = tb.command(t, "/filter/", `{"search":"lim","kinds":"get-stream-action-file"}`)
	require.Equal(t, http.StatusOK, code)
	got = tb.waitFrom(t, at, limitedPath)
	require.Contains(t, got, `"search":"lim","kinds":""}`)
	require.NotContains(t, got, rootPath)

	at, otherAt := tb.at(), other.at()
	code = tb.command(t, "/mode/?slot=limit_data_downstream", `{"mode":"on"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, `aria-label="Bytes"`)
	got = other.waitFrom(t, otherAt, `title="1 overrides"`)
	require.Contains(t, got, "<h2>Global toxics</h2>")
	require.Equal(t, int64(1024), limitBytes())

	at = tb.at()
	code = tb.command(t, "/value/?slot=limit_data_downstream&field=bytes",
		`{"value":"10"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, `value="10"`)
	require.Equal(t, int64(10), limitBytes())

	at, otherAt = tb.at(), other.at()
	code = tb.command(t, "/value/?slot=client_bandwidth_downstream&field=rate", `{"value":"0"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, "rate must be at least 1")
	require.NotContains(t, s.front.Settings().Endpoints[limited], rateDown)

	for target, signals := range map[string]string{
		"/mode/?slot=nope":                           `{"mode":"on"}`,
		"/mode/?slot=latency_downstream":             `{"mode":"sideways"}`,
		"/value/?slot=latency_downstream&field=rate": `{"value":"1"}`,
	} {
		require.Equal(t, http.StatusBadRequest, tb.command(t, target, signals), target)
	}

	code, _ = do(t, http.MethodPost, s.url+"/clear/", "", http.Header{
		"Datastar-Request":   {"true"},
		"Datapages-Instance": {tb.instance},
		"Sec-Fetch-Site":     {"cross-site"},
		"Origin":             {"https://evil.test"},
	})
	require.Equal(t, http.StatusForbidden, code)
	require.Contains(t, s.front.Settings().Endpoints, limited)

	code = tb.command(t, "/clear/", "{}")
	require.Equal(t, http.StatusOK, code)
	require.NotContains(t, s.front.Settings().Endpoints, limited)
	// The other tab sees the endpoint lose its badge, never the refused value.
	got = other.waitFrom(t, otherAt, limitedPath+" </neo-layout>")
	require.NotContains(t, got, "rate must be at least 1")

	at = tb.at()
	code = tb.command(t, "/reload/", "{}")
	require.Equal(t, http.StatusOK, code)
	// The notice is a toast, appended to the toaster.
	got = tb.waitFrom(t, at, "Loaded settings from datapages.yaml.")
	require.Contains(t, got, "data: selector #tp-toaster\n")
	require.Contains(t, got, "data: mode append\n")
	require.Contains(t, got, `<span slot="title">Config reloaded</span>`)
	// The changed No throttling became toxics no preset holds, which a reload
	// replaces with No throttling.
	st := s.front.Settings()
	require.Equal(t, "No throttling", st.Active)
	require.Len(t, st.Presets, 1)
}

// TestPresets tests that the preset picker selects the matching preset,
// applies one, creates one from the current toxics or a copy, refuses bad names,
// deletes one, and that a save writes the presets without the built-in ones.
func TestPresets(t *testing.T) {
	s := startSettings(t)
	// The presets of the datapages.yaml that init writes.
	require.NoError(t, s.front.Update(func(st *toxic.Settings) error {
		st.Presets = toxic.DefaultPresets()
		return nil
	}))
	tb := openTab(t, s.url)
	names := func() []string {
		var n []string
		for _, p := range s.front.Settings().Presets {
			n = append(n, p.Name)
		}
		return n
	}
	create := func(signals string) string {
		t.Helper()
		code, body := do(t,
			http.MethodPost,
			s.url+"/create-preset/",
			signals,
			http.Header{
				"Datastar-Request":   {"true"},
				"Datapages-Instance": {tb.instance},
			})
		require.Equal(t, http.StatusOK, code)
		return body
	}

	code, body := do(t, http.MethodGet, s.url+"/", "", nil)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `<neo-combobox value="No throttling"`)
	require.NotContains(t, body, "563 ms")
	require.Contains(t, body, `aria-label="Delete preset Slow 4G"`)
	require.Contains(t, body, `value=" new"`)
	// The label of the picker is the text addon of its input group.
	require.Regexp(t, `<neo-input-group[^>]*><span aria-hidden="true">Preset</span>`+
		`\s*<neo-combobox [^>]*aria-label="Preset"`, body)
	// No throttling and Unreachable have icons of their own.
	// The presets of the app share 1.
	for name, icon := range map[string]string{
		"No throttling": "gauge",
		"Unreachable":   "server-off",
		"Slow 4G":       "sliders-horizontal",
		"3G":            "sliders-horizontal",
	} {
		require.Regexp(t, `<neo-option [^>]*value="`+name+`"[^>]*>`+
			`<neo-layout[^>]*><neo-layout[^>]*><neo-icon name="`+icon+`"`, body, name)
	}
	// The built-in presets have no delete button and no copy option.
	require.Contains(t, body, `value="No throttling"`)
	require.Contains(t, body, `value="Unreachable"`)
	require.NotContains(t, body, `aria-label="Delete preset No throttling"`)
	require.NotContains(t, body, `aria-label="Delete preset Unreachable"`)
	require.NotContains(t, body, `value="app:Unreachable"`)
	require.NotContains(t, body, `value="datapages:Unreachable"`)
	require.NotContains(t, body, `value="datapages:No throttling"`)
	// The copy combobox lists each preset of the app, then each of Datapages.
	require.Regexp(t, `value="app:3G"[^>]*><neo-layout[^>]*><span>3G</span> `+
		`<span class="tp-small tp-muted">shop preset</span>`, body)
	require.Regexp(t, `value="datapages:3G"[^>]*><neo-layout[^>]*><span>3G</span> `+
		`<span class="tp-small tp-muted">Datapages preset</span>`, body)

	at := tb.at()
	code = tb.command(t, "/apply-preset/", `{"preset":"Slow 4G"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, `<neo-combobox value="Slow 4G"`)
	require.Equal(t, toxic.DefaultPresets()[1].Toxics, s.front.Settings().Global)

	at = tb.at()
	code = tb.command(t, "/apply-preset/", `{"preset":"Unreachable"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, `<neo-combobox value="Unreachable"`)

	at = tb.at()
	require.Contains(t, create(`{"name":" Train ","from":""}`),
		"document.getElementById('tp-preset-new').hide()")
	tb.waitFrom(t, at, `<neo-combobox value="Train"`)
	require.Equal(t, []string{"Fast 4G", "Slow 4G", "3G", "Train"}, names())

	at = tb.at()
	create(`{"name":"Bus","from":"datapages:3G"}`)
	tb.waitFrom(t, at, `<neo-combobox value="Bus"`)
	require.Equal(t, toxic.DefaultPresets()[2].Toxics, s.front.Settings().Global)

	at = tb.at()
	create(`{"name":"Car","from":"app:Fast 4G"}`)
	tb.waitFrom(t, at, `<neo-combobox value="Car"`)
	require.Equal(t, toxic.DefaultPresets()[0].Toxics, s.front.Settings().Global)

	for signals, want := range map[string]string{
		`{"name":"3G","from":""}`:              "A preset with this name exists.",
		`{"name":"Unreachable","from":""}`:     "A preset with this name exists.",
		`{"name":"  ","from":""}`:              "Type a name for the preset.",
		`{"name":"Van","from":"app:5G"}`:       "The preset to copy no longer exists.",
		`{"name":"Van","from":"datapages:5G"}`: "The preset to copy no longer exists.",
		`{"name":"Van","from":"Fast 4G"}`:      "The preset to copy no longer exists.",
	} {
		at = tb.at()
		require.NotContains(t, create(signals), ".hide()", signals)
		got := tb.waitFrom(t, at, `id="tp-preset-new-error"`)
		require.Contains(t, got, `role="alert">`+want+`</p>`, signals)
	}
	require.Len(t, names(), 6)

	at = tb.at()
	code = tb.command(t, "/delete-preset/", `{"preset":"Train"}`)
	require.Equal(t, http.StatusOK, code)
	got := tb.waitFrom(t, at, `id="tp-main"`)
	require.NotContains(t, got, `value="Train"`)
	require.Equal(t,
		http.StatusBadRequest,
		tb.command(t, "/delete-preset/", `{"preset":"Train"}`))
	require.Equal(t,
		http.StatusBadRequest,
		tb.command(t, "/apply-preset/", `{"preset":"Train"}`))
	require.Equal(t,
		http.StatusBadRequest,
		tb.command(t, "/delete-preset/", `{"preset":"Unreachable"}`))

	at = tb.at()
	require.Equal(t, http.StatusOK, tb.command(t, "/save/", "{}"))
	tb.waitFrom(t, at, "Saved presets to datapages.yaml.")
	b, err := os.ReadFile(filepath.Join(s.dir, "datapages.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(b), "    presets:\n      - name: \"Fast 4G\"\n")
	require.NotContains(t, string(b), "Unreachable")
	require.True(t, s.front.Saved())
}

// createButton and presetNameField match the Create button and the name field
// of the new preset dialog.
var (
	createButton = regexp.MustCompile(
		`<neo-button[^>]*@post\(&#39;/create-preset/&#39;\)[^>]*>`,
	)
	presetNameField = regexp.MustCompile(
		`<neo-textinput[^>]*id="tp-preset-name"[^>]*>`,
	)
)

// TestPresetDraft tests the new preset dialog as rendered from its posted fields.
// A Datapages preset fills in an empty name, which follows the copy combobox
// until edited. A taken name shows an error and disables Create,
// and the action refuses it.
func TestPresetDraft(t *testing.T) {
	s := startSettings(t)
	require.NoError(t, s.front.Update(func(st *toxic.Settings) error {
		st.Presets = toxic.DefaultPresets()[:1] // Fast 4G
		return nil
	}))
	tb := openTab(t, s.url)
	draft := func(signals string) string {
		t.Helper()
		at := tb.at()
		require.Equal(t, http.StatusOK, tb.command(t, "/preset-draft/", signals))
		return tb.waitFrom(t, at, `id="tp-preset-new-error"`)
	}
	nameField := func(got, want string) {
		t.Helper()
		require.Contains(t, presetNameField.FindString(got), ` value="`+want+`"`)
	}

	got := draft(`{"name":"","from":"datapages:Slow 4G"}`)
	nameField(got, "Slow 4G")
	require.Contains(t, got, `<neo-combobox value="datapages:Slow 4G"`)
	require.Contains(t, got, `role="alert"></p>`)
	require.NotContains(t, createButton.FindString(got), "disabled")

	nameField(draft(`{"name":"Slow 4G","from":"datapages:3G"}`), "3G")
	nameField(draft(`{"name":"3G","from":""}`), "")

	got = draft(`{"name":"","from":"datapages:Fast 4G"}`)
	nameField(got, "Fast 4G")
	require.Contains(t, got, `role="alert">A preset with this name exists.</p>`)
	require.Contains(t, createButton.FindString(got), `aria-disabled="true"`)

	nameField(draft(`{"name":"Mine","from":"datapages:Fast 4G"}`), "Mine")
	nameField(draft(`{"name":"Mine","from":"datapages:3G"}`), "Mine")

	code, _ := do(t, http.MethodPost, s.url+"/create-preset/",
		`{"name":"Fast 4G","from":"datapages:3G"}`, http.Header{
			"Datastar-Request":   {"true"},
			"Datapages-Instance": {tb.instance},
		})
	require.Equal(t, http.StatusOK, code)
	require.Len(t, s.front.Settings().Presets, 1, "the action refuses a taken name")
}

// presetPicker matches the preset combobox of the header.
var presetPicker = regexp.MustCompile(`<neo-combobox[^>]*id="tp-preset-combobox"[^>]*>`)

// TestPresetOverrides tests that presets include endpoint overrides.
// An override matches no preset, a preset created from the current toxics
// holds it, and applying a preset replaces the overrides.
func TestPresetOverrides(t *testing.T) {
	s := startSettings(t)
	require.NoError(t, s.front.Update(func(st *toxic.Settings) error {
		st.Presets = toxic.DefaultPresets()
		return nil
	}))
	tb := openTab(t, s.url)
	// run sends a command and returns the preset combobox of its render.
	run := func(target, signals string) string {
		t.Helper()
		at := tb.at()
		require.Equal(t, http.StatusOK, tb.command(t, target, signals), target)
		return presetPicker.FindString(tb.waitFrom(t, at, `id="tp-preset-combobox"`))
	}
	override := toxic.Set{latencyDown: toxic.NewToxic(toxic.Latency)}

	require.Contains(t, run("/show/?endpoint="+limitedKey, "{}"),
		`value="No throttling"`)
	require.NotContains(t, run("/mode/?slot=latency_downstream", `{"mode":"on"}`),
		"value=", "Unsaved")
	require.Equal(t,
		map[string]toxic.Set{limited: override},
		s.front.Settings().Endpoints)

	at := tb.at()
	code, _ := do(t, http.MethodPost, s.url+"/create-preset/",
		`{"name":"Slow page","from":""}`, http.Header{
			"Datastar-Request":   {"true"},
			"Datapages-Instance": {tb.instance},
		})
	require.Equal(t, http.StatusOK, code)
	require.Contains(t,
		presetPicker.FindString(tb.waitFrom(t, at, `id="tp-preset-combobox"`)),
		`value="Slow page"`)
	p, err := s.front.Settings().Preset("Slow page")
	require.NoError(t, err)
	require.Equal(t, map[string]toxic.Set{limited: override}, p.Endpoints)

	require.Contains(t, run("/apply-preset/", `{"preset":"3G"}`), `value="3G"`)
	require.Empty(t, s.front.Settings().Endpoints)
	require.Contains(t, run("/apply-preset/", `{"preset":"Slow page"}`),
		`value="Slow page"`)
	require.Equal(t,
		map[string]toxic.Set{limited: override},
		s.front.Settings().Endpoints)
}

// TestStatusCombobox tests the status combobox of the http-error toxic,
// picking a status and refusing one out of range.
func TestStatusCombobox(t *testing.T) {
	s := startSettings(t)
	tb := openTab(t, s.url)
	status := func() int64 {
		slot := toxic.Slot{Type: toxic.HTTPError, Stream: toxic.Upstream}
		return s.front.Settings().Global[slot].Values[0]
	}

	at := tb.at()
	code := tb.command(t, "/mode/?slot=http_error_upstream", `{"mode":"on"}`)
	require.Equal(t, http.StatusOK, code)
	got := tb.waitFrom(t, at, `<neo-combobox value="500"`)
	// Any disabled attribute, "false" too, disables the form-associated combobox.
	require.NotRegexp(t, `<neo-combobox value="500"[^>]* disabled`, got)
	require.Contains(t, got, `value="503">503 Service Unavailable</neo-option>`)
	require.Contains(t, got, `$value = evt.detail.value`)
	require.NotContains(t, got, `aria-label="Status" type="number"`)

	at = tb.at()
	code = tb.command(t, "/value/?slot=http_error_upstream&field=status",
		`{"value":"503"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, `<neo-combobox value="503"`)
	require.Equal(t, int64(503), status())

	at = tb.at()
	code = tb.command(t, "/value/?slot=http_error_upstream&field=status",
		`{"value":"302"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, "status must be at least 400")
	require.Equal(t, int64(503), status())
}

// TestByteUnit tests that picking the unit of a byte field converts the number
// the tab shows and keeps the value, and that a number typed then reads in that unit.
func TestByteUnit(t *testing.T) {
	s := startSettings(t)
	tb := openTab(t, s.url)
	value := func() int64 { return s.front.Settings().Global[limitDown].Values[0] }
	input := func(v string) string {
		return `<neo-textinput aria-label="Bytes"[^>]* value="` + v + `"`
	}

	at := tb.at()
	code := tb.command(t, "/mode/?slot=limit_data_downstream", `{"mode":"on"}`)
	require.Equal(t, http.StatusOK, code)
	got := tb.waitFrom(t, at, `aria-label="Bytes unit"`)
	require.Regexp(t, input("1"), got)
	require.Regexp(t, `<neo-select value="KiB"[^>]* aria-label="Bytes unit"`, got)

	at = tb.at()
	code = tb.command(t, "/unit/?slot=limit_data_downstream&field=bytes", `{"unit":"KB"}`)
	require.Equal(t, http.StatusOK, code)
	got = tb.waitFrom(t, at, `<neo-select value="KB"`)
	require.Regexp(t, input("1.024"), got)
	require.Equal(t, int64(1024), value())

	at = tb.at()
	code = tb.command(t, "/value/?slot=limit_data_downstream&field=bytes&unit=KB",
		`{"value":"2.5"}`)
	require.Equal(t, http.StatusOK, code)
	got = tb.waitFrom(t, at, `<neo-select value="KB"`)
	require.Regexp(t, input("2.5"), got)
	require.Equal(t, int64(2500), value())

	code = tb.command(t, "/unit/?slot=limit_data_downstream&field=bytes", `{"unit":"TB"}`)
	require.Equal(t, http.StatusBadRequest, code)
	code = tb.command(t, "/unit/?slot=latency_downstream&field=latency", `{"unit":"KB"}`)
	require.Equal(t, http.StatusBadRequest, code)
	code = tb.command(t, "/value/?slot=limit_data_downstream&field=bytes&unit=TB",
		`{"value":"1"}`)
	require.Equal(t, http.StatusBadRequest, code)
}

// TestDataCap tests the initial volume, sharing selection and reset notice.
func TestDataCap(t *testing.T) {
	s := startSettings(t)
	tb := openTab(t, s.url)
	capDown := toxic.Slot{Type: toxic.DataCap, Stream: toxic.Downstream}

	at := tb.at()
	code := tb.command(t, "/mode/?slot=data_cap_downstream", `{"mode":"on"}`)
	require.Equal(t, http.StatusOK, code)
	got := tb.waitFrom(t, at, `aria-label="Volume unit"`)
	require.Regexp(t, `<neo-textinput aria-label="Volume"[^>]* value="1"`, got)
	require.Regexp(t, `<neo-select value="MiB"[^>]* aria-label="Volume unit"`, got)
	require.Regexp(t, `<neo-select value="client"[^>]* aria-label="Shared by"`, got)

	at = tb.at()
	code = tb.command(t, "/value/?slot=data_cap_downstream&field=per",
		`{"value":"request"}`)
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, `<neo-select value="request"`)
	require.Equal(t, toxic.PerRequest, s.front.Settings().Global[capDown].Values[2])

	at = tb.at()
	code = tb.command(t, "/reset-data-caps/", "{}")
	require.Equal(t, http.StatusOK, code)
	tb.waitFrom(t, at, "Restored each browser's full volume.")
}

// TestChangedFromOutside tests that an EventChanged published on the broker,
// as datapages watch does when the endpoints change, renders the tabs.
func TestChangedFromOutside(t *testing.T) {
	s := startSettings(t)
	tb := openTab(t, s.url)

	at := tb.at()
	_ = s.front.Update(func(st *toxic.Settings) error {
		st.Global[latencyDown] = toxic.NewToxic(toxic.Latency)
		return nil
	})
	require.NoError(t, s.broker.Publish(context.Background(), messaging.NoopMetrics{},
		datapagesgen.EvSubjChanged, []byte("{}")))
	got := tb.waitFrom(t, at, `aria-label="Latency"`)
	require.Contains(t, got, `aria-label="About jitter"`)
	require.Contains(t, got, `<neo-slider min="0" max="100" step="1" value="100"`)
}

// saveButton matches the opening tag of the save button.
var saveButton = regexp.MustCompile(
	`<neo-button[^>]*title="Save presets to datapages.yaml"[^>]*>`,
)

// TestSave tests that a save keeps the file outside watch.toxiproxy and
// disables the save button until the presets or the file change. Toxics no
// preset holds take a name first, and a change of a preset of the app saves
// without one.
func TestSave(t *testing.T) {
	s := startSettings(t)
	tb := openTab(t, s.url)
	file := filepath.Join(s.dir, "datapages.yaml")
	// The front has no presets, the file has 1.
	require.False(t, s.front.Saved())
	// save sends a command and returns the render with its notice.
	save := func(target, signals, notice string) string {
		t.Helper()
		at := tb.at()
		require.Equal(t, http.StatusOK, tb.command(t, target, signals))
		return tb.waitFrom(t, at, notice)
	}

	got := save("/save/", "{}", "Saved presets to datapages.yaml.")
	require.Contains(t, saveButton.FindString(got), `aria-disabled="true"`)
	b, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "cmd: cmd/server\nwatch:\n  toxiproxy: {}\n", string(b))
	require.True(t, s.front.Saved())

	// A change of No throttling leaves toxics no preset holds.
	at := tb.at()
	code := tb.command(t, "/mode/?slot=latency_downstream", `{"mode":"on"}`)
	require.Equal(t, http.StatusOK, code)
	got = tb.waitFrom(t, at, `aria-label="Latency"`)
	require.NotContains(t, presetPicker.FindString(got), "value=", "Unsaved")
	button := saveButton.FindString(got)
	require.NotContains(t, button, "disabled")
	require.Contains(t, button, "$save = true")
	require.NotContains(t, button, "/save/")
	// The toast of the save shows once.
	require.NotContains(t, got, "Config saved")
	save("/save/", "{}", "Could not save config")
	require.False(t, s.front.Saved())

	save("/create-preset/", `{"name":"Mine","from":"","save":true}`,
		"Saved presets to datapages.yaml.")
	require.True(t, s.front.Saved())
	b, err = os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(b), `- name: "Mine"`)

	// A change of Mine goes into Mine, which saves without a name.
	at = tb.at()
	code = tb.command(t,
		"/value/?slot=latency_downstream&field=latency", `{"value":"20"}`)
	require.Equal(t, http.StatusOK, code)
	got = tb.waitFrom(t, at, `value="20"`)
	require.Contains(t, presetPicker.FindString(got), `value="Mine"`)
	require.Contains(t, saveButton.FindString(got), "@post(&#39;/save/&#39;)")
	require.False(t, s.front.Saved())
	save("/save/", "{}", "Saved presets to datapages.yaml.")
	require.True(t, s.front.Saved())

	// Change the file by hand.
	require.NoError(t, os.WriteFile(file, []byte(configFile), 0o644))
	require.False(t, s.front.Saved())
	at = tb.at()
	require.Equal(t, http.StatusOK, tb.command(t, "/show/", "{}"))
	got = tb.waitFrom(t, at, `id="tp-main"`)
	require.NotContains(t, saveButton.FindString(got), "disabled")
}

// discardDialog matches the dialog that confirms discarding unsaved toxics.
var discardDialog = regexp.MustCompile(
	`(?s)<neo-dialog[^>]* id="tp-discard".*?</neo-dialog>`,
)

// TestDiscardConfirm tests that the picker asks before a preset replaces
// toxics no preset holds, and applies at once otherwise.
func TestDiscardConfirm(t *testing.T) {
	s := startSettings(t)
	_, body := do(t, http.MethodGet, s.url+"/", "", nil)
	require.NotContains(t, presetPicker.FindString(body), "tp-discard")

	require.NoError(t, s.front.Update(func(st *toxic.Settings) error {
		st.Global[latencyDown] = toxic.NewToxic(toxic.Latency)
		return nil
	}))
	_, body = do(t, http.MethodGet, s.url+"/", "", nil)
	require.Contains(t, presetPicker.FindString(body),
		"document.getElementById(&#39;tp-discard&#39;).show()")
	dialog := discardDialog.FindString(body)
	require.Contains(t, dialog, "Discard unsaved toxics?")
	require.Regexp(t,
		`<neo-button[^>]*@post\(&#39;/apply-preset/&#39;\)[^>]*>\s*Discard`, dialog)
}

var (
	// reloadButton matches the opening tag of the reload button.
	reloadButton = regexp.MustCompile(
		`<neo-button[^>]*title="Discard the changes made on this page"[^>]*>`,
	)
	// reloadDialog matches the dialog that confirms a reload.
	reloadDialog = regexp.MustCompile(
		`(?s)<neo-dialog[^>]* id="tp-reload".*?</neo-dialog>`,
	)
)

// TestReloadConfirm tests that the reload button opens a confirm dialog while
// settings are unsaved and reloads at once after a save.
func TestReloadConfirm(t *testing.T) {
	s := startSettings(t)
	// The front has no presets, the file has 1.
	require.False(t, s.front.Saved())

	_, body := do(t, http.MethodGet, s.url+"/", "", nil)
	button := reloadButton.FindString(body)
	require.Contains(t, button, `document.getElementById(&#39;tp-reload&#39;).show()`)
	require.NotContains(t, button, "/reload/")
	dialog := reloadDialog.FindString(body)
	require.Contains(t, dialog, "Discard unsaved changes?")
	require.Contains(t, dialog, "lucide-triangle-alert")
	require.Contains(t, dialog,
		"Changes not saved to <code>datapages.yaml</code> cannot be restored.")
	require.Regexp(t,
		`<neo-button[^>]*@post\(&#39;/reload/&#39;\)[^>]*>\s*Discard and reload`,
		dialog)

	require.NoError(t, s.front.Save())
	_, body = do(t, http.MethodGet, s.url+"/", "", nil)
	require.Contains(t, reloadButton.FindString(body), `@post(&#39;/reload/&#39;)`)
}

// clearDialog matches the dialog that confirms clearing the overrides.
var clearDialog = regexp.MustCompile(`(?s)<neo-dialog[^>]* id="tp-clear".*?</neo-dialog>`)

// TestClearConfirm tests that the Clear overrides button opens a confirm dialog,
// whose confirm button clears them.
func TestClearConfirm(t *testing.T) {
	s := startSettings(t)
	require.NoError(t, s.front.Update(func(st *toxic.Settings) error {
		st.Endpoints[limited] = toxic.Set{latencyDown: toxic.NewToxic(toxic.Latency)}
		return nil
	}))

	_, body := do(t,
		http.MethodGet,
		s.url+"/?endpoint="+limitedKey,
		"",
		nil)
	require.Regexp(t,
		`<neo-button[^>]*document.getElementById\(&#39;tp-clear&#39;\).show\(\)[^>]*>`+
			`\s*Clear overrides`, body)
	dialog := clearDialog.FindString(body)
	require.Contains(t, dialog, `<dialog id="tp-clear-dialog">`)
	require.Contains(t, dialog, "Clear overrides?")
	require.Contains(t, dialog,
		"Overrides not saved to <code>datapages.yaml</code> cannot be restored.")
	require.Regexp(t,
		`<neo-button[^>]*@post\(&#39;/clear/&#39;\)[^>]*>\s*Clear overrides`,
		dialog)
}

// TestInherit tests an inherited slot row. It shows a summary while the
// global toxic is off and its disabled fields while it is on.
func TestInherit(t *testing.T) {
	s := startSettings(t)
	scoped := s.url + "/?endpoint=" + limitedKey
	row := regexp.MustCompile(
		`(?s)id="tp-latency_downstream".*?id="tp-latency_upstream"`,
	)

	_, body := do(t, http.MethodGet, scoped, "", nil)
	require.Contains(t, row.FindString(body), "Global: off")

	_ = s.front.Update(func(st *toxic.Settings) error {
		st.Global[latencyDown] = toxic.NewToxic(toxic.Latency)
		return nil
	})
	_, body = do(t, http.MethodGet, scoped, "", nil)
	got := row.FindString(body)
	require.NotContains(t, got, "Global:")
	require.Regexp(t,
		`<neo-textinput aria-label="Latency"[^>]* disabled[^>]* value="300"`, got)
	require.Regexp(t,
		`<neo-slider[^>]* value="100"[^>]* disabled`, got)

	_, body = do(t, http.MethodGet, s.url, "", nil)
	require.NotRegexp(t,
		`<neo-textinput aria-label="Latency"[^>]* disabled`, row.FindString(body))
}
