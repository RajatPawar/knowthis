package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"knowthis/internal/services"
	"knowthis/internal/storage"
)

type SlabHandler struct {
	webhookSecret     string
	store             storage.Store
	embeddingService  *services.EmbeddingService
}

type SlabWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		ID      string `json:"id"`
		Title   string `json:"title,omitempty"`
		Content string `json:"content,omitempty"`
		PostID  string `json:"post_id,omitempty"`
		Author  struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"data"`
}

func NewSlabHandler(webhookSecret string, store storage.Store, embeddingService *services.EmbeddingService) *SlabHandler {
	return &SlabHandler{
		webhookSecret:    webhookSecret,
		store:            store,
		embeddingService: embeddingService,
	}
}

func (h *SlabHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Verify HMAC signature
	signature := r.Header.Get("X-Slab-Signature")
	if !h.verifyHMAC(body, signature) {
		log.Printf("Invalid HMAC signature")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse payload
	var payload SlabWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("Error parsing webhook payload: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Process event
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := h.processWebhookEvent(ctx, payload); err != nil {
		log.Printf("Error processing webhook event: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *SlabHandler) verifyHMAC(body []byte, signature string) bool {
	if h.webhookSecret == "" || signature == "" {
		return false
	}

	// Remove "sha256=" prefix if present
	if strings.HasPrefix(signature, "sha256=") {
		signature = signature[7:]
	}

	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (h *SlabHandler) processWebhookEvent(ctx context.Context, payload SlabWebhookPayload) error {
	switch payload.Event {
	case "post.published", "post.updated":
		return h.processPost(ctx, payload)
	case "comment.created", "comment.updated":
		return h.processComment(ctx, payload)
	default:
		log.Printf("Unhandled event type: %s", payload.Event)
		return nil
	}
}

func (h *SlabHandler) processPost(ctx context.Context, payload SlabWebhookPayload) error {
	if payload.Data.Content == "" {
		return nil
	}

	// Clean content - remove markdown formatting for better embedding
	cleanContent := h.cleanSlabContent(payload.Data.Content)
	
	document := &storage.Document{
		ID:          fmt.Sprintf("slab_post_%s", payload.Data.ID),
		Content:     cleanContent,
		Source:      "slab",
		SourceID:    payload.Data.ID,
		Title:       payload.Data.Title,
		UserID:      payload.Data.Author.ID,
		UserName:    payload.Data.Author.Name,
		Timestamp:   payload.Data.CreatedAt,
		ContentHash: storage.HashContent(cleanContent),
	}

	return h.store.StoreDocument(ctx, document)
}

func (h *SlabHandler) processComment(ctx context.Context, payload SlabWebhookPayload) error {
	if payload.Data.Content == "" {
		return nil
	}

	// Clean content
	cleanContent := h.cleanSlabContent(payload.Data.Content)
	
	document := &storage.Document{
		ID:          fmt.Sprintf("slab_comment_%s", payload.Data.ID),
		Content:     cleanContent,
		Source:      "slab",
		SourceID:    payload.Data.ID,
		PostID:      payload.Data.PostID,
		UserID:      payload.Data.Author.ID,
		UserName:    payload.Data.Author.Name,
		Timestamp:   payload.Data.CreatedAt,
		ContentHash: storage.HashContent(cleanContent),
	}

	return h.store.StoreDocument(ctx, document)
}

func (h *SlabHandler) cleanSlabContent(content string) string {
	// Remove common markdown patterns that might hurt embedding quality
	content = strings.ReplaceAll(content, "**", "")
	content = strings.ReplaceAll(content, "*", "")
	content = strings.ReplaceAll(content, "_", "")
	content = strings.ReplaceAll(content, "`", "")
	content = strings.ReplaceAll(content, "#", "")
	
	// Remove multiple spaces and newlines
	content = strings.ReplaceAll(content, "\n\n", "\n")
	content = strings.ReplaceAll(content, "  ", " ")
	
	return strings.TrimSpace(content)
}