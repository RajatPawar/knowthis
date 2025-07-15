package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status_code"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "knowthis_http_request_duration_seconds",
			Help: "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// Slack metrics
	SlackMessagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_slack_messages_processed_total",
			Help: "Total number of Slack messages processed",
		},
		[]string{"channel", "status"},
	)

	SlackMentions = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "knowthis_slack_mentions_total",
			Help: "Total number of Slack mentions received",
		},
	)

	// Slab metrics
	SlabWebhooksReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_slab_webhooks_received_total",
			Help: "Total number of Slab webhooks received",
		},
		[]string{"event_type", "status"},
	)

	// Storage metrics
	DocumentsStored = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_documents_stored_total",
			Help: "Total number of documents stored",
		},
		[]string{"source", "status"},
	)

	EmbeddingGenerations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_embedding_generations_total",
			Help: "Total number of embedding generations",
		},
		[]string{"status"},
	)

	EmbeddingGenerationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "knowthis_embedding_generation_duration_seconds",
			Help: "Duration of embedding generation in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	// RAG metrics
	QueriesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_queries_processed_total",
			Help: "Total number of queries processed",
		},
		[]string{"status"},
	)

	QueryDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "knowthis_query_duration_seconds",
			Help: "Duration of query processing in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	AnthropicAPICalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_anthropic_api_calls_total",
			Help: "Total number of Anthropic API calls",
		},
		[]string{"status"},
	)

	AnthropicAPICallDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "knowthis_anthropic_api_call_duration_seconds",
			Help: "Duration of Anthropic API calls in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	// OpenAI metrics
	OpenAIAPICalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_openai_api_calls_total",
			Help: "Total number of OpenAI API calls",
		},
		[]string{"status"},
	)

	OpenAIAPICallDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "knowthis_openai_api_call_duration_seconds",
			Help: "Duration of OpenAI API calls in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	// Database metrics
	DatabaseConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "knowthis_database_connections",
			Help: "Number of active database connections",
		},
	)

	DatabaseOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "knowthis_database_operations_total",
			Help: "Total number of database operations",
		},
		[]string{"operation", "status"},
	)

	DatabaseOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "knowthis_database_operation_duration_seconds",
			Help: "Duration of database operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// Application metrics
	DocumentsWithoutEmbeddings = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "knowthis_documents_without_embeddings",
			Help: "Number of documents without embeddings",
		},
	)

	TotalDocuments = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "knowthis_total_documents",
			Help: "Total number of documents in the system",
		},
	)
)