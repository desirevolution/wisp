package model

// MessageVersion is one received text/caption snapshot, not a reconstructed version.
type MessageVersion struct {
	Body        string   `json:"body"`
	Type        string   `json:"type"`
	TimestampMS int64    `json:"timestamp_ms"`
	Original    bool     `json:"original"`
	MentionJIDs []string `json:"-"`
}
type MessageEdit struct {
	ChatID      string
	MessageID   string
	TimestampMS int64
	Body        string
	Type        string
	MentionJIDs []string
	Raw         []byte
}
type EditState struct {
	ChatID            string           `json:"chat_id"`
	MessageID         string           `json:"message_id"`
	Body              string           `json:"body"`
	EditedAt          int64            `json:"edited_at"`
	Version           int64            `json:"edit_version"`
	History           []MessageVersion `json:"edit_history"`
	OriginalAvailable bool             `json:"original_available"`
	MentionJIDs       []string         `json:"-"`
	CurrentRaw        []byte           `json:"-"`
}
