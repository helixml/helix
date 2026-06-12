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

Helix already ships a subdomain-routing middleware that this design extends
rather than parallels:

- **`SERVER_URL`** (`config.go:635`) — the existing canonical Helix URL.
  Hostname is extracted via the same parsing already in
  `parseDevSubdomainConfig` (`subdomain_proxy.go:152`). **No
  `HELIX_CANONICAL_DOMAIN` env var is introduced.** If the operator needs to
  declare additional canonical hostnames, add an optional comma-separated
  `SERVER_URL_ALIASES`.
- **`DEV_SUBDOMAIN`** (`config.go:669`) — already enables wildcard-subdomain
  vhost routing for dev container ports (`p{port}-{session_id}.dev.<base>`).
  The existing `SubdomainProxyMiddleware` (`subdomain_proxy.go:48`) is the
  starting point; this design **adds new dispatch rules to that same
  middleware** rather than introducing a parallel router. **No
  `HELIX_VHOST_BASE_DOMAIN` env var is introduced** — the base is whatever
  `DEV_SUBDOMAIN` already resolves to.
- **Existing `p{port}-{session_id}` and `{name}-{session_id}` schemes**
  remain unchanged (backwards-compatible). The new schemes layered on top:
  - `<random-alias>.<dev_subdomain>.<base>` — looked up in `vhost_routes`,
    proxies to a specific sandbox + port (used by the "Share preview"
    toggle for any sandbox, including agent workspaces, spec tasks, and
    human org desktops; not session-port-specific).
  - `<project-slug>.<dev_subdomain>.<base>` — project default subdomain for
    web services.
  - Any custom hostname registered against a project.
- **No new env var** for "dynamic previews enabled" — if `DEV_SUBDOMAIN` is
  set the feature is available, same gate the existing scheme uses.

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
(`api/pkg/server/subdomain_proxy.go`), extended with two more rule
branches **before** falling through to the next handler:

1. If `Host` matches the canonical hostname parsed from `SERVER_URL` (or
   any `SERVER_URL_ALIASES` entry) → fall through to the existing API/UI
   mux. The control-plane app is just another consumer of the same listener.
2. Else if `Host` matches the existing `p{port}-{session_id}` or
   `{name}-{session_id}` schemes → existing dispatch (unchanged).
3. Else if `Host` matches a verified `vhost_routes` row (in-memory cache,
   change-notified via existing pubsub) → resolve the target sandbox (project
   → active sandbox, or direct lookup for previews); get its `HostDeviceID`
   and configured port; proxy via `connman.Dial(deviceID, port)` +
   `ResilientProxy` to the runner. No new transport; same revdial used for
   desktop bridges today (api/pkg/sandbox/controller.go:260).
4. Else → 404 unknown-host page (Helix-branded; clearly identifies the
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
- any host already present in `vhost_routes` (DB unique constraint on
  `hostname`).

Default-subdomain allocation that collides with the reserved set or an
existing row appends `-2`, `-3`, … to the slug. Random preview generation
loops until it finds a free name. The reserved set is consulted from a single
place — the helper — so the certmagic `OnDemand.DecisionFunc` reuses it to
refuse cert issuance for any forbidden hostname even if a malicious row
somehow appeared.

**Dynamic subdomain minting** (sandbox previews):

- Random segment is two adjective+noun words plus a 4-hex suffix
  (`purple-otter-3f8a`); generated from `crypto/rand` to give ≥64 bits of
  entropy. Concatenated with `<DEV_SUBDOMAIN>.<base>` (using the existing
  parsed config), so a preview URL looks like
  `purple-otter-3f8a.dev.helix.example.com`.
- Lifecycle is tied to the sandbox: enabling the "Share preview" toggle
  creates a `vhost_routes` row; disabling it or stopping/deleting the
  sandbox removes it. The sandbox controller's existing cleanup hook
  (`controller_cleanup.go`) is extended to drop preview routes when it
  reaps a sandbox.
- Port to expose is sandbox-typed: the `RuntimeRegistry` entry gains a
  `DefaultPreviewPort` field — `5800` for `ubuntu-desktop` (noVNC), `8080`
  for `headless-ubuntu` and `web-service`, overridable per-sandbox.

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
  hostname text unique
  target_kind text  -- 'project_web_service' | 'sandbox_preview'
  target_id text    -- project_id or sandbox_id depending on kind
  port int          -- destination port inside the container
  is_default bool   -- auto-generated <slug>.<base> for a project
  is_dynamic bool   -- random per-sandbox subdomain
  verified_at timestamp nullable     -- null=pending, set=usable
  verification_token text nullable   -- null for default/dynamic rows
  created_at

web_service_deploys
  id (PK)
  project_id, sandbox_id, commit_sha
  status text  -- pending|building|live|failed|superseded
  started_at, finished_at
  log_path text
```

A `sandbox_preview_enabled` boolean is added to the `sandboxes` table for the
"Share preview" toggle state (mirrors the `vhost_routes` row but is the
canonical source so the cleanup hook can find it without joining).

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
