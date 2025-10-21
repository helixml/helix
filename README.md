<div align="center">
<img alt="logo" src="https://helix.ml/assets/helix-logo.png" width="250px">

<br/>
<br/>

</div>

<p align="center">
  <a href="https://app.helix.ml/">SaaS</a> •
  <a href="https://docs.helixml.tech/docs/controlplane">Private Deployment</a> •
  <a href="https://docs.helixml.tech/docs/overview">Docs</a> •
  <a href="https://discord.gg/VJftd844GE">Discord</a>
</p>

# HelixML - AI Agents on a Private GenAI Stack

[👥 Discord](https://discord.gg/VJftd844GE)

**Deploy AI agents in your own data center or VPC and retain complete data security & control.**

HelixML is an enterprise-grade platform for building and deploying AI agents with support for RAG (Retrieval-Augmented Generation), API calling, vision, and multi-provider LLM support. Build and deploy LLM applications by writing a simple [`helix.yaml`](https://docs.helixml.tech/helix/develop/getting-started/) configuration file.

Our intelligent GPU scheduler packs models efficiently into available GPU memory and dynamically loads and unloads models based on demand, optimizing resource utilization.

## ✨ Key Features

### 🤖 AI Agents
- **Easy-to-use Web UI** for agent interaction and management
- **Session-based architecture** with pause/resume capabilities
- **Multi-step reasoning** with tool orchestration
- **Memory management** for context-aware interactions
- **Support for multiple LLM providers** (OpenAI, Anthropic, and local models)

<img width="1768" height="1053" alt="AI Agents Interface" src="https://github.com/user-attachments/assets/0e945ace-4f54-46a2-8d20-49485169486f" />

### 🛠️ Skills and Tools
- **REST API integration** with OpenAPI schema support
- **MCP (Model Context Protocol) server** compatibility
- **GPTScript integration** for advanced scripting
- **OAuth token management** for secure third-party access
- **Custom tool development** with flexible SDK

<img width="1767" height="1057" alt="Skills and Tools" src="https://github.com/user-attachments/assets/575330f7-cfda-4e68-acd2-31617690ae69" />

### 📚 Knowledge Management
- **Built-in document ingestion** (PDFs, Word, text files)
- **Web scraper** for automatic content extraction
- **Multiple RAG backends**: Typesense, Haystack, PGVector, LlamaIndex
- **Vector embeddings** with PGVector for semantic search
- **Vision RAG support** for multimodal content

<img width="1772" height="1055" alt="Knowledge Base" src="https://github.com/user-attachments/assets/c9112362-5f0e-4318-a648-4c478cd8d3fa" />

**Main use cases:**
- Upload and analyze corporate documents
- Add website documentation URLs to create instant customer support agents
- Build knowledge bases from multiple sources

### 🔍 Tracing and Observability
Context is everything. Agents can process tens of thousands of tokens per step—Helix provides complete visibility under the hood:

<img width="1767" height="1053" alt="Tracing Interface" src="https://github.com/user-attachments/assets/81539015-18ae-4818-b396-3d872e55907f" />

**Tracing features:**
- View all agent execution steps
- Inspect requests and responses to LLM providers, third-party APIs, and MCP servers
- Real-time token usage tracking
- Pricing and cost analysis
- Performance metrics and debugging

### 🚀 Additional Features
- **Multi-tenancy** with organization, team, and role-based access control
- **Scheduled tasks** and cron jobs
- **Webhook triggers** for event-driven workflows
- **Evaluation framework** for testing and quality assurance
- **Payment integration** with Stripe support
- **Notifications** via Slack, Discord, and email
- **Keycloak authentication** with OAuth and OIDC support

## 🏗️ Architecture

HelixML uses a microservices architecture with the following components:

```
┌─────────────────────────────────────────────────────────┐
│                      Frontend (React)                    │
│                     vite + TypeScript                    │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────┐
│                  API / Control Plane (Go)                │
│  ┌──────────────┬──────────────┬──────────────────────┐ │
│  │   Agents     │  Knowledge   │   Auth & Sessions    │ │
│  │   Skills     │  RAG Pipeline│   Organizations      │ │
│  │   Tools      │  Vector DB   │   Usage Tracking     │ │
│  └──────────────┴──────────────┴──────────────────────┘ │
└─────────┬──────────────────────────────────┬───────────┘
          │                                  │
┌─────────▼──────────┐            ┌─────────▼──────────┐
│   PostgreSQL       │            │   GPU Runners      │
│   + PGVector       │            │   Model Scheduler  │
└────────────────────┘            └────────────────────┘
          │
┌─────────▼──────────────────────────────────────────────┐
│  Supporting Services: Keycloak, Typesense, Haystack,   │
│  GPTScript Runner, Chrome/Rod, Tika, SearXNG           │
└────────────────────────────────────────────────────────┘
```

**Three-layer agent hierarchy:**
1. **Session**: Manages agent lifecycle and state
2. **Agent**: Coordinates skills and handles LLM interactions
3. **Skills**: Group related tools for specific capabilities
4. **Tools**: Individual actions (API calls, functions, scripts)

## 💻 Tech Stack

### Backend
- **Go 1.24.0** - Main backend language
- **PostgreSQL + PGVector** - Data storage and vector embeddings
- **GORM** - ORM for database operations
- **Gorilla Mux** - HTTP routing
- **Keycloak** - Identity and access management
- **NATS** - Message queue
- **Zerolog** - Structured logging

### Frontend
- **React 18.3.1** - UI framework
- **TypeScript** - Type-safe JavaScript
- **Material-UI (MUI)** - Component library
- **MobX** - State management
- **Vite** - Build tool
- **Monaco Editor** - Code editing

### AI/ML
- **OpenAI SDK** - GPT models integration
- **Anthropic SDK** - Claude models integration
- **LangChain Go** - LLM orchestration
- **GPTScript** - Scripting capabilities
- **Typesense / Haystack / LlamaIndex** - RAG backends

### Infrastructure
- **Docker & Docker Compose** - Containerization
- **Kubernetes + Helm** - Orchestration
- **Flux** - GitOps operator

## 🚀 Quick Start

### Install on Docker

Use our quickstart installer:

```bash
curl -sL -O https://get.helixml.tech/install.sh
chmod +x install.sh
sudo ./install.sh
```

The installer will prompt you before making changes to your system. By default, the dashboard will be available on `http://localhost:8080`.

For setting up a deployment with a DNS name, see `./install.sh --help` or read [the detailed docs](https://docs.helixml.tech/helix/private-deployment/controlplane/). We've documented easy TLS termination for you.

**Next steps:**
- Attach your own GPU runners per [runners docs](https://docs.helixml.tech/helix/private-deployment/controlplane/#attach-a-runner-to-an-existing-control-plane)
- Use any [external OpenAI-compatible LLM](https://docs.helixml.tech/helix/private-deployment/controlplane/#install-control-plane-pointing-at-any-openai-compatible-api)

### Install on Kubernetes

Use our Helm charts for production deployments:
- [Control Plane Helm Chart](https://docs.helixml.tech/helix/private-deployment/helix-controlplane-helm-chart/)
- [Runner Helm Chart](https://docs.helixml.tech/helix/private-deployment/helix-runner-helm-chart/)

## 🔧 Configuration

All server configuration is done via environment variables. You can find the complete list of configuration options in [`api/pkg/config/config.go`](https://github.com/helixml/helix/blob/main/api/pkg/config/config.go).

**Key environment variables:**
- `OPENAI_API_KEY` - OpenAI API credentials
- `ANTHROPIC_API_KEY` - Anthropic API credentials
- `POSTGRES_*` - Database connection settings
- `KEYCLOAK_*` - Authentication settings
- `SERVER_URL` - Public URL for the deployment
- `RUNNER_*` - GPU runner configuration

See the [configuration documentation](https://docs.helixml.tech/docs/controlplane) for detailed setup instructions.

## 👨‍💻 Development

For local development, refer to the [Helix local development guide](./local-development.md).

**Prerequisites:**
- Docker Desktop (or Docker + Docker Compose)
- Go 1.24.0+
- Node.js 18+
- Make

**Quick development setup:**

```bash
# Clone the repository
git clone https://github.com/helixml/helix.git
cd helix

# Start supporting services
docker-compose up -d postgres keycloak

# Run the backend
cd api
go run . serve

# Run the frontend (in a new terminal)
cd frontend
npm install
npm run dev
```

See [`local-development.md`](./local-development.md) for comprehensive setup instructions.

## 📖 Documentation

- **[Overview](https://docs.helixml.tech/docs/overview)** - Platform introduction
- **[Getting Started](https://docs.helixml.tech/helix/develop/getting-started/)** - Build your first agent
- **[Control Plane Deployment](https://docs.helixml.tech/docs/controlplane)** - Production deployment guide
- **[Runner Deployment](https://docs.helixml.tech/helix/private-deployment/controlplane/#attach-a-runner-to-an-existing-control-plane)** - GPU runner setup
- **[Agent Architecture](./api/pkg/agent/SPEC.md)** - Technical specification
- **[API Reference](https://docs.helixml.tech/)** - REST API documentation
- **[Contributing Guide](./CONTRIBUTING.md)** - How to contribute
- **[Upgrading Guide](./UPGRADING.md)** - Migration instructions

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](./CONTRIBUTING.md) for details.

By contributing, you confirm that:
- Your changes will fall under the same license
- Your changes will be owned by HelixML, Inc.

## 📄 License

Helix is [licensed](https://github.com/helixml/helix/blob/main/LICENSE.md) under a similar license to Docker Desktop. You can run the source code (in this repo) for free for:

- **Personal Use:** Individuals or people personally experimenting
- **Educational Use:** Schools and universities
- **Small Business Use:** Companies with under $10M annual revenue and less than 250 employees

If you fall outside of these terms, please use the [Launchpad](https://deploy.helix.ml) to purchase a license for large commercial use. Trial licenses are available for experimentation.

You are not allowed to use our code to build a product that competes with us.

### Why these license clauses?

- We generate revenue to support the development of Helix. We are an independent software company.
- We don't want cloud providers to take our open source code and build a rebranded service on top of it.

If you would like to use some part of this code under a more permissive license, please [get in touch](mailto:info@helix.ml).

## 🆘 Support

- **[Discord Community](https://discord.gg/VJftd844GE)** - Join our community for help and discussions
- **[GitHub Issues](https://github.com/helixml/helix/issues)** - Report bugs or request features
- **[Documentation](https://docs.helixml.tech/)** - Comprehensive guides and references
- **[Email](mailto:info@helix.ml)** - Contact us for commercial inquiries

## 🌟 Star History

If you find Helix useful, please consider giving us a star on GitHub!

---

Built with ❤️ by [HelixML, Inc.](https://helix.ml)
