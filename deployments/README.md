# Deployment Guide

This directory contains deployment configurations for various platforms.

## Platform Options

### 1. Railway (Recommended for MVP)
**Pros:**
- Automatic SSL/TLS
- Built-in PostgreSQL with pgvector
- Git-based deployments
- Simple environment management
- Good for rapid prototyping

**Setup:**
```bash
# Install Railway CLI
npm install -g @railway/cli

# Login and deploy
railway login
railway link
railway up
```

**Environment Variables to Set:**
```bash
railway variables set SLACK_BOT_TOKEN=xoxb-your-token
railway variables set SLACK_APP_TOKEN=xapp-your-token
railway variables set SLAB_WEBHOOK_SECRET=your-secret
railway variables set OPENAI_API_KEY=your-key
railway variables set ANTHROPIC_API_KEY=your-key
```

### 2. Render (Alternative)
**Pros:**
- Free tier available
- PostgreSQL with extensions
- Auto-scaling
- Good monitoring

**Setup:**
1. Connect GitHub repository
2. Use `render.yml` configuration
3. Set environment variables in dashboard

### 3. Docker Compose (Local/VPS)
**Pros:**
- Full control
- Includes monitoring stack
- Local development
- Cost-effective on VPS

**Setup:**
```bash
# Copy environment file
cp .env.example .env
# Edit .env with your values

# Start services
docker-compose -f deployments/docker-compose.yml up -d
```

### 4. Kubernetes (Production)
**Pros:**
- Highly scalable
- Production-ready
- Advanced features
- Multi-environment support

**Setup:**
```bash
# Apply configurations
kubectl apply -f deployments/kubernetes.yml

# Update secrets
kubectl create secret generic knowthis-secrets \
  --from-literal=database-url="postgres://..." \
  --from-literal=slack-bot-token="xoxb-..." \
  --from-literal=slack-app-token="xapp-..." \
  --from-literal=slab-webhook-secret="..." \
  --from-literal=openai-api-key="..." \
  --from-literal=anthropic-api-key="..."
```

## Recommendation for Production

### For MVP/Startup (Quick to Market):
**Railway** is the best choice because:
- Fastest time to deployment
- Built-in database with pgvector extension
- Automatic SSL and domain management
- Simple scaling
- Cost-effective for small teams

### For Enterprise/Scale:
**Kubernetes** with managed services:
- AWS EKS + RDS (PostgreSQL with pgvector)
- GCP GKE + Cloud SQL
- Azure AKS + PostgreSQL

## Database Requirements

All platforms need PostgreSQL with pgvector extension:
- **Railway**: Built-in support
- **Render**: Extension available
- **Docker**: Uses `pgvector/pgvector:pg16` image
- **Kubernetes**: Requires managed DB with pgvector

## Monitoring Setup

All deployments include:
- Prometheus metrics at `/metrics`
- Health checks at `/health`
- Readiness checks at `/ready`
- Structured logging (JSON in production)

## Security Considerations

1. **API Keys**: Store in environment variables, never in code
2. **HTTPS**: All platforms provide SSL/TLS
3. **Rate Limiting**: Configured in middleware
4. **HMAC Verification**: For Slab webhooks
5. **Input Validation**: In all handlers

## Cost Estimates

### Railway (Recommended for MVP):
- **Starter Plan**: $5/month
- **PostgreSQL**: $5/month
- **Total**: ~$10/month for MVP

### Render:
- **Starter Plan**: $7/month
- **PostgreSQL**: $7/month
- **Total**: ~$14/month

### AWS (Kubernetes):
- **EKS**: $70/month
- **RDS**: $20-50/month
- **Load Balancer**: $15/month
- **Total**: ~$105-135/month

### VPS (Docker Compose):
- **DigitalOcean**: $6-20/month
- **Hetzner**: $4-15/month
- **Total**: ~$10-35/month

## Scaling Considerations

### Railway:
- Vertical scaling: Up to 8GB RAM, 8 vCPU
- Horizontal scaling: Limited
- Database: Managed PostgreSQL scales automatically

### Kubernetes:
- Horizontal Pod Autoscaler
- Vertical Pod Autoscaler
- Database read replicas
- Multiple availability zones

## Next Steps

1. **Choose Platform**: Start with Railway for MVP
2. **Set Environment Variables**: Use platform-specific methods
3. **Configure Webhooks**: Set up Slab webhook endpoint
4. **Set Up Monitoring**: Connect to Prometheus/Grafana
5. **Test Integrations**: Verify Slack and Slab connections
6. **Monitor Usage**: Track API costs and performance