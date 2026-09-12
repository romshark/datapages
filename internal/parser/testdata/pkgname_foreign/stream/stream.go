// Package stream is named after github.com/romshark/datapages/runtime/stream,
// which app_gen.go imports. Its types reach app_gen.go as the session data and
// as an event declared outside the app package.
package stream

type Data struct {
	Name string
}

// EventTick is "tick"
type EventTick struct {
	N int `json:"n"`
}
