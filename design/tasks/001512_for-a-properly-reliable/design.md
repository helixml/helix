# Design: Anthropic via Google Vertex AI Provider

## Architecture Overview

The change is confined to the Anthropic reverse proxy layer in the Helix API server. Instead of always forwarding requests to `https://api.anthropic.com`, the proxy will optionally forward them to Google Vertex AI's Anthropic endpoint using Google OAuth2 credentials.

There are two configuration paths:
1. **Environment variables** — built-in provider for simple deployments (Docker Compose, SaaS)
2. **Database-configured provider endpoints** — admin or user creates a provider endpoint with Vertex fields via the API/UI

Both paths produce a `ProviderEndpoint` with `VertexProjectID` and `VertexRegion` populated, which the proxy director uses to decide how to transform the request.

```
Zed / API Client
    │
    │  POST /v1/messages (Anthropic native format)
    │  Header: x-api-key: <helix-user-token>
    ▼
┌─────────────────────────────┐
│  Helix API Server           │
│  anthropic_api_proxy_handler│
│  (auth, billing, balance)   │
└──────────┬──────────────────┘
           │
           ▼
┌──────────────────────────────────────┐
│  getProviderEndpoint()               │
│  1. Check DB (org, then user/global) │
│  2. Fall back to built-in from env   │
│  → returns ProviderEndpoint          │
│    (may have VertexProjectID set)    │
└──────────┬───────────────────────────┘
           │
           ▼
┌─────────────────────────────────────┐
│  Anthropic Proxy Director            │
│                                      │
│  if endpoint.VertexProjectID != "":  │
│    - Rewrite URL to Vertex format    │
│    - Use OAuth2 Bearer token         │
│    - Inject anthropic_version        │
│  else:                               │
│    - Rewrite URL to api.anthropic.com│
│    - Use x-api-key header            │
└──────────┬──────────────────────────┘
           │
           ▼
   api.anthropic.com  OR  {region}-aiplatform.googleapis.com
```

Clients (Zed, API consumers) send standard Anthropic-format requests. The proxy is the only thing that changes.

## Key Design Decisions

### Decision 1: Transform in the reverse proxy director, don't use the SDK client

The Anthropic Go SDK's `vertex` package (`vertex.WithGoogleAuth`) is designed for SDK client usage — it registers middleware on an `anthropic.Client`. Our proxy is a `httputil.ReverseProxy`, not an SDK client. We receive raw HTTP requests from downstream and forward them upstream.

**Approach:** Replicate the Vertex middleware's transformation logic directly in `anthropicAPIProxyDirector`. The transformation is simple and well-defined (see `vertex/vertex.go` in the SDK source):

1. Change base URL to `https://{region}-aiplatform.googleapis.com/` (or `https://aiplatform.googleapis.com/` if region is `global`)
2. Rewrite `/v1/messages` → `/v1/projects/{projectID}/locations/{region}/publishers/anthropic/models/{model}:{rawPredict|streamRawPredict}`
3. Extract `model` from request body, remove it from body, put it in URL
4. Inject `anthropic_version: "vertex-2023-10-16"` into body if not present
5. Set `Authorization: Bearer <google-oauth2-token>` instead of `x-api-key`

This is ~50 lines of code. Using the SDK client would require deserializing the request, calling the SDK, then re-serializing the response — far more complex and fragile for a proxy.

### Decision 2: Google OAuth2 token management

Google OAuth2 access tokens expire (typically 1 hour). We use `golang.org/x/oauth2/google` (already an indirect dependency via `cloud.google.com/go/storage`) to get a `TokenSource` that auto-refreshes.

**At startup (for env-var-configured built-in provider):**
- Load credentials via `google.FindDefaultCredentials(ctx)` or from a specific file via `google.CredentialsFromJSON(ctx, jsonBytes, scopes...)`
- Store the `oauth2.TokenSource` on the `Proxy` struct

**For DB-configured provider endpoints:**
- The `vertex_credentials_file` field on the endpoint points to a mounted credentials file
- The proxy lazily initializes and caches a `TokenSource` per unique credentials file path
- A `sync.Map` on the `Proxy` struct maps credentials file path → `TokenSource`

