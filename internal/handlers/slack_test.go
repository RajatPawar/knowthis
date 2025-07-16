package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"knowthis/internal/storage"

	"github.com/slack-go/slack"
)

// Mock storage for testing
type mockStore struct {
	documents []storage.Document
	stored    []storage.Document
}

func (m *mockStore) StoreDocument(ctx context.Context, doc *storage.Document) error {
	m.stored = append(m.stored, *doc)
	return nil
}

func (m *mockStore) UpdateEmbedding(ctx context.Context, documentID string, embedding []float32) error {
	return nil
}

func (m *mockStore) SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]*storage.Document, error) {
	return nil, nil
}

func (m *mockStore) GetDocumentsWithoutEmbeddings(ctx context.Context, limit int) ([]*storage.Document, error) {
	return nil, nil
}

func (m *mockStore) Close() error {
	return nil
}

func TestSlackHandler_CleanMessageText(t *testing.T) {
	handler := &SlackHandler{}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal text",
			input:    "This is normal text",
			expected: "This is normal text",
		},
		{
			name:     "user mention only",
			input:    "<@U095Z0GRZGS>",
			expected: "",
		},
		{
			name:     "text with user mention",
			input:    "Hello <@U095Z0GRZGS> how are you?",
			expected: "Hello  how are you?",
		},
		{
			name:     "multiple user mentions",
			input:    "<@U095Z0GRZGS> <@U123456789> hello",
			expected: "hello",
		},
		{
			name:     "channel mention only",
			input:    "<#C06DTMSH03E|general>",
			expected: "",
		},
		{
			name:     "text with channel mention",
			input:    "Check out <#C06DTMSH03E|general> channel",
			expected: "Check out  channel",
		},
		{
			name:     "mixed mentions and text",
			input:    "Hey <@U095Z0GRZGS> check <#C06DTMSH03E|general> for updates",
			expected: "Hey  check  for updates",
		},
		{
			name:     "only whitespace after cleaning",
			input:    "   <@U095Z0GRZGS>   <#C06DTMSH03E|general>   ",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := handler.cleanMessageText(tc.input)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestSlackHandler_StoreMessage(t *testing.T) {
	mockStorage := &mockStore{}
	handler := &SlackHandler{
		store: mockStorage,
	}

	testCases := []struct {
		name           string
		message        slack.Message
		expectStored   bool
		expectedReason string
	}{
		{
			name: "valid message",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "This is a valid message",
					Timestamp: "1234567890.123456",
					User:      "U123456",
				},
			},
			expectStored: true,
		},
		{
			name: "empty message",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "",
					Timestamp: "1234567890.123456",
					User:      "U123456",
				},
			},
			expectStored:   false,
			expectedReason: "empty text",
		},
		{
			name: "bot message",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "Bot message",
					Timestamp: "1234567890.123456",
					User:      "U123456",
					SubType:   "bot_message",
				},
			},
			expectStored:   false,
			expectedReason: "bot message",
		},
		{
			name: "pure mention message",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "<@U095Z0GRZGS>",
					Timestamp: "1234567890.123456",
					User:      "U123456",
				},
			},
			expectStored:   false,
			expectedReason: "pure mentions",
		},
		{
			name: "multiple pure mentions",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "<@U095Z0GRZGS> <@U123456789>",
					Timestamp: "1234567890.123456",
					User:      "U123456",
				},
			},
			expectStored:   false,
			expectedReason: "pure mentions",
		},
		{
			name: "very short message",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "ok",
					Timestamp: "1234567890.123456",
					User:      "U123456",
				},
			},
			expectStored:   false,
			expectedReason: "too short",
		},
		{
			name: "mention with text",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "<@U095Z0GRZGS> this is a longer message with content",
					Timestamp: "1234567890.123456",
					User:      "U123456",
				},
			},
			expectStored: true,
		},
		{
			name: "whitespace only after cleaning",
			message: slack.Message{
				Msg: slack.Msg{
					Text:      "   <@U095Z0GRZGS>   <#C06DTMSH03E|general>   ",
					Timestamp: "1234567890.123456",
					User:      "U123456",
				},
			},
			expectStored:   false,
			expectedReason: "empty after cleaning",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock storage
			mockStorage.stored = []storage.Document{}

			err := handler.storeMessage(context.Background(), tc.message, "C06DTMSH03E")

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tc.expectStored {
				if len(mockStorage.stored) == 0 {
					t.Errorf("Expected message to be stored but it wasn't")
				} else {
					stored := mockStorage.stored[0]
					if stored.Content == "" {
						t.Errorf("Stored message should not have empty content")
					}
					if len(stored.Content) < 10 && tc.expectStored {
						t.Errorf("Stored message content too short: '%s'", stored.Content)
					}
				}
			} else {
				if len(mockStorage.stored) > 0 {
					t.Errorf("Expected message NOT to be stored (reason: %s) but it was: %+v", 
						tc.expectedReason, mockStorage.stored[0])
				}
			}
		})
	}
}

func TestParseSlackTimestamp(t *testing.T) {
	testCases := []struct {
		name      string
		timestamp string
		expected  time.Time
	}{
		{
			name:      "valid timestamp",
			timestamp: "1234567890.123456",
			expected:  time.Unix(1234567890, 0),
		},
		{
			name:      "timestamp without decimal",
			timestamp: "1234567890",
			expected:  time.Unix(1234567890, 0),
		},
		{
			name:      "very long timestamp",
			timestamp: "1234567890.123456789012",
			expected:  time.Unix(1234567890, 0),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseSlackTimestamp(tc.timestamp)
			if result.Unix() != tc.expected.Unix() {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestContentValidation(t *testing.T) {
	// Test the exact conditions that caused our production issues
	problematicMessages := []string{
		"<@U095Z0GRZGS>",                    // Pure mention
		"  <@U095Z0GRZGS>  ",               // Pure mention with whitespace  
		"<@U095Z0GRZGS><@U123456789>",      // Multiple pure mentions
		"",                                  // Empty
		"   \t\n   ",                       // Whitespace only
		"ok",                               // Too short
		"hi",                               // Too short
		"👍",                                // Emoji only (short)
		"<#C06DTMSH03E|general>",           // Channel mention only
	}

	validMessages := []string{
		"This is a valid message",
		"<@U095Z0GRZGS> this has content after the mention",
		"Check this out: https://example.com",
		"Let's discuss the project requirements in detail",
	}

	handler := &SlackHandler{}

	for _, msg := range problematicMessages {
		t.Run("problematic: "+msg, func(t *testing.T) {
			cleaned := handler.cleanMessageText(msg)
			finalContent := strings.TrimSpace(cleaned)
			
			// These should all be filtered out
			if len(finalContent) >= 10 {
				t.Errorf("Message '%s' should be filtered out but wasn't. Cleaned: '%s'", 
					msg, finalContent)
			}
		})
	}

	for _, msg := range validMessages {
		t.Run("valid: "+msg, func(t *testing.T) {
			cleaned := handler.cleanMessageText(msg)
			finalContent := strings.TrimSpace(cleaned)
			
			// These should all pass validation
			if len(finalContent) < 10 {
				t.Errorf("Valid message '%s' was incorrectly filtered out. Cleaned: '%s'", 
					msg, finalContent)
			}
		})
	}
}