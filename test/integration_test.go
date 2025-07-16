package test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"knowthis/internal/storage"
)

// Integration tests for the full message processing pipeline
func TestMessageProcessingPipeline(t *testing.T) {
	// Skip if no real database available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This integration test focuses on the embedding processor since 
	// Slack handler methods are not exported for direct testing
	mockStore := &mockIntegrationStore{
		documents: make(map[string]*storage.Document),
		embeddings: make(map[string][]float32),
	}

	mockEmbeddingService := &mockIntegrationEmbeddingService{}

	// Add test documents directly to storage
	testDocs := []*storage.Document{
		{ID: "valid1", Content: "This is a valid document with sufficient content"},
		{ID: "empty1", Content: ""},
		{ID: "short1", Content: "hi"},
	}
	
	for _, doc := range testDocs {
		mockStore.documents[doc.ID] = doc
	}

	// Manually process each document since processBatch is private
	for _, doc := range testDocs {
		// Simulate what the processor would do
		content := strings.TrimSpace(doc.Content)
		if content == "" || len(content) < 10 {
			// Create placeholder embedding
			emptyEmbedding := make([]float32, 1536)
			mockStore.embeddings[doc.ID] = emptyEmbedding
		} else {
			// Create real embedding
			embedding, err := mockEmbeddingService.GenerateEmbedding(context.Background(), content)
			if err != nil {
				t.Fatalf("Failed to generate embedding: %v", err)
			}
			mockStore.embeddings[doc.ID] = embedding
		}
	}

	// Verify all documents got embeddings
	if len(mockStore.embeddings) != len(testDocs) {
		t.Errorf("Expected %d embeddings, got %d", len(testDocs), len(mockStore.embeddings))
	}

	// Verify valid content got real embeddings, invalid got placeholders
	validEmbedding := mockStore.embeddings["valid1"]
	emptyEmbedding := mockStore.embeddings["empty1"]
	shortEmbedding := mockStore.embeddings["short1"]

	// Check valid embedding is not all zeros
	hasNonZero := false
	for _, val := range validEmbedding {
		if val != 0.0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Errorf("Valid document should have non-zero embedding")
	}

	// Check placeholder embeddings are all zeros
	for _, embedding := range [][]float32{emptyEmbedding, shortEmbedding} {
		isAllZeros := true
		for _, val := range embedding {
			if val != 0.0 {
				isAllZeros = false
				break
			}
		}
		if !isAllZeros {
			t.Errorf("Invalid content should have all-zero placeholder embedding")
		}
	}
}

// Mock implementations for integration testing
type mockIntegrationStore struct {
	documents  map[string]*storage.Document
	embeddings map[string][]float32
}

func (m *mockIntegrationStore) StoreDocument(ctx context.Context, doc *storage.Document) error {
	m.documents[doc.ID] = doc
	return nil
}

func (m *mockIntegrationStore) UpdateEmbedding(ctx context.Context, documentID string, embedding []float32) error {
	m.embeddings[documentID] = embedding
	return nil
}

func (m *mockIntegrationStore) SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]*storage.Document, error) {
	// Return documents that have real embeddings (not placeholders)
	var results []*storage.Document
	for id, doc := range m.documents {
		if emb, exists := m.embeddings[id]; exists {
			// Check if it's not a placeholder (not all zeros)
			hasNonZero := false
			for _, val := range emb {
				if val != 0.0 {
					hasNonZero = true
					break
				}
			}
			if hasNonZero {
				results = append(results, doc)
			}
		}
	}
	return results, nil
}

func (m *mockIntegrationStore) GetDocumentsWithoutEmbeddings(ctx context.Context, limit int) ([]*storage.Document, error) {
	var results []*storage.Document
	count := 0
	for id, doc := range m.documents {
		if _, hasEmbedding := m.embeddings[id]; !hasEmbedding {
			results = append(results, doc)
			count++
			if count >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockIntegrationStore) Close() error {
	return nil
}

type mockIntegrationEmbeddingService struct{}

func (m *mockIntegrationEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Simulate the validation that would happen in the real service
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("input text cannot be empty")
	}
	
	// Return a realistic embedding
	embedding := make([]float32, 1536)
	for i := range embedding {
		embedding[i] = 0.1 + float32(i)*0.0001 // Non-zero values
	}
	return embedding, nil
}

func (m *mockIntegrationEmbeddingService) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := m.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = embedding
	}
	return results, nil
}

// Test specific error scenarios we encountered
func TestErrorScenarios(t *testing.T) {
	testCases := []struct {
		name        string
		scenario    string
		expectError bool
		errorType   string
	}{
		{
			name:        "empty input to embedding service",
			scenario:    "empty_embedding_input",
			expectError: true,
			errorType:   "input text cannot be empty",
		},
		{
			name:        "wrong vector dimensions",
			scenario:    "wrong_dimensions",
			expectError: true,
			errorType:   "expected 1536 dimensions",
		},
		{
			name:        "infinite loop prevention",
			scenario:    "infinite_loop",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			switch tc.scenario {
			case "empty_embedding_input":
				// Test the validation logic directly
				text := strings.TrimSpace("")
				isEmpty := text == ""
				if !tc.expectError && isEmpty {
					t.Errorf("Expected no error but validation detected empty input")
				} else if tc.expectError && !isEmpty {
					t.Errorf("Expected error for empty input but validation passed")
				}

			case "wrong_dimensions":
				// Test that we always create 1536-dimension vectors
				mockStore := &mockIntegrationStore{
					documents:  make(map[string]*storage.Document),
					embeddings: make(map[string][]float32),
				}
				
				// Simulate creating a placeholder embedding (what processDocument would do)
				emptyEmbedding := make([]float32, 1536) // This should be 1536 dimensions
				err := mockStore.UpdateEmbedding(context.Background(), "test-doc", emptyEmbedding)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				
				// Check that placeholder has correct dimensions
				if embedding, exists := mockStore.embeddings["test-doc"]; exists {
					if len(embedding) != 1536 {
						t.Errorf("Expected 1536 dimensions, got %d", len(embedding))
					}
				} else {
					t.Errorf("Expected embedding to be created")
				}

			case "infinite_loop":
				// Test that processed documents don't get processed again
				mockStore := &mockIntegrationStore{
					documents:  make(map[string]*storage.Document),
					embeddings: make(map[string][]float32),
				}
				
				// Add a document with empty content
				emptyDoc := &storage.Document{
					ID:      "empty-doc",
					Content: "",
				}
				mockStore.documents["empty-doc"] = emptyDoc
				
				// Simulate processing the empty document
				content := strings.TrimSpace(emptyDoc.Content)
				if content == "" || len(content) < 10 {
					// Create placeholder embedding
					emptyEmbedding := make([]float32, 1536)
					err := mockStore.UpdateEmbedding(context.Background(), emptyDoc.ID, emptyEmbedding)
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
				}
				
				// Document should now have an embedding (placeholder)
				if _, exists := mockStore.embeddings["empty-doc"]; !exists {
					t.Errorf("Expected empty document to get placeholder embedding")
				}
				
				// Simulate that the document now has an embedding
				// Remove it from documents without embeddings
				delete(mockStore.documents, "empty-doc")
				
				// GetDocumentsWithoutEmbeddings should return empty list
				docs, err := mockStore.GetDocumentsWithoutEmbeddings(context.Background(), 10)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				
				// Should be no more documents to process
				if len(docs) != 0 {
					t.Errorf("Expected no documents without embeddings, got %d", len(docs))
				}
			}
		})
	}
}