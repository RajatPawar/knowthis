package slack

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	_ "github.com/lib/pq"
)

// SlackStorage handles Slack-specific database operations
type SlackStorage struct {
	db *sql.DB
}

// NewSlackStorage creates a new Slack storage instance
func NewSlackStorage(db *sql.DB) *SlackStorage {
	return &SlackStorage{db: db}
}

// InitSchema creates the Slack-specific tables
func (s *SlackStorage) InitSchema() error {
	slog.Info("Initializing Slack schema...")
	
	// Create slack_messages table
	createMessagesTable := `
		CREATE TABLE IF NOT EXISTS slack_messages (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			channel_id TEXT NOT NULL,
			thread_id TEXT NOT NULL,
			message_timestamp TEXT NOT NULL,
			user_id TEXT NOT NULL,
			user_name TEXT,
			content TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			client_msg_id TEXT,
			is_thread_root BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
	`
	if _, err := s.db.Exec(createMessagesTable); err != nil {
		return fmt.Errorf("failed to create slack_messages table: %w", err)
	}
	
	// Create slack_message_embeddings table
	createEmbeddingsTable := `
		CREATE TABLE IF NOT EXISTS slack_message_embeddings (
			message_id UUID PRIMARY KEY,
			embedding VECTOR(1536),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			FOREIGN KEY (message_id) REFERENCES slack_messages(id) ON DELETE CASCADE
		);
	`
	if _, err := s.db.Exec(createEmbeddingsTable); err != nil {
		return fmt.Errorf("failed to create slack_message_embeddings table: %w", err)
	}
	
	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_slack_channel_thread ON slack_messages(channel_id, thread_id);",
		"CREATE INDEX IF NOT EXISTS idx_slack_content_hash ON slack_messages(content_hash);",
		"CREATE INDEX IF NOT EXISTS idx_slack_timestamp ON slack_messages(message_timestamp);",
		"CREATE INDEX IF NOT EXISTS idx_slack_thread_root ON slack_messages(thread_id, is_thread_root);",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_slack_unique_message ON slack_messages(channel_id, message_timestamp);",
	}
	
	for _, indexSQL := range indexes {
		if _, err := s.db.Exec(indexSQL); err != nil {
			slog.Warn("Failed to create index", "error", err, "sql", indexSQL)
		}
	}
	
	slog.Info("Slack schema initialized successfully")
	return nil
}

// StoreMessage stores a Slack message, handling updates for edited messages
func (s *SlackStorage) StoreMessage(ctx context.Context, msg SlackMessage) (*SlackMessage, bool, error) {
	// Generate content hash
	msg.ContentHash = hashContent(msg.Content)
	
	query := `
		INSERT INTO slack_messages (
			channel_id, thread_id, message_timestamp, user_id, user_name,
			content, content_hash, client_msg_id, is_thread_root
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (channel_id, message_timestamp) 
		DO UPDATE SET
			content = EXCLUDED.content,
			content_hash = EXCLUDED.content_hash,
			updated_at = NOW()
		RETURNING id, created_at, updated_at, (xmax = 0) as was_inserted
	`
	
	var stored SlackMessage
	var wasInserted bool
	
	err := s.db.QueryRowContext(ctx, query,
		msg.ChannelID, msg.ThreadID, msg.MessageTimestamp, msg.UserID, msg.UserName,
		msg.Content, msg.ContentHash, msg.ClientMsgID, msg.IsThreadRoot,
	).Scan(&stored.ID, &stored.CreatedAt, &stored.UpdatedAt, &wasInserted)
	
	if err != nil {
		return nil, false, fmt.Errorf("failed to store message: %w", err)
	}
	
	// Copy the rest of the fields
	stored.ChannelID = msg.ChannelID
	stored.ThreadID = msg.ThreadID
	stored.MessageTimestamp = msg.MessageTimestamp
	stored.UserID = msg.UserID
	stored.UserName = msg.UserName
	stored.Content = msg.Content
	stored.ContentHash = msg.ContentHash
	stored.ClientMsgID = msg.ClientMsgID
	stored.IsThreadRoot = msg.IsThreadRoot
	
	// For now, we'll handle content changes by checking if it's an update
	// In a future version, we could add logic to detect content changes
	if !wasInserted {
		// This was an update, so invalidate embedding to be safe
		_, err = s.db.ExecContext(ctx, `
			DELETE FROM slack_message_embeddings 
			WHERE message_id = $1
		`, stored.ID)
		if err != nil {
			slog.Error("Failed to invalidate embedding for updated message", "error", err, "message_id", stored.ID)
		} else {
			slog.Debug("Message updated, embedding invalidated", "message_id", stored.ID)
		}
	}
	
	return &stored, wasInserted, nil
}