**Per-request (in director):**
- Look up the `TokenSource` for this endpoint's credentials file (or the default one for the built-in provider)
- Call `tokenSource.Token()` to get a valid (auto-refreshed) token
- Set `Authorization: Bearer <token>` header

The `TokenSource` returned by the Google auth libraries is safe for concurrent use and handles refresh internally.

### Decision 3: Vertex wins unconditionally

When `ANTHROPIC_VERTEX_PROJECT_ID` is set (env var or DB endpoint), all Anthropic **inference** traffic goes through Vertex. Vertex is strictly better when available — same pricing, better reliability. No mutual exclusivity validation, no precedence logic, no configuration to choose between them. Vertex is on or it's off.

`ANTHROPIC_API_KEY` is ignored for inference but is **required for model discovery** (see Decision 7). Vertex has no API to list available models, so model listing always hits `api.anthropic.com` directly. For SaaS, both `ANTHROPIC_VERTEX_PROJECT_ID` and `ANTHROPIC_API_KEY` must be set.

### Decision 4: Config changes

The `Anthropic` config struct gets three new fields:
```go
// In config.go, type Anthropic struct:
VertexProjectID       string `envconfig:"ANTHROPIC_VERTEX_PROJECT_ID"`
VertexRegion          string `envconfig:"ANTHROPIC_VERTEX_REGION" default:"global"`
VertexCredentialsFile string `envconfig:"ANTHROPIC_VERTEX_CREDENTIALS_FILE"` // path to service account JSON; empty = ADC
```

### Decision 5: ProviderEndpoint Vertex fields

Add Vertex fields to `ProviderEndpoint` (persisted in DB via GORM, exposed via API):

```go
// In types/provider.go, type ProviderEndpoint struct:
VertexProjectID       string `json:"vertex_project_id,omitempty" gorm:"column:vertex_project_id"`
VertexRegion          string `json:"vertex_region,omitempty" gorm:"column:vertex_region"`
VertexCredentialsFile string `json:"vertex_credentials_file,omitempty" gorm:"column:vertex_credentials_file"`
```

And the corresponding update struct:

```go
// In types/provider.go, type UpdateProviderEndpoint struct:
VertexProjectID       *string `json:"vertex_project_id,omitempty"`
VertexRegion          *string `json:"vertex_region,omitempty"`
VertexCredentialsFile *string `json:"vertex_credentials_file,omitempty"`
```

This means:
- Admin creates a global provider endpoint via `POST /api/v1/provider-endpoints` with `vertex_project_id`, `vertex_region`, and `vertex_credentials_file` set — requests routed to this endpoint use Vertex
- Users (when `ENABLE_CUSTOM_USER_PROVIDERS=true`) can do the same with their own GCP project credentials
- The `vertex_credentials_file` follows the same pattern as `api_key_file` — must be mounted/accessible to the API container

### Decision 6: Deployment — Docker Compose (SaaS) and Helm

**Docker Compose (SaaS uses this):**
- Pass `ANTHROPIC_VERTEX_*` env vars through `docker-compose.dev.yaml` and `docker-compose.yaml`
- Mount the Google service account JSON file as a Docker volume
- Example:
```yaml
services:
  api:
    environment:
      - ANTHROPIC_VERTEX_PROJECT_ID=${ANTHROPIC_VERTEX_PROJECT_ID:-}
      - ANTHROPIC_VERTEX_REGION=${ANTHROPIC_VERTEX_REGION:-}
      - ANTHROPIC_VERTEX_CREDENTIALS_FILE=${ANTHROPIC_VERTEX_CREDENTIALS_FILE:-}
    volumes:
      - ${GOOGLE_APPLICATION_CREDENTIALS:-/dev/null}:/run/secrets/gcp-credentials.json:ro
```

**Helm chart (for k8s deployments):**
```yaml
# values.yaml under controlplane.providers.anthropic:
anthropic:
  baseUrl: ""
  apiKey: ""
  vertexProjectID: ""
  vertexRegion: "global"
  vertexCredentialsSecret: ""        # k8s secret name containing service-account.json
  vertexCredentialsSecretKey: "key"  # key within the secret
```

