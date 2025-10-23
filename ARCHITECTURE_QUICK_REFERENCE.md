# Helix Architecture - Quick Reference for SOC 2

## Core Components Map

```
┌─────────────────────────────────────────────────────────────────┐
│                     HELIX PLATFORM ARCHITECTURE                 │
└─────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│ USER TIER                                                         │
├──────────────────────────────────────────────────────────────────┤
│  Frontend (React 18 + TypeScript on port 8081)                  │
│  ├─ Dashboard                                                     │
│  ├─ Agent Builder                                                 │
│  ├─ Knowledge Management                                          │
│  ├─ Chat/Session Interface                                        │
│  └─ Admin Panel                                                    │
└────────────────────┬─────────────────────────────────────────────┘
                     │ HTTP/WebSocket
                     │ API calls
                     ▼
┌──────────────────────────────────────────────────────────────────┐
│ API & ORCHESTRATION TIER (Control Plane)                         │
├──────────────────────────────────────────────────────────────────┤
│ Helix API Server (Go 1.24, port 8080)                           │
│ ├─ REST API (RESTful endpoints)                                 │
│ ├─ WebSocket Handler (real-time updates)                        │
│ ├─ Authentication (Keycloak + JWT)                              │
│ ├─ Authorization (Multi-level RBAC)                             │
│ ├─ Session Manager                                              │
│ ├─ File Storage Handler                                         │
│ ├─ Billing/Wallet Manager                                       │
│ ├─ Trigger Orchestrator                                         │
│ ├─ App/Agent Lifecycle Manager                                  │
│ ├─ Knowledge Manager                                            │
│ └─ Scheduler (GPU job distribution)                             │
└────────────┬──────────────────────────────────────────┬──────────┘
             │                                          │
             │ NATS PubSub                             │ HTTP/WS
             │                                          │
    ┌────────▼──────────┐              ┌────────────────▼────┐
    │ Message Queue     │              │ External Services   │
    │ NATS JetStream    │              ├────────────────────┤
    │ (port 4222)       │              │ Keycloak (OIDC)    │
    └───────┬───────────┘              │ Stripe (Billing)   │
            │                          │ Slack/Discord      │
            │                          │ GitHub OAuth       │
    ┌───────▼──────────────┐           │ Azure DevOps       │
    │ Runner Communication │           │ Crisp Chat         │
    │                      │           │ OpenAI/Anthropic   │
    └──────────────────────┘           │ Together AI, etc.  │
                                       └────────────────────┘

            ┌──────────────────────────────┐
            │ GPU/Inference Tier           │
            ├──────────────────────────────┤
            │ Runner Instances             │
            │ ├─ vLLM (model serving)     │
            │ ├─ Model Scheduler           │
            │ ├─ CUDA/GPU Support          │
            │ └─ Helix Diffusers           │
            └──────────────────────────────┘

            ┌──────────────────────────────┐
            │ Processing Services          │
            ├──────────────────────────────┤
            │ ├─ Haystack (RAG, py)       │
            │ ├─ Tika (doc extraction)    │
            │ ├─ Chrome (web crawl)       │
            │ ├─ Searxng (meta search)    │
            │ └─ GPTScript Runner         │
            └──────────────────────────────┘

│                                                                   │
└──────────────────────┬────────────────────────────────────────┬──┘
                       │                                        │
                       │ SQL                                    │
                       ▼                                        ▼
        ┌──────────────────────────┐        ┌──────────────────────────┐
        │ PostgreSQL (port 5432)   │        │ pgvector (port 5433)     │
        ├──────────────────────────┤        ├──────────────────────────┤
        │ Primary Data Store       │        │ Vector Embeddings        │
        │ ├─ Sessions              │        │ ├─ Document vectors     │
        │ ├─ Interactions          │        │ ├─ Embedding search     │
        │ ├─ Users/Orgs            │        │ └─ Similarity search    │
        │ ├─ Apps/Agents           │        └──────────────────────────┘
        │ ├─ Knowledge Base        │
        │ ├─ LLM Calls History     │        ┌──────────────────────────┐
        │ ├─ Wallets/Billing       │        │ Typesense (port 8108)    │
        │ ├─ API Keys              │        ├──────────────────────────┤
        │ └─ Secrets               │        │ Full-Text Search Index   │
        │                          │        │ └─ Document indexing    │
        │ Keycloak Database:       │        └──────────────────────────┘
        │ └─ User identities       │
        └──────────────────────────┘        ┌──────────────────────────┐
                                            │ File Storage             │
            ┌────────────────────────────┐  ├──────────────────────────┤
            │ Caching Layer              │  │ Local FS or GCS         │
            ├────────────────────────────┤  │ ├─ Documents            │
            │ Ristretto (in-memory)      │  │ ├─ Attachments         │
            │ └─ Models, Sessions        │  │ ├─ Avatars             │
            └────────────────────────────┘  │ └─ Datasets            │
                                            └──────────────────────────┘
```

