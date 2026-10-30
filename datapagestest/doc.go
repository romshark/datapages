// Package datapagestest tests a Datapages server over HTTP without
// a browser or network socket.
//
// Pass the handler returned by [github.com/romshark/datapages.NewServer] to [New]
// and use the page as a visitor does:
//
//	srv := datapagestest.New(t, s)
//	tab := srv.Visitor(t).Load(t, href.PageIndex())
//	tab.Connect(t)
//	tab.Fill(t, datapagestest.Label("Title"), "Buy milk")
//	tab.Click(t, datapagestest.Role("button", "Add"))
//	tab.WaitFor(t, datapagestest.Role("link", "Buy milk"))
//	tab.RequireTree(t, datapagestest.Role("list", "Todos"), datapagestest.Tree{
//		{Role: "listitem", Children: datapagestest.Tree{
//			{Role: "checkbox", Name: "Buy milk"},
//			{Role: "link", Name: "Buy milk", URL: href.PageItem(id)},
//		}},
//	})
//
// A [Visitor] shares cookies among its [Tab]s. A tab keeps its page as a browser would,
// which is the loaded document with the events of its streams and action responses
// applied, and the page's signals. [Tab.Click], [Tab.Fill], [Tab.Select] and
// [Tab.Submit] run the Datastar handlers of the markup, which sends the actions
// the page calls with the signals it holds. A test that names no URL thus also
// tests that the page calls the action.
//
// A [Query] selects elements as a visitor finds them, by [Role] and accessible
// name, by [Label] and by [HasText]. [ID], [Tag] and [Where] select what has no role.
// [Tab.RequireTree] compares the accessibility tree of a region with a [Tree].
//
// [Tab.Do], [Stream] and [Match] work on the wire, with a request carrying signals
// the test chooses and the events a stream carries. Use them for what no visitor can do,
// such as a request without its CSRF token, and to prove that an event didn't
// arrive with [Stream.Absent]. Each request and wait fails after [DefaultTimeout]
// unless [WithTimeout] changes it.
//
// Tabs run no JavaScript. They evaluate the expressions of data-show, data-text,
// data-attr, data-class and data-computed when they use signals, literals,
// comparisons, !, &&, ||, + and ?:, and run the handlers of data-on that call
// generated actions and assign signals. A query or a click that needs anything
// else fails and names the expression. Tabs apply no CSS, so a hidden element is
// one with the hidden attribute, aria-hidden, display: none in its style attribute,
// a false data-show or a closed <dialog>. A Web Component's role, and a dialog it
// opens, are not in the HTML. Use a browser test for such client-only behavior.
//
// [New] uses in-memory connections, which support testing/synctest.
// Build the server in the bubble. The bubble clock controls sleeps, timers,
// HTTP deadlines and session expiry. t.Run and socket-based brokers don't work there.
package datapagestest
