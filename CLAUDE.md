# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

KnowThis is a Go-based internal knowledge & search MVP bot that integrates with Slack and Slab to capture, store, and retrieve organizational knowledge using embeddings and RAG (Retrieval-Augmented Generation).

## Architecture

### Core Components
- **Slack Integration**: Socket Mode for real-time event processing
- **Slab Integration**: Webhook endpoint with HMAC verification
- **Storage Layer**: PostgreSQL with pgvector for embeddings
- **Embeddings**: OpenAI text-embedding-3-small
- **RAG Service**: Vector similarity search + OpenAI GPT-4o Mini for responses

### Key Technologies
- Go 1.22
- PostgreSQL with pgvector extension
- OpenAI API for embeddings and chat completions
- Slack Socket Mode API
- Slab Webhook API

## Common Development Commands

### Build and Run
```bash
go mod download
go build -o knowthis main.go
./knowthis
```

### Testing
```bash
# Run all tests
go test ./...

# Run specific test package
go test ./internal/storage
go test ./internal/handlers

# Run tests with coverage
go test -cover ./...
```

### Database Setup
```bash
# Create database with pgvector extension
createdb knowthis
psql knowthis -c "CREATE EXTENSION vector;"

# The schema is auto-created on first run
```

## Environment Variables

Required environment variables:
- `SLACK_BOT_TOKEN`: Slack bot token (xoxb-)
- `SLACK_APP_TOKEN`: Slack app token (xapp-)
- `SLAB_WEBHOOK_SECRET`: Secret for HMAC verification
- `OPENAI_API_KEY`: OpenAI API key for embeddings and chat completions
- `DATABASE_URL`: PostgreSQL connection string (defaults to localhost)
- `PORT`: HTTP server port (defaults to 8080)

## Slack Bot Setup

Required OAuth scopes:
- `app_mentions:read` - detect mentions
- `channels:history` - read channel messages
- `groups:history` - read private channel messages
- `im:history` - read DM history
- `mpim:history` - read group DM history

## API Endpoints

### Slab Webhook
- `POST /webhook/slab` - Handles Slab events with HMAC verification
- Supported events: `post.published`, `post.updated`, `comment.created`, `comment.updated`

### Query API
- `POST /api/query` - RAG query endpoint
- Request: `{"query": "your question"}`
- Response: `{"answer": "...", "sources": [...], "query": "..."}`

### Health Check
- `GET /health` - Returns 200 OK

## Storage Schema

### Documents Table
- Stores all content with deduplication via content hash
- Includes embeddings for vector similarity search
- Supports both Slack messages and Slab posts/comments

### Deduplication Strategy
- Content hash (SHA256) prevents duplicate storage
- Unique constraint on (content_hash, source, source_id)
- Updates existing documents instead of creating duplicates

## Code Patterns

### Error Handling
- All handlers return errors wrapped with `%w`
- Context-aware API calls with 10s timeout
- Graceful error logging without exposing internals

### Testing
- Unit tests for deduplication logic: `internal/storage/dedup_test.go`
- HMAC verification tests: `internal/handlers/slab_test.go`
- Use table-driven tests for multiple scenarios

### Integration Design
- Modular handlers for easy addition of new integrations
- Interface-based storage layer for flexibility
- Service layer separation for business logic

## Development Notes

### Slack Integration
- Uses Socket Mode for real-time events
- Processes app mentions and fetches conversation history
- Cleans message text by removing user/channel mentions

### Slab Integration
- Webhook with HMAC-SHA256 signature verification
- Processes posts and comments separately
- Cleans markdown formatting for better embeddings

### Embeddings Processing
- Background processing for documents without embeddings
- Batch processing with configurable limits
- OpenAI text-embedding-3-small (1536 dimensions)

### RAG Implementation
- Vector similarity search with cosine distance
- Relevance threshold filtering (>0.7 similarity)
- Context building from top relevant documents
- OpenAI GPT-4o Mini for response generation

## Production Features

✅ **Completed:**
- OpenAI GPT-4o Mini integration with proper error handling
- Structured logging with slog (JSON/text formats)
- Prometheus metrics and monitoring
- Rate limiting for API and webhook endpoints
- Background job processing for embeddings
- Configuration validation
- Graceful shutdown handling
- Docker containerization
- Multiple deployment configurations

📋 **Additional Production TODOs:**
1. Database migrations system
2. Circuit breaker for external API calls
3. Distributed tracing with OpenTelemetry
4. Authentication/authorization for API endpoints
5. Database connection pooling optimization
6. Caching layer (Redis) for frequently accessed data
7. Message queuing system for high-throughput scenarios
8. API versioning strategy
9. Comprehensive integration tests
10. Performance testing and optimization