package slack

import (
	"time"

	"github.com/google/uuid"
)

// SlackMessage represents a Slack message stored in our database
type SlackMessage struct {
	ID               uuid.UUID `json:"id"`
	ChannelID        string    `json:"channel_id"`
	ThreadID         string    `json:"thread_id"`
	MessageTimestamp string    `json:"message_timestamp"`
	UserID           string    `json:"user_id"`
	UserName         string    `json:"user_name"`
	Content          string    `json:"content"`
	ContentHash      string    `json:"content_hash"`
	ClientMsgID      string    `json:"client_msg_id"`
	IsThreadRoot     bool      `json:"is_thread_root"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// SlackThread represents a complete thread with all messages
type SlackThread struct {
	ThreadID  string         `json:"thread_id"`
	ChannelID string         `json:"channel_id"`
	Messages  []SlackMessage `json:"messages"`
}

// SlackMessageEmbedding represents the embedding for a Slack message
type SlackMessageEmbedding struct {
	MessageID uuid.UUID `json:"message_id"`
	Embedding []float32 `json:"embedding"`
	CreatedAt time.Time `json:"created_at"`
}