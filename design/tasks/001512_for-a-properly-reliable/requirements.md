# Requirements: Anthropic via Google Vertex AI Provider

## Background

Helix currently routes Anthropic API calls through a reverse proxy (`api/pkg/anthropic/anthropic_proxy.go`) that rewrites URLs and injects `x-api-key` headers before forwarding to `https://api.anthropic.com`. This works but creates a single point of failure on Anthropic's direct API.

Google Vertex AI hosts Claude models with the same pricing but different auth (Google OAuth2 instead of API keys) and different URL patterns (`https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/anthropic/models/{model}:streamRawPredict`). Adding Vertex as an explicit Anthropic backend gives us redundancy and potentially better reliability.

The Anthropic Go SDK (`v1.12.0+`, currently in `go.mod`) already has built-in Vertex support via `github.com/anthropics/anthropic-sdk-go/vertex`, which handles URL rewriting, OAuth2 token injection, and body transformation (adding `anthropic_version`, rewriting paths, extracting model from body into URL).

## User Stories

### US-1: Operator configures Anthropic-via-Vertex via environment variables
As a Helix operator, I want to configure Vertex AI as the backend for Anthropic models using environment variables so that API calls go through Google Cloud instead of directly to Anthropic.

**Acceptance Criteria:**
- New env vars: `ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION` (default `global`), and `ANTHROPIC_VERTEX_CREDENTIALS_FILE` (path to Google service account JSON; if unset, uses Application Default Credentials)
- When `ANTHROPIC_VERTEX_PROJECT_ID` is set, the Anthropic proxy uses Vertex AI instead of direct Anthropic API
- `ANTHROPIC_API_KEY` and `ANTHROPIC_VERTEX_PROJECT_ID` are mutually exclusive for the built-in provider — if both set, fail at startup with a clear error
- All existing Anthropic model names (e.g. `claude-sonnet-4`, `claude-opus-4`) continue to work unchanged from the client perspective (Zed, API consumers)

### US-2: Admin configures Vertex as a provider endpoint via the UI
As a Helix admin, I want to create a global Anthropic-via-Vertex provider endpoint through the admin providers UI so that all users route through Vertex.

**Acceptance Criteria:**
- The create/update provider endpoint API accepts optional `vertex_project_id`, `vertex_region`, and `vertex_credentials_file` fields
- When a provider endpoint has `vertex_project_id` set, the proxy uses Vertex mode for requests routed to that endpoint
- The provider endpoint can be created via the API (and later through the frontend form)
- Works alongside direct-Anthropic provider endpoints — an org could have one of each (though only one is active per routing decision)

### US-3: User configures Vertex as a personal provider endpoint
As a Helix user (when custom user providers are enabled), I want to add my own Vertex-backed Anthropic provider using my own GCP project and credentials.

**Acceptance Criteria:**
- Same fields available on user-owned provider endpoints as admin endpoints (`vertex_project_id`, `vertex_region`, `vertex_credentials_file`)
- User's Vertex endpoint is used when they are the owner and routing selects it
- Credentials file must be mounted/accessible to the API container (same pattern as `api_key_file`)

### US-4: Transparent to downstream clients
As a Zed IDE user or API consumer, I should not need to change anything when the operator switches from direct Anthropic to Vertex AI.

**Acceptance Criteria:**
- The Anthropic proxy endpoint (`/v1/messages`, streaming, token counting) behaves identically from the client's perspective
- Billing/usage logging continues to work — model names, token counts, costs all still recorded
- Error responses are still Anthropic-format (Vertex returns Anthropic-native responses via `rawPredict`)

### US-5: SaaS deployment
As the Helix SaaS operator, I want to deploy Vertex AI as the Anthropic backend on `app.helix.ml`.

**Acceptance Criteria:**
- Docker Compose env var passthrough for `ANTHROPIC_VERTEX_*` vars (SaaS runs Docker, not k8s)
- Google service account credentials file mountable as a Docker volume
- Helm chart also updated with optional Vertex configuration fields (for k8s deployments)
- Deployed and verified on SaaS before closing this task

## Out of Scope

- Failover/fallback between direct Anthropic and Vertex (future work)
- Frontend UI changes to the create/update provider dialog for Vertex fields (the API supports it; UI polish is a follow-up)
- Changing how Claude Code subscription mode works (that bypasses the proxy entirely)