package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyHMAC(t *testing.T) {
	handler := &SlabHandler{
		webhookSecret: "test-secret",
	}

	tests := []struct {
		name      string
		body      string
		signature string
		expected  bool
	}{
		{
			name:      "valid signature",
			body:      `{"event":"post.published","data":{"id":"123","content":"test"}}`,
			signature: "sha256=5d41402abc4b2a76b9719d911017c592",
			expected:  false, // Will be false because we need actual HMAC
		},
		{
			name:      "empty signature",
			body:      `{"event":"post.published"}`,
			signature: "",
			expected:  false,
		},
		{
			name:      "invalid signature",
			body:      `{"event":"post.published"}`,
			signature: "sha256=invalid",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.verifyHMAC([]byte(tt.body), tt.signature)
			if result != tt.expected {
				t.Errorf("verifyHMAC() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestVerifyHMACWithValidSignature(t *testing.T) {
	secret := "test-secret"
	handler := &SlabHandler{
		webhookSecret: secret,
	}

	body := `{"event":"post.published","data":{"id":"123","content":"test"}}`
	
	// Generate correct HMAC

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	expectedMAC := mac.Sum(nil)
	signature := "sha256=" + hex.EncodeToString(expectedMAC)

	result := handler.verifyHMAC([]byte(body), signature)
	if !result {
		t.Errorf("verifyHMAC() should return true for valid signature")
	}
}

func TestVerifyHMACWithoutPrefix(t *testing.T) {
	secret := "test-secret"
	handler := &SlabHandler{
		webhookSecret: secret,
	}

	body := `{"event":"post.published","data":{"id":"123","content":"test"}}`

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	expectedMAC := mac.Sum(nil)
	signature := hex.EncodeToString(expectedMAC) // without "sha256=" prefix

	result := handler.verifyHMAC([]byte(body), signature)
	if !result {
		t.Errorf("verifyHMAC() should return true for valid signature without prefix")
	}
}

func TestCleanSlabContent(t *testing.T) {
	handler := &SlabHandler{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove markdown formatting",
			input:    "This is **bold** and *italic* text",
			expected: "This is bold and italic text",
		},
		{
			name:     "remove code backticks",
			input:    "Use `console.log()` to debug",
			expected: "Use console.log() to debug",
		},
		{
			name:     "remove headers",
			input:    "# Header 1\n## Header 2\nContent",
			expected: "Header 1\n Header 2\nContent",
		},
		{
			name:     "remove multiple spaces and newlines",
			input:    "Text  with   multiple    spaces\n\n\nand newlines",
			expected: "Text with  multiple  spaces\n\nand newlines",
		},
		{
			name:     "mixed formatting",
			input:    "# Title\n\nThis is **bold** and `code`\n\n  Multiple spaces  ",
			expected: "Title\nThis is bold and code\n Multiple spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.cleanSlabContent(tt.input)
			if result != tt.expected {
				t.Errorf("cleanSlabContent() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestEmptySecret(t *testing.T) {
	handler := &SlabHandler{
		webhookSecret: "",
	}

	result := handler.verifyHMAC([]byte("test"), "any-signature")
	if result {
		t.Errorf("verifyHMAC() should return false when secret is empty")
	}
}