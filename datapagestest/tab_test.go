package datapagestest_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/romshark/datapages/datapagestest"
)

// load serves markup at / and the routes, and loads / in a tab.
func load(
	t *testing.T, markup string, routes map[string]http.Handler,
	opts ...datapagestest.Option,
) *datapagestest.Tab {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", page(markup, ""))
	for pattern, h := range routes {
		mux.Handle(pattern, h)
	}
	return datapagestest.New(t, mux, opts...).Visitor(t).Load(t, "/")
}

// event returns an SSE event of type with the data lines fields.
func event(typ string, fields ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "event: %s\n", typ)
	for _, f := range fields {
		fmt.Fprintf(&b, "data: %s\n", f)
	}
	b.WriteByte('\n')
	return b.String()
}

// inner renders the children of the element with id.
func inner(t *testing.T, tab *datapagestest.Tab, id string) string {
	t.Helper()
	var b strings.Builder
	for c := range tab.Get(t, datapagestest.ID(id)).Node().ChildNodes() {
		require.NoError(t, html.Render(&b, c))
	}
	return b.String()
}

// recorded is a request an action handler received.
type recorded struct {
	Method, Body, ContentType, Header string
}

// requests records the requests its handlers answer.
type requests struct {
	mu  sync.Mutex
	all []recorded
}

func (rs *requests) handler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodGet || r.Method == http.MethodDelete {
			b = []byte(r.URL.Query().Get("datastar"))
		}
		rs.mu.Lock()
		rs.all = append(rs.all, recorded{
			Method: r.Method, Body: string(b),
			ContentType: r.Header.Get("Content-Type"), Header: r.Header.Get("X-Test"),
		})
		rs.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func (rs *requests) last(t *testing.T) recorded {
	t.Helper()
	rs.mu.Lock()
	defer rs.mu.Unlock()
	require.NotEmpty(t, rs.all, "no request arrived")
	return rs.all[len(rs.all)-1]
}

const queryPage = `<!DOCTYPE html><html><head><title>Queries</title></head><body>
<h1>Todo List</h1>
<label for="search">Search</label><input id="search" type="text" placeholder="Find...">
<input type="text" placeholder="Nameless">
<label>Done <input type="checkbox" checked></label>
<button><span>Save</span></button>
<a href="/item/1/">Buy <b>milk</b></a>
<input type="submit">
<div role="button" aria-label="Close">x</div>
<span id="lbl">Notes</span><textarea aria-labelledby="lbl"></textarea>
<ul aria-label="Todos"><li>One</li><li>Two</li></ul>
<p id="secret" hidden>Secret</p>
<button aria-hidden="true">Ghost</button>
<neo-button>Start</neo-button>
<img src="a.png" alt="Logo">
<select aria-label="Show"><option value="all">All</option></select>
<section aria-label="Details"><button>Delete</button></section>
<button>Delete</button>
<button title="Help"><img src="h.png" alt=""></button>
</body></html>`

