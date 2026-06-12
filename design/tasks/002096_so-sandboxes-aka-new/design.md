# Design: Project Web Service Hosting on Sandboxes

## Architecture Overview

```
visitor ──HTTPS──► Helix API (:443)
                     │
                     ├─ TLS terminate  ◄── certmagic / Let's Encrypt (on-demand)
                     │
                     └─ Host header dispatch
                          ├─ canonical Helix domain → main API/UI mux
                          ├─ vhost_routes hit       → proxy via revdial → container:port
                          └─ otherwise              → 404 unknown-host page
                                              ▲
                                              │ deploy: git push, manual, or
                                              │  "share preview" toggle
                                              │
                            primary repo webhook ──► CD trigger ──► sandbox controller
```

## Prior Art (reuse, don't reinvent)

Helix already ships subdomain-based vhost routing. This design **extends
the existing system**; it does not parallel it.

| Existing piece | What it does | How this design uses it |
|---|---|---|
| `SERVER_URL` (`config.go:635`) | Canonical Helix URL | Hostname parsed via the same logic in `parseDevSubdomainConfig` (`subdomain_proxy.go:152`). Added: optional `SERVER_URL_ALIASES`. **No `HELIX_CANONICAL_DOMAIN`.** |
| `DEV_SUBDOMAIN` (`config.go:669`) | Base for `*.dev.example.com` vhost routing | Reused as-is. **No `HELIX_VHOST_BASE_DOMAIN`.** |
| `SubdomainProxyMiddleware` (`subdomain_proxy.go:48`) | Dispatches `p{port}-{ses_id}` and `{name}-{ses_id}` to session proxy handler | **Extended in place**, NOT paralleled. Two new branches added: (a) `share-*` prefix → DB lookup in `vhost_routes` for sandbox previews; (b) any other host → DB lookup in `vhost_routes` for project web services. |
| `proxyToSessionPort` (`session_expose_handlers.go:658`) | Public proxy to a session's dev-container port via hydra | Reused as the worker. The new dispatch resolves a preview token to `(sandbox_id, port)` and calls this same handler (or its `sbx_*` sibling). |
| Existing `p{port}-{ses_id}` / `{name}-{ses_id}` patterns | Auth-by-obscurity (knowing the session ID) URL scheme for dev-container ports | **Unchanged.** They serve a different use case (the "if you can see the session ID in the UI, you can debug its ports" workflow). The new "Share preview" feature is the opt-in shareable path. Both go through the same middleware and TLS layer. |

**Why preview tokens (not the existing `p{port}-{id}` URL) for sharing.**
The sandbox/session ID appears in many surfaces — UI breadcrumbs, API
responses, log lines, screenshots. Reusing it as the share secret means
any leak of the ID through any of those channels also leaks the preview
URL. A purpose-built random `share-<adj>-<noun>-<8hex>` token is generated
only at the moment "Share preview" is enabled and can be rotated or revoked
independently of the underlying sandbox. That's worth the one DB lookup per
hostname (cached, so functionally free after first hit).

The polymorphic `vhost_routes` table introduced below holds **two** target
kinds — sandbox previews (random tokens) and project web services
(user-named hostnames) — because both genuinely need DB lookup. Parseable
patterns stay parseable; nothing existing moves into the table.

New env vars are limited to TLS termination and the reserved-hostname
allowlist (both genuinely new capabilities):

| Env var | Purpose |
|---|---|
| `HELIX_VHOST_TLS_MODE` | `auto` \| `passthrough` \| `off` |
| `HELIX_VHOST_LETSENCRYPT_EMAIL` | ACME registration contact (auto mode) |
| `HELIX_VHOST_RESERVED_SUBDOMAINS` | Extra reserved labels, comma-separated |
| `SERVER_URL_ALIASES` | Optional extra canonical hostnames |

Three new subsystems are added; everything else extends existing code paths.
The same vhost+TLS plumbing serves **three consumers**:

