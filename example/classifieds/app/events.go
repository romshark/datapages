package app

// EventMessagingSent is "messaging.sent"
type EventMessagingSent struct {
	SubjectUser []string `json:"-"`

	ChatID string `json:"chat-id"`
	UserID string `json:"user-id"`
}

// EventMessagingRead is "messaging.read"
type EventMessagingRead struct {
	SubjectUser []string `json:"-"`

	ChatID    string `json:"chat-id"`
	UserID    string `json:"user-id"`
	MessageID string `json:"message-id"`
}

// EventMessagingWriting is "messaging.writing"
type EventMessagingWriting struct {
	SubjectUser []string `json:"-"`

	ChatID string `json:"chat-id"`
	UserID string `json:"user-id"`
}

// EventMessagingWritingStopped is "messaging.writing-stopped"
type EventMessagingWritingStopped struct {
	SubjectUser []string `json:"-"`

	ChatID string `json:"chat-id"`
	UserID string `json:"user-id"`
}

// EventPostArchived is "posts.archived"
type EventPostArchived struct {
	PostID string `json:"post-id"`
}

// EventSessionClosed is "sessions.closed"
type EventSessionClosed struct {
	SubjectUser []string `json:"-"`

	Token string `json:"token"`
}
