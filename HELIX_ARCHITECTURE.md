# Helix Platform Architecture and Components - SOC 2 Scope

## Executive Summary

Helix is a comprehensive AI agent platform designed for on-premises deployment with complete data security and control. It consists of multiple integrated services managing inference, knowledge management, data processing, authentication, billing, and orchestration.

---

## 1. CORE SERVICES AND COMPONENTS

### 1.1 Control Plane API Server (Golang)
- **Location**: `/api/` directory
- **Language**: Go 1.24
- **Port**: 8080 (default)
- **Purpose**: Central orchestration hub managing all platform operations
- **Key Components**:
  - RESTful API (Swagger/OpenAPI documented)
  - WebSocket support for real-time communication
  - Session management
  - Interaction processing and history
  - Model management and routing
  - Authentication and authorization
  - File storage management
  - Billing and wallet management
  - RAG (Retrieval Augmented Generation) coordination
  - Trigger orchestration
  - App/Agent lifecycle management

### 1.2 Frontend UI (React + TypeScript)
- **Location**: `/frontend/` directory
- **Framework**: React 18.3.1, TypeScript, Vite
- **Port**: 8081 (development)
- **Technologies**:
  - Material-UI (MUI) components
  - Mobx state management
  - React Query for data fetching
  - Monaco Editor for code editing
  - Three.js for 3D visualization
  - Recharts for data visualization
  - Socket.io for real-time updates
- **Features**:
  - Dashboard UI
  - Agent/App builder interface
  - Knowledge management interface
  - Session/conversation UI
  - Billing and account management
  - Administration panels

### 1.3 GPU Runner Service (Golang + Python/vLLM)
- **Location**: `/runner/`, `Dockerfile.runner`
- **Purpose**: Executes LLM inference workloads on GPUs
- **Components**:
  - Helix Diffusers (image generation)
  - vLLM integration (serving LLMs efficiently)
  - Model scheduling and memory management
  - Support for CPU-only mode (development)
  - GPU support (NVIDIA CUDA)
- **Communication**: 
  - Connects to Control Plane via HTTP/WebSocket
  - Registers available models and capacity
  - Reports resource utilization

### 1.4 Kubernetes Operator
- **Location**: `/operator/` directory
- **Technology**: Kubebuilder (Go)
- **Purpose**: Manages Helix deployments on Kubernetes
- **Features**:
  - Custom Resource Definition (CRD) management
  - Automated deployment orchestration
  - Scaling control
  - Health monitoring

### 1.5 GPTScript Runner
- **Purpose**: Executes GPTScript workflows
- **Technology**: Go-based runner with GPTScript interpreter
- **Docker Image**: `registry.helixml.tech/helix/gptscript-runner:latest`
- **Concurrency**: Configurable (default 20 concurrent tasks)

### 1.6 Haystack Service (Python)
- **Location**: `/haystack_service/` directory
- **Technology**: Python, FastAPI, Uvicorn
- **Purpose**: Advanced RAG and document processing
- **Features**:
  - Embedding generation
  - Document chunking and processing
  - Vector search
  - Vision RAG support
  - Integration with pgvector
  - Model agnostic (supports external VLLM servers)
  - Socket-based communication with API
- **Port**: 8000

---

## 2. DATA STORAGE & PERSISTENCE

### 2.1 PostgreSQL Database (Primary)
- **Version**: 12.13-Alpine (production), 12.13-Alpine with pgvector extensions (development)
- **Port**: 5432
- **Database**: `postgres`, `keycloak`
- **Features**:
  - JSON/JSONB support (for flexible data structures)
  - pgvector extension for vector embeddings
  - Connection pooling (configurable: 25-50 connections)
  - Automatic migrations on startup
  - SSL support
- **Key Tables** (via GORM ORM):
  - Interactions (chat history, messages)
  - Sessions (conversation contexts)
  - Users
  - Organizations, Teams, Organization Memberships
  - Apps/Agents definitions
  - Knowledge/DataEntities (documents, files)
  - LLM Calls (inference history, usage tracking)
  - Wallets (billing/credit tracking)
  - API Keys
  - Secrets
  - Tools, Skills, Providers
  - Triggers and Trigger Executions
  - Roles, Access Grants
  - Fine-tuning Jobs
  - GPTScript Tasks

