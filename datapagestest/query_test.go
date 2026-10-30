package datapagestest_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
)

// load serves markup at / and returns the response to a GET of it.
func load(t *testing.T, markup string) *datapagestest.Response {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", page(markup, ""))
	return datapagestest.New(t, mux).Visitor(t).Get(t, "/")
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
<ul aria-label="Todos"><li>One</li><li class="done late">Two</li></ul>
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
	page := load(t, queryPage)
	done := datapagestest.Where("class done", func(e *datapagestest.Element) bool {
		class, _ := e.Attr("class")
		return slices.Contains(strings.Fields(class), "done")
	})
	for name, tc := range map[string]struct {
		q    datapagestest.Query
		want string // tag and text of the element
	}{
		"heading":                {datapagestest.Role("heading", "Todo List"), "h1 Todo List"},
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
		"tag with text":          {datapagestest.Tag("neo-button").HasText("Start"), "neo-button Start"},
		"img alt":                {datapagestest.Role("img", "Logo"), "img "},
		"combobox":               {datapagestest.Label("Show"), "select All"},
		"attr":                   {datapagestest.Tag("input").Attr("type", "submit"), "input "},
		"hidden by id":           {datapagestest.ID("secret"), "p Secret"},
		"title":                  {datapagestest.Role("button", "Help"), "button "},
		"where":                  {done, "li Two"},
		"in": {
			datapagestest.Role("button", "Delete").In(datapagestest.Role("region", "Details")),
			"button Delete",
		},
	} {
		t.Run(name, func(t *testing.T) {
			el := page.Get(t, tc.q)
			require.Equal(t, tc.want, el.Node().Data+" "+el.Text())
		})
	}
	require.Equal(t, "Queries", page.Get(t, datapagestest.Tag("title")).Text())
	require.Len(t, page.All(datapagestest.Role("listitem", "")), 2)
	require.Len(t, page.All(datapagestest.Role("button", "Delete")), 2)
	require.Empty(t, page.All(datapagestest.HasText("Secret")))
	require.Empty(t, page.All(datapagestest.Role("button", "Ghost")))
	require.Empty(t, page.All(datapagestest.Label("Find...")),
		"a placeholder is no label")
}

// TestQueryFailures tests the messages of a query that matches 0 or 2 elements.
func TestQueryFailures(t *testing.T) {
	page := load(t, queryPage)
	for name, tc := range map[string]struct {
		f    func(testing.TB)
		want []string
	}{
		"none": {
			func(tb testing.TB) { page.Get(tb, datapagestest.Role("button", "Nope")) },
			[]string{
				`GET /: no element matches role button "Nope", want 1`,
				`{Role: "button", Name: "Save"}`,
			},
		},
		"2": {
			func(tb testing.TB) { page.Get(tb, datapagestest.Role("button", "Delete")) },
			[]string{`2 elements match role button "Delete", want 1`},
		},
		"element": {
			func(tb testing.TB) {
				page.Get(tb, datapagestest.Role("region", "")).
					Get(tb, datapagestest.Role("link", ""))
			},
			[]string{`GET /: role region: no element matches role link, want 1`},
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

// TestHidden tests that a visitor query skips what the HTML hides, and that
// data-show, which Datastar evaluates in the browser, decides nothing.
func TestHidden(t *testing.T) {
	page := load(t, `
		<div hidden><button>Attribute</button></div>
		<p style="color: red; display: none">Styled</p>
		<div style="display:none" data-show="$open"><button>Until shown</button></div>
		<div data-show="$open"><button>Server shown</button></div>
		<dialog><button>Closed</button></dialog>
		<dialog open><button>Open</button></dialog>
		<template><button>Template</button></template>
	`)
	for text, shown := range map[string]bool{
		"Attribute": false, "Styled": false, "Until shown": false, "Server shown": true,
		"Closed": false, "Open": true, "Template": false,
	} {
		require.Equal(t, shown, len(page.All(datapagestest.HasText(text))) == 1, text)
	}
	require.Len(t, page.All(datapagestest.Tag("button")), 6)
}

// TestValueAndChecked tests the values and states the HTML sets on controls.
func TestValueAndChecked(t *testing.T) {
	page := load(t, `
		<input aria-label="Name" value="ann" data-bind="name">
		<textarea aria-label="Note">some
text</textarea>
		<select aria-label="Show"><option value="all">All</option>
			<option value="done" selected>Done</option></select>
		<select aria-label="Sort"><option value="alpha">A-Z</option></select>
		<input type="checkbox" aria-label="On" checked>
		<input type="checkbox" aria-label="Off">
		<input type="checkbox" aria-label="Default">
		<div role="switch" aria-label="Dark" aria-checked="true"></div>
	`)
	for label, want := range map[string]string{
		"Name": "ann", "Note": "some\ntext", "Show": "done", "Sort": "alpha", "Default": "on",
	} {
		require.Equal(t, want, page.Get(t, datapagestest.Label(label)).Value(), label)
	}
	for label, want := range map[string]bool{"On": true, "Off": false, "Dark": true} {
		require.Equal(t, want, page.Get(t, datapagestest.Label(label)).Checked(), label)
	}
}

// TestRequireTree tests the accessibility tree of a region and the message when
// it differs.
func TestRequireTree(t *testing.T) {
	page := load(t, `<main id="m">
		<h2>Items</h2>
		<ul aria-label="Items"><li>
			<input type="checkbox" aria-label="Buy milk" checked>
			<a href="/i/1/">Buy milk</a> <span>Due <b>Nov 2</b></span>
		</li></ul>
		<p>Total: 1</p>
		<label>Title <input value="x"></label>
		<select aria-label="Show">
			<option value="all">All</option><option value="done" selected>Done</option>
		</select>
		<button disabled>Save</button>
		<button aria-expanded="true" aria-pressed="true">Menu</button>
		<div hidden><button>Hidden</button></div>
	</main>`)
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
	main := page.Get(t, datapagestest.ID("m"))
	main.RequireTree(t, want)
	require.Equal(t, want, main.Tree())

	want[2].Text = "Total: 2"
	msg := failure(t, func(tb testing.TB) { main.RequireTree(tb, want) })
	require.Contains(t, msg, "GET /: id \"m\": the accessibility tree differs:\n")
	require.Contains(t, msg, "- \t{Role: \"paragraph\", Text: \"Total: 2\"},\n"+
		"+ \t{Role: \"paragraph\", Text: \"Total: 1\"},")
	require.Contains(t, msg, "actual tree:\ndatapagestest.Tree{\n")
}

// TestEventGet tests queries on the elements of an event, which a <template>
// holds as Datastar parses them.
func TestEventGet(t *testing.T) {
	ev := datapagestest.Event{
		Type: datapagestest.PatchElements,
		Data: map[string]string{"elements": `<tr id="r"><td>cell</td></tr>`},
	}
	require.Equal(t, "cell", ev.Get(t, datapagestest.Role("cell", "")).Text())
	ev.Get(t, datapagestest.ID("r")).RequireTree(t, datapagestest.Tree{
		{Role: "cell", Name: "cell"},
	})
	require.Len(t, ev.All(datapagestest.Tag("td")), 1)
	msg := failure(t, func(tb testing.TB) { ev.Get(tb, datapagestest.ID("x")) })
	require.Contains(t, msg, `datastar-patch-elements elements="<tr id=\"r\"><td>cell</td></tr>": `+
		`no element matches id "x", want 1`)
}
