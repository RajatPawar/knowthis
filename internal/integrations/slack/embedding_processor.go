package slack

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// EmbeddingServiceInterface to avoid circular dependencies
type EmbeddingServiceInterface interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
}

// EmbeddingProcessor handles background processing of embeddings for Slack messages
type EmbeddingProcessor struct {
	storage          *SlackStorage
	embeddingService EmbeddingServiceInterface
	batchSize        int
	interval         time.Duration
	done             chan struct{}
}

// NewEmbeddingProcessor creates a new embedding processor for Slack
func NewEmbeddingProcessor(storage *SlackStorage, embeddingService EmbeddingServiceInterface) *EmbeddingProcessor {
	return &EmbeddingProcessor{
		storage:          storage,
		embeddingService: embeddingService,
		batchSize:        10,               // Reduced batch size for cost control
		interval:         60 * time.Second, // Increased interval to reduce API calls
		done:             make(chan struct{}),
	}
}

// Start begins the background processing of embeddings
func (e *EmbeddingProcessor) Start(ctx context.Context) {
	slog.Info("Starting Slack embedding processor",
		"batch_size", e.batchSize,
		"interval", e.interval)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Slack embedding processor stopped due to context cancellation")
			return
		case <-e.done:
			slog.Info("Slack embedding processor stopped")
			return
		case <-ticker.C:
			if err := e.processBatch(ctx); err != nil {
				slog.Error("Failed to process embedding batch", "error", err)
			}
		}
	}
}

// Stop stops the embedding processor
func (e *EmbeddingProcessor) Stop() {
	close(e.done)
}

// processBatch processes a batch of threads that need embeddings
func (e *EmbeddingProcessor) processBatch(ctx context.Context) error {
	// Get threads without embeddings
	threadIDs, err := e.storage.GetThreadsWithoutEmbeddings(ctx, e.batchSize)
	if err != nil {
		return err
	}

	if len(threadIDs) == 0 {
		slog.Debug("No threads found needing embeddings")
		return nil
	}

	slog.Info("Processing embedding batch", "count", len(threadIDs))

	// Process each thread
	for _, threadID := range threadIDs {
		if err := e.processThread(ctx, threadID); err != nil {
			slog.Error("Failed to process thread embedding",
				"error", err,
				"thread_id", threadID)
			continue
		}
	}

	return nil
}

// processThread processes a single thread for embedding generation
func (e *EmbeddingProcessor) processThread(ctx context.Context, threadID string) error {
	// Get all messages in the thread
	messages, err := e.storage.GetMessagesInThread(ctx, threadID)
	if err != nil {
		return fmt.Errorf("failed to get messages in thread: %w", err)
	}

	if len(messages) == 0 {
		slog.Debug("No messages found in thread", "thread_id", threadID)
		return nil
	}

	// Build thread content with human-readable timestamps
	threadContent := e.buildThreadContent(messages)

	// Validate content quality
	if !e.isQualityContent(threadContent) {
		slog.Debug("Skipping low quality thread content",
			"thread_id", threadID,
			"content_length", len(threadContent))
		return nil
	}

	// Chunk the content if needed (7K words max per chunk)
	chunks := e.chunkContent(threadContent)

	// Process each chunk
	for chunkIndex, chunk := range chunks {
		contentHash := e.hashContent(chunk)

		// Generate embedding
		embedding, err := e.embeddingService.GenerateEmbedding(ctx, chunk)
		if err != nil {
			return fmt.Errorf("failed to generate embedding for chunk %d: %w", chunkIndex, err)
		}

		// Store thread embedding
		if err := e.storage.StoreThreadEmbedding(ctx, threadID, chunkIndex, contentHash, embedding); err != nil {
			return fmt.Errorf("failed to store thread embedding for chunk %d: %w", chunkIndex, err)
		}

		slog.Debug("Generated embedding for thread chunk",
			"thread_id", threadID,
			"chunk_index", chunkIndex,
			"content_length", len(chunk))
	}

	return nil
}

// buildThreadContent builds formatted thread content with human-readable timestamps
func (e *EmbeddingProcessor) buildThreadContent(messages []SlackMessage) string {
	var parts []string

	for _, msg := range messages {
		// Convert timestamp to human-readable format
		timestamp := e.formatTimestamp(msg.MessageTimestamp)

		// Format: [December 15, 2024, 3:45PM] Username: Content
		formattedMsg := fmt.Sprintf("[%s] %s: %s", timestamp, msg.UserName, msg.Content)
		parts = append(parts, formattedMsg)
	}

	return strings.Join(parts, "\n")
}

// chunkContent splits content into chunks of approximately 7K words
func (e *EmbeddingProcessor) chunkContent(content string) []string {
	words := strings.Fields(content)
	maxWordsPerChunk := 7000

	if len(words) <= maxWordsPerChunk {
		return []string{content}
	}

	var chunks []string

	for i := 0; i < len(words); i += maxWordsPerChunk {
		end := i + maxWordsPerChunk
		if end > len(words) {
			end = len(words)
		}

		chunk := strings.Join(words[i:end], " ")
		chunks = append(chunks, chunk)
	}

	return chunks
}

// formatTimestamp converts Slack timestamp to human-readable format
func (e *EmbeddingProcessor) formatTimestamp(slackTimestamp string) string {
	// Parse Slack timestamp (Unix timestamp with decimal)
	parts := strings.Split(slackTimestamp, ".")
	if len(parts) == 0 {
		return slackTimestamp // fallback
	}

	unixSeconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return slackTimestamp // fallback
	}

	t := time.Unix(unixSeconds, 0)
	return t.Format("January 2, 2006, 3:04PM")
}

// hashContent generates a SHA256 hash of content
func (e *EmbeddingProcessor) hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// isQualityContent checks if content is worth generating an embedding for
func (e *EmbeddingProcessor) isQualityContent(content string) bool {
	// Skip empty content
	if strings.TrimSpace(content) == "" {
		return false
	}

	// Filter out obvious test/placeholder content (less strict)
	testPatterns := []string{
		"hello world", "sample", "example", "placeholder",
		"lorem ipsum", "dummy text", "test message",
	}

	contentLower := strings.ToLower(content)
	for _, pattern := range testPatterns {
		if strings.Contains(contentLower, pattern) {
			return false
		}
	}

	// Require minimum meaningful length (after cleaning) - reduced threshold
	if len(content) < 5 {
		return false
	}

	// Require some meaningful words - reduced threshold
	words := strings.Fields(content)
	if len(words) < 1 {
		return false
	}

	return true
}

// GetStats returns processing statistics
func (e *EmbeddingProcessor) GetStats(ctx context.Context) (int, error) {
	threadIDs, err := e.storage.GetThreadsWithoutEmbeddings(ctx, 1000)
	if err != nil {
		return 0, err
	}
	return len(threadIDs), nil
}
