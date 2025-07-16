package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"knowthis/internal/storage"
	
	"github.com/sashabaranov/go-openai"
)

type RAGService struct {
	openaiClient     *openai.Client
	store            storage.Store
	embeddingService *EmbeddingService
}

type QueryResult struct {
	Answer    string                `json:"answer"`
	Sources   []*storage.Document   `json:"sources"`
	Query     string                `json:"query"`
}

func NewRAGService(openaiAPIKey string, store storage.Store, embeddingService *EmbeddingService) *RAGService {
	client := openai.NewClient(openaiAPIKey)
	
	return &RAGService{
		openaiClient:     client,
		store:            store,
		embeddingService: embeddingService,
	}
}

func (r *RAGService) Query(ctx context.Context, query string) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	slog.Info("RAG Query started", "query", query)

	// Generate embedding for the query
	queryEmbedding, err := r.embeddingService.GenerateEmbedding(ctx, query)
	if err != nil {
		slog.Error("Failed to generate query embedding", "error", err)
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}
	slog.Info("Query embedding generated", "embedding_length", len(queryEmbedding))

	// Search for similar documents
	documents, err := r.store.SearchSimilar(ctx, queryEmbedding, 10)
	if err != nil {
		slog.Error("Failed to search similar documents", "error", err)
		return nil, fmt.Errorf("failed to search similar documents: %w", err)
	}
	slog.Info("Vector search completed", "documents_found", len(documents))

	// Filter documents with good similarity (>0.5)
	var relevantDocs []*storage.Document
	for i, doc := range documents {
		contentPreview := doc.Content
		if len(contentPreview) > 100 {
			contentPreview = contentPreview[:100] + "..."
		}
		slog.Info("Document similarity", 
			"index", i,
			"similarity", doc.Similarity, 
			"content", contentPreview,
			"source", doc.Source,
			"id", doc.ID)
		
		if doc.Similarity > 0.5 {
			relevantDocs = append(relevantDocs, doc)
		}
	}

	slog.Info("Similarity filtering completed", 
		"total_documents", len(documents),
		"relevant_documents", len(relevantDocs),
		"threshold", 0.5)

	if len(relevantDocs) == 0 {
		slog.Warn("No relevant documents found", "query", query)
		return &QueryResult{
			Answer:  "I couldn't find any relevant information to answer your question.",
			Sources: []*storage.Document{},
			Query:   query,
		}, nil
	}

	// Generate answer using OpenAI GPT
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
	
	// Create system prompt for OpenAI
	systemPrompt := "You are a helpful assistant that answers questions based on internal company knowledge. Be concise and cite relevant sources by their numbers when possible."
	
	userPrompt := fmt.Sprintf(`Based on the following context from our internal knowledge base, please answer the question. Be concise and cite relevant sources by their numbers.

Context:
%s

Question: %s`, context, query)

	return r.callOpenAIAPI(ctx, systemPrompt, userPrompt)
}

func (r *RAGService) callOpenAIAPI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := r.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     "gpt-4o-mini",
		MaxTokens: 1000,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userPrompt,
			},
		},
		Temperature: 0.7,
	})
	
	if err != nil {
		slog.Error("Failed to call OpenAI API", "error", err)
		return "", fmt.Errorf("failed to call OpenAI API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "I couldn't generate a response. Please try again.", nil
	}

	return resp.Choices[0].Message.Content, nil
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