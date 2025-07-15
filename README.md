# KnowThis - Internal Knowledge & Search MVP Bot

A Go-based bot that integrates with Slack and Slab to capture, store, and retrieve organizational knowledge using embeddings and RAG (Retrieval-Augmented Generation).

## Features

### Slack Integration
- **Socket Mode**: Real-time event processing
- **App Mentions**: Responds to @mentions in channels and threads
- **Message History**: Captures last 15 messages when mentioned
- **Deduplication**: Prevents storing duplicate messages

### Slab Integration
- **Webhook Support**: Handles post and comment events
- **HMAC Verification**: Secure webhook payload validation
- **Content Processing**: Cleans markdown for better embeddings

### Knowledge Storage
- **PostgreSQL + pgvector**: Vector database for embeddings
- **Content Deduplication**: SHA256 hash-based duplicate prevention
- **Metadata Storage**: User, channel, timestamp information

### RAG (Retrieval-Augmented Generation)
- **OpenAI Embeddings**: text-embedding-3-small for document encoding
- **Vector Search**: Cosine similarity for relevant content retrieval
- **Claude Integration**: Anthropic Claude for generating responses

## Quick Start

### Prerequisites
- Go 1.22+
- PostgreSQL with pgvector extension
- OpenAI API key
- Anthropic API key
- Slack app tokens
- Slab webhook secret

### Installation

1. Clone and build:
```bash
git clone <repository>
cd knowthis
go mod download
go build -o knowthis main.go
```

2. Set up PostgreSQL:
```bash
createdb knowthis
psql knowthis -c "CREATE EXTENSION vector;"
```

3. Configure environment variables:
```bash
export SLACK_BOT_TOKEN="xoxb-your-token"
export SLACK_APP_TOKEN="xapp-your-token"
export SLAB_WEBHOOK_SECRET="your-webhook-secret"
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export DATABASE_URL="postgres://localhost/knowthis?sslmode=disable"
export PORT="8080"
```

4. Run the bot:
```bash
./knowthis
```

## Usage

### Slack
1. Mention the bot in any channel: `@knowthis`
2. Bot will acknowledge and store the conversation
3. In threads, it captures the entire thread
4. In channels, it captures the last 15 messages

### Slab
1. Configure webhook endpoint: `https://your-domain.com/webhook/slab`
2. Bot automatically processes published posts and comments
3. Content is cleaned and stored for search

### Querying
Send POST requests to `/api/query`:
```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "How do we handle user authentication?"}'
```

Response:
```json
{
  "answer": "Based on the available context...",
  "sources": [
    {
      "id": "slack_C123_1234567890",
      "content": "We use JWT tokens for authentication...",
      "source": "slack",
      "user_name": "john.doe",
      "timestamp": "2024-01-15T10:30:00Z",
      "similarity": 0.85
    }
  ],
  "query": "How do we handle user authentication?"
}
```

## Slack Bot Setup

1. Create a Slack app at https://api.slack.com/apps
2. Enable Socket Mode and generate App Token
3. Add Bot Token Scopes:
   - `app_mentions:read`
   - `channels:history`
   - `groups:history`
   - `im:history`
   - `mpim:history`
4. Subscribe to Events:
   - `app_mention`
5. Install app to workspace

## Slab Webhook Setup

1. In Slab settings, add webhook endpoint
2. Configure events:
   - `post.published`
   - `post.updated`
   - `comment.created`
   - `comment.updated`
3. Set webhook secret for HMAC verification

## API Endpoints

- `POST /webhook/slab` - Slab webhook handler
- `POST /api/query` - RAG query endpoint
- `GET /health` - Health check

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Test specific components
go test ./internal/storage
go test ./internal/handlers
```

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    Slack    │    │    Slab     │    │  Query API  │
│   Socket    │    │  Webhook    │    │   Client    │
│    Mode     │    │             │    │             │
└─────────────┘    └─────────────┘    └─────────────┘
       │                   │                   │
       │                   │                   │
       ▼                   ▼                   ▼
┌─────────────────────────────────────────────────────┐
│                 HTTP Server                         │
│   ┌─────────────┐ ┌─────────────┐ ┌─────────────┐  │
│   │   Slack     │ │    Slab     │ │   Query     │  │
│   │  Handler    │ │  Handler    │ │  Handler    │  │
│   └─────────────┘ └─────────────┘ └─────────────┘  │
└─────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────┐
│                Services Layer                       │
│   ┌─────────────┐ ┌─────────────┐ ┌─────────────┐  │
│   │ Embedding   │ │    RAG      │ │   Other     │  │
│   │  Service    │ │  Service    │ │  Services   │  │
│   └─────────────┘ └─────────────┘ └─────────────┘  │
└─────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────┐
│              Storage Layer                          │
│   ┌─────────────────────────────────────────────┐  │
│   │         PostgreSQL + pgvector              │  │
│   │                                             │  │
│   │  ┌─────────────┐ ┌─────────────────────┐   │  │
│   │  │ Documents   │ │     Embeddings      │   │  │
│   │  │   Table     │ │    (vector data)    │   │  │
│   │  └─────────────┘ └─────────────────────┘   │  │
│   └─────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

## Contributing

1. Follow existing code patterns
2. Add tests for new functionality
3. Update documentation
4. Use context-aware timeouts (10s for API calls)
5. Wrap errors with `%w` for proper error chains

## License

MIT License