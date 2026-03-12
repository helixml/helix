# Implementation Tasks

## Config & Types

- [ ] Add `VertexProjectID`, `VertexRegion`, `VertexCredentialsFile` fields to `Anthropic` struct in `api/pkg/config/config.go` with `envconfig` tags (`ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION` default `global`, `ANTHROPIC_VERTEX_CREDENTIALS_FILE`)
- [ ] Add startup validation: if both `ANTHROPIC_API_KEY` and `ANTHROPIC_VERTEX_PROJECT_ID` are set, fail with a clear error message
- [ ] Add `VertexProjectID` and `VertexRegion` fields to `ProviderEndpoint` struct in `api/pkg/types/provider.go`

## Vertex Proxy Logic

- [ ] Create `api/pkg/anthropic/vertex.go` with:
  - Google OAuth2 `TokenSource` initialization (from credentials file or Application Default Credentials)
  - `vertexTransformRequest(r *http.Request, projectID, region string)` — reads body, extracts `model`, rewrites URL path to `/v1/projects/{project}/locations/{region}/publishers/anthropic/models/{model}:{rawPredict|streamRawPredict}`, injects `anthropic_version` into body if missing, removes `model` from body
  - Helper to compute Vertex base URL from region (`global` → `https://aiplatform.googleapis.com/`, otherwise `https://{region}-aiplatform.googleapis.com/`)
- [ ] Update `Proxy` struct in `anthropic_proxy.go` to hold an optional `oauth2.TokenSource` and Vertex config (project ID, region)
- [ ] Update `anthropicAPIProxyDirector` to detect Vertex mode (endpoint has `VertexProjectID` set) and call Vertex transform + set `Authorization: Bearer <token>` instead of `x-api-key`
- [ ] Update `New()` constructor in `anthropic_proxy.go` to accept Vertex config and initialize the token source at startup

## Wiring

- [ ] Update `getBuiltInProviderEndpoint` in `api/pkg/server/anthropic_api_proxy_handler.go` to populate `VertexProjectID` and `VertexRegion` on the endpoint when Vertex is configured (and skip requiring `ANTHROPIC_API_KEY`)
- [ ] Update proxy initialization in server startup to pass Vertex config to `anthropic.New()` when `ANTHROPIC_VERTEX_PROJECT_ID` is set
- [ ] Add `ANTHROPIC_VERTEX_PROJECT_ID`, `ANTHROPIC_VERTEX_REGION`, `ANTHROPIC_VERTEX_CREDENTIALS_FILE` passthrough to `docker-compose.dev.yaml`

## Tests

- [ ] Add unit tests in `api/pkg/anthropic/vertex_test.go` for URL rewriting logic (non-streaming → `rawPredict`, streaming → `streamRawPredict`, `global` region URL, body transformation)
- [ ] Add unit test for mutual exclusivity validation (both API key and Vertex project ID set → error)
- [ ] Add integration test or manual test procedure: configure Vertex credentials, send a request through the proxy, verify response and billing logging

## Helm Chart

- [ ] Add `vertexProjectID`, `vertexRegion`, `vertexCredentialsSecret`, `vertexCredentialsSecretKey` fields to `charts/helix-controlplane/values.yaml` under `controlplane.providers.anthropic`
- [ ] Update `charts/helix-controlplane/templates/controlplane-deployment.yaml` to set `ANTHROPIC_VERTEX_*` env vars and mount the credentials secret as a volume when configured
- [ ] Update `charts/helix-controlplane/values-example.yaml` with commented Vertex configuration example

## SaaS Deployment

- [ ] Create GCP service account with Vertex AI User role in the `helixml` project
- [ ] Generate service account key JSON and store as a Kubernetes secret in the SaaS cluster
- [ ] Deploy updated Helm chart with Vertex config to SaaS (`app.helix.ml`)
- [ ] Verify: send Anthropic API requests through SaaS, confirm they route through Vertex (check logs for Vertex URL), confirm billing records are correct