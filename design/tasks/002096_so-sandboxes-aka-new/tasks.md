# Implementation Tasks: Project Web Service Hosting on Sandboxes

## Scope decision (MVP for this PR)

User confirmed the existing `p{port}-{session_id}` / `{name}-{session_id}`
URL scheme has no consumers and can be deleted entirely. That removes the
backwards-compat burden — this PR ships a single share-token URL scheme and
nothing else user-facing.

**In this PR (MVP — dev preview tokens for sessions):**
- Delete `session_expose_handlers.go`, `subdomain_proxy.go`,
  `subdomain_proxy_test.go`, related wiring in `server.go`, related routes.
  (Hydra-side data plane stays.)
- `vhost_routes` migration + types + store
- `vhost` package: share-token generator, `ReserveHostname` helper
- New `VHostMiddleware` (canonical hostname fall-through → `share-*` lookup
  → 404). Replaces `SubdomainProxyMiddleware`.
- New `proxyToTarget` handler — sessions (`ses_*`) only in MVP, calls
  existing hydra `/dev-containers/{id}/proxy/{port}` path.
- Preview-token API endpoints under `/api/v1/sessions/:id/preview-tokens`
  (mint / list / rotate / delete). Authorization via existing
  `authorizeUserToSession`.
- Session cleanup hook drops tokens on stop.
- Regenerate OpenAPI client (`./stack update_openapi`).
- Unit tests: share-token generator, reserve helper, middleware dispatch,
  store round-trip.

**Deferred to follow-up PRs:**
- Sandbox (`sbx_*`) preview support — one more dispatch + hydra route.
- Project web services (full subsystem: toggle, runtime, redeploy state
  machine, web-service-deploy triggers, custom-domain CRUD + verification,
  middleware project-web-service branch).
- certmagic `auto` TLS termination (MVP works in `passthrough` mode behind
  a Caddy/upstream with wildcard cert).
- Frontend `WebServiceTab` and `<SharePreviewSection>`.
- Agent `deploy_web_service` tool.
- Docs (env vars, DNS setup, Caddy snippet).

The deferred subsystems can pick up from this same design doc; the
`vhost_routes` table, `VHostMiddleware`, and `ReserveHostname` helper are
designed to slot the project-web-service rows in without rework.

---

## MVP tasks