### 2.2 Vector Store - pgvector (PostgreSQL Extension)
- **Purpose**: Store and search vector embeddings
- **Database**: Separate PostgreSQL 17 instance with pgvector/VectorChord
- **Models Supported**:
  - text-embedding-3-small (OpenAI)
  - text-embedding-3-large (OpenAI)
  - Custom embeddings via external services
  - MrLight/dse-qwen2-2b-mrl-v1
- **Functionality**:
  - Similarity search
  - Approximate nearest neighbor (ANN) search
  - BM25 full-text search

### 2.3 Search Index - Typesense
- **Purpose**: Full-text and semantic search
- **API Key**: Default "typesense"
- **Port**: 8108
- **Features**:
  - Full-text search
  - Faceted search
  - Relevance ranking
  - Sub-second search
- **Data**: Knowledge/document indexing

### 2.4 File Storage
- **Types Supported**:
  - Local filesystem (development/on-premises)
  - Google Cloud Storage (GCS)
- **Configuration**:
  - `FILESTORE_TYPE`: "fs" or "gcs"
  - `FILESTORE_LOCALFS_PATH`: Default `/filestore`
  - `FILESTORE_GCS_BUCKET`: For cloud deployments
- **Stored Data**:
  - User-uploaded documents
  - Session attachments
  - User avatars
  - Knowledge base files
  - Fine-tuning datasets
  - Results/outputs

### 2.5 Message Queue - NATS
- **Technology**: NATS with JetStream
- **Port**: 4222 (NATS), 8433 (WebSocket proxy)
- **Features**:
  - Persistent message queue (JetStream enabled by default)
  - Pub/Sub messaging
  - Request/Reply pattern
  - Max payload: 32MB (configurable)
- **Use Cases**:
  - Runner communication
  - Event publishing
  - Inter-service messaging

### 2.6 Cache
- **Technology**: Ristretto (in-memory cache, Go library)
- **Purpose**: Models cache, session data caching
- **TTL**: Configurable (default 1 minute for models)

---

## 3. TECHNOLOGY STACK DETAILS

### 3.1 API Dependencies (Go Modules)
**Authentication & Authorization**:
- keycloak (v13.9.0)
- go-oidc/v3
- golang-jwt/jwt/v5
- coreos/go-oidc

**LLM Integration**:
- go-openai (v1.38.1)
- anthropic-sdk-go (v1.12.0)
- ollama/ollama (v0.11.4)
- gptscript-ai/gptscript (custom fork)
- langchaingo (custom fork)

**RAG & Vector Databases**:
- pgvector-go (v0.2.3)
- typesense-go/v2
- firecrawl-go

**External Integrations**:
- stripe-go (v76.8.0) - Billing
- discord-go (v0.28.1) - Discord integration
- slack-go/slack (v0.12.2) - Slack integration
- github/go-github (v61.0.0) - GitHub integration
- azure-devops-go-api (v7.1.0) - Azure DevOps
- jfrog/froggit-go (v1.20.1) - Git operations
- go-crisp-api (v3.0.0) - Crisp chat integration
- MCP (Model Context Protocol) support

**Data Processing**:
- go-tika (v0.3.1) - Document extraction
- go-rod (v0.116.2) - Browser automation (Chrome)
- colly/v2 - Web scraping
- go-readability - Article extraction
- html-to-markdown

**Database & ORM**:
- gorm.io/gorm (v1.30.1)
- gorm.io/driver/postgres
- lib/pq
- golang-migrate/migrate/v4

**Utilities**:
- zerolog - Structured logging
- sentry-go - Error tracking
- notify - Notifications framework
- cron/v3 - Job scheduling
- godotenv - Environment management
- uuid - Unique ID generation
- gocloud.dev - Cloud abstraction

---

## 4. EXTERNAL INTEGRATIONS