When `vertexProjectID` is set, the chart mounts the credentials secret as a volume and sets `ANTHROPIC_VERTEX_CREDENTIALS_FILE` to the mount path.

### Decision 7: Model listing — Vertex has no model discovery API

**The problem:** Helix dynamically discovers Anthropic models by calling `GET /v1/models` on `api.anthropic.com`. This model list populates the Helix model cache, drives the UI model selectors, and determines what models users can configure for agents. Without it, no Anthropic models appear in Helix at all.

Vertex AI does **not** support this endpoint. The Vertex REST API for publisher models (`publishers.models`) only has a `get` method for fetching a single model by name — there is **no `list` method** to enumerate available models from a publisher. The project-scoped resource (`projects.locations.publishers.models`) only exposes inference endpoints (`rawPredict`, `streamRawPredict`, etc.) with no metadata operations at all. Google simply does not provide an API to answer "what Anthropic models are available on Vertex?"

This is an industry-wide gap. Claude Code sidesteps it by hardcoding model names via environment variables (`ANTHROPIC_MODEL`, `ANTHROPIC_DEFAULT_OPUS_MODEL`, etc.) and warns users to pin versions. Zed hardcodes models as a Rust enum. Neither calls a discovery API.

**We will NOT hardcode a list of Anthropic models into Helix.** That creates a maintenance burden — every new Anthropic model release (Opus 4.7, etc.) would require a Helix code change and redeployment. The whole point of calling `/v1/models` is to avoid exactly this.

**Approach — two tiers:**

**Tier 1: SaaS and most deployments (recommended).** Set `ANTHROPIC_API_KEY` alongside the Vertex config. Model discovery calls `GET api.anthropic.com/v1/models` using the API key — this is a metadata-only query, no user data or inference traffic touches Anthropic. All inference goes through Vertex. When Anthropic ships new models, they appear automatically. This is a **hard requirement for the Helix SaaS deployment** (`app.helix.ml`).

Concretely:
- **`listAnthropicModels`** (in `openai_client_anthropic.go`): When Vertex is configured, ignore the Vertex base URL for model listing. Always call `api.anthropic.com/v1/models` with `ANTHROPIC_API_KEY`. The existing code already does this when `baseURL` is set to the Anthropic default — we just need to force that path when Vertex is active.
- **`listModelsAnthropic`** (in `openai_model_handlers.go`, the proxy path): Same — proxy model list requests to `api.anthropic.com` regardless of Vertex config.

**Tier 2: Vertex-only deployments (no Anthropic API key).** For self-hosted operators who only have Google Cloud credentials and no Anthropic API key, we promote the existing `ANTHROPIC_MODELS` env var from a filter to also act as a **source**. Currently `ANTHROPIC_MODELS` only marks models as enabled/disabled from the list fetched via the API. When no API key is available and the API call returns nothing, `ANTHROPIC_MODELS` should be used to synthesize the model list directly. The operator explicitly declares which models they want:

```
ANTHROPIC_MODELS=claude-sonnet-4-6,claude-opus-4-6,claude-haiku-4-5
```

The change is in `listAnthropicModels`: if the API call fails or is skipped (no API key), and `ANTHROPIC_MODELS` is non-empty, return those model IDs as the model list instead of returning empty. If both the API key and `ANTHROPIC_MODELS` are missing, return an empty list — the operator has misconfigured the deployment.

This is why `ANTHROPIC_API_KEY` isn't truly "ignored" when Vertex is configured — it's still essential for model discovery. Decision 3 ("Vertex wins unconditionally") applies to **inference** only.

## Codebase Patterns Discovered

