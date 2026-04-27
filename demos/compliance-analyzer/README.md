# GDPR Privacy Policy Compliance Analyzer

A demo app that uses Helix's RAG (Retrieval-Augmented Generation) capabilities to analyze privacy policy documents against GDPR requirements.

Upload a privacy policy PDF or text file, and the app creates a Helix agent with RAG knowledge, indexes the document, then evaluates it against 10 key GDPR articles — producing a compliance matrix showing what's covered, partially addressed, or missing.

## Prerequisites

- Node.js 18+
- A Helix account with an API key (from [app.helix.ml](https://app.helix.ml) or a self-hosted instance)

## Setup

1. Install dependencies:

```bash
cd demos/compliance-analyzer
npm install
```

2. Create a `.env` file with your Helix credentials:

```bash
cp .env.example .env
# Edit .env with your values
```

```env
VITE_HELIX_URL=https://app.helix.ml
VITE_HELIX_API_KEY=your-api-key-here
```

| Variable | Description | Default |
|----------|-------------|---------|
| `VITE_HELIX_URL` | Helix instance URL | `http://localhost:8080` |
| `VITE_HELIX_API_KEY` | Your Helix API key | (required) |
| `VITE_HELIX_PROVIDER` | LLM provider | `anthropic` |
| `VITE_HELIX_MODEL` | Model to use | `claude-sonnet-4-6` |

3. Start the dev server:

```bash
npm run dev
```

The app runs at `http://localhost:5173`.

## How It Works

1. **Upload** — Drop a privacy policy document (PDF, TXT, or MD). The app creates a new Helix agent and indexes the document as RAG knowledge.

2. **Analysis** — Once indexing completes, the app sends all 10 GDPR requirements to the agent in a single batch. The agent searches the indexed document via RAG and evaluates each requirement.

3. **Results** — A compliance matrix shows the status of each GDPR article. Click any row to see the detailed AI analysis.

### Architecture

```
Browser  →  Vite Dev Proxy  →  Helix API (SaaS or self-hosted)
                                    ↓
                              Creates app with RAG knowledge
                              Uploads & indexes document
                              Evaluates via /api/v1/sessions/chat
```

The Vite dev proxy avoids CORS issues by forwarding all `/api/*` requests to the configured Helix instance. The target is determined by the `VITE_HELIX_URL` env var.

## Sample Data

A sample privacy policy is included in `sample-data/` for testing.
