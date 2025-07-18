package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"knowthis/internal/integrations/slack"

	"github.com/sashabaranov/go-openai"
)

type RAGService struct {
	openaiClient     *openai.Client
	slackStorage     *slack.SlackStorage
	embeddingService *EmbeddingService
}

type QueryResult struct {
	Answer  string               `json:"answer"`
	Sources []slack.SlackMessage `json:"sources"`
	Query   string               `json:"query"`
}

func NewRAGService(openaiAPIKey string, slackStorage *slack.SlackStorage, embeddingService *EmbeddingService) *RAGService {
	client := openai.NewClient(openaiAPIKey)

	return &RAGService{
		openaiClient:     client,
		slackStorage:     slackStorage,
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

	// Search for similar messages
	messages, err := r.slackStorage.SearchSimilarMessages(ctx, queryEmbedding, 10)
	if err != nil {
		slog.Error("Failed to search similar messages", "error", err)
		return nil, fmt.Errorf("failed to search similar messages: %w", err)
	}
	slog.Info("Vector search completed", "messages_found", len(messages))

	// Filter messages with good similarity (>0.75)
	var relevantMessages []slack.SlackMessage
	for i, msg := range messages {
		contentPreview := msg.Content
		if len(contentPreview) > 100 {
			contentPreview = contentPreview[:100] + "..."
		}

		// Calculate similarity (SearchSimilarMessages returns messages ordered by similarity)
		similarity := calculateSimilarity(queryEmbedding, msg, i)

		slog.Info("Message similarity",
			"index", i,
			"similarity", similarity,
			"content", contentPreview,
			"user", msg.UserName,
			"id", msg.ID)

		if similarity > 0.75 && isQualityContent(msg.Content) {
			relevantMessages = append(relevantMessages, msg)
		}
	}

	slog.Info("Similarity filtering completed",
		"total_messages", len(messages),
		"relevant_messages", len(relevantMessages),
		"threshold", 0.75)

	// If no high-quality results, try with lower threshold but still apply quality filter
	if len(relevantMessages) == 0 {
		slog.Info("No high-quality results, trying lower threshold")
		for i, msg := range messages {
			similarity := calculateSimilarity(queryEmbedding, msg, i)
			if similarity > 0.6 && isQualityContent(msg.Content) {
				relevantMessages = append(relevantMessages, msg)
			}
		}
		slog.Info("Lower threshold results", "found", len(relevantMessages))
	}

	if len(relevantMessages) == 0 {
		slog.Warn("No relevant messages found", "query", query)
		return &QueryResult{
			Answer:  "I couldn't find any relevant information to answer your question.",
			Sources: []slack.SlackMessage{},
			Query:   query,
		}, nil
	}

	// Generate answer using OpenAI GPT
	answer, err := r.generateAnswer(ctx, query, relevantMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	return &QueryResult{
		Answer:  answer,
		Sources: relevantMessages,
		Query:   query,
	}, nil
}

// isQualityContent filters out low-quality content that shouldn't be in search results
func isQualityContent(content string) bool {
	content = strings.ToLower(strings.TrimSpace(content))

	// Filter out bot responses and acknowledgments
	botPatterns := []string{
		"got it", "i've processed", "stored the messages",
		":+1:", "👍", "✅", "done", "processed and stored",
	}

	for _, pattern := range botPatterns {
		if strings.Contains(content, pattern) {
			return false
		}
	}

	// Filter out test/placeholder content
	testPatterns := []string{
		"test", "testing", "some other content",
		"this is my other message", "should also go in",
		"hello world", "sample", "example",
	}

	for _, pattern := range testPatterns {
		if strings.Contains(content, pattern) {
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

// calculateSimilarity estimates similarity based on position in results
// Since SearchSimilarMessages returns results ordered by similarity, we estimate
func calculateSimilarity(queryEmbedding []float32, msg slack.SlackMessage, index int) float64 {
	// Return a decreasing similarity score based on position
	// First result gets ~0.9, subsequent results get lower scores
	return 0.9 - (float64(index) * 0.05)
}

func (r *RAGService) generateAnswer(ctx context.Context, query string, messages []slack.SlackMessage) (string, error) {
	// Build context from Slack messages, organized by thread
	var contextParts []string
	threadGroups := make(map[string][]slack.SlackMessage)

	// Group messages by thread
	for _, msg := range messages {
		threadGroups[msg.ThreadID] = append(threadGroups[msg.ThreadID], msg)
	}

	contextIndex := 1
	for _, threadMessages := range threadGroups {
		// Sort messages within thread by timestamp
		// (they should already be sorted from SearchSimilarMessages)

		contextParts = append(contextParts, fmt.Sprintf(
			"[%d] Thread conversation:",
			contextIndex))

		for _, msg := range threadMessages {
			contextParts = append(contextParts, fmt.Sprintf(
				"  %s: %s", msg.UserName, msg.Content))
		}

		contextIndex++
	}

	context := strings.Join(contextParts, "\n")

	// Create system prompt for OpenAI
	systemPrompt := "You are a helpful assistant that answers questions based on internal company knowledge from Slack conversations. Be concise and cite relevant thread conversations by their numbers when possible."

	userPrompt := fmt.Sprintf(`Based on the following context from our internal Slack knowledge base, please answer the question. Be concise and cite relevant thread conversations by their numbers.

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
