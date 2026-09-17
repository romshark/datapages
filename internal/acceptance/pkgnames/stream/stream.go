// Package stream is named after github.com/romshark/datapages/runtime/stream,
// which app_gen.go imports. The generated server embeds auth.Manager[SessionData],
// which resolves to the wrong package unless the generated file
// imports this one under a name of its own.
package stream

// SessionData is what the application keeps in the session.
type SessionData struct {
	Nickname string `json:"nickname"`
}
