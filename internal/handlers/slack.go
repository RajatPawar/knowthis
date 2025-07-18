package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"knowthis/internal/services"
	"knowthis/internal/storage"

	"github.com/slack-go/slack"
)

type SlackHandler struct {
	client     *slack.Client
	store      storage.Store
	ragService *services.RAGService
	botUserID  string
}

func NewSlackHandler(botToken string, store storage.Store, ragService *services.RAGService) *SlackHandler {
	client := slack.New(botToken)
	
	// Get bot user ID
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	authTest, err := client.AuthTestContext(ctx)
	var botUserID string
	if err != nil {
		slog.Warn("Could not get bot user ID", "error", err)
	} else {
		botUserID = authTest.UserID
		slog.Info("Bot user ID retrieved", "bot_user_id", botUserID)
	}
	
	return &SlackHandler{
		client:     client,
		store:      store,
		ragService: ragService,
		botUserID:  botUserID,
	}
}

// HandleMessageAction handles Slack message actions (interactive components)
func (h *SlackHandler) HandleMessageAction(w http.ResponseWriter, r *http.Request) {
	// Parse the interaction payload
	payload := r.FormValue("payload")
	if payload == "" {
		http.Error(w, "Missing payload", http.StatusBadRequest)
		return
	}

	var interaction slack.InteractionCallback
	if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
		slog.Error("Failed to parse interaction payload", "error", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Handle collect_context action
	if interaction.CallbackID == "collect_context" {
		// Start processing in background
		go h.handleCollectContext(interaction)

		// Respond immediately with ephemeral message
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"response_type": "ephemeral",
			"text":          "✅ Collecting thread context and generating summary...",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Unknown action
	w.WriteHeader(http.StatusOK)
}

// handleCollectContext processes the thread context collection
func (h *SlackHandler) handleCollectContext(interaction slack.InteractionCallback) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	message := interaction.Message
	channelID := interaction.Channel.ID
	userID := interaction.User.ID

	// Determine thread timestamp
	threadTS := message.ThreadTimestamp
	if threadTS == "" {
		// If not in a thread, use the message timestamp as thread root
		threadTS = message.Timestamp
	}

	slog.Info("Processing thread context collection", 
		"channel", channelID, 
		"thread_ts", threadTS, 
		"user", userID)

	// Get all thread messages
	messages, err := h.getThreadMessages(ctx, channelID, threadTS)
	if err != nil {
		slog.Error("Failed to get thread messages", "error", err)
		h.sendProcessingError(userID, channelID)
		return
	}

	// Generate thread summary
	threadSummary, err := h.generateThreadSummary(ctx, messages)
	if err != nil {
		slog.Error("Failed to generate thread summary", "error", err)
		// Continue without summary
		threadSummary = "Summary unavailable"
	}

	// Store thread as single document with summary
	if err := h.storeThreadDocument(ctx, threadTS, threadSummary, messages, channelID); err != nil {
		slog.Error("Failed to store thread document", "error", err)
		h.sendProcessingError(userID, channelID)
		return
	}

	// Send completion message to user
	h.sendCompletionMessage(userID, channelID, len(messages))
}

// generateThreadSummary creates an AI-generated summary of the thread
func (h *SlackHandler) generateThreadSummary(ctx context.Context, messages []slack.Message) (string, error) {
	if len(messages) == 0 {
		return "Empty thread", nil
	}

	// Build thread context
	var threadContent strings.Builder
	for i, msg := range messages {
		if msg.Text == "" || msg.SubType == "bot_message" {
			continue
		}

		cleanText := h.cleanMessageText(msg.Text)
		if strings.TrimSpace(cleanText) == "" {
			continue
		}

		threadContent.WriteString(fmt.Sprintf("Message %d: %s\n", i+1, cleanText))
	}

	if threadContent.Len() == 0 {
		return "No meaningful content in thread", nil
	}

	// Generate summary using RAG service
	prompt := fmt.Sprintf(`Summarize this Slack thread conversation in 2-3 sentences. Focus on the main topic, key decisions, and important outcomes. Be concise but informative.

Thread content:
%s`, threadContent.String())

	// Use a simple query to the RAG service for summarization
	// Note: This is a simplified approach - in production you might want a dedicated summarization endpoint
	result, err := h.ragService.Query(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	return result.Answer, nil
}

// storeThreadDocument stores the entire thread as a single document
func (h *SlackHandler) storeThreadDocument(ctx context.Context, threadTS, summary string, messages []slack.Message, channelID string) error {
	if len(messages) == 0 {
		return nil
	}

	// Build full thread content
	var threadContent strings.Builder
	var participants []string
	participantSet := make(map[string]bool)

	// Add summary at the top
	threadContent.WriteString(fmt.Sprintf("Thread Summary: %s\n\n", summary))

	// Add each message
	for i, msg := range messages {
		if msg.Text == "" || msg.SubType == "bot_message" {
			continue
		}

		// Skip messages from our own bot
		if h.botUserID != "" && msg.User == h.botUserID {
			continue
		}

		cleanText := h.cleanMessageText(msg.Text)
		if strings.TrimSpace(cleanText) == "" || len(strings.TrimSpace(cleanText)) < 10 {
			continue
		}

		// Track participants
		if msg.User != "" && !participantSet[msg.User] {
			participants = append(participants, msg.User)
			participantSet[msg.User] = true
		}

		// Add message to thread content
		threadContent.WriteString(fmt.Sprintf("Message %d: %s\n", i+1, cleanText))
	}

	// Create thread title from first message
	threadTitle := "Thread"
	if len(messages) > 0 && messages[0].Text != "" {
		firstMessage := h.cleanMessageText(messages[0].Text)
		if len(firstMessage) > 50 {
			threadTitle = firstMessage[:50] + "..."
		} else if len(firstMessage) > 0 {
			threadTitle = firstMessage
		}
	}

	finalContent := threadContent.String()
	if strings.TrimSpace(finalContent) == "" {
		return fmt.Errorf("no meaningful content in thread")
	}

	document := &storage.Document{
		ID:          fmt.Sprintf("slack_thread_%s_%s", channelID, threadTS),
		Content:     finalContent,
		Source:      "slack",
		SourceID:    threadTS, // Use thread timestamp as source ID
		Title:       threadTitle,
		ChannelID:   channelID,
		UserName:    strings.Join(participants, ", "), // List all participants
		Timestamp:   parseSlackTimestamp(threadTS),
		ContentHash: storage.HashContent(finalContent),
	}

	return h.store.StoreDocument(ctx, document)
}

// sendCompletionMessage sends a completion notification to the user
func (h *SlackHandler) sendCompletionMessage(userID, channelID string, totalMessages int) {
	message := fmt.Sprintf("✅ Processed %d messages from thread and generated summary", totalMessages)

	// Send ephemeral message to user
	_, err := h.client.PostEphemeral(
		channelID,
		userID,
		slack.MsgOptionText(message, false),
	)
	if err != nil {
		slog.Error("Failed to send completion message", "error", err)
	}
}

// sendProcessingError sends an error message to the user
func (h *SlackHandler) sendProcessingError(userID, channelID string) {
	_, err := h.client.PostEphemeral(
		channelID,
		userID,
		slack.MsgOptionText("❌ Failed to process thread. Please try again.", false),
	)
	if err != nil {
		slog.Error("Failed to send error message", "error", err)
	}
}


func (h *SlackHandler) getThreadMessages(ctx context.Context, channel, threadTS string) ([]slack.Message, error) {
	params := &slack.GetConversationRepliesParameters{
		ChannelID: channel,
		Timestamp: threadTS,
		Limit:     100,
	}
	
	msgs, _, _, err := h.client.GetConversationRepliesContext(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread messages: %w", err)
	}
	
	return msgs, nil
}

func (h *SlackHandler) getChannelMessages(ctx context.Context, channel string, limit int) ([]slack.Message, error) {
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channel,
		Limit:     limit,
	}
	
	history, err := h.client.GetConversationHistoryContext(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel messages: %w", err)
	}
	
	return history.Messages, nil
}


func (h *SlackHandler) cleanMessageText(text string) string {
	// Remove user mentions like <@U123456>
	for strings.Contains(text, "<@") {
		start := strings.Index(text, "<@")
		end := strings.Index(text[start:], ">")
		if end == -1 {
			break
		}
		text = text[:start] + text[start+end+1:]
	}
	
	// Remove channel references like <#C123456|general>
	for strings.Contains(text, "<#") {
		start := strings.Index(text, "<#")
		end := strings.Index(text[start:], ">")
		if end == -1 {
			break
		}
		text = text[:start] + text[start+end+1:]
	}
	
	return strings.TrimSpace(text)
}


func parseSlackTimestamp(ts string) time.Time {
	// Slack timestamps are in format "1234567890.123456"
	if len(ts) > 10 {
		ts = ts[:10]
	}
	
	var unixTime int64
	fmt.Sscanf(ts, "%d", &unixTime)
	return time.Unix(unixTime, 0)
}