// TestQueries tests which element each kind of query selects.
func TestQueries(t *testing.T) {
	tab := load(t, queryPage, nil)
	for name, tc := range map[string]struct {
		q    datapagestest.Query
		want string // tag and text of the element
	}{
		"heading": {
			datapagestest.Role("heading", "Todo List"), "h1 Todo List",
		},
		"label for":              {datapagestest.Label("Search"), "input "},
		"textbox named by label": {datapagestest.Role("textbox", "Search"), "input "},
		"placeholder":            {datapagestest.Role("textbox", "Nameless"), "input "},
		"label around":           {datapagestest.Label("Done"), "input "},
		"checkbox":               {datapagestest.Role("checkbox", "Done"), "input "},
		"button from content":    {datapagestest.Role("button", "Save"), "button Save"},
		"innermost text":         {datapagestest.HasText("Save"), "span Save"},
		"link":                   {datapagestest.Role("link", "Buy milk"), "a Buy milk"},
		"default submit name":    {datapagestest.Role("button", "Submit"), "input "},
		"explicit role":          {datapagestest.Role("button", "Close"), "div x"},
		"labelledby":             {datapagestest.Label("Notes"), "textarea "},
		"list":                   {datapagestest.Role("list", "Todos"), "ul OneTwo"},
		"tag with text": {
			datapagestest.Tag("neo-button").HasText("Start"), "neo-button Start",
		},
		"img alt":  {datapagestest.Role("img", "Logo"), "img "},
		"combobox": {datapagestest.Label("Show"), "select All"},
		"in": {
			datapagestest.Role("button", "Delete").
				In(datapagestest.Role("region", "Details")), "button Delete",
		},
		"attr": {
			datapagestest.Tag("input").Attr("type", "submit"), "input ",
		},
		"hidden by id": {datapagestest.ID("secret"), "p Secret"},
		"title":        {datapagestest.Role("button", "Help"), "button "},
	} {
		t.Run(name, func(t *testing.T) {
			el := tab.Get(t, tc.q)
			require.Equal(t, tc.want, el.Node().Data+" "+el.Text())
		})
	}
	require.Equal(t, "Queries", tab.Title(t))
	require.Len(t, tab.All(t, datapagestest.Role("listitem", "")), 2)
	require.Len(t, tab.All(t, datapagestest.Role("button", "Delete")), 2)
	tab.RequireNone(t, datapagestest.HasText("Secret"))
	tab.RequireNone(t, datapagestest.Role("button", "Ghost"))
	tab.RequireNone(t, datapagestest.Role("button", "Placeholder"))
	tab.RequireNone(t, datapagestest.Label("Find..."))
}

// TestQueryFailures tests the messages of a query that matches 0 or 2 elements.
func TestQueryFailures(t *testing.T) {
	tab := load(t, queryPage, nil)
	for name, tc := range map[string]struct {
		f    func(testing.TB)
		want []string
	}{
		"none": {
			func(tb testing.TB) {
				tab.Get(tb, datapagestest.Role("button", "Nope"))
			},
			[]string{
				`tab /: no element matches role button "Nope", want 1`,
				`{Role: "button", Name: "Save"}`,
			},
		},
		"2": {
			func(tb testing.TB) {
				tab.Get(tb, datapagestest.Role("button", "Delete"))
			},
			[]string{`2 elements match role button "Delete", want 1`},
		},
		"present": {
			func(tb testing.TB) {
				tab.RequireNone(tb, datapagestest.Role("button", "Save"))
			},
			[]string{`1 element matches role button "Save", want none`},
		},
		"element": {
			func(tb testing.TB) {
				tab.Get(tb, datapagestest.Role("region", "")).
					Get(tb, datapagestest.Role("link", ""))
			},
			[]string{`tab /: role region: no element matches role link, want 1`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			msg := failure(t, tc.f)
			for _, w := range tc.want {
				require.Contains(t, msg, w)
			}
		})
	}
}

// TestDataShow tests that data-show hides an element by a signal,
// and that a query fails on an expression that needs a browser.
func TestDataShow(t *testing.T) {
	tab := load(t, `<div data-signals:open="false">
		<div style="display:none" data-show="$open"><button>Inside</button></div>
		<div data-show="!$open"><button>Outside</button></div>
		<div data-show="window.innerWidth > 600"><button>Unknown</button></div>
		<p style="display: none">Styled</p>
	</div>`, nil)
	tab.RequireNone(t, datapagestest.Role("button", "Inside"))
	tab.Get(t, datapagestest.Role("button", "Outside"))
	tab.RequireNone(t, datapagestest.HasText("Styled"))

	tab.SetSignal(t, "open", true)
	tab.Get(t, datapagestest.Role("button", "Inside"))
	tab.RequireNone(t, datapagestest.Role("button", "Outside"))

	msg := failure(t, func(tb testing.TB) {
		tab.Get(tb, datapagestest.Role("button", "Unknown"))
	})
	require.Contains(t, msg,
		`cannot tell whether role button "Unknown" <button> is shown: `+
			`data-show="window.innerWidth > 600" needs a browser`)
	msg = failure(t, func(tb testing.TB) { tab.Get(tb, datapagestest.ID("x")).Tree(tb) })
	require.Contains(t, msg, "no element matches")
}

