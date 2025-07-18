package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"knowthis/internal/config"
	"knowthis/internal/handlers"
	"knowthis/internal/integrations/slack"
	"knowthis/internal/logging"
	"knowthis/internal/middleware"
	"knowthis/internal/services"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "github.com/lib/pq"
)

type ServiceBundle struct {
	EmbeddingService         *services.EmbeddingService
	RAGService               *services.RAGService
	SlackStorage             *slack.SlackStorage
	SlackHandler             *slack.SlackHandler
	SlackEmbeddingProcessor  *slack.EmbeddingProcessor
	QueryHandler             *handlers.QueryHandler
	Config                   *config.Config
}

func initializeServices() *ServiceBundle {
	for {
		slog.Info("Loading configuration...")
		
		// Load and validate configuration with retry
		var cfg *config.Config
		for {
			cfg = config.Load()
			if err := cfg.Validate(); err != nil {
				slog.Error("Invalid configuration, retrying in 30s", "error", err)
				time.Sleep(30 * time.Second)
				continue
			}
			break
		}
		
		slog.Info("Initializing services...")
		
		// Initialize database connection for Slack storage
		var db *sql.DB
		for {
			var err error
			db, err = sql.Open("postgres", cfg.DatabaseURL)
			if err != nil {
				slog.Error("Failed to open database connection, retrying in 30s", "error", err)
				time.Sleep(30 * time.Second)
				continue
			}
			
			// Test connection
			if err = db.Ping(); err != nil {
				slog.Error("Failed to ping database, retrying in 30s", "error", err)
				db.Close()
				time.Sleep(30 * time.Second)
				continue
			}
			
			// Create vector extension
			if _, err = db.Exec("CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
				slog.Error("Failed to create vector extension, retrying in 30s", "error", err)
				db.Close()
				time.Sleep(30 * time.Second)
				continue
			}
			
			break
		}
		
		// Initialize embedding service with retry
		var embeddingService *services.EmbeddingService
		for {
			embeddingService = services.NewEmbeddingService(cfg.OpenAIAPIKey)
			if embeddingService == nil {
				slog.Error("Failed to initialize embedding service, retrying in 30s")
				time.Sleep(30 * time.Second)
				// Reload configuration on retry
				cfg = config.Load()
				if err := cfg.Validate(); err != nil {
					slog.Error("Invalid configuration on retry", "error", err)
					time.Sleep(30 * time.Second)
					continue
				}
				continue
			}
			break
		}
		
		// Initialize Slack storage and handler
		var slackStorage *slack.SlackStorage
		var slackHandler *slack.SlackHandler
		var slackEmbeddingProcessor *slack.EmbeddingProcessor
		for {
			slackStorage = slack.NewSlackStorage(db)
			if err := slackStorage.InitSchema(); err != nil {
				slog.Error("Failed to initialize Slack schema, retrying in 30s", "error", err)
				time.Sleep(30 * time.Second)
				continue
			}
			
			slackHandler = slack.NewSlackHandler(cfg.SlackBotToken, slackStorage)
			if slackHandler == nil {
				slog.Error("Failed to initialize Slack handler, retrying in 30s")
				time.Sleep(30 * time.Second)
				continue
			}
			
			slackEmbeddingProcessor = slack.NewEmbeddingProcessor(slackStorage, embeddingService)
			if slackEmbeddingProcessor == nil {
				slog.Error("Failed to initialize Slack embedding processor, retrying in 30s")
				time.Sleep(30 * time.Second)
				continue
			}
			
			break
		}

		// Initialize RAG service with retry
		var ragService *services.RAGService
		for {
			ragService = services.NewRAGService(cfg.OpenAIAPIKey, slackStorage, embeddingService)
			if ragService == nil {
				slog.Error("Failed to initialize RAG service, retrying in 30s")
				time.Sleep(30 * time.Second)
				// Reload configuration on retry
				cfg = config.Load()
				if err := cfg.Validate(); err != nil {
					slog.Error("Invalid configuration on retry", "error", err)
					time.Sleep(30 * time.Second)
					continue
				}
				continue
			}
			break
		}
		
		
		
		// Initialize query handler with retry
		var queryHandler *handlers.QueryHandler
		for {
			queryHandler = handlers.NewQueryHandler(ragService)
			if queryHandler == nil {
				slog.Error("Failed to initialize query handler, retrying in 30s")
				time.Sleep(30 * time.Second)
				continue
			}
			break
		}
		
		slog.Info("All services initialized successfully")
		
		return &ServiceBundle{
			EmbeddingService:        embeddingService,
			RAGService:              ragService,
			SlackStorage:            slackStorage,
			SlackHandler:            slackHandler,
			SlackEmbeddingProcessor: slackEmbeddingProcessor,
			QueryHandler:            queryHandler,
			Config:                  cfg,
		}
	}
}

func main() {
	// Setup structured logging
	logging.SetupLogger()
	
	slog.Info("Starting KnowThis application", slog.String("version", "1.0.0"))
	
	// Initialize all services with retry logic (includes config validation)
	services := initializeServices()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background jobs
	go services.SlackEmbeddingProcessor.Start(ctx)
	
	// Note: Slack now uses message actions instead of Socket Mode
	// No background goroutine needed - handled via HTTP endpoints

	// Setup HTTP server with middleware
	router := mux.NewRouter()
	
	// Add middleware
	router.Use(middleware.LoggingMiddleware)
	router.Use(middleware.MetricsMiddleware)
	
	// API routes with rate limiting
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Use(middleware.APIRateLimitMiddleware())
	apiRouter.HandleFunc("/query", services.QueryHandler.HandleQuery).Methods("POST")
	
	// Webhook routes with rate limiting (reserved for future integrations)
	webhookRouter := router.PathPrefix("/webhook").Subrouter()
	webhookRouter.Use(middleware.WebhookRateLimitMiddleware())
	// Future integrations will be added here
	
	// Slack routes with rate limiting
	slackRouter := router.PathPrefix("/slack").Subrouter()
	slackRouter.Use(middleware.WebhookRateLimitMiddleware())
	slackRouter.HandleFunc("/actions", services.SlackHandler.HandleMessageAction).Methods("POST")
	
	// Test endpoint for Slack actions (for debugging)
	slackRouter.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Slack actions endpoint is working"})
	}).Methods("GET")
	
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
		Addr:         ":" + services.Config.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		slog.Info("Server starting", slog.String("port", services.Config.Port))
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
	
	// Stop embedding processors
	services.SlackEmbeddingProcessor.Stop()
	
	// Shutdown server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}
	
	slog.Info("Server exited gracefully")
}