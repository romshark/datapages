// Package datapagestest tests a Datapages HTTP handler without a browser or network
// socket. It models visitors, tabs, Datastar actions, and SSE streams.
//
// Pass the handler returned by [github.com/romshark/datapages.NewServer] to [New]:
//
//	srv := datapagestest.New(t, s)
//	v := srv.Visitor(t)
//	tab := v.Load(t, href.PageIndex())
//	st := tab.Connect(t, tab.Page().Signals(t))
//	tab.Do(t, http.MethodPost, "/add/", map[string]any{"title": "Buy milk"})
//	st.WaitFor(t, datapagestest.TextContains("rows", "Buy milk"))
//
// A [Visitor] shares cookies among its [Tab]s. Each tab keeps its page response and
// Datapages instance. [Tab.Connect] opens its [Stream].
//
// The package does not run JavaScript. Use a browser test for client-only behavior.
package datapagestest
