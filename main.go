package main

import (
	"context"
	"encoding/json"
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

type ServiceBundle struct {
	Store               *storage.PostgresStore
	EmbeddingService    *services.EmbeddingService
	RAGService          *services.RAGService
	EmbeddingProcessor  *jobs.EmbeddingProcessor
	SlackHandler        *handlers.SlackHandler
	SlabHandler         *handlers.SlabHandler
	QueryHandler        *handlers.QueryHandler
	Config              *config.Config
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
		
		// Initialize storage with retry
		var store *storage.PostgresStore
		for {
			var err error
			store, err = storage.NewPostgresStore(cfg.DatabaseURL)
			if err != nil {
				slog.Error("Failed to initialize storage, retrying in 30s", "error", err)
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
		
		// Initialize RAG service with retry
		var ragService *services.RAGService
		for {
			ragService = services.NewRAGService(cfg.OpenAIAPIKey, store, embeddingService)
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
		
		// Initialize background jobs with retry
		var embeddingProcessor *jobs.EmbeddingProcessor
		for {
			embeddingProcessor = jobs.NewEmbeddingProcessor(store, embeddingService)
			if embeddingProcessor == nil {
				slog.Error("Failed to initialize embedding processor, retrying in 30s")
				time.Sleep(30 * time.Second)
				continue
			}
			break
		}
		
		// Initialize Slack handler with retry
		var slackHandler *handlers.SlackHandler
		for {
			slackHandler = handlers.NewSlackHandler(cfg.SlackBotToken, store, ragService)
			if slackHandler == nil {
				slog.Error("Failed to initialize Slack handler, retrying in 30s")
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
		
		// Initialize Slab handler with retry
		var slabHandler *handlers.SlabHandler
		for {
			slabHandler = handlers.NewSlabHandler(cfg.SlabWebhookSecret, store, embeddingService)
			if slabHandler == nil {
				slog.Error("Failed to initialize Slab handler, retrying in 30s")
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
			Store:               store,
			EmbeddingService:    embeddingService,
			RAGService:          ragService,
			EmbeddingProcessor:  embeddingProcessor,
			SlackHandler:        slackHandler,
			SlabHandler:         slabHandler,
			QueryHandler:        queryHandler,
			Config:              cfg,
		}
	}
}

func main() {
	// Setup structured logging
	logging.SetupLogger()
	
	slog.Info("Starting KnowThis application", slog.String("version", "1.0.0"))
	
	// Initialize all services with retry logic (includes config validation)
	services := initializeServices()
	defer services.Store.Close()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background jobs
	go services.EmbeddingProcessor.Start(ctx)
	
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
	
	// Webhook routes with rate limiting
	webhookRouter := router.PathPrefix("/webhook").Subrouter()
	webhookRouter.Use(middleware.WebhookRateLimitMiddleware())
	webhookRouter.HandleFunc("/slab", services.SlabHandler.HandleWebhook).Methods("POST")
	
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
	
	// Stop embedding processor
	services.EmbeddingProcessor.Stop()
	
	// Shutdown server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}
	
	slog.Info("Server exited gracefully")
}