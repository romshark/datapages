// Package datapages provides the core handler-parameter types shared by
// Datapages applications.
//
// Handler signatures reference only the standard library, templ, the
// application's own types and this package. They do not reference a third-party
// runtime or pluggable module. The generated server passes concrete
// implementations to handlers.
//
//   - [SSE] is the server-sent-event handle for action and event handlers.
//   - [PageCacheWriter] is the service worker cache handle for the current page.
package datapages
