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

## Phase 5 — Web service runtime + controller

- [ ] Add `SandboxRuntimeWebService = "web-service"` to `api/pkg/types/sandbox.go`; add `Purpose` field to `Sandbox`.
- [ ] Add `web-service` runtime entry to default `RuntimeRegistry` (`api/pkg/sandbox/runtimes.go`): image `ubuntu:22.04` with build tools, persistent, no idle TTL reaping.
- [ ] Extend `sandbox.Controller.Create` to accept `Purpose=web-service` and enforce one-per-project (atomic check + insert).
- [ ] In runner-side hydra, after container starts for a web-service sandbox: clone primary repo at requested SHA into `/workspace`, install dependencies via `.helix/startup.sh`, supervise (restart on exit). Stream log output to API via existing mechanism.

## Phase 6 — Redeploy orchestration

- [ ] `api/pkg/webservice/controller.go`: `Redeploy(ctx, projectID, sha)` — creates `web_service_deploys` row (status=pending), provisions new sandbox with that SHA, polls `http://<container_ip>:<port>/` for up to 90s, on success atomically updates `active_sandbox_id` and marks deploy `live`, stops the previous sandbox; on failure marks `failed` and leaves the previous active.

## Phase 7 — Webhook trigger

- [ ] New `TriggerKind = "web-service-deploy"`.
- [ ] Auto-create one such trigger when `enabled=true` is first set on a project; auto-delete when disabled.
- [ ] In `api/pkg/server/webhook_trigger_handlers.go`, dispatch this trigger kind on pushes where the head matches the primary repo's default branch; call `webservice.Redeploy(projectID, sha)`.

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

- [ ] Add `web-service` tab to `frontend/src/pages/ProjectSettings.tsx`.
- [ ] `<WebServiceTab>`: enable toggle, default URL display, custom-domain list with per-row verification badge + add/remove, container-port input, "Deploy now" button, deploys table (SHA, time, status, log link).
- [ ] `<SharePreviewSection>` in session detail page: list current preview URLs, add/rotate/revoke controls.

## Phase 11 — Regenerate + tests + docs

- [ ] `./stack update_openapi`.
- [ ] Unit tests: share-token format + entropy, reserve helper, middleware dispatch, store CRUD, slug allocator collision handling, redeploy state machine (success / health-fail / cutover-fail).
- [ ] Docs: env vars (`DEV_SUBDOMAIN`), required DNS (wildcard `A`/`CNAME`), Caddy snippet for `passthrough` mode, sample project `.helix/startup.sh`.

---

## Explicitly deferred (real follow-up PRs)

- certmagic embedded `auto` TLS mode (passthrough ships this PR).
- RevDial `TARGET <port>\n` handshake + runner host-allowlist + connection pooling + configurable proxy buffer — current revdial already supports per-device tunnels; arbitrary-port targeting can ship through the existing hydra HTTP proxy path for v1.
- Agent `deploy_web_service` MCP tool — manual deploy via API endpoint covers the use case for v1.
- Integration tests requiring real sandbox boot loops in CI.
