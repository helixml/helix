# Design: Project Web Service Hosting on Sandboxes

## Architecture Overview

```
visitor ──HTTPS──► Helix API (:443)
                     │
                     ├─ TLS terminate  ◄── certmagic / Let's Encrypt (on-demand)
                     ├─ vhost lookup   ◄── vhost_routes (project web services
                     │                       AND ephemeral sandbox previews)
                     └─ proxy via revdial → runner host → container:port
                                              ▲
                                              │ deploy: git push, manual, or
                                              │  "share preview" toggle
                                              │
                            primary repo webhook ──► CD trigger ──► sandbox controller
```

Three new subsystems are added; everything else extends existing code paths.
The same vhost+TLS plumbing serves **two consumers**:

1. **Project web services** — durable, per-project, user-named domains.
2. **Sandbox dev previews** — ephemeral, per-sandbox, random subdomains, lifecycle
   bound to the sandbox.

## Key Decisions

### 1. TLS termination: embedded `certmagic`, with `passthrough` escape hatch

Let's Encrypt termination lives in the Helix API server using
`github.com/caddyserver/certmagic` (the same ACME library Caddy uses; pure-Go,
file-backed cert storage we already have on disk, supports HTTP-01 and
TLS-ALPN-01 — no DNS plugin required for non-wildcard certs).

Env vars (renamed from `HELIX_WEB_SERVICE_*` because the same plumbing serves
sandbox previews too):

- **`HELIX_VHOST_TLS_MODE=auto`** (default when `HELIX_VHOST_BASE_DOMAIN` set):
  Helix binds `:443` + `:80`, terminates TLS, issues per-hostname certs
  on-demand. ACME challenges served from the same listener.
- **`HELIX_VHOST_TLS_MODE=passthrough`**: Helix binds HTTP only on its
  existing API port, trusts `X-Forwarded-Proto` / `X-Forwarded-Host` / `Host`.
  Operator runs Caddy or similar in front. Only legal when dynamic vhosting
  is **off** — i.e. the only hostnames in use are static custom domains the
  operator has pre-listed in their reverse proxy. Startup validation:
  `tls_mode=passthrough` + `enable_dynamic_vhosts=true` → fatal error
  pointing at docs.
- **`HELIX_VHOST_TLS_MODE=off`** (default when no base domain set):
  Feature disabled, panels hidden in UI.

Why not stick with the cloud-managed cert pattern already used in GKE Helm?
Cloud-managed certs require a fixed list of SANs; per-project domains added at
runtime can't be issued without operator intervention. certmagic issues on
first request. Same reason rules out Caddy `passthrough` for the dynamic
case: even with on-demand TLS in Caddy, the operator would still have to
authorize each random subdomain via an ask-endpoint round-trip, which is
strictly more moving parts than just terminating in Helix.

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

On request:
1. HTTP middleware looks at `Host`, queries the table (cached in memory with
   change-notify via existing pubsub).
2. Resolves the target sandbox (project → active sandbox, or direct lookup
   for previews); gets its `HostDeviceID` and configured port.
3. Reuses `connman.Dial(deviceID)` + `ResilientProxy` to bridge the
   client connection to the runner, which forwards locally to `127.0.0.1:port`
   inside the container. No new transport; same revdial used for desktop
   bridges today (api/pkg/sandbox/controller.go:260).

WebSocket and HTTP/2 just work because `ResilientProxy` already handles
upgrades.

**Dynamic subdomain minting** (sandbox previews):

- Random segment is two adjective+noun words plus a 4-hex suffix
  (`purple-otter-3f8a`); generated from `crypto/rand` to give ≥64 bits of
  entropy. Concatenated with `HELIX_VHOST_BASE_DOMAIN`.
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
- Set `HELIX_WEB_SERVICE_TLS_MODE=passthrough`.
- Caddy config: `*.apps.example.com, my-custom.example.com { reverse_proxy helix-api:80 }`.
  Caddy already does on-demand TLS with DNS plugins for the wildcard.
- Helix still owns vhost → sandbox routing because Caddy doesn't know about
  per-project sandboxes; it just hands every matching hostname to Helix.

## Open Questions (resolve during implementation)

- **Wildcard certs vs per-domain in `auto` mode?** Start with per-hostname
  HTTP-01 (no DNS plugin needed). Revisit if many default subdomains create
  ACME pressure.
- **Resource quotas for web-service sandboxes?** Reuse existing sandbox vCPU
  / RAM defaults; org-level quotas are out of scope here.
- **Log persistence for deploys?** Reuse the existing sandbox-log storage
  pattern; cap at last 10 deploys per project to start.