1. **The main Helix control-plane app itself** — the canonical hostname
   parsed from `SERVER_URL` (plus any `SERVER_URL_ALIASES`) is dispatched
   to the existing API and UI mux. There is no separate listener for the
   main app vs hosted services; one listener, one cert manager.
2. **Project web services** — durable, per-project, user-named domains.
3. **Sandbox dev previews** — ephemeral, per-sandbox, random subdomains, lifecycle
   bound to the sandbox.

## Key Decisions

### 1. TLS termination: embedded `certmagic`, with `passthrough` escape hatch

Let's Encrypt termination lives in the Helix API server using
`github.com/caddyserver/certmagic` (the same ACME library Caddy uses; pure-Go,
file-backed cert storage we already have on disk, supports HTTP-01 and
TLS-ALPN-01 — no DNS plugin required for non-wildcard certs).

Env vars:

- **`HELIX_VHOST_TLS_MODE=auto`** (default when `DEV_SUBDOMAIN` set and
  `LETSENCRYPT_EMAIL` provided):
  Helix binds `:443` + `:80`, terminates TLS, issues per-hostname certs
  on-demand. ACME challenges served from the same listener.
- **`HELIX_VHOST_TLS_MODE=passthrough`**: Helix binds HTTP only on its
  existing API port, trusts `X-Forwarded-Proto` / `X-Forwarded-Host` / `Host`.
  Operator runs Caddy or similar in front. **Compatible with dynamic
  previews** provided the upstream holds a wildcard cert for
  `*.<DEV_SUBDOMAIN base>` (e.g. Caddy with a DNS-01 plugin), which covers
  every minted subdomain automatically. If `DEV_SUBDOMAIN` is set in this
  mode, Helix emits a single startup warning describing the wildcard-cert
  requirement — it does not refuse to boot. Custom domains outside the base
  must be added to the upstream proxy by the operator.
- **`HELIX_VHOST_TLS_MODE=off`** (default when `DEV_SUBDOMAIN` unset):
  Hosted-web-service panels hidden in UI; the existing dev-port subdomain
  scheme still works if the operator has set `DEV_SUBDOMAIN`.

Why not stick with the cloud-managed cert pattern already used in GKE Helm?
Cloud-managed certs require a fixed list of SANs; per-project domains added at
runtime can't be issued without operator intervention. certmagic issues on
first request, and the `passthrough` + wildcard-cert pattern achieves the
same result when the operator already runs a capable reverse proxy.

### 2. Vhost routing: hostname → target → revdial → container

A single table `vhost_routes` maps `hostname → target` and supersedes the
earlier `project_web_service_domains` design. Each row has a polymorphic
`target_kind`:

- `target_kind = project_web_service` — `target_id` references a row in
  `project_web_service_state`; resolution picks that project's currently
  active web-service sandbox.
- `target_kind = sandbox_preview` — `target_id` references a sandbox directly;
  resolution proxies to that specific sandbox.

Other columns: `is_default` (default subdomain for a project), `is_dynamic`
(random per-sandbox subdomain — never user-edited, deleted with the sandbox),
`verified_at` / `verification_token` (only meaningful for user-supplied custom
domains; default and dynamic rows are auto-verified).

The dispatch lives inside the existing `SubdomainProxyMiddleware`
(`api/pkg/server/subdomain_proxy.go`), extended with new branches **before**
falling through to the next handler. Order matters — most specific first:

1. If `Host` matches the canonical hostname parsed from `SERVER_URL` (or
   any `SERVER_URL_ALIASES` entry) → fall through to the existing API/UI
   mux. The control-plane app is just another consumer of the same listener.
2. Else if `Host` matches the existing `p{port}-{ses_id}` or
   `{name}-{ses_id}` schemes → existing dispatch (unchanged).
