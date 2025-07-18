package slack

import (
	"context"
	"log/slog"
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
		batchSize:        10, // Reduced batch size for cost control
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

// processBatch processes a batch of messages that need embeddings
func (e *EmbeddingProcessor) processBatch(ctx context.Context) error {
	// Get messages without embeddings
	messages, err := e.storage.GetMessagesWithoutEmbeddings(ctx, e.batchSize)
	if err != nil {
		return err
	}

	if len(messages) == 0 {
		slog.Debug("No messages found needing embeddings")
		return nil
	}

	slog.Info("Processing embedding batch", "count", len(messages))

	// Process each message
	for _, msg := range messages {
		if err := e.processMessage(ctx, msg); err != nil {
			slog.Error("Failed to process message embedding", 
				"error", err, 
				"message_id", msg.ID)
			continue
		}
	}

	return nil
}

// processMessage processes a single message for embedding generation
func (e *EmbeddingProcessor) processMessage(ctx context.Context, msg SlackMessage) error {
	// Validate content quality
	if !e.isQualityContent(msg.Content) {
		slog.Debug("Skipping low quality content", 
			"message_id", msg.ID,
			"content_length", len(msg.Content))
		
		// Create placeholder embedding (zeros) so we don't process this again
		placeholderEmbedding := make([]float32, 1536)
		return e.storage.StoreEmbedding(ctx, msg.ID, placeholderEmbedding)
	}

	// Generate embedding
	embedding, err := e.embeddingService.GenerateEmbedding(ctx, msg.Content)
	if err != nil {
		return err
	}

	// Store embedding
	if err := e.storage.StoreEmbedding(ctx, msg.ID, embedding); err != nil {
		return err
	}

	slog.Debug("Generated embedding for message", 
		"message_id", msg.ID,
		"content_length", len(msg.Content))

	return nil
}

// isQualityContent checks if content is worth generating an embedding for
func (e *EmbeddingProcessor) isQualityContent(content string) bool {
	// Skip empty content
	if strings.TrimSpace(content) == "" {
		return false
	}
	
	// Filter out test/placeholder content
	testPatterns := []string{
		"test", "testing", "some other content", 
		"this is my other message", "should also go in",
		"hello world", "sample", "example",
	}
	
	contentLower := strings.ToLower(content)
	for _, pattern := range testPatterns {
		if strings.Contains(contentLower, pattern) {
			return false
		}
	}
	
	// Require minimum meaningful length (after cleaning)
	if len(content) < 20 {
		return false
	}
	
	// Require some meaningful words
	words := strings.Fields(content)
	if len(words) < 4 {
		return false
	}
	
	return true
}

// GetStats returns processing statistics
func (e *EmbeddingProcessor) GetStats(ctx context.Context) (int, error) {
	messages, err := e.storage.GetMessagesWithoutEmbeddings(ctx, 1000)
	if err != nil {
		return 0, err
	}
	return len(messages), nil
}