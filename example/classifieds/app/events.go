package app

import "github.com/romshark/datapages"

// EventMessagingSent is "messaging.sent"
type EventMessagingSent struct {
	Recipient datapages.SubjectUser
	ChatID    string `json:"chat-id"`
	UserID    string `json:"user-id"`
}

// EventMessagingRead is "messaging.read"
type EventMessagingRead struct {
	Recipient datapages.SubjectUser
	ChatID    string `json:"chat-id"`
	UserID    string `json:"user-id"`
	MessageID string `json:"message-id"`
}

// EventMessagingWriting is "messaging.writing"
type EventMessagingWriting struct {
	Recipient datapages.SubjectUser
	ChatID    string `json:"chat-id"`
	UserID    string `json:"user-id"`
}

// EventMessagingWritingStopped is "messaging.writing-stopped"
type EventMessagingWritingStopped struct {
	Recipient datapages.SubjectUser
	ChatID    string `json:"chat-id"`
	UserID    string `json:"user-id"`
}

// EventPostArchived is "posts.archived"
type EventPostArchived struct {
	PostID string `json:"post-id"`
}

// EventSessionClosed is "sessions.closed"
type EventSessionClosed struct {
	Recipient datapages.SubjectUser
	Token     string `json:"token"`
}
