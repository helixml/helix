# Design: Project Web Service Hosting on Sandboxes

## Architecture Overview

```
visitor ──HTTPS──► Helix API (:443)
                     │
                     ├─ TLS terminate  ◄── certmagic / Let's Encrypt
                     ├─ vhost lookup   ◄── project_web_service_domains
                     └─ proxy via revdial → runner host → container:port
                                              ▲
                                              │ deploy: git push or manual
                                              │
                            primary repo webhook ──► CD trigger ──► sandbox controller
```

Three new subsystems are added; everything else extends existing code paths.

## Key Decisions

### 1. TLS termination: embedded `certmagic`, with `passthrough` escape hatch

Let's Encrypt termination lives in the Helix API server using
`github.com/caddyserver/certmagic` (the same ACME library Caddy uses; pure-Go,
file-backed cert storage we already have on disk, supports HTTP-01 and
TLS-ALPN-01 — no DNS plugin required for non-wildcard certs).

- **`HELIX_WEB_SERVICE_TLS_MODE=auto`** (default when feature enabled):
  Helix binds `:443` + `:80`, terminates TLS, issues per-hostname certs
  on-demand. ACME challenges served from the same listener.
- **`HELIX_WEB_SERVICE_TLS_MODE=passthrough`**: Helix binds HTTP only on its
  existing API port, trusts `X-Forwarded-Proto` / `X-Forwarded-Host` / `Host`.
  Operator runs Caddy or similar in front. Default-subdomain certs are
  Caddy's problem.
- **`HELIX_WEB_SERVICE_TLS_MODE=off`** (default when no base domain set):
  Feature disabled, panel hidden in UI.

Why not stick with the cloud-managed cert pattern already used in GKE Helm?
Cloud-managed certs require a fixed list of SANs; per-project domains added at
runtime can't be issued without operator intervention. certmagic issues on
first request.

### 2. Vhost routing: hostname → project → revdial → container

A new table `project_web_service_domains` maps `hostname → project_id` with a
unique constraint on hostname. Default subdomains are pre-seeded as
`<project-slug>.<base-domain>` when the feature is toggled on; custom domains
are inserted only after the operator's DNS resolves to the API server (we
verify via a short HTTP token at `/.well-known/helix-domain-verify/<token>`).

On request:
1. HTTP middleware looks at `Host`, queries the table (cached in memory with
   change-notify via existing pubsub).
2. Resolves the project's active web-service sandbox; gets its `HostDeviceID`
   and configured port.
3. Reuses `connman.Dial(deviceID)` + `ResilientProxy` to bridge the
   client connection to the runner, which forwards locally to `127.0.0.1:port`
   inside the container. No new transport; same revdial used for desktop
   bridges today (api/pkg/sandbox/controller.go:260).

WebSocket and HTTP/2 just work because `ResilientProxy` already handles
upgrades.

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

project_web_service_domains
  id (PK)
  project_id (FK projects)
  hostname text unique
  is_default bool   -- true for auto-generated <slug>.<base>
  verified_at timestamp nullable
  verification_token text
  created_at

web_service_deploys
  id (PK)
  project_id, sandbox_id, commit_sha
  status text  -- pending|building|live|failed|superseded
  started_at, finished_at
  log_path text
```

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
