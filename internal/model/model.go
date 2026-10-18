package model

import "time"

type Message struct {
	Silent                bool             `json:"silent,omitempty"`
	Edited                bool             `json:"edited"`
	EditedAt              int64            `json:"edited_at"`
	EditVersion           int64            `json:"edit_version"`
	EditHistory           []MessageVersion `json:"edit_history"`
	EditOriginalAvailable bool             `json:"edit_original_available"`
	CurrentRaw            []byte           `json:"-"`
	Deleted               bool             `json:"deleted"`
	OriginalInfo          []byte           `json:"-"`
	HistoryRaw            []byte           `json:"-"`
	ReactionVersion       int64            `json:"reaction_version"`
	Raw                   []byte           `json:"-"`
	OriginalChat          string           `json:"-"`
	OriginalSender        string           `json:"-"`
	Reply                 *Quote           `json:"reply,omitempty"`
	Reactions             []Reaction       `json:"reactions"`
	MentionJIDs           []string         `json:"-"`
	ID                    string           `json:"id"`
	WhatsAppID            string           `json:"whatsapp_id,omitempty"`
	ChatID                string           `json:"chat_id"`
	SenderID              *string          `json:"sender_id"`
	SenderName            *string          `json:"sender_name"`
	FromMe                int              `json:"from_me"`
	Type                  string           `json:"type"`
	Body                  string           `json:"body"`
	HasMedia              int              `json:"has_media"`
	MediaWidth            uint32           `json:"media_width,omitempty"`
	MediaHeight           uint32           `json:"media_height,omitempty"`
	Timestamp             int64            `json:"timestamp"`
}
type Status struct {
	State            string  `json:"state"`
	Account          *string `json:"account"`
	QR               *string `json:"qr"`
	Error            *string `json:"error"`
	ErrorCode        *string `json:"errorCode"`
	Syncing          bool    `json:"syncing,omitempty"`
	SyncPhase        string  `json:"syncPhase,omitempty"`
	InitialBootstrap bool    `json:"initialBootstrap,omitempty"`
}
type Event struct {
	Type string
	Data any
}
type Anchor struct {
	ChatID, MessageID string
	Timestamp         time.Time
}