// TestRender tests that the tab shows signals through data-text, data-attr,
// data-class and data-computed, and keeps what the server rendered for an
// expression that needs a browser.
func TestRender(t *testing.T) {
	tab := load(t, `<div data-signals='{"name":"ann","orig":"ann","n":2}'
		data-computed:changed="$name != $orig">
		<p id="text" data-text="'Hi ' + $name">server</p>
		<p id="js" data-text="$name.toUpperCase()">server</p>
		<button data-attr:disabled="!$changed">Save</button>
		<p id="class" class="a" data-class:many="$n > 1" data-class:a="false">x</p>
	</div>`, nil)
	require.Equal(t, "Hi ann", tab.Get(t, datapagestest.ID("text")).Text())
	require.Equal(t, "server", tab.Get(t, datapagestest.ID("js")).Text())
	save := datapagestest.Role("button", "Save")
	require.Contains(t, failure(t, func(tb testing.TB) { tab.Click(tb, save) }),
		`role button "Save" is disabled`)
	class, _ := tab.Get(t, datapagestest.ID("class")).Attr("class")
	require.Equal(t, "many", class)

	tab.SetSignal(t, "name", "bob")
	require.Equal(t, "Hi bob", tab.Get(t, datapagestest.ID("text")).Text())
	_, disabled := tab.Get(t, save).Attr("disabled")
	require.False(t, disabled)
}

// TestRequireTree tests the accessibility tree of a region and the message
// when it differs.
func TestRequireTree(t *testing.T) {
	tab := load(t, `<main id="m" data-signals='{"title":"x","filter":"done"}'>
		<h2>Items</h2>
		<ul aria-label="Items"><li>
			<input type="checkbox" aria-label="Buy milk" checked>
			<a href="/i/1/">Buy milk</a> <span>Due <b>Nov 2</b></span>
		</li></ul>
		<p>Total: 1</p>
		<label>Title <input data-bind="title"></label>
		<select aria-label="Show" data-bind="filter">
			<option value="all">All</option><option value="done">Done</option>
		</select>
		<button disabled>Save</button>
		<button aria-expanded="true" aria-pressed="true">Menu</button>
		<div hidden><button>Hidden</button></div>
	</main>`, nil)
	want := datapagestest.Tree{
		{Role: "heading", Name: "Items", Level: 2},
		{Role: "list", Name: "Items", Children: datapagestest.Tree{
			{Role: "listitem", Children: datapagestest.Tree{
				{Role: "checkbox", Name: "Buy milk", Checked: true},
				{Role: "link", Name: "Buy milk", URL: "/i/1/"},
				{Role: "text", Text: "Due Nov 2"},
			}},
		}},
		{Role: "paragraph", Text: "Total: 1"},
		{Role: "text", Text: "Title"},
		{Role: "textbox", Name: "Title", Value: "x"},
		{Role: "combobox", Name: "Show", Value: "Done"},
		{Role: "button", Name: "Save", Disabled: true},
		{Role: "button", Name: "Menu", Expanded: true, Pressed: true},
	}
	tab.RequireTree(t, datapagestest.ID("m"), want)

	want[2].Text = "Total: 2"
	msg := failure(t, func(tb testing.TB) {
		tab.RequireTree(tb, datapagestest.ID("m"), want)
	})
	require.Contains(t, msg, "tab /: id \"m\": the accessibility tree differs:\n")
	require.Contains(t, msg, "- \t{Role: \"paragraph\", Text: \"Total: 2\"},\n"+
		"+ \t{Role: \"paragraph\", Text: \"Total: 1\"},")
	require.Contains(t, msg, "actual tree:\ndatapagestest.Tree{\n")
}

