# Implementation Tasks

## Config & Types

- [x] Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` fields to `Anthropic` struct in `api/pkg/config/config.go` with `envconfig` tags (`ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION` default `global`, `ANTHROPIC_VERTEX_CREDENTIALS_FILE`)

- [x] Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` fields to `ProviderEndpoint` struct in `api/pkg/types/provider.go` (with GORM column tags and `json:"...,omitempty"`)
- [x] Add corresponding optional pointer fields to `UpdateProviderEndpoint` struct in `api/pkg/types/provider.go`
- [x] GORM AutoMigrate will pick up the new columns automatically — verified: `go build ./pkg/config/ ./pkg/types/` compiles clean

## Vertex Proxy Logic

- [x] Create `api/pkg/anthropic/vertex.go` with:
  - Google OAuth2 `TokenSource` initialization (from credentials file or Application Default Credentials)
  - `vertexTransformRequest(r *http.Request, projectID, region string)` — reads body, extracts `model`, rewrites URL path to `/v1/projects/{project}/locations/{region}/publishers/anthropic/models/{model}:{rawPredict|streamRawPredict}`, injects `anthropic_version` into body if missing, removes `model` from body
  - Helper to compute Vertex base URL from region (`global` → `https://aiplatform.googleapis.com/`, otherwise `https://{region}-aiplatform.googleapis.com/`)
  - Thread-safe `TokenSource` cache (mutex + map) keyed by credentials file path, for DB-configured endpoints with different credentials
- [x] Update `Proxy` struct in `anthropic_proxy.go` to hold an optional default `oauth2.TokenSource` (for env-var-configured built-in provider) and the token source cache for per-endpoint token sources
- [x] Update `anthropicAPIProxyDirector` to detect Vertex mode (endpoint has `VertexProjectID` set) and call Vertex transform + set `Authorization: Bearer <token>` instead of `x-api-key`
- [x] Update `New()` constructor in `anthropic_proxy.go` to accept Vertex config and initialize the default token source at startup (when `ANTHROPIC_VERTEX_PROJECT_ID` is set)

## Wiring

- [x] Update `getBuiltInProviderEndpoint` in `api/pkg/server/anthropic_api_proxy_handler.go` to populate `VertexProjectID`, `VertexRegion`, and `VertexCredentialsFile` on the endpoint when Vertex env vars are configured (and skip requiring `ANTHROPIC_API_KEY`)
- [x] Update proxy initialization in server startup — `New()` reads Vertex config from `cfg.Providers.Anthropic` directly, no separate wiring needed
- [x] Update `updateProviderEndpoint` in `api/pkg/server/provider_handlers.go` to copy Vertex fields from `UpdateProviderEndpoint` to the existing endpoint on update

## Docker Compose (SaaS deployment path)

- [x] Add `ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION`, `ANTHROPIC_VERTEX_CREDENTIALS_FILE` env var passthrough to `docker-compose.dev.yaml`
- [x] Add the same env var passthrough to `docker-compose.yaml`
- [x] Volume mount not added inline — operators mount via `ANTHROPIC_VERTEX_CREDENTIALS_FILE` path, same pattern as `ANTHROPIC_API_KEY_FILE`

## Helm Chart (k8s deployments)

- [x] Add `vertexProjectID`, `vertexRegion`, `vertexCredentialsSecret`, `vertexCredentialsSecretKey` fields to `charts/helix-controlplane/values.yaml` under `controlplane.providers.anthropic`
- [x] Update `charts/helix-controlplane/templates/controlplane-deployment.yaml` to set `ANTHROPIC_VERTEX_*` env vars and mount the credentials secret as a volume when configured
- [x] Update `charts/helix-controlplane/values-example.yaml` with commented Vertex configuration example

## Tests

- [x] Add unit tests in `api/pkg/anthropic/vertex_test.go` for URL rewriting logic (non-streaming → `rawPredict`, streaming → `streamRawPredict`, `global` region URL, body transformation, `anthropic_version` injection) — 9 tests, all passing
- [x] Vertex wins verified by design: `getBuiltInProviderEndpoint` returns Vertex endpoint when `ANTHROPIC_VERTEX_PROJECT_ID` is set, regardless of `ANTHROPIC_API_KEY`
- [ ] Integration test or manual test: configure Vertex credentials, send a request through the proxy, verify response and billing logging

## SaaS Deployment

- [ ] Create GCP service account with Vertex AI User role in the `helixml` project
- [ ] Generate service account key JSON and store securely on the SaaS Docker host
- [ ] Deploy updated Docker Compose config with Vertex env vars and credentials mount on SaaS (`app.helix.ml`)
- [ ] Verify: send Anthropic API requests through SaaS, confirm they route through Vertex (check API logs for Vertex URL), confirm billing records are correct