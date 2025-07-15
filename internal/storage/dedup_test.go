package storage

import (
	"testing"
	"time"
)

func TestHashContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "simple content",
			content:  "hello world",
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "same content same hash",
			content:  "duplicate content",
			expected: HashContent("duplicate content"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HashContent(tt.content)
			if result != tt.expected {
				t.Errorf("HashContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDeduplication(t *testing.T) {
	content1 := "This is a test message"
	content2 := "This is a different message"
	content3 := "This is a test message" // duplicate of content1

	hash1 := HashContent(content1)
	hash2 := HashContent(content2)
	hash3 := HashContent(content3)

	// Same content should produce same hash
	if hash1 != hash3 {
		t.Errorf("Same content should produce same hash: %v != %v", hash1, hash3)
	}

	// Different content should produce different hash
	if hash1 == hash2 {
		t.Errorf("Different content should produce different hash: %v == %v", hash1, hash2)
	}
}

func TestDocumentDeduplication(t *testing.T) {
	doc1 := &Document{
		ID:          "test1",
		Content:     "Hello world",
		Source:      "slack",
		SourceID:    "msg1",
		UserID:      "user1",
		Timestamp:   time.Now(),
		ContentHash: HashContent("Hello world"),
	}

	doc2 := &Document{
		ID:          "test2",
		Content:     "Hello world", // same content
		Source:      "slack",
		SourceID:    "msg2", // different source ID
		UserID:      "user2",
		Timestamp:   time.Now(),
		ContentHash: HashContent("Hello world"),
	}

	doc3 := &Document{
		ID:          "test3",
		Content:     "Different content",
		Source:      "slack",
		SourceID:    "msg3",
		UserID:      "user1",
		Timestamp:   time.Now(),
		ContentHash: HashContent("Different content"),
	}

	// doc1 and doc2 should have same content hash (duplicate content)
	if doc1.ContentHash != doc2.ContentHash {
		t.Errorf("Documents with same content should have same hash: %v != %v", doc1.ContentHash, doc2.ContentHash)
	}

	// doc1 and doc3 should have different content hash
	if doc1.ContentHash == doc3.ContentHash {
		t.Errorf("Documents with different content should have different hash: %v == %v", doc1.ContentHash, doc3.ContentHash)
	}
}

func TestContentHashConsistency(t *testing.T) {
	content := "This is a test message for consistency"
	
	// Hash should be consistent across multiple calls
	hash1 := HashContent(content)
	hash2 := HashContent(content)
	hash3 := HashContent(content)

	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("Hash should be consistent: %v, %v, %v", hash1, hash2, hash3)
	}

	// Hash should be exactly 64 characters (SHA256 hex)
	if len(hash1) != 64 {
		t.Errorf("Hash length should be 64 characters, got %d", len(hash1))
	}
}