// TestPatchModes tests how the tab applies each mode of an element patch.
func TestPatchModes(t *testing.T) {
	const start = `<div id="box"><p id="a">A</p><p id="b">B</p></div>`
	for name, tc := range map[string]struct {
		event, want string
	}{
		"outer by id": {
			patch(`<p id="a">A2</p>`),
			`<p id="a">A2</p><p id="b">B</p>`,
		},
		"replace": {
			event(datapagestest.PatchElements, "mode replace", `elements <p id="b">B2</p>`),
			`<p id="a">A</p><p id="b">B2</p>`,
		},
		"inner": {
			event(datapagestest.PatchElements,
				"selector #a", "mode inner", "elements <b>x</b>"),
			`<p id="a"><b>x</b></p><p id="b">B</p>`,
		},
		"append": {
			event(datapagestest.PatchElements,
				"selector #box", "mode append", `elements <p id="c">C</p>`),
			`<p id="a">A</p><p id="b">B</p><p id="c">C</p>`,
		},
		"prepend": {
			event(datapagestest.PatchElements,
				"selector #box", "mode prepend", `elements <p id="c">C</p>`),
			`<p id="c">C</p><p id="a">A</p><p id="b">B</p>`,
		},
		"before": {
			event(datapagestest.PatchElements,
				"selector #b", "mode before", "elements <hr/>"),
			`<p id="a">A</p><hr/><p id="b">B</p>`,
		},
		"after": {
			event(datapagestest.PatchElements,
				"selector #a", "mode after", "elements <hr/>"),
			`<p id="a">A</p><hr/><p id="b">B</p>`,
		},
		"remove by selector": {
			event(datapagestest.PatchElements, "selector #a", "mode remove"),
			`<p id="b">B</p>`,
		},
		"remove by id": {
			event(datapagestest.PatchElements, "mode remove", `elements <p id="b"></p>`),
			`<p id="a">A</p>`,
		},
		"escaped selector": {
			event(datapagestest.PatchElements,
				`selector #\61`, "mode inner", "elements 1"),
			`<p id="a">1</p><p id="b">B</p>`,
		},
		"signals": {
			event(datapagestest.PatchSignals, `signals {"x":1}`),
			`<p id="a">A</p><p id="b">B</p>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			tab := load(t, start, map[string]http.Handler{
				"POST /act": stream(tc.event, true),
			})
			tab.Do(t, http.MethodPost, "/act", nil).RequireStatus(t, http.StatusOK)
			require.Equal(t, tc.want, inner(t, tab, "box"))
		})
	}
}

// TestPatchTableRows tests that a patched row keeps its table structure.
func TestPatchTableRows(t *testing.T) {
	tab := load(t, `<table><tbody id="rows"><tr id="r1"><td>1</td></tr></tbody></table>`,
		map[string]http.Handler{
			"POST /act": stream(
				event(
					datapagestest.PatchElements,
					"selector #rows",
					"mode append",
					`elements <tr id="r2"><td>2</td></tr>`,
				),
				true,
			),
		})
	tab.Do(t, http.MethodPost, "/act", nil)
	require.Len(t, tab.All(t, datapagestest.Role("row", "")), 2)
	require.Equal(t, "2", tab.Get(t, datapagestest.Role("cell", "2")).Text())
}

// TestIgnoreMorph tests that a morph keeps an element marked data-ignore-morph.
func TestIgnoreMorph(t *testing.T) {
	tab := load(t, `<div id="box"><p id="kept" data-ignore-morph>old</p><p>x</p></div>`,
		map[string]http.Handler{
			"POST /act": stream(
				patch(`<div id="box"><p id="kept">new</p><p>y</p></div>`),
				true,
			),
		})
	tab.Do(t, http.MethodPost, "/act", nil)
	require.Equal(t,
		`<p id="kept" data-ignore-morph="">old</p><p>y</p>`,
		inner(t, tab, "box"))
}

// TestPatchProblems tests that a patch without a target is reported with a
// failing query, and that an unsupported selector fails every query.
func TestPatchProblems(t *testing.T) {
	tab := load(t, `<p id="a">A</p>`, map[string]http.Handler{
		"POST /missing": stream(patch(`<p id="zz">Z</p>`), true),
		"POST /class": stream(event(datapagestest.PatchElements,
			"selector .a", "mode inner", "elements x"), true),
	})
	tab.Do(t, http.MethodPost, "/missing", nil)
	msg := failure(t, func(tb testing.TB) { tab.Get(tb, datapagestest.ID("zz")) })
	require.Contains(t, msg, "skipped patches:\nevent 1 of POST /missing: "+
		"no element matches #zz; Datastar skips it")

	tab.Do(t, http.MethodPost, "/class", nil)
	msg = failure(t, func(tb testing.TB) { tab.Get(tb, datapagestest.ID("a")) })
	require.Contains(t, msg, `applying event 1 of POST /class: selector ".a": `+
		`datapagestest supports #id and tag selectors`)
}

// TestSignalPatches tests how the tab merges signal patches and the signals of
// patched elements.
func TestSignalPatches(t *testing.T) {
	const start = `<div id="d" data-signals='{"a":1,"o":{"x":1,"y":2}}'></div>`
	for name, tc := range map[string]struct {
		event string
		path  string
		want  any
	}{
		"set":    {event(datapagestest.PatchSignals, `signals {"a":2}`), "a", 2.0},
		"delete": {event(datapagestest.PatchSignals, `signals {"a":null}`), "a", nil},
		"nested": {event(datapagestest.PatchSignals,
			`signals {"o":{"y":null,"z":3}}`), "o", map[string]any{"x": 1.0, "z": 3.0}},
		"if missing": {event(datapagestest.PatchSignals,
			"onlyIfMissing true", `signals {"a":5,"b":6}`), "a", 1.0},
		"added": {event(datapagestest.PatchSignals,
			"onlyIfMissing true", `signals {"a":5,"b":6}`), "b", 6.0},
		"element": {patch(`<div id="d" data-signals:a="7"></div>`), "a", 7.0},
	} {
		t.Run(name, func(t *testing.T) {
			tab := load(t, start, map[string]http.Handler{
				"POST /act": stream(tc.event, true),
			})
			tab.Do(t, http.MethodPost, "/act", nil)
			require.Equal(t, tc.want, tab.Signal(t, tc.path))
		})
	}
}

// TestUnchangedSignalAttr tests that a morph that keeps a data-signals
// attribute keeps the signal's current value, as Datastar does.
func TestUnchangedSignalAttr(t *testing.T) {
	tab := load(t, `<div id="d" data-signals:a="1"><p>x</p></div>`,
		map[string]http.Handler{
			"POST /act": stream(
				patch(`<div id="d" data-signals:a="1"><p>y</p></div>`),
				true,
			),
		})
	tab.SetSignal(t, "a", 5)
	tab.Do(t, http.MethodPost, "/act", nil)
	require.Equal(t, 5.0, tab.Signal(t, "a"))
	tab.Get(t, datapagestest.HasText("y"))
}

// TestWaitFor tests that WaitFor and WaitGone wait for
// the tab's stream to change the page.
func TestWaitFor(t *testing.T) {
	release := make(chan struct{})
	tab := load(t, `<ul id="list"><li id="old">old</li></ul>`, map[string]http.Handler{
		"GET /_$/": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			<-release
			_, _ = io.WriteString(w, patch(`<ul id="list"><li>new</li></ul>`))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}),
	})
	tab.Connect(t)
	close(release)
	require.Equal(t, "li", tab.WaitFor(t, datapagestest.HasText("new")).Node().Data)
	tab.WaitGone(t, datapagestest.ID("old"))
}