### Cleanup of obsolete scheme
- [ ] Delete `api/pkg/server/session_expose_handlers.go`.
- [ ] Delete `api/pkg/server/subdomain_proxy.go` and `_test.go`.
- [ ] Remove `exposedPortManager` field, `initExposedPortManager()` call, `NewSubdomainProxyMiddleware` wiring, session-expose routes, and `proxyToSessionPort` route from `api/pkg/server/server.go`.
- [ ] Remove `DevSubdomain` env var documentation references that mention the `p{port}-` format (the env var itself stays — it's the base domain for the new scheme).

### Data model
- [ ] `vhost_routes` migration: id, hostname (unique, lowercased), target_kind, target_id, port, is_default, verified_at, verification_token, created_at, rotated_at.
- [ ] `types.VHostRoute` struct + `VHostTargetKind` enum.
- [ ] Store CRUD methods: `CreateVHostRoute`, `GetVHostRouteByHostname`, `ListVHostRoutesByTarget`, `DeleteVHostRoute`, `DeleteVHostRoutesByTarget`, `RotateVHostRouteHostname`.

### vhost helpers (new `api/pkg/vhost/` package)
- [ ] `sharetoken.go`: ~150 adjectives × ~150 nouns + 8-hex `crypto/rand` suffix → `share-<adj>-<noun>-<8hex>`.
- [ ] `reserve.go`: `ReserveHostname(ctx, hostname, opts) error` — rejects canonical hostname (parsed from `SERVER_URL`), `DEV_SUBDOMAIN` apex, built-in reserved labels, `share-` prefix (unless caller is the minter), and existing `vhost_routes` rows.
- [ ] `mint.go`: loops share-token generation + uniqueness check up to 8 attempts.

### Middleware + proxy handler
- [ ] `api/pkg/server/vhost_middleware.go`: new minimal middleware. Dispatch: canonical hostname (`SERVER_URL` + optional `SERVER_URL_ALIASES`) → fall through; `share-*` prefix under `<DEV_SUBDOMAIN base>` → store lookup → `proxyToTarget`; else 404 unknown-host page.
- [ ] `api/pkg/server/vhost_proxy.go`: `proxyToTarget(w, r, kind, id, port)` — for sessions, calls hydra `/dev-containers/{id}/proxy/{port}` (extracted from the deleted `proxyToSessionPort`). For `sandbox_preview` of an `sbx_*` ID, returns 503 (deferred).

### API endpoints
- [ ] `api/pkg/server/session_preview_handlers.go`: `POST /api/v1/sessions/:id/preview-tokens {port}` mints, `GET` lists, `POST .../:token_id/rotate` rotates, `DELETE .../:token_id` removes. Auth via `authorizeUserToSession(ActionUpdate)`.
- [ ] Cleanup: session-stop path deletes `vhost_routes` rows where `target_kind=sandbox_preview` and `target_id=<session_id>`.

### Wiring & generation
- [ ] Wire `VHostMiddleware` into `server.go` ahead of the main router (replaces old `SubdomainProxyMiddleware` slot).
- [ ] `./stack update_openapi` to regenerate the OpenAPI client.

### Tests
- [ ] Unit: share-token generator format, ≥79-bit entropy claim, no collisions across 10k generations.
- [ ] Unit: `ReserveHostname` rejects each forbidden category.
- [ ] Unit: middleware dispatch — canonical / `share-*` hit / `share-*` miss → 404 / unknown host → 404.
- [ ] Unit: store CRUD round-trip for `vhost_routes`.

---

## Deferred (next PRs) — kept for traceability

### Sandbox (`sbx_*`) preview support
- [ ] `proxyToTarget` handles `sbx_*` IDs by calling the appropriate hydra sandbox route (may need a new hydra route mirroring `/dev-containers/{id}/proxy/{port}`).
- [ ] Mirror preview-tokens endpoints under `/api/v1/sandboxes/:id/preview-tokens`.
- [ ] Sandbox cleanup hook deletes preview rows on reap.

### Project web service backend
- [ ] `project_web_service_state`, `web_service_deploys` migrations + store.
- [ ] `SandboxRuntimeWebService` + `Purpose` field; `web-service` runtime entry.
- [ ] `Controller.Create` enforces one-per-project for `Purpose=web-service`, no idle TTL.
- [ ] Runner web-service workload handler.
- [ ] `webservice.Redeploy(projectID, sha)` orchestration.
- [ ] `TriggerKind = web-service-deploy`; webhook dispatch on default-branch push.
- [ ] `POST /api/v1/projects/:id/web-service/deploy` + agent tool.
- [ ] Custom-domain CRUD + verification flow.
- [ ] Middleware project-web-service branch — replace any stub with real lookup → revdial proxy through `connman.Dial(deviceID, port)`.

### RevDial + proxy plumbing
- [ ] `TARGET 127.0.0.1:<port>\n` handshake; runner host-allowlist.
- [ ] `connman` wrapped in `net/http.Transport` for keepalive pooling.
- [ ] Configurable `ResilientProxy` buffer per route kind.

### TLS auto mode
- [ ] `HELIX_VHOST_TLS_MODE`, `HELIX_VHOST_LETSENCRYPT_EMAIL`, `HELIX_VHOST_RESERVED_SUBDOMAINS`, `SERVER_URL_ALIASES` env vars.
- [ ] Startup validation.
- [ ] `certmagic` on-demand TLS with `ReserveHostname`-aware decision func.

### Frontend
- [ ] `<WebServiceTab>` in `ProjectSettings.tsx`.
- [ ] `<SharePreviewSection>` in session detail pages.

### Docs & ops
- [ ] Env vars, DNS setup, Caddy snippet for `passthrough` mode.
- [ ] Sample project with `.helix/startup.sh`.

### Integration tests
- [ ] Mint preview token for a running session → URL returns 200; rotate → old 404, new 200; stop session → all 404.
- [ ] `passthrough` mode behind stub upstream.
- [ ] Security: reserved-hostname registration rejected.
