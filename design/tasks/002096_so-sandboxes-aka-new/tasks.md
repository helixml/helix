# Implementation Tasks: Project Web Service Hosting on Sandboxes

## Scope decision (in this PR)

User wants to host **actual customer websites and web applications** on
this — so project web service hosting must be production-ready in this PR,
not a follow-up. The existing `p{port}-{session_id}` scheme has no
consumers and is deleted entirely.

**Production-priority order:**
1. Data model + vhost router (TLS via passthrough; `auto`/certmagic in
   follow-up if not done here)
2. Web service controller — sandbox provisioning, redeploy orchestration
3. Webhook trigger on primary-repo default-branch push
4. Custom domain CRUD + verification
5. Frontend `WebServiceTab`
6. Dev preview tokens (smaller add-on once vhost + reserve helpers exist)
7. Tests + docs

**TLS posture for v1:** `passthrough` only. Operator runs Caddy (or similar)
in front with a wildcard DNS-01 cert for `*.<DEV_SUBDOMAIN base>`. The
`auto` (embedded certmagic) mode is documented but shipping it is a
follow-up — the data plane and routing work independently of where TLS
terminates.

---

## Phase 1 — Cleanup of obsolete scheme

- [x] Delete `api/pkg/server/session_expose_handlers.go`.
- [x] Delete `api/pkg/server/subdomain_proxy.go` and `subdomain_proxy_test.go`.
- [x] Remove `exposedPortManager` field, `initExposedPortManager()` call, `NewSubdomainProxyMiddleware` wiring, and the `/sessions/{id}/proxy/{port}` + `/sessions/{id}/expose` routes from `api/pkg/server/server.go`.

## Phase 2 — Data model

- [x] `vhost_routes` migration: id, hostname (unique, lowercased), target_kind, target_id, port, is_default, verified_at, verification_token, created_at, rotated_at.
- [x] `project_web_service_state` migration: project_id (PK), enabled, container_port (default 8080), active_sandbox_id (nullable), updated_at.
- [x] `web_service_deploys` migration: id, project_id, sandbox_id, commit_sha, status, started_at, finished_at, log_path.
- [x] `types.VHostRoute`, `types.VHostTargetKind`, `types.ProjectWebServiceState`, `types.WebServiceDeploy`.
- [x] Store CRUD: `CreateVHostRoute`, `GetVHostRouteByHostname`, `ListVHostRoutesByTarget`, `DeleteVHostRoute`, `DeleteVHostRoutesByTarget`, `RotateVHostRouteHostname`, `UpsertProjectWebServiceState`, `GetProjectWebServiceState`, `SetActiveWebServiceSandbox`, `CreateWebServiceDeploy`, `UpdateWebServiceDeploy`, `ListWebServiceDeploys`.

## Phase 3 — vhost helpers (new `api/pkg/vhost/` package)

- [x] `sharetoken.go`: ~190 adjectives × ~225 nouns + 8-hex `crypto/rand` → `share-<adj>-<noun>-<8hex>`.
- [x] `reserve.go`: walks every label, rejects canonical hostname (from `SERVER_URL`), aliases, `DEV_SUBDOMAIN` apex, built-in reserved labels + sub-of-reserved, operator extras, `share-` prefix unless caller is minter, existing rows.
- [x] `slug.go`: `AllocateDefaultSubdomain` with collision suffixing; `MintShareHostname` loops generate + reserve.
- [x] Unit tests pass: share format, 5k uniqueness, reserve rejection table, slug normalisation.

## Phase 4 — Middleware + proxy handler

- [x] `api/pkg/server/vhost_middleware.go`: dispatches canonical hostname → main mux; `share-*` prefix under `<DEV_SUBDOMAIN base>` → `vhost_routes` (preview) lookup; any other host → `vhost_routes` (project_web_service) lookup; falls through to main mux for unknown hosts.
- [x] `api/pkg/server/vhost_proxy.go`: shared `proxyToContainer(w, r, sandboxID, hydraContainerID, port, path)` over RevDial. Used by both sandbox-preview and project-web-service dispatch.
- [x] Project-web-service dispatch loads `project_web_service_state`, uses `active_sandbox_id` as both RevDial device and hydra container ID, returns 503 when no active deploy.
- [x] Cache deferred — first cut hits store on each request; cache + pubsub invalidation can land in follow-up if profiling shows it's hot.

> **Wait** — most things marked DONE below were actually built. The
> `DEFERRED` -> `DONE` rename above corrects the section headers from
> when those phases were scoped out. See per-task checkboxes for the
> real status.

## Phase 5 — Web service runtime + controller

- [x] Reused existing `SandboxRuntimeHeadlessUbuntu` runtime — no new runtime kind needed for v1. The bootstrap script is what makes it a web service, not the image.
- [x] `Persistent=true, TimeoutSeconds=-1` on the deploy primitive keeps web service sandboxes long-lived and out of the TTL reaper.
- [x] Runner-side workload "supervisor" is the user's `.helix/startup.sh` running as a detached exec inside the headless sandbox via the existing hydra exec API. No new runner code; the user's script becomes the long-running process.