// TestWaitFails tests a wait without a stream and a wait that times out.
func TestWaitFails(t *testing.T) {
	tab := load(t, `<p>page</p>`, map[string]http.Handler{
		"GET /_$/": stream("", false),
	}, datapagestest.WithTimeout(50*time.Millisecond))
	msg := failure(t, func(tb testing.TB) {
		tab.WaitFor(tb, datapagestest.HasText("never"))
	})
	require.Contains(t,
		msg,
		`tab /: no open stream can change the page while waiting for text "never"`)

	tab.Connect(t)
	msg = failure(t, func(tb testing.TB) {
		tab.WaitFor(tb, datapagestest.HasText("never"))
	})
	require.Contains(t, msg, `tab /: waited 50ms for text "never"`)
	msg = failure(t, func(tb testing.TB) {
		tab.WaitGone(tb, datapagestest.HasText("page"))
	})
	require.Contains(t, msg, `tab /: waited 50ms for no text "page"`)
}

// TestClick tests what a click runs: the assignments around an action in
// order, and the action with the tab's public signals and its options.
func TestClick(t *testing.T) {
	var rs requests
	tab := load(t, `<div data-signals='{"title":"milk","_local":1,"n":2}'>
		<button data-on:click="$_open = true; @post('/add', {headers: {'X-Test': 'yes'}}); $title = ''">Add</button>
		<button data-on:click="@post('/add', {filterSignals: {include: /^title$/}})">Title only</button>
		<button data-on:click="@get('/add')">Get</button>
		<ul id="list"></ul>
		<p data-show="$_open">Open</p>
	</div>`, map[string]http.Handler{
		"/add": rs.handler(http.StatusOK, patch(`<ul id="list"><li>added</li></ul>`)),
	})
	resp := tab.Click(t, datapagestest.Role("button", "Add"))
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, recorded{
		Method: http.MethodPost, Body: `{"n":2,"title":"milk"}`,
		ContentType: "application/json", Header: "yes",
	}, rs.last(t))
	require.Empty(t, tab.Signal(t, "title"))
	require.Equal(t, "added", tab.Get(t, datapagestest.Role("listitem", "")).Text())
	tab.Get(t, datapagestest.HasText("Open"))

	tab.SetSignal(t, "title", "eggs")
	tab.Click(t, datapagestest.Role("button", "Title only"))
	require.Equal(t, `{"title":"eggs"}`, rs.last(t).Body)

	tab.Click(t, datapagestest.Role("button", "Get"))
	require.Equal(t, recorded{Method: http.MethodGet, Body: `{"n":2,"title":"eggs"}`},
		rs.last(t))
}

