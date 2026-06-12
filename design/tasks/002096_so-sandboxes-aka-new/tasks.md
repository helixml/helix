# Implementation Tasks: Project Web Service Hosting on Sandboxes

## Backend — data model & types

- [ ] Add `SandboxRuntimeWebService` and `Purpose` field to `api/pkg/types/sandbox.go`; add `DefaultPreviewPort` to runtime spec.
- [ ] No new column on `sandboxes` / sessions for "share preview enabled" — the existence of one or more `vhost_routes` rows with `target_kind=sandbox_preview, target_id=<id>` is the canonical state.
- [ ] Add `web-service` runtime entry to the default `RuntimeRegistry` in `api/pkg/sandbox/runtimes.go` (image `ubuntu:22.04`, headless, persistent, no idle TTL, preview port `8080`); set `5800` on `ubuntu-desktop` and `8080` on `headless-ubuntu`.
- [ ] Add migrations for `project_web_service_state`, `vhost_routes`, `web_service_deploys` tables with FKs and the `(project_id, purpose=web-service)` uniqueness constraint on sandboxes.
- [ ] Add CRUD store methods in `api/pkg/store/` for the three new tables, including a `LookupRoute(hostname)` that returns `(target_kind, target_id, port)`.

## Backend — vhost router & TLS

