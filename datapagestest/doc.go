// Package datapagestest tests a Datapages server over HTTP without
// a browser or network socket.
//
// Pass the handler returned by [github.com/romshark/datapages.NewServer] to [New]:
//
//	srv := datapagestest.New(t, s)
//	v := srv.Visitor(t)
//	tab := v.Load(t, href.PageIndex())
//	tab.Page().Get(t, datapagestest.Role("heading", "Todo List"))
//	st := tab.Connect(t, nil)
//	add := tab.Page().Get(t, datapagestest.Role("button", "Add")).Action(t, "click")
//	tab.Do(t, add, map[string]any{"title": "Buy milk"}).RequireStatus(t, http.StatusOK)
//	ev := st.WaitFor(t, datapagestest.Patched("todos"))
//	ev.Get(t, datapagestest.Role("list", "Todos")).RequireTree(t, datapagestest.Tree{
//		{Role: "listitem", Children: datapagestest.Tree{
//			{Role: "checkbox", Name: "Buy milk"},
//			{Role: "link", Name: "Buy milk", URL: href.PageItem(id)},
//		}},
//	})
//
// A [Visitor] shares cookies among its [Tab]s. A tab keeps its page response,
// Datapages instance and CSRF token. [Tab.Connect] opens a [Stream], which retains
// events for [Stream.WaitFor] and [Stream.Absent]. Each request and wait fails after
// [DefaultTimeout] unless [WithTimeout] changes it.
//
// A [Query] selects elements of a [Response] or an [Event] as a visitor finds
// them, by [Role] and accessible name, by [Label] and by [HasText]. [ID], [Tag]
// and [Where] select what has no role. [Element.RequireTree] compares the
// accessibility tree of a region with a [Tree].
//
// The package reads what the server sends and keeps no client-side state. Tabs do
// not run JavaScript, apply patches, retry or reconnect. [Tab.Page] remains the
// loaded response. [Tab.Do] sends the action an expression of the generated
// action package or [Element.Action] calls, as JSON signals or form fields the
// test supplies, as the call says. A hidden element is one the HTML hides with
// the hidden attribute, aria-hidden, display: none in its style attribute or a
// closed <dialog> around it. An element with data-show counts as present,
// [Node.ShownIf] and [Element.ShownIf] hold its expression, and a query whose
// answer the expression decides fails. Use a browser test for client-only
// behavior.
//
// [New] uses in-memory connections, which support testing/synctest.
// Build the server in the bubble. The bubble clock controls sleeps, timers,
// HTTP deadlines and session expiry. t.Run and socket-based brokers don't work there.
package datapagestest