// TestClickCheckbox tests that a click toggles a checkbox through its signal or
// its checked attribute and runs its change handler.
func TestClickCheckbox(t *testing.T) {
	var rs requests
	tab := load(t, `<div data-signals:done="false">
		<input type="checkbox" aria-label="Done" data-bind="done" data-on:change="@put('/toggle')">
		<input type="checkbox" aria-label="Plain">
		<input type="radio" name="c" aria-label="Red" checked><input type="radio" name="c" aria-label="Blue">
	</div>`, map[string]http.Handler{"/toggle": rs.handler(http.StatusOK, "")})
	done := datapagestest.Role("checkbox", "Done")
	tab.Click(t, done)
	require.True(t, tab.Get(t, done).Checked())
	require.Equal(t, `{"done":true}`, rs.last(t).Body)

	plain := datapagestest.Role("checkbox", "Plain")
	tab.Click(t, plain)
	require.True(t, tab.Get(t, plain).Checked())
	tab.Click(t, plain)
	require.False(t, tab.Get(t, plain).Checked())

	tab.Click(t, datapagestest.Role("radio", "Blue"))
	require.True(t, tab.Get(t, datapagestest.Role("radio", "Blue")).Checked())
	require.False(t, tab.Get(t, datapagestest.Role("radio", "Red")).Checked())
}

