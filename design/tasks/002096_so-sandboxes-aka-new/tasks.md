# Implementation Tasks: Project Web Service Hosting on Sandboxes

## Scope decision (MVP for this PR)

This design spans ~5 backend subsystems + 2 frontend pages + ops/docs. Landing
all of it in a single PR would be a fiction. This PR lands a coherent **dev
preview MVP** end-to-end and explicitly defers the rest to follow-up PRs.

**In this PR (MVP — sandbox dev previews):**
- `vhost_routes` migration + types + store (preview rows only)
- `share-<adj>-<noun>-<8hex>` token generator
- `vhost.ReserveHostname()` helper (`share-` prefix + canonical hostname)
- `SubdomainProxyMiddleware` extension: canonical fall-through + `share-*`
  preview-token dispatch (project-web-service branch is a no-op stub)
- New `proxyToSandboxPort` sibling handler for `sbx_*` IDs
- Preview-token API endpoints under `/api/v1/sandboxes/:id/preview-tokens`
  and `/api/v1/sessions/:id/preview-tokens` (mint/list/rotate/delete)
- Sandbox cleanup hook drops tokens on stop/reap
- Unit tests for the share-token generator, the reserve helper, and the
  middleware dispatch ordering

**Deferred to follow-up PRs (NOT in this PR):**
- Project web services (toggle, sandbox provisioning, redeploy state
  machine, web-service-deploy triggers, custom-domain CRUD, verification)
- `web-service` runtime, `Purpose=web-service` sandbox semantics
- certmagic `auto` TLS termination (this PR ships only `passthrough` /
  `off`; works behind Caddy with a wildcard cert)
- Frontend `WebServiceTab` and `<Share preview>` UI section
- Agent `deploy_web_service` tool
- Caddy/DNS docs (will follow web-service PR)
- Integration tests requiring a real sandbox boot loop

The follow-up PRs can pick up the deferred subsystems from this same
design doc; the data model, dispatch middleware, and reserve helper are
designed to slot the project-web-service rows in without rework.

---

## MVP: data model & types

- [ ] Add `vhost_routes` migration: id, hostname (unique, lowercased), target_kind, target_id, port, is_default, verified_at, verification_token, created_at, rotated_at.
- [ ] Add `types.VHostRoute` struct, `VHostTargetKind` enum (`project_web_service`, `sandbox_preview`).
- [ ] Store CRUD: `CreateVHostRoute`, `GetVHostRouteByHostname`, `ListVHostRoutesByTarget`, `DeleteVHostRoute`, `RotateVHostRouteHostname`.

## MVP: vhost helpers

- [ ] `api/pkg/vhost/sharetoken.go`: word lists (~150 adjectives × ~150 nouns) + 8-hex `crypto/rand` suffix → `share-<adj>-<noun>-<8hex>`.
- [ ] `api/pkg/vhost/reserve.go`: `ReserveHostname(ctx, hostname, store, cfg) error` — rejects canonical hostname (from `SERVER_URL`), `DEV_SUBDOMAIN` apex, built-in reserved labels under the base, `share-` prefix unless the caller asserts it's the minter, and existing `vhost_routes` rows.
- [ ] `api/pkg/vhost/mint.go`: loops share-token generation + `ReserveHostname` check until unique (cap at 8 attempts → error).

## MVP: middleware extension

- [ ] Extend `SubdomainProxyMiddleware.ServeHTTP` (`subdomain_proxy.go`): keep existing `p{port}-{ses_id}` / `{name}-{ses_id}` branches first; add canonical-hostname fall-through; add `share-*` prefix branch that does a store lookup; project-web-service branch is a stub returning 503 ("project web services not enabled in this build") so the dispatch order is in place.
- [ ] New `proxyToSandboxPort` handler in `api/pkg/server/sandbox_proxy_handlers.go` — mirror of `proxyToSessionPort` but loads the sandbox row and routes through hydra's sandbox path.

## MVP: preview-token API

- [ ] `POST /api/v1/sandboxes/:id/preview-tokens {port}`, `GET`, `POST .../:token_id/rotate`, `DELETE` (handlers in `api/pkg/server/sandbox_preview_handlers.go`).
- [ ] Same endpoints under `/api/v1/sessions/:id/preview-tokens` (in existing session handler file).
- [ ] Authorization: caller must be authorized to update the sandbox/session (reuse existing `authorizeUserToSandbox` / `authorizeUserToSession`).
- [ ] Sandbox cleanup: `controller_cleanup.go` deletes `vhost_routes` rows with `target_kind=sandbox_preview` and matching `target_id` when reaping.

## MVP: tests

- [ ] Unit: share-token generator produces ≥79 bits entropy, hits `share-` prefix, no duplicates across 10k generations.
- [ ] Unit: `ReserveHostname` rejects canonical hostname, base apex, reserved labels, `share-` prefix when caller is not the minter, existing rows.
- [ ] Unit: middleware dispatch order — `p{port}-{id}` (existing) → canonical → `share-*` → project-web-service stub → 404. Existing `p{port}-{ses_id}` regression unchanged.
- [ ] Unit: store CRUD round-trip for `vhost_routes`.

---

## Deferred (next PRs) — kept here for traceability

### Project web service backend
- [ ] `project_web_service_state`, `web_service_deploys` migrations + store.
- [ ] `SandboxRuntimeWebService` + `Purpose` field; `web-service` runtime in `RuntimeRegistry`.
- [ ] `Controller.Create` enforces one-per-project for `Purpose=web-service`, no idle TTL.
- [ ] Runner web-service workload handler (clone repo, run `.helix/startup.sh`, supervise).
- [ ] `webservice.Redeploy(projectID, sha)` orchestration.
- [ ] `TriggerKind = web-service-deploy`; webhook dispatch.
- [ ] `POST /api/v1/projects/:id/web-service/deploy` (manual / agent tool).
- [ ] Custom-domain CRUD + `.well-known/helix-domain-verify/<token>` flow.
- [ ] Middleware project-web-service branch replaces the 503 stub with real lookup → revdial proxy.

### RevDial + proxy plumbing
- [ ] `TARGET 127.0.0.1:<port>\n` handshake on each stream; runner host-allowlist (`127.0.0.1`/`::1`).
- [ ] `connman` wrapped in `net/http.Transport` for keepalive pooling.
- [ ] `ResilientProxy` buffer size configurable per route kind.

### TLS auto mode
- [ ] `HELIX_VHOST_TLS_MODE`, `HELIX_VHOST_LETSENCRYPT_EMAIL`, `HELIX_VHOST_RESERVED_SUBDOMAINS`, `SERVER_URL_ALIASES` env vars.
- [ ] Startup validation (passthrough+dynamic warning; auto without email error).
- [ ] `certmagic` on-demand TLS gated on `ReserveHostname`-aware decision func.

### Frontend
- [ ] `<WebServiceTab>` in `ProjectSettings.tsx` (enable toggle, default URL, custom domains, port, deploys list).
- [ ] `<SharePreviewSection>` in sandbox/session detail pages.

### Docs & ops
- [ ] Env-var docs, DNS setup, Caddy snippet for `passthrough` mode.
- [ ] Sample project with `.helix/startup.sh` boot script.

### Integration tests (deferred until web services land)
- [ ] Enable web service → push to primary repo → assert deploy.
- [ ] Preview-token mint/rotate/revoke against a real headless sandbox.
- [ ] `passthrough` mode behind stub upstream proxy.
- [ ] Security: reserved-hostname registration rejected.
