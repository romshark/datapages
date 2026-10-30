// Package datapagestest tests a Datapages server over HTTP without
// a browser or network socket.
//
// Pass the handler returned by [github.com/romshark/datapages.NewServer] to [New]:
//
//	srv := datapagestest.New(t, s)
//	v := srv.Visitor(t)
//	tab := v.Load(t, href.PageIndex())
//	tab.Page().Get(t, datapagestest.Role("heading", "Todo List"))
//	st := tab.Connect(t, tab.Page().PublicSignals(t))
//	tab.Do(t, http.MethodPost, "/add/", map[string]any{"title": "Buy milk"}).
//		RequireStatus(t, http.StatusOK)
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
// loaded response, and actions send the signals supplied by the test. A hidden
// element is one the HTML hides with the hidden attribute, aria-hidden,
// display: none in its style attribute or a closed <dialog> around it. Datastar
// attributes such as data-show are not evaluated. Use a browser test for
// client-only behavior.
//
// [New] uses in-memory connections, which support testing/synctest.
// Build the server in the bubble. The bubble clock controls sleeps, timers,
// HTTP deadlines and session expiry. t.Run and socket-based brokers don't work there.
package datapagestest
