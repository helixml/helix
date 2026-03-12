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

### Decision 3: Mutual exclusivity for built-in provider only

For the **built-in env-var provider**, `ANTHROPIC_API_KEY` and `ANTHROPIC_VERTEX_PROJECT_ID` are mutually exclusive — if both set, fail at startup with a clear error. This prevents ambiguous routing for the default provider.

For **DB-configured provider endpoints**, there is no such restriction. An org could have both a direct-Anthropic endpoint and a Vertex endpoint, with routing determined by which endpoint is selected for a given project/request.

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
| `api/pkg/server/server.go` (or wherever proxy is initialized) | Pass Vertex config to proxy constructor; validate mutual exclusivity of env vars |
| `docker-compose.dev.yaml` | Add `ANTHROPIC_VERTEX_*` env var passthrough |
| `docker-compose.yaml` | Add `ANTHROPIC_VERTEX_*` env var passthrough |
| `charts/helix-controlplane/values.yaml` | Add Vertex config fields |
| `charts/helix-controlplane/templates/controlplane-deployment.yaml` | Add Vertex env vars and credentials volume mount |
| `api/pkg/anthropic/vertex_test.go` | Tests for URL rewriting and body transformation |

## Risks

- **Token refresh latency:** First request after token expiry may be slightly slower (~100ms for refresh). Acceptable.
- **Vertex API differences:** Vertex uses `rawPredict`/`streamRawPredict` which should return native Anthropic responses. If there are subtle response format differences, billing parsing could break. Mitigate with integration testing.
- **Region selection:** Wrong region = higher latency or missing model availability. Default `global` routes through Google's global endpoint (`https://aiplatform.googleapis.com/`). Operators can override to a specific region if needed.
- **Credentials file for DB-configured endpoints:** The `vertex_credentials_file` path must be accessible inside the API container. For Docker, this means a volume mount. This is the same constraint as `api_key_file` — not a new pattern, but worth documenting.