// Package deep declares the events the app package reaches through aliases.
package deep

// EventDeep is "deep"
type EventDeep struct {
	Text string `json:"text"`
}

// EventOther is "other"
type EventOther struct {
	Text string `json:"text"`
}