### 4.1 LLM Providers
- **OpenAI**: GPT-4, GPT-3.5, embeddings
- **Anthropic**: Claude models
- **Together AI**: Open-source models
- **Ollama**: Self-hosted models
- **Hugging Face**: Model access
- **vLLM**: Model serving infrastructure
- **Groq**: Fast LLM inference
- **Custom/Dynamic Providers**: Configurable via environment

### 4.2 Authentication & Authorization
- **Keycloak**: User management, OIDC/OpenID provider
- **OIDC**: Standard OpenID Connect integration
- **OAuth 2.0**: GitHub, Azure DevOps integration
- **JWT**: Token-based auth

### 4.3 Billing & Payments
- **Stripe**: 
  - Credit/subscription management
  - Webhook processing
  - Usage-based billing
  - Wallet/balance tracking

### 4.4 Chat/Communication Integrations
- **Slack**: 
  - Trigger source
  - Webhook integration
  - Channel posting
  - Thread management
- **Discord**: Bot integration
- **Crisp**: Live chat integration

### 4.5 Source Code Management
- **GitHub**: 
  - OAuth authentication
  - Repository cloning/analysis
  - Webhook triggers
- **Azure DevOps**: Pipeline integration
- **Bitbucket/GitLab**: Via froggit-go abstraction

### 4.6 Document Processing
- **Apache Tika**: Document type detection and text extraction
- **Firecrawl**: Web scraping and document extraction
- **Chrome/Chromium**: Browser automation for web crawling

### 4.7 Search & Analytics
- **SearXNG**: Meta search engine
- **Sentry**: Error tracking
- **Google Analytics**: Frontend analytics
- **RudderStack**: Product analytics

### 4.8 Email
- **Mailgun**: Email delivery
- **SMTP**: Standard email protocol support

---

## 5. AUTHENTICATION & AUTHORIZATION ARCHITECTURE

### 5.1 Auth Methods
- **Keycloak-based**: OIDC/OAuth 2.0
- **API Keys**: Per-user/per-app authentication
- **Service Tokens**: Runner authentication
- **JWT**: Token validation

### 5.2 Authorization Layers
- **Role-Based Access Control (RBAC)**:
  - Organization-level roles
  - Team-level permissions
  - App-level access grants
  - User-level API key scopes
- **Admin Users**: 
  - Configurable via `ADMIN_USER_IDS`
  - Source: Environment variables or JWT claims
- **Resource-Based Access**:
  - Owner-based access (User/Organization)
  - Shared sessions/apps

### 5.3 Secrets Management
- Stored encrypted in PostgreSQL
- Per-user/per-organization
- Accessible via Secrets API
- Used for API keys, credentials storage

---

## 6. DATA PROCESSING & RAG

### 6.1 Knowledge Management
- **Document Upload**: Web UI, API
- **Extraction**: Tika, Firecrawl
- **Processing**: Text chunking, processing
- **Indexing**: Typesense, pgvector (embeddings)
- **Query**: Semantic + full-text search

### 6.2 RAG Providers
- **Typesense** (default): Full-text search
- **Haystack**: Advanced RAG with embeddings
- **pgvector**: Vector similarity search
- **LlamaIndex**: Document indexing and querying

### 6.3 Embedding Services
- External VLLM servers
- OpenAI embeddings
- Together AI embeddings
- Custom embedding models
- Configurable dimensions (384, 512, 1024, 3584)

### 6.4 Web Crawling
- **Chrome Browser Pool**: Configurable size (default 5)
- **Page Pool**: Configurable (default 50)
- **Depth Limits**: Maximum crawl depth control
- **Frequency Limits**: Rate limiting per domain

---

## 7. SCHEDULER & JOB ORCHESTRATION

### 7.1 Inference Scheduling
- **Helix Scheduler**: GPU memory and task scheduling
- **Strategies**: Max spread, bin packing
- **TTLs**:
  - Model TTL: 10s (keep model warm)
  - Slot TTL: 600s (slot timeout)
  - Runner TTL: 30s (runner heartbeat)
- **Queue Size**: Buffered job queue (default 100)

### 7.2 Cron Jobs
- **Task Types**:
  - Fine-tuning jobs
  - Knowledge indexing
  - Cleanup/maintenance
  - Usage aggregation
- **Execution**: Via gocron scheduler