// TestSubmitForm tests that a submit button sends the fields of its form and
// that the browser's required check blocks an empty field.
func TestSubmitForm(t *testing.T) {
	var rs requests
	tab := load(t,
		`<form aria-label="Order" data-on:submit__prevent="@post('/form', {contentType: 'form'})">
		<label>Name <input name="name" value="ann" required></label>
		<input type="checkbox" name="agree" checked>
		<input type="checkbox" name="spam">
		<select name="color"><option>red</option><option selected>blue</option></select>
		<textarea name="note">hi</textarea>
		<input name="off" value="x" disabled>
		<button>Send</button>
	</form>`, map[string]http.Handler{"/form": rs.handler(http.StatusOK, "")})
	tab.Click(t, datapagestest.Role("button", "Send"))
	require.Equal(t, recorded{
		Method: http.MethodPost, Body: "agree=on&color=blue&name=ann&note=hi",
		ContentType: "application/x-www-form-urlencoded",
	}, rs.last(t))

	tab.Fill(t, datapagestest.Label("Name"), "bob")
	tab.Submit(t, datapagestest.Role("form", "Order"))
	require.Equal(t, "agree=on&color=blue&name=bob&note=hi", rs.last(t).Body)

	tab.Fill(t, datapagestest.Label("Name"), "")
	msg := failure(t, func(tb testing.TB) {
		tab.Click(tb, datapagestest.Role("button", "Send"))
	})
	require.Contains(t, msg, `the browser does not submit the form: <input name="name" `)
}

// TestClickFailures tests what a click refuses.
func TestClickFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		markup string
		q      datapagestest.Query
		want   string
	}{
		"javascript": {
			`<button data-on:click="alert(1)">Go</button>`,
			datapagestest.Role("button", "Go"),
			`data-on:click="alert(1)" of <button data-on:click="alert(1)"> needs a browser: ` +
				`"alert(1)": not a generated action call or a signal assignment; ` +
				`send its request with Tab.Do`,
		},
		"disabled": {
			`<button disabled data-on:click="@post('/ok')">Go</button>`,
			datapagestest.Role("button", "Go"), `role button "Go" is disabled`,
		},
		"hidden": {
			`<button id="go" hidden data-on:click="@post('/ok')">Go</button>`,
			datapagestest.ID("go"), `id "go" is hidden`,
		},
		"no handler": {
			`<button type="button">Go</button>`,
			datapagestest.Role("button", "Go"), `role button "Go" has no data-on:click`,
		},
		"link": {
			`<a href="/x">Go</a>`,
			datapagestest.Role("link", "Go"), "is a link without data-on:click; load it with Tab.Follow",
		},
		"status": {
			`<button data-on:click="@post('/fail')">Go</button>`,
			datapagestest.Role("button", "Go"),
			"POST /fail: 500; send a request expected to fail with Tab.Do",
		},
		"native form": {
			`<form><button>Go</button></form>`,
			datapagestest.Role("button", "Go"),
			"<form> has no data-on:submit, so the browser would submit it without the CSRF token",
		},
		"payload": {
			`<button data-on:click="@post('/ok', {payload: {a: 1}})">Go</button>`,
			datapagestest.Role("button", "Go"), "the payload option needs a browser",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var rs requests
			tab := load(t, tc.markup, map[string]http.Handler{
				"/ok":   rs.handler(http.StatusOK, ""),
				"/fail": rs.handler(http.StatusInternalServerError, ""),
			})
			msg := failure(t, func(tb testing.TB) { tab.Click(tb, tc.q) })
			require.Contains(t, msg, tc.want)
		})
	}
}

