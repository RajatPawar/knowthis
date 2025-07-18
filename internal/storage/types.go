package storage

import (
	"context"
	"time"
)

type Document struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	Source      string    `json:"source"`      // "slack" or "slab"
	SourceID    string    `json:"source_id"`   // Original ID from source (thread_ts for Slack threads)
	Title       string    `json:"title,omitempty"`
	ChannelID   string    `json:"channel_id,omitempty"`  // For Slack
	PostID      string    `json:"post_id,omitempty"`     // For Slab comments
	UserID      string    `json:"user_id"`
	UserName    string    `json:"user_name,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	ContentHash string    `json:"content_hash"`
	Embedding   []float32 `json:"embedding,omitempty"`
	Similarity  float64   `json:"similarity,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store interface {
	StoreDocument(ctx context.Context, doc *Document) error
	UpdateEmbedding(ctx context.Context, documentID string, embedding []float32) error
	SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]*Document, error)
	GetDocumentsWithoutEmbeddings(ctx context.Context, limit int) ([]*Document, error)
	Close() error
}