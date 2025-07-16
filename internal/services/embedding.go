package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

type EmbeddingService struct {
	client *openai.Client
}

func NewEmbeddingService(apiKey string) *EmbeddingService {
	client := openai.NewClient(apiKey)
	return &EmbeddingService{client: client}
}

func (e *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Validate and clean input
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("input text cannot be empty")
	}

	// Truncate text if it exceeds token limit (approximate: 1 token ≈ 4 characters)
	const maxTokens = 8000
	const avgCharsPerToken = 4
	maxChars := maxTokens * avgCharsPerToken

	if len(text) > maxChars {
		text = text[:maxChars]
		// Try to cut at word boundary
		if lastSpace := strings.LastIndex(text[:maxChars], " "); lastSpace > maxChars-100 {
			text = text[:lastSpace]
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req := openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.AdaEmbeddingV2, // More cost-efficient than AdaV2
	}

	resp, err := e.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}

	return resp.Data[0].Embedding, nil
}

func (e *EmbeddingService) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	// Validate and clean input
	if len(texts) == 0 {
		return nil, fmt.Errorf("input texts array cannot be empty")
	}

	// Clean and validate each text
	cleanTexts := make([]string, 0, len(texts))
	const maxTokens = 8000
	const avgCharsPerToken = 4
	maxChars := maxTokens * avgCharsPerToken

	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text != "" {
			// Truncate if too long
			if len(text) > maxChars {
				text = text[:maxChars]
				// Try to cut at word boundary
				if lastSpace := strings.LastIndex(text[:maxChars], " "); lastSpace > maxChars-100 {
					text = text[:lastSpace]
				}
			}
			cleanTexts = append(cleanTexts, text)
		}
	}

	if len(cleanTexts) == 0 {
		return nil, fmt.Errorf("no valid non-empty texts found")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := openai.EmbeddingRequest{
		Input: cleanTexts,
		Model: openai.AdaEmbeddingV2, // More cost-efficient than AdaV2
	}

	resp, err := e.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	if len(resp.Data) != len(cleanTexts) {
		return nil, fmt.Errorf("embedding count mismatch: expected %d, got %d", len(cleanTexts), len(resp.Data))
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, data := range resp.Data {
		embeddings[i] = data.Embedding
	}

	return embeddings, nil
}