## Service Port Reference

| Service | Port | Protocol | Environment |
|---------|------|----------|-------------|
| Control Plane API | 8080 | HTTP/WebSocket | All |
| Frontend Dev | 8081 | HTTP | Dev only |
| PostgreSQL Primary | 5432 | TCP/SQL | All |
| pgvector Database | 5433 | TCP/SQL | Dev/Optional |
| Keycloak | 8080 | HTTP | All |
| NATS Server | 4222 | TCP | All |
| NATS WebSocket | 8433 | WebSocket | Optional |
| Typesense Search | 8108 | HTTP/REST | Important |
| Haystack RAG | 8000 | HTTP/Python | Optional |
| Chrome Automation | 7317 | HTTP | Optional |
| Tika Extraction | 9998 | HTTP | Optional |
| SearXNG Search | 8112 | HTTP | Optional |

## Data Flow Architecture

### Inference Flow
```
User Query (Frontend)
    ↓
Control Plane API
    ↓
Authentication (Keycloak)
    ↓
Session/Context Lookup (PostgreSQL)
    ↓
RAG Context Retrieval (pgvector/Typesense)
    ↓
Trigger Scheduler
    ↓
Job Queue (NATS)
    ↓
GPU Runner Selection (Scheduler)
    ↓
Model Execution (vLLM on Runner)
    ↓
Response Processing
    ↓
LLM Call Logging (PostgreSQL)
    ↓
Billing Update (Stripe)
    ↓
WebSocket Response to Frontend
```

### Knowledge Management Flow
```
Document Upload (Frontend)
    ↓
File Storage (Local/GCS)
    ↓
Text Extraction (Tika/Firecrawl)
    ↓
Chunking & Processing (Haystack)
    ↓
Embedding Generation (VLLM/OpenAI)
    ↓
Vector Storage (pgvector)
    ↓
Indexing (Typesense)
    ↓
Search Ready for RAG
```

## Critical Data Elements

### User Data
- **Stored In**: PostgreSQL (users table)
- **Sensitive**: Passwords, API keys, OAuth tokens
- **Access**: Keycloak authentication required
- **Scope**: Per-organization multi-tenancy

### Session/Conversation Data
- **Stored In**: PostgreSQL (interactions, sessions)
- **Contents**: Chat history, system prompts, responses
- **Retention**: Configurable, user-deletable
- **Privacy**: User/Organization scoped

### Knowledge/Documents
- **Stored In**: File storage (Local FS or GCS) + pgvector (embeddings)
- **Indexing**: Typesense full-text search
- **Access Control**: Organization/user level
- **Lifecycle**: Manual + auto-cleanup

### Billing/Usage
- **Stored In**: PostgreSQL (wallets, llm_calls)
- **Integration**: Stripe webhooks
- **Tracking**: Token counts, duration, costs
- **Reporting**: Aggregated metrics