// TestFillAndSelect tests that typing and choosing set the bound signals and
// run the input handlers.
func TestFillAndSelect(t *testing.T) {
	var rs requests
	tab := load(t, `<div data-signals='{"q":"","n":1}'>
		<label>Query <input data-bind="q" data-on:input__debounce.200ms="@post('/search')"></label>
		<label>Count <input type="number" data-bind="n"></label>
		<label>Note <textarea></textarea></label>
		<label>Show <select data-bind="filter" data-on:change="@post('/search')">
			<option value="all">All</option><option value="done">Done</option>
		</select></label>
	</div>`, map[string]http.Handler{"/search": rs.handler(http.StatusOK, "")})

	tab.Fill(t, datapagestest.Label("Query"), "milk")
	require.Equal(t, `{"filter":"all","n":1,"q":"milk"}`, rs.last(t).Body)
	require.Equal(t, "milk", tab.Get(t, datapagestest.Label("Query")).Value())

	tab.Fill(t, datapagestest.Label("Count"), "5")
	require.Equal(t, 5.0, tab.Signal(t, "n"))

	tab.Fill(t, datapagestest.Label("Note"), "some text")
	require.Equal(t, "some text", tab.Get(t, datapagestest.Label("Note")).Value())

	tab.Select(t, datapagestest.Label("Show"), "Done")
	require.Equal(t, "done", tab.Signal(t, "filter"))
	require.Equal(t, `{"filter":"done","n":5,"q":"milk"}`, rs.last(t).Body)
	require.Equal(t, "done", tab.Get(t, datapagestest.Label("Show")).Value())

	msg := failure(t, func(tb testing.TB) {
		tab.Select(tb, datapagestest.Label("Show"), "Gone")
	})
	require.Contains(t, msg, `has no enabled option "Gone"; options: "All", "Done"`)
	msg = failure(t, func(tb testing.TB) {
		tab.Fill(tb, datapagestest.Label("Show"), "x")
	})
	require.Contains(t, msg, "Fill takes a text input or a textarea")
}

// TestFollow tests that a link loads its target, resolved against the tab's
// URL, in a new tab, and that a link off the server fails.
func TestFollow(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /list/",
		page(`<a href="item/1/?x=1">Item</a><a href="https://example.com/">Out</a>`, ""))
	mux.Handle("GET /list/item/1/",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page(`<h1>Item `+r.URL.Query().Get("x")+`</h1>`, "")(w, r)
		}))
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/list/")
	item := tab.Follow(t, datapagestest.Role("link", "Item"))
	require.Equal(t, "Item 1", item.Get(t, datapagestest.Role("heading", "")).Text())

	msg := failure(t, func(tb testing.TB) {
		tab.Follow(tb, datapagestest.Role("link", "Out"))
	})
	require.Contains(t, msg, `"https://example.com/" leads off the server`)
}

// TestCall tests that Call sends the action of an expression with the tab's signals.
func TestCall(t *testing.T) {
	var rs requests
	tab := load(t, `<div data-signals:rev="3"></div>`, map[string]http.Handler{
		"/sync": rs.handler(
			http.StatusOK,
			event(datapagestest.PatchSignals, `signals {"rev":4}`),
		),
	})
	resp := tab.Call(t, `@post('/sync')`)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, `{"rev":3}`, rs.last(t).Body)
	require.Equal(t, 4.0, tab.Signal(t, "rev"))
	require.Nil(t, tab.Call(t, `$rev = 9`))
	require.Equal(t, 9.0, tab.Signal(t, "rev"))
}

// TestResponseAndEventGet tests queries on a response and on an event,
// which no tab updates.
func TestResponseAndEventGet(t *testing.T) {
	resp := load(t, `<h1>Title</h1>`, nil).Page()
	require.Equal(t, "Title", resp.Get(t, datapagestest.Role("heading", "")).Text())
	require.Len(t, resp.All(t, datapagestest.Role("heading", "")), 1)

	ev := datapagestest.Event{
		Type: datapagestest.PatchElements,
		Data: map[string]string{"elements": `<tr id="r"><td>cell</td></tr>`},
	}
	require.Equal(t, "cell", ev.Get(t, datapagestest.Role("cell", "")).Text())
	ev.Get(t, datapagestest.ID("r")).RequireTree(t, datapagestest.Tree{
		{Role: "cell", Name: "cell"},
	})
	require.Equal(t, "cell", ev.All(t, datapagestest.Tag("td"))[0].Text())
}