3. Else if the subdomain label starts with `share-` and the full hostname
   ends with `<DEV_SUBDOMAIN base>` → look up `vhost_routes` (in-memory
   cache, pubsub-invalidated) where `target_kind=sandbox_preview`; if
   found, resolve to `(sandbox_id, port)` and proxy via the same hydra
   path used by `proxyToSessionPort` / `proxyToSandboxPort`.
4. Else if `Host` matches any other verified `vhost_routes` row with
   `target_kind=project_web_service` → resolve the project's currently
   active web-service sandbox and proxy via `connman.Dial(deviceID, port)`
   + `ResilientProxy` to the runner (same revdial used for desktop
   bridges today, `api/pkg/sandbox/controller.go:260`).
5. Else → 404 unknown-host page (Helix-branded; clearly identifies the
   responding server).

WebSocket and HTTP/2 just work because `ResilientProxy` already handles
upgrades.

**Per-port targeting over RevDial.** The existing RevDial bridge dials a
device — not a `(device, port)` pair — because the current desktop bridge
always lands on one well-known port. Web services and previews need an
arbitrary internal port (8080, 3000, 5800, etc.), so the protocol gains a
small handshake: when the API dials, it sends a one-line target header
(`TARGET 127.0.0.1:<port>\n`) on the freshly-opened stream; the runner-side
agent reads the header, opens a local TCP connection to that address inside
the sandbox's network namespace, and splice-copies both directions. No port
is opened on the runner host or container's external interface — all
traffic enters via the existing outbound tunnel the runner already maintains
to the API. This is a security win: runners can sit behind NAT or strict
egress-only firewalls and still serve public web traffic.

The runner-side helper restricts target hosts to `127.0.0.1` / `::1` (and
optionally `helix-services` link-local names inside a `docker compose` stack
the project is running) so a compromised API row can't be used to scan the
runner host's internal network.

**Connection reuse.** `connman.Dial` is currently called per request for
short-lived RPCs. Web services see real HTTP-keepalive load, so the proxy
wraps `connman` in a `net/http.Transport` whose `DialContext` calls
`connman.Dial` and lets `Transport` pool idle connections per `(device,
port)` key. Pool sizing follows Go defaults; tune later if profiling shows
contention. WebSocket / HTTP/2 connections bypass the pool (one stream per
upgrade) — same as any reverse proxy.

**Buffer ceilings.** `ResilientProxy` today buffers 512 KB per direction.
For static-asset downloads this caps single-request throughput at
buffer-size × refill rate. Keep the existing buffer for the dev-preview
case; for project web services raise the cap (configurable; default 4 MB
each way) so file downloads aren't pathologically slow. Tracked as a
follow-up if real workloads need streaming with no buffer.

**Anti-hijack: reserved hostnames.** Domain-registration code (custom-domain
POST, default-subdomain allocation on enable, random-preview minting) all go
through a single `vhost.ReserveHostname()` helper that rejects:

- the canonical hostname from `SERVER_URL` and any `SERVER_URL_ALIASES`
  (case-insensitive, normalised);
- the apex of `DEV_SUBDOMAIN`'s base domain;
- any `<label>.<base>` whose label is in the built-in reserved set
  (`api`, `app`, `www`, `auth`, `admin`, `helix`, `console`, `dashboard`,
  `helix-admin`, `mail`, `ns`) or in `HELIX_VHOST_RESERVED_SUBDOMAINS`;
- any label starting with the reserved prefix `share-` (used by sandbox
  preview tokens — projects can't claim a slug colliding with the preview
  namespace);
- any host already present in `vhost_routes` (DB unique constraint on
  `hostname`).

Default-subdomain allocation that collides with the reserved set or an
existing row appends `-2`, `-3`, … to the slug. Random preview generation
loops until it finds a free name. The reserved set is consulted from a single
place — the helper — so the certmagic `OnDemand.DecisionFunc` reuses it to
refuse cert issuance for any forbidden hostname even if a malicious row
somehow appeared.

