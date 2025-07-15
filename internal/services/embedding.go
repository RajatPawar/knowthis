package services

import (
	"context"
	"fmt"
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req := openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.AdaEmbeddingV2,
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := openai.EmbeddingRequest{
		Input: texts,
		Model: openai.AdaEmbeddingV2,
	}

	resp, err := e.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: expected %d, got %d", len(texts), len(resp.Data))
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, data := range resp.Data {
		embeddings[i] = data.Embedding
	}

	return embeddings, nil
}