package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// SlackHandler handles Slack message actions and API interactions
type SlackHandler struct {
	client    *slack.Client
	storage   *SlackStorage
	botUserID string
}

// NewSlackHandler creates a new Slack handler
func NewSlackHandler(botToken string, storage *SlackStorage) *SlackHandler {
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
		client:    client,
		storage:   storage,
		botUserID: botUserID,
	}
}

// HandleMessageAction handles Slack message actions (interactive components)
func (h *SlackHandler) HandleMessageAction(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received Slack message action", "method", r.Method, "url", r.URL.Path)

	// Parse the interaction payload
	payload := r.FormValue("payload")
	if payload == "" {
		slog.Error("Missing payload in Slack action request")
		http.Error(w, "Missing payload", http.StatusBadRequest)
		return
	}

	slog.Debug("Received payload", "payload", payload)

	var interaction slack.InteractionCallback
	if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
		slog.Error("Failed to parse interaction payload", "error", err, "payload", payload)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	slog.Info("Parsed interaction", 
		"callback_id", interaction.CallbackID, 
		"type", interaction.Type,
		"action_id", func() string {
			if len(interaction.ActionCallback.BlockActions) > 0 {
				return interaction.ActionCallback.BlockActions[0].ActionID
			}
			return ""
		}())

	// Handle collect_context action (check both callback_id and action_id)
	if interaction.CallbackID == "collect_context" || 
		(len(interaction.ActionCallback.BlockActions) > 0 && 
		 interaction.ActionCallback.BlockActions[0].ActionID == "collect_context") {
		slog.Info("Processing collect_context action")
		
		// Check if the action was triggered on a bot message
		if h.isTriggeredOnBotMessage(interaction) {
			slog.Info("Action triggered on bot message, skipping")
			// Respond with helpful message
			w.Header().Set("Content-Type", "application/json")
			response := map[string]interface{}{
				"response_type": "ephemeral",
				"text":          "ℹ️ Cannot collect context from bot messages. Please use this action on human messages.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		
		// Start processing in background
		go h.handleCollectContext(interaction)

		// Respond immediately with ephemeral message
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"response_type": "ephemeral",
			"text":          "✅ Collecting thread context for knowledge base...",
		}
		
		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("Failed to encode response", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		
		slog.Info("Sent immediate response to Slack")
		return
	}

	// Unknown action
	slog.Warn("Unknown action received", "callback_id", interaction.CallbackID)
	w.WriteHeader(http.StatusOK)
}

// isTriggeredOnBotMessage checks if the message action was triggered on a bot message
func (h *SlackHandler) isTriggeredOnBotMessage(interaction slack.InteractionCallback) bool {
	message := interaction.Message
	
	// Check if message has bot_id (indicates it's from a bot)
	if message.BotID != "" {
		slog.Debug("Message has bot_id", "bot_id", message.BotID)
		return true
	}
	
	// Check if message subtype indicates it's a bot message
	if message.SubType == "bot_message" {
		slog.Debug("Message subtype is bot_message")
		return true
	}
	
	// Check if the message user is our own bot
	if h.botUserID != "" && message.User == h.botUserID {
		slog.Debug("Message is from our own bot", "bot_user_id", h.botUserID)
		return true
	}
	
	// Check if the message user starts with B (Bot users in Slack start with B)
	if message.User != "" && strings.HasPrefix(message.User, "B") {
		slog.Debug("Message user starts with B (likely a bot)", "user", message.User)
		return true
	}
	
	return false
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
	slackMessages, err := h.getThreadMessages(ctx, channelID, threadTS)
	if err != nil {
		slog.Error("Failed to get thread messages", "error", err)
		h.sendProcessingError(userID, channelID)
		return
	}

	slog.Info("Retrieved thread messages", 
		"count", len(slackMessages),
		"channel", channelID,
		"thread_ts", threadTS)

	// Convert and store messages
	storedCount := 0
	processedCount := 0
	for i, slackMsg := range slackMessages {
		processedCount++
		slog.Info("Processing message", 
			"index", i,
			"timestamp", slackMsg.Timestamp,
			"user", slackMsg.User,
			"text_length", len(slackMsg.Text),
			"is_root", slackMsg.Timestamp == threadTS)
		
		// Convert Slack message to our format
		msg := h.convertSlackMessage(slackMsg, channelID, threadTS)
		if msg == nil {
			slog.Info("Message skipped during conversion", "timestamp", slackMsg.Timestamp)
			continue // Skip invalid messages
		}

		// Store message
		stored, wasInserted, err := h.storage.StoreMessage(ctx, *msg)
		if err != nil {
			slog.Error("Failed to store message", "error", err, "message_ts", slackMsg.Timestamp)
			continue
		}

		slog.Info("Message stored", 
			"timestamp", msg.MessageTimestamp,
			"was_inserted", wasInserted,
			"stored_id", stored.ID,
			"user_name", stored.UserName)

		if wasInserted {
			storedCount++
		}
	}

	slog.Info("Processing complete", 
		"processed", processedCount,
		"stored", storedCount,
		"total_retrieved", len(slackMessages))

	// Send completion message to user
	h.sendCompletionMessage(userID, channelID, storedCount, len(slackMessages))
}

// getThreadMessages retrieves all messages in a thread from Slack
func (h *SlackHandler) getThreadMessages(ctx context.Context, channelID, threadTS string) ([]slack.Message, error) {
	params := &slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
		Limit:     100,
		Inclusive: true, // Include the parent message (thread root)
	}
	
	msgs, _, _, err := h.client.GetConversationRepliesContext(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread messages: %w", err)
	}
	
	return msgs, nil
}

// convertSlackMessage converts a Slack message to our internal format
func (h *SlackHandler) convertSlackMessage(slackMsg slack.Message, channelID, threadTS string) *SlackMessage {
	slog.Debug("Converting Slack message", 
		"timestamp", slackMsg.Timestamp,
		"user", slackMsg.User,
		"text", slackMsg.Text,
		"subtype", slackMsg.SubType,
		"thread_ts", threadTS,
		"bot_id", slackMsg.BotID)
	
	// Skip messages without text or from bots
	if slackMsg.Text == "" {
		slog.Debug("Skipping message: no text", "timestamp", slackMsg.Timestamp)
		return nil
	}
	
	if slackMsg.SubType == "bot_message" {
		slog.Debug("Skipping message: bot message", "timestamp", slackMsg.Timestamp)
		return nil
	}
	
	// Skip messages from our own bot
	if h.botUserID != "" && slackMsg.User == h.botUserID {
		slog.Debug("Skipping message: from our bot", "timestamp", slackMsg.Timestamp)
		return nil
	}
	
	// Clean message text
	cleanText := h.cleanMessageText(slackMsg.Text)
	slog.Debug("Cleaned text", "original", slackMsg.Text, "cleaned", cleanText)
	
	// Skip messages that are purely mentions (empty after cleaning)
	if strings.TrimSpace(cleanText) == "" {
		slog.Debug("Skipping message: empty after cleaning", "timestamp", slackMsg.Timestamp)
		return nil
	}
	
	// Skip very short messages (not worth embedding cost), but allow thread roots
	if len(strings.TrimSpace(cleanText)) < 10 && slackMsg.Timestamp != threadTS {
		slog.Debug("Skipping message: too short", "timestamp", slackMsg.Timestamp, "length", len(strings.TrimSpace(cleanText)))
		return nil
	}
	
	// Get user display name
	userName := h.getUserDisplayName(slackMsg.User)
	slog.Debug("Got user display name", "user_id", slackMsg.User, "user_name", userName)
	
	// Determine if this is the thread root
	isThreadRoot := slackMsg.Timestamp == threadTS
	slog.Debug("Thread root check", "msg_timestamp", slackMsg.Timestamp, "thread_ts", threadTS, "is_root", isThreadRoot)
	
	msg := &SlackMessage{
		ChannelID:        channelID,
		ThreadID:         threadTS,
		MessageTimestamp: slackMsg.Timestamp,
		UserID:           slackMsg.User,
		UserName:         userName,
		Content:          strings.TrimSpace(cleanText),
		ClientMsgID:      slackMsg.ClientMsgID,
		IsThreadRoot:     isThreadRoot,
	}
	
	slog.Info("Successfully converted message", 
		"timestamp", msg.MessageTimestamp,
		"user_name", msg.UserName,
		"content_length", len(msg.Content),
		"is_thread_root", msg.IsThreadRoot)
	
	return msg
}

// getUserDisplayName gets the display name for a user ID
func (h *SlackHandler) getUserDisplayName(userID string) string {
	if userID == "" {
		return ""
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	user, err := h.client.GetUserInfoContext(ctx, userID)
	if err != nil {
		slog.Warn("Failed to get user info", "error", err, "user_id", userID)
		return userID // Fallback to user ID
	}
	
	// Try display name first, then real name, then name
	if user.Profile.DisplayName != "" {
		return user.Profile.DisplayName
	}
	if user.Profile.RealName != "" {
		return user.Profile.RealName
	}
	if user.Name != "" {
		return user.Name
	}
	
	return userID // Fallback to user ID
}

// cleanMessageText removes user mentions and channel references
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

// sendCompletionMessage sends a completion notification to the user
func (h *SlackHandler) sendCompletionMessage(userID, channelID string, storedCount, totalCount int) {
	var message string
	if storedCount == totalCount {
		message = fmt.Sprintf("✅ Stored %d messages from thread in knowledge base", storedCount)
	} else {
		message = fmt.Sprintf("✅ Stored %d new messages from thread (%d total messages)", storedCount, totalCount)
	}

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