### 7.3 Trigger System
- **Types**:
  - Slack messages
  - Discord messages
  - HTTP webhooks
  - Cron schedules
  - Azure DevOps
  - Crisp chat
- **Execution**: Managed via trigger service
- **Persistence**: Trigger configurations and executions stored

---

## 8. INFRASTRUCTURE & DEPLOYMENT

### 8.1 Docker Compose Deployment
- **Production**: `docker-compose.yaml`
- **Development**: `docker-compose.dev.yaml`
- **Runner Standalone**: `docker-compose.runner.yaml`
- **Services**: All orchestrated with volume mounts and environment variables

### 8.2 Kubernetes Deployment
- **Helm Charts**:
  - `/charts/helix-controlplane/`: Main deployment
  - `/charts/helix-runner/`: Distributed GPU runners
  - Dependencies: PostgreSQL (Bitnami chart), common utilities
- **Templates**:
  - Deployments (Control Plane, Chat UI, Chrome, Searxng, Tika)
  - Services (ClusterIP, NodePort)
  - PersistentVolumeClaims
  - ConfigMaps
  - ServiceAccounts
  - Ingress (with TLS support)

### 8.3 Network Architecture
- **Ports**:
  - 8080: API Server
  - 8081: Frontend UI (dev)
  - 5432: PostgreSQL
  - 5433: pgvector (dev)
  - 4222: NATS
  - 8433: NATS WebSocket
  - 8108: Typesense
  - 8000: Haystack/Chrome
  - 8112: Searxng
  - 9998: Tika
- **Socket Communication**: Unix sockets for embeddings service

### 8.4 Container Images
- **Base**: Alpine for minimal footprint
- **API**: `registry.helixml.tech/helix/controlplane:latest`
- **Runner**: `registry.helixml.tech/helix/runner:*`
- **GPTScript**: `registry.helixml.tech/helix/gptscript-runner:latest`
- **Keycloak**: Custom Keycloak build
- **Haystack**: Custom Python service
- **Supporting**: Postgres, Typesense, Chrome, Tika, Searxng

---

## 9. ADVANCED FEATURES

### 9.1 Fine-Tuning
- **Support**: Model fine-tuning on custom data
- **Providers**: Together AI, Helix
- **Quotas**: Per subscription tier
- **Execution**: Async job processing

### 9.2 Apps/Agents
- **Builder UI**: Web-based agent configuration
- **YAML Support**: helix.yaml format
- **Types**: LLM apps, tool-using agents, RAG-enabled agents
- **Deployment**: Serverless execution

### 9.3 Skills & Tools
- **API Skills**: HTTP-based tool integration
- **MCP Support**: Model Context Protocol integration
- **Tool Discovery**: Dynamic tool catalog
- **Execution**: Managed by tool orchestrator

### 9.4 Streaming & Real-time
- **WebSocket Support**: Streaming inference
- **SSE**: Server-sent events for updates
- **Live Updates**: Model availability, status changes

### 9.5 Multi-tenancy
- **Organizations**: Workspace isolation
- **Teams**: Team-level access control
- **Users**: Individual accounts with roles
- **Resource Sharing**: Apps, tools can be shared

---

## 10. OPERATIONAL COMPONENTS

### 10.1 Janitor Service
- **Purpose**: Cleanup, logging, notifications
- **Features**:
  - Slack webhook integration
  - Error tracking (Sentry)
  - Google Analytics
  - RudderStack events

### 10.2 Notifications
- **Channels**: Email (Mailgun/SMTP), Slack
- **Triggers**: Fine-tuning completion, errors
- **Management**: Notification preferences per user

### 10.3 Usage Tracking
- **Metrics Collected**:
  - LLM calls (tokens, models, duration)
  - Inference costs
  - Fine-tuning data
  - API usage
- **Storage**: PostgreSQL
- **Reporting**: Aggregated metrics APIs

### 10.4 Health Monitoring
- **Keycloak**: Health endpoint monitoring
- **Services**: Docker health checks
- **Readiness**: Kubernetes readiness probes