**Preview token minting** (sandbox previews):

- Hostname format: `share-<adj>-<noun>-<8hex>.<DEV_SUBDOMAIN base>`
  (e.g. `share-purple-otter-3f8a91c4.dev.helix.example.com`). The leading
  `share-` prefix is what the middleware regex matches on, and it's a
  reserved label so no project slug can collide.
- Entropy: ~150-word adjective list × ~150-word noun list × 32-bit hex
  segment ≈ 47 + 32 = 79 bits. Generated with `crypto/rand`. Stored as
  the full `hostname` column on the `vhost_routes` row.
- Lifecycle: enabling "Share preview" on a sandbox creates a
  `vhost_routes` row with `target_kind=sandbox_preview, target_id=<sbx_or_ses_id>,
  port=<chosen>`. Rotating regenerates a new hostname on the same row.
  Disabling or stopping the sandbox deletes the row(s). The sandbox
  controller's existing cleanup hook (`controller_cleanup.go`) is extended
  to drop preview rows when it reaps a sandbox; the session lifecycle gains
  the same hook.
- Port to expose is sandbox-typed: the `RuntimeRegistry` entry gains a
  `DefaultPreviewPort` field — `5800` for `ubuntu-desktop` (noVNC), `8080`
  for `headless-ubuntu` and `web-service`, overridable per-sandbox. The UI
  defaults to this; the user can pick a different port and create multiple
  preview tokens (one per port).

### 3. Sandbox runtime: new kind `web-service`

Add `SandboxRuntimeWebService` to `api/pkg/types/sandbox.go` alongside
`UbuntuDesktop` and `HeadlessUbuntu`. Image is `ubuntu:22.04` (same as
headless), unprivileged, but with extra characteristics:

- `Persistent = true` and no idle TTL reaping — web services are long-lived.
- `Purpose = "web-service"` field added to `Sandbox` so we can list all
  web-service sandboxes for a project distinctly from agent sandboxes.
- One container per project, enforced by the sandbox controller (a uniqueness
  constraint on `(project_id, purpose=web-service)` in the sandboxes table).
- Runner agent gains a "run startup script + supervise" task: clones the
  primary repo at the target SHA into `/workspace`, runs
  `bash .helix/startup.sh`, and re-execs the same script if it exits.

### 4. Deploy trigger: extend existing webhook → CD path

Add a new `TriggerConfiguration` kind `web-service-deploy`, auto-created on
the project when the feature is enabled. The existing webhook handler
(`api/pkg/server/webhook_trigger_handlers.go`) dispatches it on pushes whose
`ref == refs/heads/<primary-repo-default-branch>`. The trigger calls a new
`webservice.Redeploy(projectID, sha)` which:

1. Provisions a fresh web-service sandbox at the target SHA (re-uses
   `sandbox.Controller.Create`).
2. Polls `http://container:port/` until 2xx/3xx or timeout (default 90s).
3. Atomically updates `project_web_service_state.active_sandbox_id` to the new
   one.
4. Stops the previous sandbox.

Failures leave the old sandbox active and record a failed `web_service_deploy`
row. The "Deploy now" UI button calls the same `Redeploy` with the current
HEAD SHA, as does the agent tool.

### 5. UI: new "Web Service" tab in Project Settings

Add an entry to the tab conditional block in
`frontend/src/pages/ProjectSettings.tsx:1958-1964` rendering a new
`<WebServiceTab>` component. Sections:

- **Enable** toggle (PATCH project spec).
- **Default URL** (read-only, shown when `HELIX_WEB_SERVICE_BASE_DOMAIN` set).
- **Custom domains** list with add/remove and per-row status badge.
- **Container** sub-panel: internal port input (default `8080`), link
  ("startup script is shared with sandbox tab — edit there").
- **Deploys** table: SHA, time, status, "View logs", "Deploy now" header
  button.

## Data Model Additions