### Secrets/Credentials
- **Stored In**: PostgreSQL (secrets table, encrypted)
- **Types**: API keys, OAuth tokens, custom secrets
- **Access**: Secrets API with proper auth
- **Scope**: User/organization level

## Authentication & Authorization

### Auth Chain
```
1. User Login → Keycloak OIDC Provider
2. JWT Token Generation (RS256 signed)
3. API Requests with JWT in headers
4. Token Validation by API middleware
5. Authorization checks (RBAC)
   - Organization membership?
   - Role permissions?
   - Resource ownership?
```

### Authorization Levels
- **Organization Level**: Admin, Member
- **Team Level**: Owner, Editor, Viewer
- **Resource Level**: Owner, Shared/Public, Private
- **Feature Level**: Based on subscription

## Deployment Options

### Docker Compose (Single Machine)
```bash
./stack up  # Starts all services with networking
```
- Production config: `docker-compose.yaml`
- Dev config: `docker-compose.dev.yaml`
- Runner config: `docker-compose.runner.yaml`

### Kubernetes (Distributed)
```bash
helm install helix ./charts/helix-controlplane/
helm install runner ./charts/helix-runner/
```
- High availability setup
- Auto-scaling support
- Persistent volumes for data
- RBAC policies

## Environment Variable Groups

### Core Configuration
- `INFERENCE_PROVIDER`: helix, openai, together
- `POSTGRES_HOST/PASSWORD`: Database connection
- `SERVER_URL`: Public API endpoint
- `LOG_LEVEL`: debug, info, warn, error

### LLM Providers
- `OPENAI_API_KEY`: OpenAI access
- `ANTHROPIC_API_KEY`: Claude access
- `TOGETHER_API_KEY`: Together AI access
- `VLLM_BASE_URL`: Self-hosted LLM

### Storage & RAG
- `FILESTORE_TYPE`: fs or gcs
- `FILESTORE_LOCALFS_PATH`: Local storage
- `RAG_DEFAULT_PROVIDER`: typesense, haystack, pgvector
- `RAG_TYPESENSE_URL`: Search backend

### Auth & Security
- `KEYCLOAK_URL`: OIDC provider
- `RUNNER_TOKEN`: Runner authentication
- `STRIPE_SECRET_KEY`: Billing system
- `SSL_CERT_FILE`: TLS certificates

### Third-party Integrations
- `GITHUB_INTEGRATION_*`: GitHub OAuth
- `SLACK_ENABLED`: Slack triggers
- `DISCORD_BOT_TOKEN`: Discord integration
- `STRIPE_WEBHOOK_*`: Payment webhooks

## For SOC 2 Documentation

### Key Areas to Audit
1. **Access Control**: Keycloak OIDC, JWT validation, RBAC matrix
2. **Data Protection**: Encryption (TLS, at-rest), key management
3. **Change Management**: CI/CD pipeline (Drone), deployment process
4. **Logging & Monitoring**: Sentry, structured logs, metrics
5. **Third-party Risk**: Stripe, GitHub, Slack data handling
6. **Incident Response**: Error handling, failure scenarios, recovery
7. **Data Retention**: Cleanup policies, backup strategies
8. **Network Security**: Network segmentation, firewall rules
9. **Availability**: Redundancy, failover, disaster recovery
10. **Compliance**: Regulatory requirements, data locality

### Critical Paths for Testing
1. User authentication flow (Keycloak OIDC)
2. Inference request (API → Runner → Response)
3. Knowledge upload and indexing
4. Billing event processing (API → Stripe → Wallet)
5. Trigger execution (External event → App execution)
6. Multi-tenancy isolation (User A cannot see User B data)

---

**Document Generated**: 2025-10-23
**Helix Repository**: `/home/priya/helix/helix`
**Architecture Scope**: All production-deployable services and components
