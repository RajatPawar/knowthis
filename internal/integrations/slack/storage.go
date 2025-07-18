package slack

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
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

	// Create slack_thread_embeddings table
	createEmbeddingsTable := `
		CREATE TABLE IF NOT EXISTS slack_thread_embeddings (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			thread_id TEXT NOT NULL,
			chunk_index INTEGER NOT NULL DEFAULT 0,
			content_hash TEXT NOT NULL,
			embedding VECTOR(1536),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			UNIQUE(thread_id, chunk_index)
		);
	`
	if _, err := s.db.Exec(createEmbeddingsTable); err != nil {
		return fmt.Errorf("failed to create slack_thread_embeddings table: %w", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_slack_channel_thread ON slack_messages(channel_id, thread_id);",
		"CREATE INDEX IF NOT EXISTS idx_slack_content_hash ON slack_messages(content_hash);",
		"CREATE INDEX IF NOT EXISTS idx_slack_timestamp ON slack_messages(message_timestamp);",
		"CREATE INDEX IF NOT EXISTS idx_slack_thread_root ON slack_messages(thread_id, is_thread_root);",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_slack_unique_message ON slack_messages(channel_id, message_timestamp);",
		"CREATE INDEX IF NOT EXISTS idx_slack_thread_embeddings_thread ON slack_thread_embeddings(thread_id);",
		"CREATE INDEX IF NOT EXISTS idx_slack_thread_embeddings_hash ON slack_thread_embeddings(content_hash);",
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
		// This was an update, so invalidate thread embeddings to be safe
		_, err = s.db.ExecContext(ctx, `
			DELETE FROM slack_thread_embeddings
			WHERE thread_id = $1
		`, stored.ThreadID)
		if err != nil {
			slog.Error("Failed to invalidate thread embeddings for updated message", "error", err, "thread_id", stored.ThreadID)
		} else {
			slog.Debug("Message updated, thread embeddings invalidated", "thread_id", stored.ThreadID)
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

// GetThreadsWithoutEmbeddings retrieves threads that need embeddings
func (s *SlackStorage) GetThreadsWithoutEmbeddings(ctx context.Context, limit int) ([]string, error) {
	query := `
		SELECT DISTINCT m.thread_id
		FROM slack_messages m
		LEFT JOIN slack_thread_embeddings e ON m.thread_id = e.thread_id
		WHERE e.thread_id IS NULL
		ORDER BY MIN(m.created_at) ASC
		LIMIT $1
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get threads without embeddings: %w", err)
	}
	defer rows.Close()

	var threadIDs []string
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			return nil, fmt.Errorf("failed to scan thread ID: %w", err)
		}
		threadIDs = append(threadIDs, threadID)
	}

	return threadIDs, nil
}

// GetMessagesInThread retrieves all messages in a specific thread
func (s *SlackStorage) GetMessagesInThread(ctx context.Context, threadID string) ([]SlackMessage, error) {
	query := `
		SELECT id, channel_id, thread_id, message_timestamp, user_id, user_name,
			   content, content_hash, client_msg_id, is_thread_root, created_at, updated_at
		FROM slack_messages
		WHERE thread_id = $1
		ORDER BY message_timestamp ASC
	`

	rows, err := s.db.QueryContext(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages in thread: %w", err)
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

// StoreThreadEmbedding stores an embedding for a thread chunk
func (s *SlackStorage) StoreThreadEmbedding(ctx context.Context, threadID string, chunkIndex int, contentHash string, embedding []float32) error {
	query := `
		INSERT INTO slack_thread_embeddings (thread_id, chunk_index, content_hash, embedding)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (thread_id, chunk_index) DO UPDATE SET
			content_hash = EXCLUDED.content_hash,
			embedding = EXCLUDED.embedding,
			created_at = NOW()
	`

	embeddingVector := pgvector.NewVector(embedding)
	_, err := s.db.ExecContext(ctx, query, threadID, chunkIndex, contentHash, embeddingVector)
	if err != nil {
		return fmt.Errorf("failed to store thread embedding: %w", err)
	}

	return nil
}

// SearchSimilarMessages searches for similar messages using thread embeddings
func (s *SlackStorage) SearchSimilarMessages(ctx context.Context, embedding []float32, limit int) ([]SlackMessage, error) {
	// First, find similar threads using embeddings
	threadQuery := `
		SELECT e.thread_id, 1 - (e.embedding <=> $1) as similarity
		FROM slack_thread_embeddings e
		WHERE e.embedding IS NOT NULL
		ORDER BY e.embedding <=> $1
		LIMIT $2
	`

	embeddingVector := pgvector.NewVector(embedding)
	rows, err := s.db.QueryContext(ctx, threadQuery, embeddingVector, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar threads: %w", err)
	}
	defer rows.Close()

	var threadIDs []string
	for rows.Next() {
		var threadID string
		var similarity float64

		if err := rows.Scan(&threadID, &similarity); err != nil {
			return nil, fmt.Errorf("failed to scan thread result: %w", err)
		}
		threadIDs = append(threadIDs, threadID)
	}

	if len(threadIDs) == 0 {
		return []SlackMessage{}, nil
	}

	// Now get all messages from these threads
	placeholders := make([]string, len(threadIDs))
	args := make([]interface{}, len(threadIDs))
	for i, threadID := range threadIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = threadID
	}

	messageQuery := fmt.Sprintf(`
		SELECT id, channel_id, thread_id, message_timestamp, user_id, user_name,
			   content, content_hash, client_msg_id, is_thread_root, created_at, updated_at
		FROM slack_messages
		WHERE thread_id IN (%s)
		ORDER BY thread_id, message_timestamp ASC
	`, strings.Join(placeholders, ","))

	messageRows, err := s.db.QueryContext(ctx, messageQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages from similar threads: %w", err)
	}
	defer messageRows.Close()

	var messages []SlackMessage
	for messageRows.Next() {
		var msg SlackMessage

		err := messageRows.Scan(
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

// hashContent generates a SHA256 hash of content
func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}
