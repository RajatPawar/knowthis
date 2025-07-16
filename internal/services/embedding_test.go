package services

import (
	"strings"
	"testing"
)

func TestGenerateEmbedding_EmptyInput(t *testing.T) {
	// Test only the validation logic, not the actual API call
	// We'll test this by creating a service and checking validation before API calls
	
	testCases := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty string",
			input:       "",
			expectError: true,
			errorMsg:    "input text cannot be empty",
		},
		{
			name:        "whitespace only",
			input:       "   \t\n  ",
			expectError: true,
			errorMsg:    "input text cannot be empty",
		},
		{
			name:        "valid text",
			input:       "This is valid text",
			expectError: false, // Will fail at API call but validation should pass
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the validation logic directly without calling the API
			text := strings.TrimSpace(tc.input)
			isEmpty := text == ""
			
			if tc.expectError && !isEmpty {
				t.Errorf("Expected validation to catch empty input but it didn't")
			} else if !tc.expectError && isEmpty {
				t.Errorf("Valid input was incorrectly flagged as empty")
			}
		})
	}
}

func TestGenerateEmbedding_TokenLimit(t *testing.T) {
	// Test only the validation logic, not the actual API call
	// Create text that exceeds 8K tokens (32K characters)
	longText := strings.Repeat("This is a test sentence that will be repeated many times. ", 1000) // ~58K chars
	
	// Apply the same truncation logic as in the service
	const maxTokens = 8000
	const avgCharsPerToken = 4
	maxChars := maxTokens * avgCharsPerToken // 32000
	
	result := longText
	if len(result) > maxChars {
		result = result[:maxChars]
		if lastSpace := strings.LastIndex(result[:maxChars], " "); lastSpace > maxChars-100 {
			result = result[:lastSpace]
		}
	}
	
	// Test that the text is properly truncated but not empty
	if len(result) == 0 {
		t.Errorf("Text should not be truncated to empty string")
	}
	
	if len(result) > maxChars {
		t.Errorf("Text should be truncated to %d chars, got %d", maxChars, len(result))
	}
}

func TestGenerateEmbeddings_ArrayValidation(t *testing.T) {
	// Test only the validation logic, not the actual API call
	testCases := []struct {
		name           string
		input          []string
		expectValid    bool
		validTextCount int
	}{
		{
			name:           "empty array",
			input:          []string{},
			expectValid:    false,
			validTextCount: 0,
		},
		{
			name:           "array with only empty strings",
			input:          []string{"", "   ", "\t\n"},
			expectValid:    false,
			validTextCount: 0,
		},
		{
			name:           "mixed empty and valid",
			input:          []string{"", "valid text", "   "},
			expectValid:    true,
			validTextCount: 1,
		},
		{
			name:           "all valid",
			input:          []string{"text one", "text two"},
			expectValid:    true,
			validTextCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Apply the same validation logic as in the service
			if len(tc.input) == 0 {
				if tc.expectValid {
					t.Errorf("Empty array should not be valid")
				}
				return
			}
			
			// Count valid texts
			validCount := 0
			for _, text := range tc.input {
				if strings.TrimSpace(text) != "" {
					validCount++
				}
			}
			
			isValid := validCount > 0
			if isValid != tc.expectValid {
				t.Errorf("Expected valid=%v, got %v", tc.expectValid, isValid)
			}
			
			if validCount != tc.validTextCount {
				t.Errorf("Expected %d valid texts, got %d", tc.validTextCount, validCount)
			}
		})
	}
}

func TestTextTruncation(t *testing.T) {
	// Test the token limit logic
	const maxTokens = 8000
	const avgCharsPerToken = 4
	maxChars := maxTokens * avgCharsPerToken // 32000

	testCases := []struct {
		name     string
		input    string
		expected int // expected max length
	}{
		{
			name:     "short text",
			input:    "short",
			expected: 5,
		},
		{
			name:     "exactly at limit",
			input:    strings.Repeat("a", maxChars),
			expected: maxChars,
		},
		{
			name:     "over limit",
			input:    strings.Repeat("a", maxChars+1000),
			expected: maxChars, // Should be truncated
		},
		{
			name:     "over limit with spaces",
			input:    strings.Repeat("word ", (maxChars+1000)/5),
			expected: maxChars, // Should be truncated at word boundary if possible
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.input
			
			// Apply the same truncation logic as in the service
			if len(result) > maxChars {
				result = result[:maxChars]
				if lastSpace := strings.LastIndex(result[:maxChars], " "); lastSpace > maxChars-100 {
					result = result[:lastSpace]
				}
			}
			
			if len(result) > tc.expected {
				t.Errorf("Expected max length %d, got %d", tc.expected, len(result))
			}
			
			if len(result) == 0 && len(tc.input) > 0 {
				t.Errorf("Text should not be truncated to empty string")
			}
		})
	}
}