# Requirements: Anthropic via Google Vertex AI Provider

## Background

Helix currently routes Anthropic API calls through a reverse proxy (`api/pkg/anthropic/anthropic_proxy.go`) that rewrites URLs and injects `x-api-key` headers before forwarding to `https://api.anthropic.com`. This works but creates a single point of failure on Anthropic's direct API.

Google Vertex AI hosts Claude models with the same pricing but different auth (Google OAuth2 instead of API keys) and different URL patterns (`https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/anthropic/models/{model}:streamRawPredict`). Adding Vertex as an explicit Anthropic backend gives us redundancy and potentially better reliability.

The Anthropic Go SDK (`v1.12.0+`, currently in `go.mod`) already has built-in Vertex support via `github.com/anthropics/anthropic-sdk-go/vertex`, which handles URL rewriting, OAuth2 token injection, and body transformation (adding `anthropic_version`, rewriting paths, extracting model from body into URL).

## User Stories

### US-1: Operator configures Anthropic-via-Vertex as a provider
As a Helix operator, I want to configure Vertex AI as the backend for Anthropic models so that API calls go through Google Cloud instead of directly to Anthropic.

**Acceptance Criteria:**
- New env vars: `ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION` (default `us-east5`), and `ANTHROPIC_VERTEX_CREDENTIALS_FILE` (path to Google service account JSON; if unset, uses Application Default Credentials)
- When `ANTHROPIC_VERTEX_PROJECT_ID` is set, the Anthropic proxy uses Vertex AI instead of direct Anthropic API
- `ANTHROPIC_API_KEY` and `ANTHROPIC_VERTEX_PROJECT_ID` are mutually exclusive — if both set, fail at startup with a clear error
- All existing Anthropic model names (e.g. `claude-sonnet-4`, `claude-opus-4`) continue to work unchanged from the client perspective (Zed, API consumers)

### US-2: Transparent to downstream clients
As a Zed IDE user or API consumer, I should not need to change anything when the operator switches from direct Anthropic to Vertex AI.

**Acceptance Criteria:**
- The Anthropic proxy endpoint (`/v1/messages`, streaming, token counting) behaves identically from the client's perspective
- Billing/usage logging continues to work — model names, token counts, costs all still recorded
- Error responses are still Anthropic-format (Vertex returns Anthropic-native responses via `rawPredict`)

### US-3: SaaS deployment
As the Helix SaaS operator, I want to deploy Vertex AI as the Anthropic backend on `app.helix.ml`.

**Acceptance Criteria:**
- Helm chart updated with optional Vertex configuration fields
- Google Cloud service account credentials mountable as a Kubernetes secret
- Deployed and verified on SaaS before closing this task

## Out of Scope

- Failover/fallback between direct Anthropic and Vertex (future work)
- Exposing Vertex as a user-selectable provider in the UI (it's an operator-level backend choice)
- Changing how Claude Code subscription mode works (that bypasses the proxy entirely)