### 10.5 Licensing
- **License Key**: Optional deployment identifier
- **Version Ping**: Phone home (can be disabled)
- **Launchpad**: Version/license management service

---

## 11. SECURITY ARCHITECTURE

### 11.1 Data in Transit
- **TLS/SSL**: Configurable certificates
- **WebSocket WSS**: Secure WebSocket support
- **Environment Variables**: Secrets configuration
- **File-based Secrets**: Docker secrets support

### 11.2 Data at Rest
- **Database**: PostgreSQL with SSL support
- **File Storage**: Local filesystem or GCS
- **Sensitive Data**: Encrypted secrets storage
- **Logs**: Structured JSON logging

### 11.3 Access Control
- **Authentication**: Keycloak + JWT
- **Authorization**: Multi-level RBAC
- **API Keys**: Scoped access tokens
- **Admin Controls**: Configuration management

### 11.4 Third-party Integrations Security
- **API Keys**: Stored as secrets
- **Webhooks**: Signed (Stripe, GitHub)
- **OAuth**: Standard flows with state management
- **TLS Verification**: Configurable (default enabled)

---

## 12. DEVELOPMENT & TESTING

### 12.1 Development Tools
- **Linting**: golangci-lint, eslint
- **Testing**: Go tests, vitest
- **Documentation**: Swagger/OpenAPI
- **Hot Reload**: Air (Go), Vite (React)

### 12.2 Integration Tests
- **Location**: `/integration-test/`
- **Browsers**: Chrome for web automation
- **Scope**: End-to-end workflow testing

---

## 13. ENVIRONMENT CONFIGURATION

Key configurable components via environment variables:
- `INFERENCE_PROVIDER`: LLM provider selection
- `RAG_DEFAULT_PROVIDER`: Search backend
- `FILESTORE_TYPE`: Storage backend
- `STRIPE_BILLING_ENABLED`: Billing system
- `GITHUB_INTEGRATION_ENABLED`: GitHub hooks
- `SLACK_ENABLED`, `DISCORD_ENABLED`, `CRISP_ENABLED`: Triggers
- `KEYCLOAK_*`: Authentication configuration
- `ADMIN_USER_IDS`: Admin user list
- `LOG_LEVEL`: Logging verbosity
- Database connection strings and credentials

---

## SUMMARY TABLE

| Component | Technology | Purpose | Criticality |
|-----------|-----------|---------|------------|
| Control Plane API | Golang 1.24 | Orchestration | Critical |
| Frontend | React 18 + TS | User Interface | Critical |
| PostgreSQL | v12.13 | Primary Data Store | Critical |
| pgvector | PostgreSQL Extension | Vector Storage | Critical |
| GPU Runner | Golang + vLLM | Inference Execution | Critical |
| Keycloak | OIDC/OAuth Provider | Authentication | Critical |
| NATS | Message Queue | Inter-service Communication | Important |
| Typesense | Search Index | Full-text Search | Important |
| Haystack | Python/FastAPI | Advanced RAG | Important |
| Chrome | Browser Automation | Web Crawling | Important |
| Kubernetes Operator | Golang/Kubebuilder | K8s Orchestration | Conditional |
| Stripe | Payment Provider | Billing | Conditional |
| Slack/Discord | External APIs | Chat Integration | Conditional |
| GitHub | External API | CI/CD Integration | Conditional |

---

## SCOPE CONSIDERATIONS FOR SOC 2

Based on this architecture, SOC 2 audit should cover:

1. **Data Security**: PostgreSQL encryption, file storage access control, secrets management
2. **Access Controls**: Keycloak OIDC, JWT validation, RBAC implementation
3. **Network Security**: TLS/SSL configuration, network segmentation
4. **Third-party Management**: Stripe, GitHub, Slack integrations and data handling
5. **Deployment Security**: Kubernetes RBAC, container security, secrets management
6. **Operational Controls**: Logging (Sentry), monitoring, incident response
7. **Change Management**: CI/CD pipeline (Drone), deployment procedures
8. **Data Retention**: User data cleanup, log retention policies
9. **Availability**: Service redundancy, failover mechanisms, backup procedures
10. **Compliance**: License compliance, data sovereignty, regulatory requirements

