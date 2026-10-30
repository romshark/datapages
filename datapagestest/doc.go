// Package datapagestest tests a Datapages application through its HTTP handler. It sends
// the requests and headers that Datastar sends, parses page HTML and SSE events, and
// keeps cookies and per-tab Datapages instances.
//
// Build the handler with [github.com/romshark/datapages.NewServer] in the app's external
// test package. Use the server command's type arguments and options, then pass the
// handler to [New]:
//
//	srv := datapagestest.New(t, s)
//	v := srv.Visitor(t)
//	tab := v.Load(t, href.PageIndex())
//	st := tab.Connect(t, tab.Page().Signals(t))
//	tab.Do(t, http.MethodPost, "/add/", map[string]any{"title": "Buy milk"})
//	st.WaitFor(t, datapagestest.TextContains("rows", "Buy milk"))
//
// A [Visitor] owns one cookie jar shared by its tabs. A [Tab] holds one page response
// and sends actions with its Datapages-Instance. [Tab.Connect] opens a [Stream],
// which reads SSE events in the background.
//
// Requests default to Accept-Encoding: identity, which prevents response compression.
// [Visitor.Send] follows no redirects. [Tab.Connect] follows only the 303 from a page
// stream to its anonymous stream. [Response.Redirect] returns the target from an HTTP
// redirect or a redirect script in a Datastar response.
//
// [New] runs the net/http server on in-memory connections instead of a socket. A test
// can therefore run inside a testing/synctest bubble.
//
// Actions for a signed-in visitor send X-CSRF-Token derived from the session cookie.
// [WithSessionCookie] and [WithCSRFTokens] configure a non-default cookie name or
// token writer.
//
// The package doesn't run JavaScript. Use a browser test for client-side only behavior.
package datapagestest
