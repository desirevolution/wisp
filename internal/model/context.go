package model

// Quote is a snapshot, not a requirement that the original is stored locally.
type Quote struct {
	OriginalEdited bool   `json:"original_edited"`
	Deleted        bool   `json:"deleted"`
	MessageID      string `json:"message_id"`
	ChatID         string `json:"chat_id"`
	SenderID       string `json:"sender_id"`
	SenderName     string `json:"sender_name"`
	FromMe         bool   `json:"from_me"`
	Body           string `json:"body"`
	Type           string `json:"type"`
}

type Reaction struct {
	RawKind     string `json:"-"`
	ChatID      string `json:"chat_id"`
	MessageID   string `json:"message_id"`
	ActorID     string `json:"actor_id"`
	ActorName   string `json:"actor_name"`
	FromMe      bool   `json:"from_me"`
	Emoji       string `json:"emoji"`
	TimestampMS int64  `json:"timestamp_ms"`
	EventID     string `json:"-"`
	Raw         []byte `json:"-"`
}

type ReactionState struct {
	ChatID    string     `json:"chat_id"`
	MessageID string     `json:"message_id"`
	Reactions []Reaction `json:"reactions"`
	Version   int64      `json:"version"`
}
