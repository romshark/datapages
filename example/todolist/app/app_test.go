package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/todolist/app"
	"github.com/romshark/datapages/example/todolist/app/datapagesgen"
	"github.com/romshark/datapages/example/todolist/app/datapagesgen/href"
	"github.com/romshark/datapages/example/todolist/list"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// TestIndexPage tests what an empty list shows on load. The dialog for a new
// todo shows while the showModal signal is set.
func TestIndexPage(t *testing.T) {
	page := newServer(t, new(list.List)).Visitor(t).
		Get(t, href.PageIndex(href.QueryPageIndex{})).RequireStatus(t, http.StatusOK)
	require.Equal(t, "Todo List", page.Get(t, datapagestest.Tag("title")).Text())
	page.Get(t, datapagestest.ID("page-index")).RequireTree(t, datapagestest.Tree{
		{Role: "heading", Name: "Todo List", Level: 1},
		{Role: "textbox", Name: "Search"},
		{Role: "combobox", Name: "Show", Value: "All"},
		{Role: "combobox", Name: "Sort by", Value: "Created"},
		{Role: "button", Name: "+ New Todo"},
		{Role: "paragraph", Text: "No todos found."},
		{Role: "dialog", Name: "New Todo", ShownIf: "$showModal", Children: datapagestest.Tree{
			{Role: "heading", Name: "New Todo", Level: 2},
			{Role: "text", Text: "Title"},
			{Role: "textbox", Name: "Title"},
			{Role: "text", Text: "Description"},
			{Role: "textbox", Name: "Description"},
			{Role: "text", Text: "Due Date"},
			{Role: "textbox", Name: "Due Date"},
			{Role: "button", Name: "Cancel"},
			{Role: "button", Name: "Create"},
		}},
	})
}

// TestCreateTodo tests that a todo created in 1 tab reaches the list of every
// open tab, and that its item page carries what was created.
func TestCreateTodo(t *testing.T) {
	todos := new(list.List)
	srv := newServer(t, todos)
	alice := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	bob := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	streams := []*datapagestest.Stream{alice.Connect(t, nil), bob.Connect(t, nil)}

	// The dialog sends the signals its fields bind.
	due := time.Date(2026, time.November, 2, 18, 0, 0, 0, time.UTC)
	dialog := alice.Page().Get(t, datapagestest.Role("dialog", "New Todo"))
	create := dialog.Get(t, datapagestest.Role("button", "Create")).Action(t, "click")
	alice.Do(t, create, map[string]string{
		dialog.Get(t, datapagestest.Label("Title")).Bind():    "Buy milk",
		dialog.Get(t, datapagestest.Label("Due Date")).Bind(): due.Format("2006-01-02T15:04"),
	}).RequireStatus(t, http.StatusOK)

	id := todos.GetItems(list.ViewParameters{})[0].ID
	for _, st := range streams {
		ev := st.WaitFor(t, datapagestest.TextContains("todo-list", "Buy milk"))
		ev.Get(t, datapagestest.Role("list", "Todos")).RequireTree(t, datapagestest.Tree{
			{Role: "listitem", Children: datapagestest.Tree{
				{Role: "checkbox", Name: "Buy milk"},
				{Role: "link", Name: "Buy milk", URL: href.PageItem(id)},
				{Role: "text", Text: "Due by " + due.Format("Jan 2, 15:04")},
			}},
		})
	}

	item := srv.Visitor(t).Get(t, href.PageItem(id)).RequireStatus(t, http.StatusOK)
	require.Equal(t, "Buy milk", item.Get(t, datapagestest.Role("heading", "")).Text())
	// The fields show the signals they bind.
	signals := item.Signals(t)
	field := func(label string) any {
		return signals[item.Get(t, datapagestest.Label(label)).Bind()]
	}
	require.Equal(t, "Buy milk", field("Title"))
	require.Equal(t, due.Format("2006-01-02T15:04"), field("Due Date"))
	require.Equal(t, false, field("Done"))
}

// TestFilterDone tests that a toggle reaches every open tab and that the Done
// filter answers with only the todos checked done.
func TestFilterDone(t *testing.T) {
	todos := new(list.List)
	todos.AddItem("Buy milk", "", time.Time{})
	todos.AddItem("Walk the dog", "", time.Time{})
	walk := id(t, todos, "Walk the dog")
	srv := newServer(t, todos)
	tab := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	other := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	tab.Connect(t, nil)
	otherStream := other.Connect(t, nil)

	toggle := tab.Page().Get(t, datapagestest.Role("checkbox", "Walk the dog")).
		Action(t, "click")
	tab.Do(t, toggle, nil).RequireStatus(t, http.StatusOK)
	ev := otherStream.WaitFor(t, datapagestest.Patched("todo-list"))
	require.True(t, ev.Get(t, datapagestest.Role("checkbox", "Walk the dog")).Checked())
	require.False(t, ev.Get(t, datapagestest.Role("checkbox", "Buy milk")).Checked())

	filter := tab.Page().Get(t, datapagestest.Label("Show")).Action(t, "change")
	resp := tab.Do(t, filter, map[string]string{"filter": "done"}).
		RequireStatus(t, http.StatusOK)
	resp.Event(t, datapagestest.Patched("todo-list")).
		Get(t, datapagestest.Role("list", "Todos")).
		RequireTree(t, datapagestest.Tree{
			{Role: "listitem", Children: datapagestest.Tree{
				{Role: "checkbox", Name: "Walk the dog", Checked: true},
				{Role: "link", Name: "Walk the dog", URL: href.PageItem(walk)},
			}},
		})
}

// id returns the id of the todo titled title.
func id(t *testing.T, todos *list.List, title string) string {
	t.Helper()
	for _, it := range todos.GetItems(list.ViewParameters{}) {
		if it.Title == title {
			return it.ID
		}
	}
	require.FailNow(t, "no todo titled "+title)
	return ""
}

// newServer serves the app on todos with the NewServer type arguments of
// cmd/server.
func newServer(t *testing.T, todos *list.List) *datapagestest.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](app.NewApp(todos), inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS, false))
	require.NoError(t, err)
	return datapagestest.New(t, s)
}
