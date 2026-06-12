# Implementation Tasks: Project Web Service Hosting on Sandboxes

## Backend — data model & types

- [ ] Add `SandboxRuntimeWebService` and `Purpose` field to `api/pkg/types/sandbox.go`; add `DefaultPreviewPort` to runtime spec.
- [ ] Add `sandbox_preview_enabled` column to the `sandboxes` table/model.
- [ ] Add `web-service` runtime entry to the default `RuntimeRegistry` in `api/pkg/sandbox/runtimes.go` (image `ubuntu:22.04`, headless, persistent, no idle TTL, preview port `8080`); set `5800` on `ubuntu-desktop` and `8080` on `headless-ubuntu`.
- [ ] Add migrations for `project_web_service_state`, `vhost_routes`, `web_service_deploys` tables with FKs and the `(project_id, purpose=web-service)` uniqueness constraint on sandboxes.
- [ ] Add CRUD store methods in `api/pkg/store/` for the three new tables, including a `LookupRoute(hostname)` that returns `(target_kind, target_id, port)`.

## Backend — vhost router & TLS

- [ ] New package `api/pkg/vhost/` with a `Router` that resolves `Host` → `(target_kind, target_id, port)` via `vhost_routes` (in-memory cache, pubsub invalidation on writes), then dereferences to a sandbox + port (project web service: look up active sandbox; sandbox preview: direct).
- [ ] HTTP middleware in the API server that, when `Host` matches a known vhost route, proxies through `connman.Dial(deviceID)` + `ResilientProxy` to the resolved container port. All other hostnames fall through to the existing API/UI handlers.
- [ ] Add `HELIX_VHOST_TLS_MODE`, `HELIX_VHOST_BASE_DOMAIN`, `HELIX_VHOST_LETSENCRYPT_EMAIL`, `HELIX_VHOST_DYNAMIC_PREVIEWS_ENABLED`, `HELIX_CANONICAL_DOMAIN`, `HELIX_VHOST_RESERVED_SUBDOMAINS` to `api/pkg/config/`.
- [ ] Startup validation: refuse to boot if `HELIX_CANONICAL_DOMAIN` overlaps with `HELIX_VHOST_BASE_DOMAIN` in an ambiguous way. If `dynamic_previews_enabled=true` and `tls_mode=passthrough`, log a single clear warning explaining the wildcard-cert requirement on the upstream proxy; do not refuse.
- [ ] Vhost middleware dispatches on `Host`: canonical → fall through to existing main API/UI mux; `vhost_routes` hit → proxy to sandbox; else → 404 unknown-host page.
- [ ] `vhost.ReserveHostname()` helper centralising the reserved-hostname rules (canonical, base apex, built-in + operator-configured reserved labels, existing rows). Used by custom-domain POST, default-subdomain allocation, random-preview minting, and the certmagic `OnDemand.DecisionFunc`.
- [ ] Default-subdomain allocation appends `-2`, `-3`, … on slug collision with reserved labels or existing rows. Random preview minting loops on collision.
- [ ] In `auto` mode wire `github.com/caddyserver/certmagic` with on-demand TLS gated on `vhost_routes` *and* canonical domain(s); decision func rejects anything `ReserveHostname()` would reject. Bind `:443` and `:80`.
- [ ] In `passthrough` mode trust `X-Forwarded-Proto` / `X-Forwarded-Host`; skip cert mgr; keep existing listener; canonical-domain dispatch still runs in the middleware.

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

- [ ] `POST /api/v1/sandboxes/:id/preview` (enable) and `DELETE` (disable). Enable mints a random `<adj>-<noun>-<hex>.<base-domain>` subdomain, inserts a `vhost_routes` row with `target_kind=sandbox_preview, is_dynamic=true, verified_at=now()`, sets `sandbox_preview_enabled=true`. Disable is the inverse.
- [ ] Optional `?port=N` parameter to override the runtime's `DefaultPreviewPort`; persisted on the `vhost_routes` row.
- [ ] Extend `controller_cleanup.go` (and any other sandbox-stop path) to delete dynamic `vhost_routes` rows pointing at the reaped sandbox.
- [ ] Random subdomain generator (`api/pkg/vhost/subdomain.go`): word lists + `crypto/rand` hex suffix; uniqueness check against `vhost_routes`.

## Frontend — Project Settings

- [ ] Add `web-service` to the tab conditional block in `frontend/src/pages/ProjectSettings.tsx:1958-1964`.
- [ ] New `<WebServiceTab>` component: enable toggle, default URL display, custom-domain list with status badges, container-port input, deploys table with "Deploy now" button.
- [ ] Mutations via existing `updateProjectMutation` pattern plus new domain/deploy endpoints; invalidate queries after each.

## Frontend — Sandbox dev preview

- [ ] In each sandbox detail UI (agent workspace, spec-task, human org desktop) add a "Share preview" toggle row showing the current URL with a copy button when on, and a tooltip explaining DNS prerequisites when the operator has not configured `HELIX_VHOST_BASE_DOMAIN`.
- [ ] Optional port override input (advanced/disclosure), pre-filled with the runtime's `DefaultPreviewPort`.
- [ ] Reflect lifecycle: toggle auto-disables and URL disappears when the sandbox is stopped.

## Docs & ops

- [ ] Update `helix` docs: new env vars, required DNS setup (wildcard `A`/`CNAME` for base domain), Caddy snippet for `passthrough` mode.
- [ ] Add a sample project showing a tiny `.helix/startup.sh` that boots a web server on `:8080`.

## Tests

- [ ] Unit: runtime registry, vhost router (cache + invalidation, all three dispatch outcomes: canonical / route hit / unknown), `ReserveHostname` (canonical, apex, reserved labels, operator-added labels, existing-row collision), subdomain generator uniqueness, slug-collision suffixing, webhook ref filter, redeploy state machine (success / health-fail / cutover-fail), startup warning emitted on passthrough+dynamic combo (and Helix still boots), certmagic decision func refuses reserved hostnames.
- [ ] Integration: enable web-service feature → push to primary repo → assert sandbox provisioned, domain resolves, `/` returns 200 via the API proxy.
- [ ] Integration: enable preview on a running headless sandbox → assert random URL returns 200, then stop the sandbox → assert URL returns 404 and `vhost_routes` row is gone.
- [ ] Integration: `passthrough` mode behind a stub upstream proxy passes `X-Forwarded-*` correctly for canonical domain, project default subdomains, dynamic preview subdomains, and static custom domains; canonical domain still reaches the main API mux.
- [ ] Security: project owner attempts to register the canonical Helix domain as a custom domain → request rejected, no row written; same for `api.<base>`, the bare base domain, and an operator-configured reserved label.
