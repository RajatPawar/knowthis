# KnowThis - Production Deployment Guide

Complete step-by-step guide to deploy your KnowThis knowledge bot to production.

## 🚀 Quick Start (15 minutes to production)

### Prerequisites
- GitHub account
- Railway account
- Supabase account
- Slack workspace admin access
- OpenAI API key

---

## Step 1: Database Setup on Supabase

### 1.1 Create Supabase Project
1. Go to [supabase.com](https://supabase.com)
2. Click "Start your project"
3. Sign in with GitHub
4. Click "New Project"
5. Choose your organization
6. Fill in:
   - **Name**: `knowthis-db`
   - **Database Password**: Generate a strong password (save it!)
   - **Region**: Choose closest to your users
   - **Plan**: Free tier is fine for MVP
7. Click "Create new project"
8. Wait ~2 minutes for database to be ready

### 1.2 Enable pgvector Extension
1. In your Supabase dashboard, go to "SQL Editor"
2. Click "New query"
3. Run this command:
```sql
CREATE EXTENSION IF NOT EXISTS vector;
```
4. Click "Run" to execute

### 1.3 Get Database Connection String
1. Go to "Settings" → "Database"
2. Scroll down to "Connection string"
3. Copy the "URI" connection string
4. Replace `[YOUR-PASSWORD]` with your database password
5. Save this connection string - you'll need it later

Example:
```
postgresql://postgres:[YOUR-PASSWORD]@db.abcdefghijklmnop.supabase.co:5432/postgres
```

---

## Step 2: Slack Bot Setup

### 2.1 Create Slack App
1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Click "Create New App"
3. Choose "From scratch"
4. Fill in:
   - **App Name**: `KnowThis Bot`
   - **Workspace**: Select your workspace
5. Click "Create App"

### 2.2 Configure Socket Mode
1. In your app settings, go to "Socket Mode"
2. Toggle "Enable Socket Mode" to ON
3. Enter token name: `knowthis-socket`
4. Click "Generate"
5. **Copy the App-Level Token** (starts with `xapp-`) - save it!

### 2.3 Configure OAuth & Permissions
1. Go to "OAuth & Permissions"
2. Scroll to "Scopes" → "Bot Token Scopes"
3. Add these scopes:
   ```
   app_mentions:read
   channels:history
   groups:history
   im:history
   mpim:history
   chat:write
   ```
4. Click "Install to Workspace"
5. Click "Allow"
6. **Copy the Bot User OAuth Token** (starts with `xoxb-`) - save it!

### 2.4 Enable Events
1. Go to "Event Subscriptions"
2. Toggle "Enable Events" to ON
3. Subscribe to these bot events:
   - `app_mention`
4. Click "Save Changes"

---

## Step 3: Get API Keys

### 3.1 OpenAI API Key
1. Go to [platform.openai.com](https://platform.openai.com)
2. Sign in to your account
3. Go to "API Keys"
4. Click "Create new secret key"
5. Name it "KnowThis Bot"
6. **Copy the API key** - save it!

**Note**: This single OpenAI API key will be used for both:
- **Embeddings**: Converting text to vectors for semantic search
- **Chat Completions**: Generating intelligent responses using GPT-4o Mini

---

## Step 4: Deploy to Railway

### 4.1 Install Railway CLI
```bash
npm install -g @railway/cli
```

### 4.2 Login to Railway
```bash
railway login
```
This opens your browser - sign in with GitHub.

### 4.3 Deploy Your App
```bash
# Navigate to your project directory
cd knowthis

# Initialize Railway project
railway init

# Choose "Deploy from GitHub repo"
# Select your GitHub repository
# Choose "Deploy now"
```

### 4.4 Set Environment Variables
```bash
# Database
railway variables set DATABASE_URL="postgresql://postgres:[YOUR-PASSWORD]@db.abcdefghijklmnop.supabase.co:5432/postgres"

# Slack tokens
railway variables set SLACK_BOT_TOKEN="xoxb-your-bot-token"
railway variables set SLACK_APP_TOKEN="xapp-your-app-token"

# OpenAI API key (used for both embeddings and chat completions)
railway variables set OPENAI_API_KEY="sk-your-openai-key"

# Optional: Slab webhook secret (if using Slab)
railway variables set SLAB_WEBHOOK_SECRET="your-webhook-secret"

# Production settings
railway variables set ENVIRONMENT="production"
railway variables set LOG_FORMAT="json"
railway variables set LOG_LEVEL="INFO"
```

### 4.5 Get Your App URL
```bash
railway status
```
Your app URL will be shown (e.g., `https://knowthis-production-1234.up.railway.app`)

---

## Step 5: Configure Slack Event Subscriptions

### 5.1 Set Request URL
1. Back in your Slack app settings, go to "Event Subscriptions"
2. In "Request URL", enter: `https://your-railway-app.up.railway.app/slack/events`
3. Wait for verification (green checkmark)
4. Click "Save Changes"

### 5.2 Install Bot to Workspace
1. Go to "Install App"
2. Click "Reinstall to Workspace"
3. Click "Allow"

---

## Step 6: Test Your Bot

### 6.1 Test in Slack
1. Go to any channel in your Slack workspace
2. Type: `@KnowThis Bot hello`
3. The bot should respond with: "👍 Got it! I've processed and stored the messages."

### 6.2 Test Query API
```bash
curl -X POST https://your-railway-app.up.railway.app/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "What did we discuss about the project?"}'
```

---

## Step 7: Optional - Slab Integration

### 7.1 Configure Slab Webhook
1. In your Slab workspace, go to Settings → Integrations
2. Add new webhook
3. Set URL: `https://your-railway-app.up.railway.app/webhook/slab`
4. Set events: `post.published`, `post.updated`, `comment.created`, `comment.updated`
5. Set secret key (use the same value as `SLAB_WEBHOOK_SECRET`)

---

## Step 8: Monitoring & Maintenance

### 8.1 Check Application Health
- Health check: `https://your-railway-app.up.railway.app/health`
- Ready check: `https://your-railway-app.up.railway.app/ready`
- Metrics: `https://your-railway-app.up.railway.app/metrics`

### 8.2 View Logs
```bash
railway logs
```

### 8.3 Monitor Usage
1. Railway dashboard shows CPU, memory, and network usage
2. Supabase dashboard shows database usage
3. OpenAI dashboard shows API usage and costs
4. Anthropic dashboard shows API usage and costs

---

## 🎯 Your Bot is Now Live!

### What it does:
- ✅ Listens for @mentions in Slack
- ✅ Captures conversation context (threads or last 15 messages)
- ✅ Stores messages with deduplication
- ✅ Generates embeddings for semantic search using OpenAI
- ✅ Processes Slab posts and comments (if configured)
- ✅ Answers questions via `/api/query` endpoint
- ✅ Uses OpenAI GPT-4o Mini for intelligent responses

### Usage:
1. **Slack**: Mention `@KnowThis Bot` in any channel
2. **API**: POST to `/api/query` with `{"query": "your question"}`
3. **Slab**: Automatically processes posts and comments

---

## 📊 Cost Breakdown

### Monthly costs (estimates):
- **Railway**: $5/month (Starter plan)
- **Supabase**: $0/month (Free tier, up to 500MB)
- **OpenAI**: ~$10-30/month (embeddings + chat completions, depends on usage)

**Total**: ~$15-35/month for full production setup

---

## 🔧 Troubleshooting

### Common Issues:

**1. "Database connection failed"**
- Check DATABASE_URL format
- Ensure pgvector extension is enabled
- Verify Supabase database is running

**2. "Slack verification failed"**
- Check SLACK_BOT_TOKEN starts with `xoxb-`
- Check SLACK_APP_TOKEN starts with `xapp-`
- Verify bot is installed in workspace

**3. "OpenAI API error"**
- Check OPENAI_API_KEY format
- Verify API key has sufficient credits
- Check OpenAI API status
- Ensure you have access to GPT-4o Mini model

### Getting Help:
- Check Railway logs: `railway logs`
- Check app health: `https://your-app.up.railway.app/health`
- Check metrics: `https://your-app.up.railway.app/metrics`

---

## 🚀 Next Steps

1. **Test thoroughly** with real conversations
2. **Monitor costs** via API dashboards
3. **Scale up** Railway plan if needed
4. **Add team members** to Slack workspace
5. **Customize responses** by modifying the system prompt in `internal/services/rag.go`

---

## 📈 Scaling Considerations

### When to scale:
- **Railway**: Move to Pro plan ($20/month) for more resources
- **Supabase**: Move to Pro plan ($25/month) for more database storage
- **OpenAI Usage**: Monitor costs and set up billing alerts (both embeddings and chat completions)

### Performance optimization:
- Monitor `/metrics` endpoint for bottlenecks
- Adjust embedding batch size via environment variables
- Consider adding Redis caching for frequently accessed data

---

**🎉 Congratulations! Your KnowThis bot is now running in production and ready to help your team capture and search organizational knowledge.**