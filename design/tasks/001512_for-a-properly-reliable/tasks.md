# Implementation Tasks

## Config & Types

- [ ] Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` fields to `Anthropic` struct in `api/pkg/config/config.go` with `envconfig` tags (`ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION` default `global`, `ANTHROPIC_VERTEX_CREDENTIALS_FILE`)
- [ ] Add startup validation: if both `ANTHROPIC_API_KEY` and `ANTHROPIC_VERTEX_PROJECT_ID` are set, fail with a clear error message
- [ ] Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` fields to `ProviderEndpoint` struct in `api/pkg/types/provider.go` (with GORM column tags and `json:"...,omitempty"`)
- [ ] Add corresponding optional pointer fields to `UpdateProviderEndpoint` struct in `api/pkg/types/provider.go`
- [ ] GORM AutoMigrate will pick up the new columns automatically — verify with a local test

## Vertex Proxy Logic

- [ ] Create `api/pkg/anthropic/vertex.go` with:
  - Google OAuth2 `TokenSource` initialization (from credentials file or Application Default Credentials)
  - `vertexTransformRequest(r *http.Request, projectID, region string)` — reads body, extracts `model`, rewrites URL path to `/v1/projects/{project}/locations/{region}/publishers/anthropic/models/{model}:{rawPredict|streamRawPredict}`, injects `anthropic_version` into body if missing, removes `model` from body
  - Helper to compute Vertex base URL from region (`global` → `https://aiplatform.googleapis.com/`, otherwise `https://{region}-aiplatform.googleapis.com/`)
  - Thread-safe `TokenSource` cache (`sync.Map`) keyed by credentials file path, for DB-configured endpoints with different credentials
- [ ] Update `Proxy` struct in `anthropic_proxy.go` to hold an optional default `oauth2.TokenSource` (for env-var-configured built-in provider) and the `sync.Map` cache for per-endpoint token sources
- [ ] Update `anthropicAPIProxyDirector` to detect Vertex mode (endpoint has `VertexProjectID` set) and call Vertex transform + set `Authorization: Bearer <token>` instead of `x-api-key`
- [ ] Update `New()` constructor in `anthropic_proxy.go` to accept Vertex config and initialize the default token source at startup (when `ANTHROPIC_VERTEX_PROJECT_ID` is set)

## Wiring

- [ ] Update `getBuiltInProviderEndpoint` in `api/pkg/server/anthropic_api_proxy_handler.go` to populate `VertexProjectID`, `VertexRegion`, and `VertexCredentialsFile` on the endpoint when Vertex env vars are configured (and skip requiring `ANTHROPIC_API_KEY`)
- [ ] Update proxy initialization in server startup to pass Vertex config to `anthropic.New()` when `ANTHROPIC_VERTEX_PROJECT_ID` is set
- [ ] Update `updateProviderEndpoint` in `api/pkg/server/provider_handlers.go` to copy Vertex fields from `UpdateProviderEndpoint` to the existing endpoint on update

## Docker Compose (SaaS deployment path)

- [ ] Add `ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION`, `ANTHROPIC_VERTEX_CREDENTIALS_FILE` env var passthrough to `docker-compose.dev.yaml`
- [ ] Add the same env var passthrough to `docker-compose.yaml`
- [ ] Add optional volume mount for Google service account credentials file in both compose files

## Helm Chart (k8s deployments)

- [ ] Add `vertexProjectID`, `vertexRegion`, `vertexCredentialsSecret`, `vertexCredentialsSecretKey` fields to `charts/helix-controlplane/values.yaml` under `controlplane.providers.anthropic`
- [ ] Update `charts/helix-controlplane/templates/controlplane-deployment.yaml` to set `ANTHROPIC_VERTEX_*` env vars and mount the credentials secret as a volume when configured
- [ ] Update `charts/helix-controlplane/values-example.yaml` with commented Vertex configuration example

## Tests

- [ ] Add unit tests in `api/pkg/anthropic/vertex_test.go` for URL rewriting logic (non-streaming → `rawPredict`, streaming → `streamRawPredict`, `global` region URL, body transformation, `anthropic_version` injection)
- [ ] Add unit test for mutual exclusivity validation (both API key and Vertex project ID set → error)
- [ ] Add integration test or manual test procedure: configure Vertex credentials, send a request through the proxy, verify response and billing logging

## SaaS Deployment

- [ ] Create GCP service account with Vertex AI User role in the `helixml` project
- [ ] Generate service account key JSON and store securely on the SaaS Docker host
- [ ] Deploy updated Docker Compose config with Vertex env vars and credentials mount on SaaS (`app.helix.ml`)
- [ ] Verify: send Anthropic API requests through SaaS, confirm they route through Vertex (check API logs for Vertex URL), confirm billing records are correct