- [ ] **Extend the existing `SubdomainProxyMiddleware`** (`api/pkg/server/subdomain_proxy.go`) in place, NOT in parallel. Existing `p{port}-{ses_id}` and `{name}-{ses_id}` schemes stay unchanged. Add three new dispatch branches in this order: (a) canonical hostname (`SERVER_URL` + `SERVER_URL_ALIASES`) → fall through to main API/UI mux; (b) `share-*` prefix under `<DEV_SUBDOMAIN base>` → `vhost_routes` lookup for sandbox preview; (c) any other host → `vhost_routes` lookup for project web service; else 404.
- [ ] `vhost_routes` resolver: in-memory cache keyed by full hostname, pubsub invalidation on writes, returns `(target_kind, target_id, port)`.
- [ ] Sandbox-preview branch resolves to `(sandbox/session_id, port)` and calls the existing `proxyToSessionPort` (for `ses_*`) or a new sibling `proxyToSandboxPort` (for `sbx_*`).
- [ ] Project-web-service branch resolves to the project's currently active web-service sandbox and proxies via `connman.Dial(deviceID, port)` + `ResilientProxy`.
- [ ] Extend the RevDial protocol with a `TARGET 127.0.0.1:<port>\n` handshake on each new stream; runner-side agent reads it, opens the local TCP connection, splice-copies both directions.
- [ ] Runner-side helper restricts target hosts to `127.0.0.1` / `::1` (plus the project's `docker compose` link-local service names when relevant); reject everything else with an audit log line.
- [ ] Wrap `connman` in a `net/http.Transport` whose `DialContext` calls `connman.Dial(device, port)` and let `Transport` pool idle connections per `(device, port)` key for HTTP-keepalive reuse.
- [ ] Make `ResilientProxy` per-direction buffer size configurable; default 4 MB for `project_web_service` routes, 512 KB for `sandbox_preview` (matches today's value).
- [ ] Add **only the genuinely new** env vars: `HELIX_VHOST_TLS_MODE`, `HELIX_VHOST_LETSENCRYPT_EMAIL`, `HELIX_VHOST_RESERVED_SUBDOMAINS`, optional `SERVER_URL_ALIASES`. Reuse existing `SERVER_URL` (canonical hostname) and `DEV_SUBDOMAIN` (base domain) via the existing `parseDevSubdomainConfig`.
- [ ] Startup validation: if `DEV_SUBDOMAIN` set and `tls_mode=passthrough`, log a single clear warning describing the wildcard-cert requirement on the upstream proxy; do not refuse. If `tls_mode=auto` and `LETSENCRYPT_EMAIL` unset, refuse to boot with a clear error.
- [ ] `vhost.ReserveHostname()` helper centralising the reserved-hostname rules (canonical hostname from `SERVER_URL`, aliases, `DEV_SUBDOMAIN` base apex, built-in + operator-configured reserved labels, the `share-` reserved prefix, existing rows). Used by custom-domain POST, default-subdomain allocation, share-token minting, and the certmagic `OnDemand.DecisionFunc`.
- [ ] Default-subdomain allocation appends `-2`, `-3`, … on slug collision with reserved labels or existing rows. Random preview minting loops on collision.
- [ ] In `auto` mode wire `github.com/caddyserver/certmagic` with on-demand TLS gated on `vhost_routes` *and* canonical hostname(s); decision func rejects anything `ReserveHostname()` would reject. Bind `:443` and `:80`.
- [ ] In `passthrough` mode trust `X-Forwarded-Proto` / `X-Forwarded-Host`; skip cert mgr; keep existing listener; canonical-hostname dispatch still runs in the middleware.

## Backend — sandbox provisioning & supervision

- [ ] Extend `sandbox.Controller.Create` to accept `Purpose=web-service`, enforce one-per-project, and skip TTL reaping for these.
- [ ] In the runner, add a "web-service workload" handler: clone primary repo at the requested SHA into `/workspace`, run `bash .helix/startup.sh`, restart on exit, stream logs to API.
- [ ] Add `webservice.Redeploy(projectID, sha)` orchestration: provision new sandbox, poll `127.0.0.1:port` health, atomic cutover of `active_sandbox_id`, stop old sandbox, record `web_service_deploys` row.

## Backend — deploy triggers

- [ ] Add `TriggerKind = web-service-deploy`; auto-create one when feature enabled, auto-delete when disabled.
- [ ] In `api/pkg/server/webhook_trigger_handlers.go`, dispatch this kind on primary-repo pushes where `ref == refs/heads/<default-branch>` and call `Redeploy`.
- [ ] Add `POST /api/v1/projects/:id/web-service/deploy` for the "Deploy now" button (uses current HEAD).
- [ ] Add an agent tool `deploy_web_service(project_id)` that calls the same endpoint and returns URL + log tail.

## Backend — domain management

- [ ] `POST/DELETE /api/v1/projects/:id/web-service/domains` for custom domains; pre-seed default subdomain on enable.
- [ ] Verification flow: serve `/.well-known/helix-domain-verify/<token>` returning the token; mark `verified_at` once the operator's DNS resolves to us and the token check passes (cron + on-demand).

## Backend — sandbox dev previews

- [ ] `POST /api/v1/sandboxes/:id/preview-tokens {port}` (mint) → inserts a `vhost_routes` row with `target_kind=sandbox_preview, target_id=<sbx_or_ses_id>, port=<port>, verified_at=now()` and hostname `share-<adj>-<noun>-<8hex>.<DEV_SUBDOMAIN base>`. `GET` lists existing tokens for the sandbox. `POST .../:token_id/rotate` regenerates the hostname (updates `rotated_at`). `DELETE` removes the row.
- [ ] Same endpoints exposed under `/api/v1/sessions/:id/preview-tokens` for spec-task sessions.
- [ ] Extend `controller_cleanup.go` (and the session-stop path) to delete `vhost_routes` rows where `target_kind=sandbox_preview` and `target_id` matches the reaped sandbox/session.
- [ ] Random subdomain generator (`api/pkg/vhost/sharetoken.go`): two word-lists + `crypto/rand` 32-bit hex suffix → ≥79 bits entropy; uniqueness check against `vhost_routes` with retry-on-collision. `share-` prefix is hard-coded so the reserved-prefix rule covers it.

## Frontend — Project Settings

- [ ] Add `web-service` to the tab conditional block in `frontend/src/pages/ProjectSettings.tsx:1958-1964`.
- [ ] New `<WebServiceTab>` component: enable toggle, default URL display, custom-domain list with status badges, container-port input, deploys table with "Deploy now" button.
- [ ] Mutations via existing `updateProjectMutation` pattern plus new domain/deploy endpoints; invalidate queries after each.

## Frontend — Sandbox dev preview

- [ ] In each sandbox/session detail UI (agent workspace, spec-task, human org desktop, user-facing API sandbox) add a "Share preview" section listing current preview tokens (URL + port + created/rotated time + copy/rotate/revoke buttons).
- [ ] "Add preview" form with port input pre-filled with the runtime's `DefaultPreviewPort`; POST creates a new token row.
- [ ] Disabled state with tooltip when `DEV_SUBDOMAIN` is not configured.
- [ ] Reflect lifecycle: section auto-clears when the sandbox is stopped (tokens are deleted server-side).

## Docs & ops

- [ ] Update `helix` docs: new env vars, required DNS setup (wildcard `A`/`CNAME` for base domain), Caddy snippet for `passthrough` mode.
- [ ] Add a sample project showing a tiny `.helix/startup.sh` that boots a web server on `:8080`.

## Tests

- [ ] Unit: runtime registry, middleware dispatch (canonical / existing `p{port}-{id}` / `share-*` token / project-web-service hostname / unknown), `vhost_routes` cache invalidation, `ReserveHostname` (canonical, apex, reserved labels, `share-` prefix, operator-added labels, existing-row collision), share-token generator uniqueness, slug-collision suffixing, webhook ref filter, redeploy state machine (success / health-fail / cutover-fail), startup warning emitted on passthrough+dynamic combo (and Helix still boots), certmagic decision func refuses reserved hostnames.
- [ ] Integration: enable web-service feature → push to primary repo → assert sandbox provisioned, domain resolves, `/` returns 200 via the API proxy.
- [ ] Integration: mint a preview token for a running headless sandbox → assert `share-…` URL returns 200; rotate the token → old URL returns 404, new returns 200; stop the sandbox → all tokens revoked, all URLs return 404 and `vhost_routes` rows are gone.
- [ ] Integration: existing `p{port}-{ses_id}` scheme still works unchanged for sessions (regression guard).
- [ ] Integration: `passthrough` mode behind a stub upstream proxy passes `X-Forwarded-*` correctly for canonical domain, project default subdomains, dynamic preview subdomains, and static custom domains; canonical domain still reaches the main API mux.
- [ ] Security: project owner attempts to register the canonical Helix domain as a custom domain → request rejected, no row written; same for `api.<base>`, the bare base domain, and an operator-configured reserved label.