// GetThread retrieves all messages in a thread
func (s *SlackStorage) GetThread(ctx context.Context, threadID string) (*SlackThread, error) {
	query := `
		SELECT id, channel_id, thread_id, message_timestamp, user_id, user_name,
			   content, content_hash, client_msg_id, is_thread_root, created_at, updated_at
		FROM slack_messages 
		WHERE thread_id = $1 
		ORDER BY message_timestamp ASC
	`
	
	rows, err := s.db.QueryContext(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}
	defer rows.Close()
	
	var thread SlackThread
	var messages []SlackMessage
	
	for rows.Next() {
		var msg SlackMessage
		err := rows.Scan(
			&msg.ID, &msg.ChannelID, &msg.ThreadID, &msg.MessageTimestamp,
			&msg.UserID, &msg.UserName, &msg.Content, &msg.ContentHash,
			&msg.ClientMsgID, &msg.IsThreadRoot, &msg.CreatedAt, &msg.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}
	
	if len(messages) == 0 {
		return nil, nil
	}
	
	thread.ThreadID = threadID
	thread.ChannelID = messages[0].ChannelID
	thread.Messages = messages
	
	return &thread, nil
}

// GetThreadRoot retrieves the root message of a thread
func (s *SlackStorage) GetThreadRoot(ctx context.Context, threadID string) (*SlackMessage, error) {
	query := `
		SELECT id, channel_id, thread_id, message_timestamp, user_id, user_name,
			   content, content_hash, client_msg_id, is_thread_root, created_at, updated_at
		FROM slack_messages 
		WHERE thread_id = $1 AND is_thread_root = TRUE
		LIMIT 1
	`
	
	var msg SlackMessage
	err := s.db.QueryRowContext(ctx, query, threadID).Scan(
		&msg.ID, &msg.ChannelID, &msg.ThreadID, &msg.MessageTimestamp,
		&msg.UserID, &msg.UserName, &msg.Content, &msg.ContentHash,
		&msg.ClientMsgID, &msg.IsThreadRoot, &msg.CreatedAt, &msg.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get thread root: %w", err)
	}
	
	return &msg, nil
}

// GetMessagesWithoutEmbeddings retrieves messages that need embeddings
func (s *SlackStorage) GetMessagesWithoutEmbeddings(ctx context.Context, limit int) ([]SlackMessage, error) {
	query := `
		SELECT m.id, m.channel_id, m.thread_id, m.message_timestamp, m.user_id, m.user_name,
			   m.content, m.content_hash, m.client_msg_id, m.is_thread_root, m.created_at, m.updated_at
		FROM slack_messages m
		LEFT JOIN slack_message_embeddings e ON m.id = e.message_id
		WHERE e.message_id IS NULL
		ORDER BY m.created_at ASC
		LIMIT $1
	`
	
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages without embeddings: %w", err)
	}
	defer rows.Close()
	
	var messages []SlackMessage
	for rows.Next() {
		var msg SlackMessage
		err := rows.Scan(
			&msg.ID, &msg.ChannelID, &msg.ThreadID, &msg.MessageTimestamp,
			&msg.UserID, &msg.UserName, &msg.Content, &msg.ContentHash,
			&msg.ClientMsgID, &msg.IsThreadRoot, &msg.CreatedAt, &msg.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}
	
	return messages, nil
}

// StoreEmbedding stores an embedding for a message
func (s *SlackStorage) StoreEmbedding(ctx context.Context, messageID uuid.UUID, embedding []float32) error {
	query := `
		INSERT INTO slack_message_embeddings (message_id, embedding)
		VALUES ($1, $2)
		ON CONFLICT (message_id) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			created_at = NOW()
	`
	
	embeddingVector := pgvector.NewVector(embedding)
	_, err := s.db.ExecContext(ctx, query, messageID, embeddingVector)
	if err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}
	
	return nil
}

// SearchSimilarMessages searches for similar messages using embeddings
func (s *SlackStorage) SearchSimilarMessages(ctx context.Context, embedding []float32, limit int) ([]SlackMessage, error) {
	query := `
		SELECT m.id, m.channel_id, m.thread_id, m.message_timestamp, m.user_id, m.user_name,
			   m.content, m.content_hash, m.client_msg_id, m.is_thread_root, m.created_at, m.updated_at,
			   1 - (e.embedding <=> $1) as similarity
		FROM slack_messages m
		JOIN slack_message_embeddings e ON m.id = e.message_id
		WHERE e.embedding IS NOT NULL
		ORDER BY e.embedding <=> $1
		LIMIT $2
	`
	
	embeddingVector := pgvector.NewVector(embedding)
	rows, err := s.db.QueryContext(ctx, query, embeddingVector, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar messages: %w", err)
	}
	defer rows.Close()
	
	var messages []SlackMessage
	for rows.Next() {
		var msg SlackMessage
		var similarity float64
		
		err := rows.Scan(
			&msg.ID, &msg.ChannelID, &msg.ThreadID, &msg.MessageTimestamp,
			&msg.UserID, &msg.UserName, &msg.Content, &msg.ContentHash,
			&msg.ClientMsgID, &msg.IsThreadRoot, &msg.CreatedAt, &msg.UpdatedAt,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		
		messages = append(messages, msg)
	}
	
	return messages, nil
}

// hashContent generates a SHA256 hash of content
func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}