package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"knowthis/internal/storage"
	
	"github.com/anthropic-ai/anthropic-sdk-go"
	"github.com/anthropic-ai/anthropic-sdk-go/option"
)

type RAGService struct {
	anthropicClient  *anthropic.Client
	store            storage.Store
	embeddingService *EmbeddingService
}

type QueryResult struct {
	Answer    string                `json:"answer"`
	Sources   []*storage.Document   `json:"sources"`
	Query     string                `json:"query"`
}

func NewRAGService(anthropicAPIKey string, store storage.Store, embeddingService *EmbeddingService) *RAGService {
	client := anthropic.NewClient(
		option.WithAPIKey(anthropicAPIKey),
	)
	
	return &RAGService{
		anthropicClient:  client,
		store:            store,
		embeddingService: embeddingService,
	}
}

func (r *RAGService) Query(ctx context.Context, query string) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Generate embedding for the query
	queryEmbedding, err := r.embeddingService.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Search for similar documents
	documents, err := r.store.SearchSimilar(ctx, queryEmbedding, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar documents: %w", err)
	}

	// Filter documents with good similarity (>0.7)
	var relevantDocs []*storage.Document
	for _, doc := range documents {
		if doc.Similarity > 0.7 {
			relevantDocs = append(relevantDocs, doc)
		}
	}

	if len(relevantDocs) == 0 {
		return &QueryResult{
			Answer:  "I couldn't find any relevant information to answer your question.",
			Sources: []*storage.Document{},
			Query:   query,
		}, nil
	}

	// Generate answer using Claude
	answer, err := r.generateAnswer(ctx, query, relevantDocs)
	if err != nil {
		return nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	return &QueryResult{
		Answer:  answer,
		Sources: relevantDocs,
		Query:   query,
	}, nil
}

func (r *RAGService) generateAnswer(ctx context.Context, query string, documents []*storage.Document) (string, error) {
	// Build context from documents
	var contextParts []string
	for i, doc := range documents {
		var source string
		if doc.Source == "slack" {
			source = fmt.Sprintf("Slack message from %s", doc.UserName)
		} else {
			source = fmt.Sprintf("Slab %s from %s", getSlabType(doc), doc.UserName)
		}
		
		contextParts = append(contextParts, fmt.Sprintf(
			"[%d] %s (%.2f relevance):\n%s",
			i+1, source, doc.Similarity, doc.Content,
		))
	}
	
	context := strings.Join(contextParts, "\n\n")
	
	// Simple prompt for now - you can enhance this with actual Anthropic API
	prompt := fmt.Sprintf(`Based on the following context from our internal knowledge base, please answer the question. Be concise and cite relevant sources by their numbers.

Context:
%s

Question: %s

Answer:`, context, query)

	// For now, return a simple response. In production, you'd use Anthropic's API
	return r.callAnthropicAPI(ctx, prompt)
}

func (r *RAGService) callAnthropicAPI(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	message, err := r.anthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaude3Haiku20240307),
		MaxTokens: anthropic.F(int64(1000)),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		}),
		System: anthropic.F("You are a helpful assistant that answers questions based on internal company knowledge. Be concise and cite sources when possible."),
	})
	
	if err != nil {
		slog.Error("Failed to call Anthropic API", "error", err)
		return "", fmt.Errorf("failed to call Anthropic API: %w", err)
	}

	if len(message.Content) == 0 {
		return "I couldn't generate a response. Please try again.", nil
	}

	// Extract text content from the response
	var response strings.Builder
	for _, content := range message.Content {
		if textBlock, ok := content.AsTextBlock(); ok {
			response.WriteString(textBlock.Text)
		}
	}

	return response.String(), nil
}

func getSlabType(doc *storage.Document) string {
	if doc.PostID != "" {
		return "comment"
	}
	return "post"
}

// ProcessPendingEmbeddings processes documents that don't have embeddings yet
func (r *RAGService) ProcessPendingEmbeddings(ctx context.Context) error {
	documents, err := r.store.GetDocumentsWithoutEmbeddings(ctx, 50)
	if err != nil {
		return fmt.Errorf("failed to get documents without embeddings: %w", err)
	}

	for _, doc := range documents {
		embedding, err := r.embeddingService.GenerateEmbedding(ctx, doc.Content)
		if err != nil {
			return fmt.Errorf("failed to generate embedding for document %s: %w", doc.ID, err)
		}

		if err := r.store.UpdateEmbedding(ctx, doc.ID, embedding); err != nil {
			return fmt.Errorf("failed to update embedding for document %s: %w", doc.ID, err)
		}
	}

	return nil
}