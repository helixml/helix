# Design: Anthropic via Google Vertex AI Provider

## Architecture Overview

The change is confined to the Anthropic reverse proxy layer in the Helix API server. Instead of always forwarding requests to `https://api.anthropic.com`, the proxy will optionally forward them to Google Vertex AI's Anthropic endpoint using Google OAuth2 credentials.

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
┌─────────────────────────────────────┐
│  Anthropic Proxy                     │
│  anthropicAPIProxyDirector           │
│                                      │
│  if Vertex configured:               │
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

**At startup:**
- Load credentials via `google.FindDefaultCredentials(ctx)` or from a specific file via `google.CredentialsFromJSON(ctx, jsonBytes, scopes...)`
- Store the `oauth2.TokenSource` on the `Proxy` struct

**Per-request (in director):**
- Call `tokenSource.Token()` to get a valid (auto-refreshed) token
- Set `Authorization: Bearer <token>` header

The `TokenSource` returned by the Google auth libraries is safe for concurrent use and handles refresh internally.

### Decision 3: Mutual exclusivity with direct Anthropic API key

When `ANTHROPIC_VERTEX_PROJECT_ID` is set, the proxy operates in Vertex mode. `ANTHROPIC_API_KEY` must NOT be set simultaneously (ambiguous routing). The config validation at startup enforces this.

The `Anthropic` config struct gets three new fields:
```go
// In config.go, type Anthropic struct:
VertexProjectID      string `envconfig:"ANTHROPIC_VERTEX_PROJECT_ID"`
VertexRegion         string `envconfig:"ANTHROPIC_VERTEX_REGION" default:"global"`
VertexCredentialsFile string `envconfig:"ANTHROPIC_VERTEX_CREDENTIALS_FILE"` // path to service account JSON; empty = ADC
```

### Decision 4: ProviderEndpoint for Vertex

The built-in provider endpoint (returned by `getBuiltInProviderEndpoint` in `anthropic_api_proxy_handler.go`) currently constructs a `ProviderEndpoint` with `BaseURL` and `APIKey`. For Vertex mode:
- `BaseURL` is set to the Vertex URL (`https://{region}-aiplatform.googleapis.com/`)
- `APIKey` is left empty (auth handled by OAuth2 token in the director)
- A new field or marker on `ProviderEndpoint` indicates Vertex mode so the director knows to do the URL/body transformation

**Approach:** Add a `VertexProjectID` and `VertexRegion` field to `ProviderEndpoint`. When these are non-empty, the director performs Vertex transformation. This keeps the proxy self-contained and avoids global state.

```go
// In types/provider.go, type ProviderEndpoint struct:
VertexProjectID string `json:"vertex_project_id,omitempty" gorm:"column:vertex_project_id"`
VertexRegion    string `json:"vertex_region,omitempty" gorm:"column:vertex_region"`
```

### Decision 5: Helm chart changes

The Helm chart (`charts/helix-controlplane/values.yaml`) gets new optional fields under `controlplane.providers.anthropic`:

```yaml
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
- **Built-in providers:** `getBuiltInProviderEndpoint()` in `anthropic_api_proxy_handler.go` creates a `ProviderEndpoint` from env vars for when no DB-configured endpoint exists. This is where Vertex config gets injected.
- **Billing/logging:** The `ModifyResponse` handler parses Anthropic response bodies to extract usage (tokens). Vertex responses via `rawPredict` return standard Anthropic response format, so no changes needed to billing.
- **The SDK version in go.mod is `v1.12.0`** — the `vertex` package exists in this version. We don't use the SDK client directly but do reference its transformation logic.

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/config/config.go` | Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` to `Anthropic` struct |
| `api/pkg/anthropic/anthropic_proxy.go` | Add Vertex-aware director logic: URL rewriting, body transformation, OAuth2 Bearer token |
| `api/pkg/anthropic/vertex.go` | New file: Vertex URL/body transformation + Google OAuth2 token source initialization |
| `api/pkg/types/provider.go` | Add `VertexProjectID`, `VertexRegion` fields to `ProviderEndpoint` |
| `api/pkg/server/anthropic_api_proxy_handler.go` | Update `getBuiltInProviderEndpoint` to populate Vertex fields when configured |
| `api/pkg/server/server.go` (or wherever proxy is initialized) | Pass Vertex token source to proxy constructor when configured; validate mutual exclusivity |
| `charts/helix-controlplane/values.yaml` | Add Vertex config fields |
| `charts/helix-controlplane/templates/controlplane-deployment.yaml` | Add Vertex env vars and credentials volume mount |
| `docker-compose.dev.yaml` | Add Vertex env var passthrough |
| `api/pkg/anthropic/anthropic_proxy_test.go` | Add tests for Vertex URL rewriting and body transformation |

## Risks

- **Token refresh latency:** First request after token expiry may be slightly slower (~100ms for refresh). Acceptable.
- **Vertex API differences:** Vertex uses `rawPredict`/`streamRawPredict` which should return native Anthropic responses. If there are subtle response format differences, billing parsing could break. Mitigate with integration testing.
- **Region selection:** Wrong region = higher latency or missing model availability. Default `global` routes through Google's global endpoint (`https://aiplatform.googleapis.com/`). Operators can override to a specific region if needed.