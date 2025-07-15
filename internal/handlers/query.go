package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"knowthis/internal/services"
)

type QueryHandler struct {
	ragService *services.RAGService
}

type QueryRequest struct {
	Query string `json:"query"`
}

type QueryResponse struct {
	Answer  string `json:"answer"`
	Sources []struct {
		ID        string    `json:"id"`
		Content   string    `json:"content"`
		Source    string    `json:"source"`
		Title     string    `json:"title,omitempty"`
		UserName  string    `json:"user_name,omitempty"`
		Timestamp time.Time `json:"timestamp"`
		Similarity float64  `json:"similarity"`
	} `json:"sources"`
	Query string `json:"query"`
}

func NewQueryHandler(ragService *services.RAGService) *QueryHandler {
	return &QueryHandler{ragService: ragService}
}

func (h *QueryHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding query request: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := h.ragService.Query(ctx, req.Query)
	if err != nil {
		log.Printf("Error processing query: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	response := QueryResponse{
		Answer: result.Answer,
		Query:  result.Query,
		Sources: make([]struct {
			ID        string    `json:"id"`
			Content   string    `json:"content"`
			Source    string    `json:"source"`
			Title     string    `json:"title,omitempty"`
			UserName  string    `json:"user_name,omitempty"`
			Timestamp time.Time `json:"timestamp"`
			Similarity float64  `json:"similarity"`
		}, len(result.Sources)),
	}

	for i, source := range result.Sources {
		response.Sources[i] = struct {
			ID        string    `json:"id"`
			Content   string    `json:"content"`
			Source    string    `json:"source"`
			Title     string    `json:"title,omitempty"`
			UserName  string    `json:"user_name,omitempty"`
			Timestamp time.Time `json:"timestamp"`
			Similarity float64  `json:"similarity"`
		}{
			ID:        source.ID,
			Content:   source.Content,
			Source:    source.Source,
			Title:     source.Title,
			UserName:  source.UserName,
			Timestamp: source.Timestamp,
			Similarity: source.Similarity,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}