```
project_web_service_state
  project_id (PK, FK projects)
  enabled bool
  container_port int default 8080
  active_sandbox_id (FK sandboxes, nullable)
  updated_at

vhost_routes
  id (PK)
  hostname text unique               -- full lowercased hostname
  target_kind text                   -- 'project_web_service' | 'sandbox_preview'
  target_id text                     -- project_id | sandbox_id | session_id
  port int                           -- destination port inside the container
  is_default bool                    -- auto-generated <slug>.<base> for a project
  verified_at timestamp nullable     -- null=pending, set=usable; auto-set for default/preview rows
  verification_token text nullable   -- null for default/preview rows
  created_at, rotated_at

web_service_deploys
  id (PK)
  project_id, sandbox_id, commit_sha
  status text  -- pending|building|live|failed|superseded
  started_at, finished_at
  log_path text
```

Sandboxes/sessions don't gain a dedicated flag — the presence of one or
more `vhost_routes` rows with `target_kind=sandbox_preview, target_id=<id>`
*is* the "Share preview" enabled state. Cleanup queries the table by
`target_id` when reaping. This avoids a denormalised mirror.

## Caddy Compatibility Notes

When Caddy fronts Helix:
- Set `HELIX_VHOST_TLS_MODE=passthrough`.
- For dynamic previews + project default subdomains to work, give Caddy a
  wildcard cert for `*.<DEV_SUBDOMAIN base>` via a DNS-01 provider plugin.
  Example with `DEV_SUBDOMAIN=dev.helix.example.com`:
  ```
  *.dev.helix.example.com, dev.helix.example.com {
      tls { dns cloudflare {env.CF_API_TOKEN} }
      reverse_proxy helix-api:80
  }
  helix.example.com, my-custom.example.com {
      reverse_proxy helix-api:80
  }
  ```
  Include the canonical Helix domain in the same upstream so the main app
  and hosted services both reach Helix.
- Helix still owns vhost → sandbox routing and the canonical-domain dispatch
  because Caddy doesn't know about per-project sandboxes; it just hands every
  matching hostname to Helix on HTTP.
- Wildcard custom domains are still hijack-protected by the reserved-hostname
  helper (§2), so a project cannot register `helix.example.com` even if
  Caddy is configured to forward it.

## Open Questions (resolve during implementation)

- **Wildcard certs vs per-domain in `auto` mode?** Start with per-hostname
  HTTP-01 (no DNS plugin needed). Revisit if many default subdomains create
  ACME pressure.
- **Resource quotas for web-service sandboxes?** Reuse existing sandbox vCPU
  / RAM defaults; org-level quotas are out of scope here.
- **Log persistence for deploys?** Reuse the existing sandbox-log storage
  pattern; cap at last 10 deploys per project to start.

---

## Implementation Notes (added during build)

### What landed in this PR (backend, MVP)

End-to-end functional path from operator → live customer website:

1. **Data model:** `vhost_routes`, `project_web_service_state`,
   `web_service_deploys` types + GORM AutoMigrate + Store CRUD methods.
   Polymorphic `vhost_routes` holds both project_web_service hostnames
   (user-named) and sandbox_preview hostnames (random share-* tokens).
2. **`api/pkg/vhost` package** — pure helpers:
   - `GenerateShareHostname` (curated word lists + 32 random bits).
   - `ReserveHostname` (anti-hijack: canonical from SERVER_URL, alias
     list, DEV_SUBDOMAIN apex, built-in reserved labels including
     sub-of-reserved, operator extras, `share-` prefix, existing rows).
   - `AllocateDefaultSubdomain` (`-2`, `-3`, … suffixing).
   - `MintShareHostname` (retry-on-collision loop).
3. **`VHostMiddleware`** in `api/pkg/server/vhost_middleware.go`
   replaces the deleted `SubdomainProxyMiddleware`. Dispatch:
   - Canonical hostname (from SERVER_URL) → main mux fall-through.
   - `share-*.<DEV_SUBDOMAIN base>` → vhost_routes preview lookup.
   - Any other host with a verified vhost_routes row → project web
     service lookup.
   - Else → main mux (renders 404).
