// Package datapagestest tests a Datapages server over HTTP without a browser or
// network socket.
//
// Pass the handler returned by [github.com/romshark/datapages.NewServer] to [New]:
//
//	srv := datapagestest.New(t, s)
//	v := srv.Visitor(t)
//	tab := v.Load(t, href.PageIndex())
//	st := tab.Connect(t, tab.Page().PublicSignals(t))
//	tab.Do(t, http.MethodPost, "/add/", map[string]any{"title": "Buy milk"}).
//		WantStatus(t, http.StatusOK)
//	st.WaitFor(t, datapagestest.TextContains("rows", "Buy milk"))
//
// A [Visitor] shares cookies among its [Tab]s. A tab keeps its page response,
// Datapages instance and CSRF token. [Tab.Connect] opens a [Stream], which retains
// events for [Stream.WaitFor] and [Stream.Absent]. Each request and wait fails after
// [DefaultTimeout] unless [WithTimeout] changes it.
//
// Tabs do not run JavaScript, apply patches, retry or reconnect. [Tab.Page] remains
// the loaded response, and actions send the signals supplied by the test. Use a
// browser test for client-only behavior.
//
// [New] uses in-memory connections, which support testing/synctest.
// Build the server in the bubble. The bubble clock controls sleeps, timers,
// HTTP deadlines and session expiry. t.Run and socket-based brokers do not work there.
package datapagestest