- **Provider config:** Each provider (OpenAI, Anthropic, TogetherAI, VLLM) has its own config struct in `api/pkg/config/config.go` with `envconfig` tags. New env vars follow `ANTHROPIC_VERTEX_*` naming.
- **Reverse proxy pattern:** The Anthropic proxy uses `httputil.ReverseProxy` with a custom `Director` (URL/header rewrite) and `ModifyResponse` (response parsing for billing). Vertex changes only touch the Director.
- **Built-in providers:** `getBuiltInProviderEndpoint()` in `anthropic_api_proxy_handler.go` creates a `ProviderEndpoint` from env vars when no DB-configured endpoint exists. This is where Vertex env var config gets injected.
- **DB-configured providers:** `getProviderEndpoint()` checks the DB first (org-scoped, then general), falls back to built-in. Adding Vertex fields to `ProviderEndpoint` automatically makes them available via DB config, the REST API, and eventually the frontend.
- **Provider endpoint API:** `createProviderEndpoint` and `updateProviderEndpoint` in `provider_handlers.go` decode directly into `types.ProviderEndpoint` / `types.UpdateProviderEndpoint`. Adding fields to these structs makes them available via the API with no handler changes.
- **Billing/logging:** The `ModifyResponse` handler parses Anthropic response bodies to extract usage (tokens). Vertex responses via `rawPredict` return standard Anthropic response format, so no changes needed to billing.
- **The SDK version in go.mod is `v1.12.0`** — the `vertex` package exists in this version. We don't use the SDK client directly but do reference its transformation logic.
- **SaaS runs Docker Compose**, not Kubernetes. Helm chart support is for other k8s deployments.

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/config/config.go` | Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` to `Anthropic` struct |
| `api/pkg/types/provider.go` | Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` to `ProviderEndpoint` and `UpdateProviderEndpoint` |
| `api/pkg/anthropic/vertex.go` | New file: Vertex URL/body transformation + Google OAuth2 token source initialization and caching |
| `api/pkg/anthropic/anthropic_proxy.go` | Add Vertex-aware director logic, `TokenSource` management on Proxy struct |
| `api/pkg/server/anthropic_api_proxy_handler.go` | Update `getBuiltInProviderEndpoint` to populate Vertex fields; skip requiring `ANTHROPIC_API_KEY` when Vertex is configured |
| `api/pkg/server/server.go` (or wherever proxy is initialized) | Pass Vertex config to proxy constructor when `ANTHROPIC_VERTEX_PROJECT_ID` is set |
| `docker-compose.dev.yaml` | Add `ANTHROPIC_VERTEX_*` env var passthrough |
| `docker-compose.yaml` | Add `ANTHROPIC_VERTEX_*` env var passthrough |
| `charts/helix-controlplane/values.yaml` | Add Vertex config fields |
| `charts/helix-controlplane/templates/controlplane-deployment.yaml` | Add Vertex env vars and credentials volume mount |
| `api/pkg/openai/openai_client_anthropic.go` | When Vertex is active, force model listing to hit `api.anthropic.com` directly; when no API key, synthesize list from `ANTHROPIC_MODELS` |
| `api/pkg/server/openai_model_handlers.go` | Proxy model list requests to `api.anthropic.com` regardless of Vertex config |
| `api/pkg/anthropic/vertex_test.go` | Tests for URL rewriting and body transformation |

## Risks

- **Token refresh latency:** First request after token expiry may be slightly slower (~100ms for refresh). Acceptable.
- **Vertex API differences:** Vertex uses `rawPredict`/`streamRawPredict` which should return native Anthropic responses. If there are subtle response format differences, billing parsing could break. Mitigate with integration testing.
- **Model listing not available on Vertex:** Vertex has no API to list available Anthropic models (confirmed: `publishers.models` only has `get`, no `list`). SaaS **must** set `ANTHROPIC_API_KEY` alongside Vertex config for model discovery. Vertex-only deployments without an API key must set `ANTHROPIC_MODELS` explicitly or get an empty model list. This is a platform limitation, not something we can work around — Claude Code and Zed both hardcode their model lists for the same reason.
- **Region selection:** Wrong region = higher latency or missing model availability. Default `global` routes through Google's global endpoint (`https://aiplatform.googleapis.com/`). Operators can override to a specific region if needed.
- **Credentials file for DB-configured endpoints:** The `vertex_credentials_file` path must be accessible inside the API container. For Docker, this means a volume mount. This is the same constraint as `api_key_file` — not a new pattern, but worth documenting.