## Phase 6 — Redeploy orchestration

- [x] `api/pkg/webservice/controller.go`: `Redeploy(ctx, DeployRequest)` provisions a fresh sandbox, polls until status=running, execs a bootstrap shell that clones the repo + checks out the requested SHA + runs `.helix/startup.sh`, waits briefly for the app to bind, atomically updates `active_sandbox_id`, marks the deploy live, marks prior live deploys superseded, and stops the previous sandbox.
- [x] Asynchronous: the API endpoint returns the pending deploy row immediately; the long-running orchestration runs in a goroutine. UI polls deploy status.
- [x] Failures leave the previous sandbox active and record an error message on the failed deploy row; the new sandbox stays running for debug exec.
- [x] `POST /api/v1/projects/:id/web-service/deploy {commit_sha?}` endpoint that triggers it.

## Phase 7 — Webhook auto-deploy

- [x] `GitHTTPServer.SetOnDefaultBranchPush` hook fires after every successful receive-pack that touched the repo's default branch.
- [x] The API server installs a hook that calls `webservice.Controller.Redeploy` on every project whose primary repository is the pushed repo AND has web service enabled. New store method `ListEnabledWebServiceProjectsByRepo` joins `projects + project_web_service_states`.
- [ ] External-webhook `TriggerKind = "web-service-deploy"` — deferred. The manual `POST /web-service/deploy` endpoint is what GitHub Actions / GitLab CI / external webhooks call today.

## Phase 8 — Project web service API

- [x] `PUT /api/v1/projects/:id/web-service` — sets `enabled` + `container_port`. Toggling on pre-seeds the default subdomain via `AllocateDefaultSubdomain`. Toggling off removes all `vhost_routes` rows for the project.
- [x] `GET /api/v1/projects/:id/web-service` — returns state, domain list, recent deploys.
- [x] `POST /api/v1/projects/:id/web-service/active-sandbox` — operator-driven "manual deploy": point the project at a specific sandbox. Records a deploy row. (Auto-deploy on push is the deferred follow-up; this primitive is what the orchestrator will call too.)
- [x] `POST /api/v1/projects/:id/web-service/domains {hostname}` — insert custom domain row (verified_at=null) with a fresh verification token. `DELETE .../domains/{domain_id}`.
- [x] `/.well-known/helix-domain-verify/:token` endpoint returning the token in plain text. (Cron-based verifier poller is deferred — the endpoint is sufficient for manual verification flow today.)

## Phase 9 — Dev preview tokens (add-on)

- [x] `POST /api/v1/sessions/:id/preview-tokens {port}` — mints `share-…` token row. `GET` lists, `POST .../:token_id/rotate` rotates, `DELETE .../:token_id` removes.
- [x] Session-delete cleanup deletes preview rows for the session.
- [ ] (Sandbox `sbx_*` mirror endpoints — deferred.)

## Phase 10 — Frontend

- [x] Add `web-service` to `ProjectSettingsSidebar` (globe icon).
- [x] `<WebServiceTab>` component: enable toggle, container port + Save, domains list with Open/Copy/Delete per row + Verified/Pending status chips, Add domain form with DNS verification hint, Deploy now button, active sandbox display, recent deploys table.
- [x] Uses the regenerated typed API client (no raw fetch). React Query manages cache + invalidation.
- [ ] `<SharePreviewSection>` in session detail page — deferred. Session preview endpoints are callable via API today.

## Phase 11 — Tests + docs

- [x] Unit tests for share-token format, 5k uniqueness, every reserve rejection category, slug normalisation, parseVHostConfig matrix, stripPort IPv6.
- [x] Swagger annotations on every new handler so `update_openapi` regenerates a clean typed client.
- [x] Caddy `passthrough` snippet in `design.md` + implementation notes.
- [x] `./stack update_openapi` regenerated; `frontend/src/api/api.ts` now exposes `v1ProjectsWebService*` and `v1SessionsPreviewTokens*` methods plus `TypesVHostRoute`, `TypesProjectWebServiceState`, `TypesWebServiceDeploy`, `ServerProjectWebServiceResponse`.
- [x] End-to-end validation in helix-in-helix (see `screenshots/test-results.md`): vhost middleware proxies real HTTP traffic through RevDial → hydra → container; reserved-hostname guards reject `api.<base>` / `share-…` / canonical; verifier endpoint round-trips; UI renders the tab with verified domains.

---

## Explicitly deferred (real follow-up PRs)

- certmagic embedded `auto` TLS mode (passthrough ships this PR).
- RevDial `TARGET <port>\n` handshake + runner host-allowlist + connection pooling + configurable proxy buffer — current revdial already supports per-device tunnels; arbitrary-port targeting can ship through the existing hydra HTTP proxy path for v1.
- Agent `deploy_web_service` MCP tool — manual deploy via API endpoint covers the use case for v1.
- Integration tests requiring real sandbox boot loops in CI.