4. **Shared `proxyToContainer`** in `vhost_proxy.go` extracted from
   the deleted `proxyToSessionPort`. RevDial to `hydra-<sandboxID>`
   and HTTP-over-RevDial via `bufio.Read`. Both dispatch branches
   reuse this single implementation.
5. **Project web service HTTP API** (`project_web_service_handlers.go`):
   - `GET/PUT /api/v1/projects/:id/web-service`
   - `POST /api/v1/projects/:id/web-service/active-sandbox` (manual
     deploy primitive — operator/orchestrator both call this)
   - `POST/DELETE /api/v1/projects/:id/web-service/domains[/:id]`
   - `GET /.well-known/helix-domain-verify/:token` (public verifier)
6. **Session preview tokens HTTP API**
   (`session_preview_handlers.go`): mint/list/rotate/delete with
   `authorizeUserToSession(ActionUpdate)`. Session-delete cleanup
   hook revokes tokens.
7. **Unit tests:** share-token format + 5k uniqueness, every reserve
   rejection category, slug normalisation, parseVHostConfig matrix,
   stripPort IPv6 handling.

### Decisions taken during build

- **Old `p{port}-{session_id}` scheme deleted entirely** (per user
  direction "no one is using those routes"). Removed:
  `session_expose_handlers.go` (807 lines), `subdomain_proxy.go`
  (195 lines), `subdomain_proxy_test.go` (247 lines). Saves
  ~1250 lines and removes a parallel URL scheme.
- **No new env vars beyond what's already there.** Reuses `SERVER_URL`
  (parsed via existing `parseDevSubdomainConfig` logic, now inlined
  as `hostnameOf`) and `DEV_SUBDOMAIN` (already the base-domain config
  for the deleted scheme).
- **Manual-deploy primitive ships first.** `POST .../active-sandbox`
  lets an operator point a project's vhost at a sandbox they
  manually provisioned. The auto-deploy-on-push orchestrator (the
  Phase 5/6/7 work in tasks.md) is a follow-up that calls the same
  store primitive once it knows which sandbox holds the new build.
  This means production hosting works today without the runner-side
  workload supervisor being built.
- **TLS passthrough only in v1.** Operator runs Caddy with a wildcard
  DNS-01 cert for `*.<DEV_SUBDOMAIN base>`. Embedded certmagic auto
  mode is documented but deferred — the data plane is independent of
  where TLS terminates.
- **No in-memory route cache (yet).** Each request hits the store. If
  profiling shows it's a hot path, a pubsub-invalidated cache is a
  small follow-up against the same `Store.GetVHostRouteByHostname`
  call site.

### Deferred to follow-up PRs

- **Frontend `WebServiceTab` + `<SharePreviewSection>`.** Endpoints
  are callable via curl / cli today; the React tab is a clear
  follow-up. Will need `./stack update_openapi` first to regenerate
  the typed client (swagger annotations added on the new handlers
  in this PR so `update_openapi` picks them up).
- **Auto-deploy-on-push orchestrator.** The `webservice.Redeploy`
  state machine, the `web-service` sandbox runtime, the runner-side
  workload supervisor (clone + run `.helix/startup.sh` + restart),
  the `TriggerKind = web-service-deploy` webhook dispatch.
- **certmagic `auto` TLS mode.** `passthrough` ships now.
- **Sandbox (`sbx_*`) preview tokens.** Sessions cover the
  high-value cases (spec tasks, agents, desktops); a hydra route
  addressable by sandbox ID is the small bit of glue needed.
- **DNS verifier cron.** The `.well-known/helix-domain-verify/:token`
  endpoint is in place; an automated poller marking `verified_at` on
  successful round-trip is the cleanup.
