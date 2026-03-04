package redis

import (
	"encoding/json"
	"time"
)

type Message struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	SenderID  string    `json:"sender_id"`
	Content   string    `json:"content"`
	Type      string    `json:"type"`
	FileURL   string    `json:"file_url,omitempty"`
	ReplyTo   string    `json:"reply_to,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// MarshalBinary - encode message structure
func (m *Message) MarshalBinary() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalBinary - decode message structure
func (m *Message) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, m)
}

// Presence - info about online status
type Presence struct {
	UserID   string    `json:"user_id"`
	Status   string    `json:"status"`
	LastSeen time.Time `json:"last_seen"`
}

// UnreadCount - unread messages
type UnreadCount struct {
	ChatID string `json:"chat_id"`
	Count  int64  `json:"count"`
}
