package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"knowthis/internal/services"
	"knowthis/internal/storage"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type SlackHandler struct {
	client     *slack.Client
	socketMode *socketmode.Client
	store      storage.Store
	ragService *services.RAGService
}

func NewSlackHandler(botToken, appToken string, store storage.Store, ragService *services.RAGService) *SlackHandler {
	client := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	socketMode := socketmode.New(client)
	
	return &SlackHandler{
		client:     client,
		socketMode: socketMode,
		store:      store,
		ragService: ragService,
	}
}

func (h *SlackHandler) Start() error {
	go h.handleEvents()
	return h.socketMode.Run()
}

func (h *SlackHandler) handleEvents() {
	for evt := range h.socketMode.Events {
		switch evt.Type {
		case socketmode.EventTypeEventsAPI:
			// Handle events API
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				log.Printf("Ignored %+v\n", evt)
				continue
			}
			
			log.Printf("Event received: %+v\n", eventsAPIEvent)
			h.socketMode.Ack(*evt.Request)
			
			switch eventsAPIEvent.Type {
			case slackevents.CallbackEvent:
				innerEvent := eventsAPIEvent.InnerEvent
				switch ev := innerEvent.Data.(type) {
				case *slackevents.AppMentionEvent:
					h.handleAppMention(ev)
				}
			}
		}
	}
}

func (h *SlackHandler) handleAppMention(ev *slackevents.AppMentionEvent) {
	log.Printf("App mention received: %+v\n", ev)
	
	mention := &AppMentionEvent{
		Type:            ev.Type,
		User:            ev.User,
		Text:            ev.Text,
		Timestamp:       ev.TimeStamp,
		Channel:         ev.Channel,
		ThreadTimeStamp: ev.ThreadTimeStamp,
	}

	go h.processAppMention(mention)
}

// Helper struct for app mention event
type AppMentionEvent struct {
	Type            string
	User            string
	Text            string
	Timestamp       string
	Channel         string
	ThreadTimeStamp string
}


func (h *SlackHandler) processAppMention(mention *AppMentionEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if this is a thread
	var messages []slack.Message
	var err error
	
	if mention.ThreadTimeStamp != "" {
		// Get thread messages
		messages, err = h.getThreadMessages(ctx, mention.Channel, mention.ThreadTimeStamp)
	} else {
		// Get last 15 messages from channel
		messages, err = h.getChannelMessages(ctx, mention.Channel, 15)
	}
	
	if err != nil {
		log.Printf("Error fetching messages: %v", err)
		return
	}

	// Store messages with deduplication
	for _, msg := range messages {
		if err := h.storeMessage(ctx, msg, mention.Channel); err != nil {
			log.Printf("Error storing message: %v", err)
		}
	}

	// Send acknowledgment
	if err := h.sendAcknowledgment(mention.Channel, mention.ThreadTimeStamp); err != nil {
		log.Printf("Error sending acknowledgment: %v", err)
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

func (h *SlackHandler) storeMessage(ctx context.Context, msg slack.Message, channel string) error {
	// Skip messages without text or from bots
	if msg.Text == "" || msg.SubType == "bot_message" {
		return nil
	}
	
	// Clean text - remove user mentions and channel references
	cleanText := h.cleanMessageText(msg.Text)
	
	document := &storage.Document{
		ID:          fmt.Sprintf("slack_%s_%s", channel, msg.Timestamp),
		Content:     cleanText,
		Source:      "slack",
		SourceID:    msg.Timestamp,
		ChannelID:   channel,
		UserID:      msg.User,
		Timestamp:   parseSlackTimestamp(msg.Timestamp),
		ContentHash: storage.HashContent(cleanText),
	}
	
	return h.store.StoreDocument(ctx, document)
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

func (h *SlackHandler) sendAcknowledgment(channel, threadTS string) error {
	_, _, err := h.client.PostMessage(channel, slack.MsgOptionText("👍 Got it! I've processed and stored the messages.", false), slack.MsgOptionTS(threadTS))
	return err
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