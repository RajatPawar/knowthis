package jobs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"knowthis/internal/metrics"
	"knowthis/internal/services"
	"knowthis/internal/storage"
)

// EmbeddingProcessor handles background processing of embeddings
type EmbeddingProcessor struct {
	store            storage.Store
	embeddingService *services.EmbeddingService
	batchSize        int
	interval         time.Duration
	done             chan struct{}
}

func NewEmbeddingProcessor(store storage.Store, embeddingService *services.EmbeddingService) *EmbeddingProcessor {
	return &EmbeddingProcessor{
		store:            store,
		embeddingService: embeddingService,
		batchSize:        10, // Reduced batch size for cost control
		interval:         60 * time.Second, // Increased interval to reduce API calls
		done:             make(chan struct{}),
	}
}

// Start begins the background processing of embeddings
func (e *EmbeddingProcessor) Start(ctx context.Context) {
	slog.Info("Starting embedding processor", 
		slog.Int("batch_size", e.batchSize),
		slog.Duration("interval", e.interval))

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Embedding processor stopped due to context cancellation")
			return
		case <-e.done:
			slog.Info("Embedding processor stopped")
			return
		case <-ticker.C:
			if err := e.processBatch(ctx); err != nil {
				slog.Error("Error processing embedding batch", "error", err)
			}
		}
	}
}

// Stop stops the background processing
func (e *EmbeddingProcessor) Stop() {
	close(e.done)
}

// processBatch processes a batch of documents without embeddings
func (e *EmbeddingProcessor) processBatch(ctx context.Context) error {
	start := time.Now()
	
	// Get documents without embeddings
	documents, err := e.store.GetDocumentsWithoutEmbeddings(ctx, e.batchSize)
	if err != nil {
		metrics.EmbeddingGenerations.WithLabelValues("error").Inc()
		return err
	}

	if len(documents) == 0 {
		slog.Debug("No documents found without embeddings")
		return nil
	}

	slog.Info("Processing embedding batch", 
		slog.Int("document_count", len(documents)))

	// Process each document
	successCount := 0
	for _, doc := range documents {
		if err := e.processDocument(ctx, doc); err != nil {
			slog.Error("Error processing document embedding", 
				slog.String("document_id", doc.ID),
				slog.String("error", err.Error()))
			metrics.EmbeddingGenerations.WithLabelValues("error").Inc()
			continue
		}
		successCount++
		metrics.EmbeddingGenerations.WithLabelValues("success").Inc()
	}

	duration := time.Since(start)
	metrics.EmbeddingGenerationDuration.Observe(duration.Seconds())
	
	slog.Info("Completed embedding batch", 
		slog.Int("processed", successCount),
		slog.Int("total", len(documents)),
		slog.Duration("duration", duration))

	return nil
}

// processDocument processes a single document's embedding
func (e *EmbeddingProcessor) processDocument(ctx context.Context, doc *storage.Document) error {
	start := time.Now()
	
	// Skip documents with empty content but mark them so they don't get processed again
	content := strings.TrimSpace(doc.Content)
	if content == "" {
		slog.Warn("Marking document with empty content", slog.String("document_id", doc.ID))
		// Create a placeholder embedding (single zero) to mark as processed
		emptyEmbedding := []float32{0.0}
		return e.store.UpdateEmbedding(ctx, doc.ID, emptyEmbedding)
	}
	
	// Skip very short content but mark them so they don't get processed again
	if len(content) < 10 {
		slog.Debug("Marking document with very short content", 
			slog.String("document_id", doc.ID),
			slog.String("content", content))
		// Create a placeholder embedding (single zero) to mark as processed
		emptyEmbedding := []float32{0.0}
		return e.store.UpdateEmbedding(ctx, doc.ID, emptyEmbedding)
	}
	
	// Generate embedding
	embedding, err := e.embeddingService.GenerateEmbedding(ctx, content)
	if err != nil {
		return err
	}

	// Update document with embedding
	if err := e.store.UpdateEmbedding(ctx, doc.ID, embedding); err != nil {
		return err
	}

	slog.Debug("Generated embedding for document", 
		slog.String("document_id", doc.ID),
		slog.Duration("duration", time.Since(start)))

	return nil
}

// GetStats returns statistics about embedding processing
func (e *EmbeddingProcessor) GetStats(ctx context.Context) (map[string]interface{}, error) {
	documentsWithoutEmbeddings, err := e.store.GetDocumentsWithoutEmbeddings(ctx, 1000)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"documents_without_embeddings": len(documentsWithoutEmbeddings),
		"batch_size":                  e.batchSize,
		"processing_interval":         e.interval.String(),
	}

	// Update metrics
	metrics.DocumentsWithoutEmbeddings.Set(float64(len(documentsWithoutEmbeddings)))

	return stats, nil
}

// SetBatchSize updates the batch size for processing
func (e *EmbeddingProcessor) SetBatchSize(size int) {
	if size > 0 && size <= 1000 {
		e.batchSize = size
		slog.Info("Updated embedding processor batch size", slog.Int("new_size", size))
	}
}

// SetInterval updates the processing interval
func (e *EmbeddingProcessor) SetInterval(interval time.Duration) {
	if interval >= 10*time.Second && interval <= 10*time.Minute {
		e.interval = interval
		slog.Info("Updated embedding processor interval", slog.Duration("new_interval", interval))
	}
}