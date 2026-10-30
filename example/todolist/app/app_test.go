package app_test

import (
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

// TestCreateTodo tests that a todo created in 1 tab appears in every open tab,
// and that the dialog closes and opens again empty.
func TestCreateTodo(t *testing.T) {
	todos := new(list.List)
	srv := newServer(t, todos)
	alice := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	bob := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	alice.Connect(t)
	bob.Connect(t)

	require.Equal(t, "Todo List", alice.Title(t))
	alice.RequireTree(t, datapagestest.ID("page-index"), datapagestest.Tree{
		{Role: "heading", Name: "Todo List", Level: 1},
		{Role: "textbox", Name: "Search"},
		{Role: "combobox", Name: "Show", Value: "All"},
		{Role: "combobox", Name: "Sort by", Value: "Created"},
		{Role: "button", Name: "+ New Todo"},
		{Role: "paragraph", Text: "No todos found."},
	})

	dialog := datapagestest.Role("dialog", "New Todo")
	due := time.Date(2026, time.November, 2, 18, 0, 0, 0, time.UTC)
	alice.Click(t, datapagestest.Role("button", "+ New Todo"))
	alice.Fill(t, datapagestest.Label("Title").In(dialog), "Buy milk")
	alice.Fill(t,
		datapagestest.Label("Due Date").In(dialog), due.Format("2006-01-02T15:04"))
	alice.Click(t, datapagestest.Role("button", "Create").In(dialog))
	alice.RequireNone(t, dialog)

	id := todos.GetItems(list.ViewParameters{})[0].ID
	for _, tab := range []*datapagestest.Tab{alice, bob} {
		tab.WaitFor(t, datapagestest.Role("link", "Buy milk"))
		tab.RequireTree(t, datapagestest.Role("list", "Todos"), datapagestest.Tree{
			{Role: "listitem", Children: datapagestest.Tree{
				{Role: "checkbox", Name: "Buy milk"},
				{Role: "link", Name: "Buy milk", URL: href.PageItem(id)},
				{Role: "text", Text: "Due by " + due.Format("Jan 2, 15:04")},
			}},
		})
	}

	alice.Click(t, datapagestest.Role("button", "+ New Todo"))
	require.Empty(t, alice.Get(t, datapagestest.Label("Title").In(dialog)).Value())

	item := bob.Follow(t, datapagestest.Role("link", "Buy milk"))
	require.Equal(t, "Buy milk", item.Get(t, datapagestest.Label("Title")).Value())
	require.Equal(t, due.Format("2006-01-02T15:04"),
		item.Get(t, datapagestest.Label("Due Date")).Value())
	require.False(t, item.Get(t, datapagestest.Role("checkbox", "Done")).Checked())
}

// TestFilterDone tests that the Done filter of 1 tab lists only the todos
// checked done, and that a todo checked in 1 tab shows checked in the others.
func TestFilterDone(t *testing.T) {
	todos := new(list.List)
	todos.AddItem("Buy milk", "", time.Time{})
	todos.AddItem("Walk the dog", "", time.Time{})
	srv := newServer(t, todos)
	tab := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	other := srv.Visitor(t).Load(t, href.PageIndex(href.QueryPageIndex{}))
	tab.Connect(t)
	other.Connect(t)

	walk := datapagestest.Role("checkbox", "Walk the dog")
	tab.Click(t, walk)
	require.True(t, other.WaitFor(t, walk.Attr("checked", "")).Checked())

	tab.Select(t, datapagestest.Label("Show"), "Done")
	tab.RequireTree(t, datapagestest.Role("list", "Todos"), datapagestest.Tree{
		{Role: "listitem", Children: datapagestest.Tree{
			{Role: "checkbox", Name: "Walk the dog", Checked: true},
			{
				Role: "link",
				Name: "Walk the dog",
				URL:  href.PageItem(id(t, todos, "Walk the dog")),
			},
		}},
	})
	require.Len(t, other.All(t, datapagestest.Role("listitem", "")), 2)
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

// newServer serves the app on todos with the NewServer type arguments of cmd/server.
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
