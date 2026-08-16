// Package datapages provides the core handler-parameter types shared by
// Datapages applications.
//
// Handler signatures reference only the standard library, templ, the
// application's own types, and this package — never a third-party runtime
// (datastar) or a pluggable module directly. The generated server constructs
// the concrete implementations and passes them in.
//
//   - [SSE] is the server-sent-event handle for action and event handlers.
//   - [PageCacheWriter] is the service-worker cache handle for the current page.
package datapages
