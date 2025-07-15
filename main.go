package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"knowthis/internal/config"
	"knowthis/internal/handlers"
	"knowthis/internal/jobs"
	"knowthis/internal/logging"
	"knowthis/internal/middleware"
	"knowthis/internal/services"
	"knowthis/internal/storage"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Setup structured logging
	logging.SetupLogger()
	
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}
	
	slog.Info("Starting KnowThis application", slog.String("version", "1.0.0"))
	
	// Initialize storage
	store, err := storage.NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize services
	embeddingService := services.NewEmbeddingService(cfg.OpenAIAPIKey)
	ragService := services.NewRAGService(cfg.OpenAIAPIKey, store, embeddingService)
	
	// Initialize background jobs
	embeddingProcessor := jobs.NewEmbeddingProcessor(store, embeddingService)
	
	// Initialize handlers
	slackHandler := handlers.NewSlackHandler(cfg.SlackBotToken, cfg.SlackAppToken, store, ragService)
	slabHandler := handlers.NewSlabHandler(cfg.SlabWebhookSecret, store, embeddingService)
	queryHandler := handlers.NewQueryHandler(ragService)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background jobs
	go embeddingProcessor.Start(ctx)
	
	// Start Slack Socket Mode in goroutine
	go func() {
		if err := slackHandler.Start(); err != nil {
			slog.Error("Failed to start Slack handler", "error", err)
			os.Exit(1)
		}
	}()

	// Setup HTTP server with middleware
	router := mux.NewRouter()
	
	// Add middleware
	router.Use(middleware.LoggingMiddleware)
	router.Use(middleware.MetricsMiddleware)
	
	// API routes with rate limiting
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Use(middleware.APIRateLimitMiddleware())
	apiRouter.HandleFunc("/query", queryHandler.HandleQuery).Methods("POST")
	
	// Webhook routes with rate limiting
	webhookRouter := router.PathPrefix("/webhook").Subrouter()
	webhookRouter.Use(middleware.WebhookRateLimitMiddleware())
	webhookRouter.HandleFunc("/slab", slabHandler.HandleWebhook).Methods("POST")
	
	// System routes
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")
	
	router.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Add readiness checks
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	}).Methods("GET")
	
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		slog.Info("Server starting", slog.String("port", cfg.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Server shutting down...")
	
	// Cancel context to stop background jobs
	cancel()
	
	// Stop embedding processor
	embeddingProcessor.Stop()
	
	// Shutdown server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}
	
	slog.Info("Server exited gracefully")
}