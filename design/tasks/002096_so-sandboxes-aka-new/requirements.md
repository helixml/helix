# Requirements: Project Web Service Hosting on Sandboxes

## Background

Sandboxes (runners) already host two workload classes: LLM inference and agent
workspaces (desktop or headless dev containers). This adds a third class:
long-running **web services** — a single headless container per project that
runs a web app, a static site, or a full `docker compose` stack. Public traffic
reaches it via name-based virtual hosting through the Helix API server, which
also handles TLS. Deploys are triggered automatically by pushes to the primary
repo's default branch — a minimal continuous-delivery loop with no CI gates.

The same vhost+TLS plumbing is also reused for a second feature: **dev
previews**. Any running sandbox — an agent workspace, a spec-task container,
or a human org desktop — can be exposed under a randomly-generated subdomain
so its in-progress UI is shareable without setting up tunnels.

## User Stories

### As a project owner, I want to expose my project as a web service
So that I can show stakeholders a live preview without setting up my own infra.

**Acceptance:**
- A new "Web Service" panel exists in Project Settings.
- A toggle enables/disables web-service hosting for the project (off by default).
- When enabled, an auto-generated default subdomain
  (`<project-slug>.<base-domain>`) immediately serves the project.
- I can add/remove additional custom domains; each shows its DNS-readiness
  state ("needs CNAME to X", "verified", "TLS issued", "error: …").
- I can configure the internal port the container listens on (default `8080`).
- Disabling the toggle stops the sandbox and removes routing for all domains.

### As a project owner, I want my web service to redeploy on every push to main
So that the live preview always reflects the current state of the default branch.

**Acceptance:**
- When the primary repo's default branch receives a push, the web-service
  sandbox is rebuilt: the new commit is cloned, the project startup script
  (`.helix/startup.sh`) is executed, and traffic is cut over once the container
  is responding on the configured port.
- The previous sandbox stops only after the new one is healthy (zero-downtime
  best-effort; single-instance, so brief gaps are acceptable).
- A "Deploy now" button in the Web Service panel triggers the same flow on
  demand against the current HEAD.
- A "Deploys" list shows the last N deploys with commit SHA, timestamp,
  status (pending/building/live/failed), and a link to logs.

### As a visitor, I want to reach a project's web service over HTTPS
So that I can use it like any other website.

**Acceptance:**
- `https://<configured-domain>/…` reaches the project's sandbox container.
- TLS certs for default subdomains and verified custom domains are obtained
  and renewed automatically via Let's Encrypt (ACME HTTP-01 + TLS-ALPN-01).
- WebSockets and HTTP/2 work end-to-end.
- An unknown host returns a clear 404 page identifying it as Helix.

### As an operator, I want Helix to work whether exposed directly or behind Caddy
So that existing deployments don't break.

**Acceptance:**
- A single env var (`HELIX_VHOST_TLS_MODE = auto | passthrough | off`)
  selects: terminate TLS in Helix (Let's Encrypt), trust an upstream reverse
  proxy (Caddy, Cloudflare, etc.), or disable vhost-based hosting entirely.
- In `passthrough` mode Helix listens HTTP only on its existing port and
  trusts `X-Forwarded-Proto` / `X-Forwarded-Host`.
- In `auto` mode Helix listens on `:443` and `:80` for ACME challenges, and
  terminates TLS for **all** hostnames it serves — including the main Helix
  control-plane domain itself. No separate listener / cert manager for the
  main app vs hosted web services.
- **`passthrough` + dynamic vhosting is supported** when the upstream
  reverse proxy holds a wildcard certificate covering
  `*.<DEV_SUBDOMAIN's base domain>` (typically obtained via ACME DNS-01). In
  that case Helix only ever speaks HTTP and the wildcard cert covers every
  dynamically-minted subdomain. At startup Helix logs a single clear warning
  describing the operator's responsibility ("ensure upstream serves a
  wildcard cert for `*.<base>`, otherwise dynamic previews will fail TLS");
  it does not refuse to start.
- For **custom domains** (not subdomains of the base) in `passthrough` mode,
  the operator must add each one to the upstream proxy configuration just
  like any other reverse-proxied site. Helix surfaces a per-domain hint in
  the UI ("In passthrough mode, add this hostname to your reverse proxy
  configuration") once verified.

### As an operator, I want the main Helix app to be served through the same vhost layer
So that there is one consistent routing/TLS path and no parallel listener to maintain.

**Acceptance:**
- The canonical Helix hostname — derived from the existing `SERVER_URL` env
  var (parsed via the same logic `parseDevSubdomainConfig` uses today) —
  is served by the same listener and the same cert manager as project web
  services and sandbox previews. No new env var is introduced for this; if
  the operator needs to declare additional canonical hostnames (e.g. an
  internal-only alias), a new optional `SERVER_URL_ALIASES` (comma-separated)
  is the only addition.
- A request whose `Host` matches the canonical Helix hostname is dispatched
  to the existing main API / UI mux; a request whose `Host` matches a
  `vhost_routes` row is proxied to the appropriate sandbox; an unknown host
  returns a Helix-branded 404.
- This holds in both `auto` and `passthrough` modes. In `passthrough` mode
  the upstream proxy is expected to forward all relevant hostnames
  (canonical + base wildcard + custom domains) to Helix on its HTTP port.

### As an operator, I want users unable to hijack the main Helix domain
So that no project can serve its own content from the control-plane URL.

**Acceptance:**
- A reserved-hostname list blocks any user from registering, verifying, or
  having a default subdomain allocated for:
  - the canonical Helix hostname derived from `SERVER_URL` (plus any
    `SERVER_URL_ALIASES`);
  - the bare apex of `DEV_SUBDOMAIN`'s base domain;
  - a built-in set of reserved labels under that base domain
    (`api`, `app`, `www`, `auth`, `admin`, `helix`, `console`, `dashboard`,
    `helix-admin`, `mail`, `ns`) plus any extras the operator adds via
    `HELIX_VHOST_RESERVED_SUBDOMAINS`;
  - any hostname that already exists in `vhost_routes` (FK uniqueness on
    `hostname`).
- Custom-domain registration attempts that hit one of these are rejected
  at the API with a clear error; UI surfaces the same message.
- Project-slug → default-subdomain allocation skips reserved labels and
  appends a numeric suffix if the slug itself collides (e.g.
  `app-2.<base>` if the slug is `app`).
- Auto-generated random preview subdomains regenerate if the rolled name
  hits a reserved label or an existing row.
- Security test: a project owner POSTing the canonical Helix domain (or
  `api.<base>`, etc.) as a custom domain receives `409 Conflict` /
  `403 Forbidden` and no row is written.

### As an agent or human user, I want to share a live URL for my running sandbox
So that I can show a teammate the current state of an agent's work or my desktop.

**Acceptance:**
- Every sandbox detail panel (agent workspace, spec-task session, human org
  desktop, user-facing API sandbox) has a "Share preview" section. When the
  operator has not configured `DEV_SUBDOMAIN`, the section is disabled with
  a tooltip explaining why.
- Toggling "Share preview" on mints a **purpose-built random preview token**
  and exposes the chosen port at
  `https://share-<adj>-<noun>-<4hex>.<DEV_SUBDOMAIN>/` (e.g.
  `share-purple-otter-3f8a.dev.helix.example.com`). The token is unrelated
  to the sandbox/session ID so leaking the ID through screenshots, logs,
  the UI, or other channels does **not** leak the preview URL.
- For desktop sandboxes the default exposed port is the existing noVNC/web
  stream port; for headless sandboxes the user picks the port.
- The user can rotate the token (one click → old URL stops working, new URL
  takes effect) and can disable sharing entirely. Stopping the sandbox
  also revokes the token. After revocation the URL returns 404.
- A sandbox can have multiple active preview tokens (e.g. one per port);
  each is shown in the panel with copy/rotate/revoke controls.
- Token entropy ≥ 70 bits (the `share-<adj>-<noun>-<4hex>` format pulls
  from ~150-word adjective and noun lists plus 16 random hex bits, giving
  ~31 bits friendly + 16 bits hex = 47 bits — **bump the hex segment to 8
  hex chars** for ≥ 63 bits total, still readable).
- This is **distinct from**, not a replacement for, the existing
  `p{port}-{session_id}` / `{name}-{session_id}` schemes
  (`subdomain_proxy.go`), which remain for the dev-container debug workflow
  that already ships. The new feature is the *opt-in shareable* path; the
  existing one is the *if-you-know-the-session-ID-you're-already-trusted*
  path. Both go through the same middleware and the same TLS layer.

### As an agent, I want to deploy a project I'm working on
So that I can hand a live URL back to the user.

**Acceptance:**
- An agent can call a "deploy this project" tool that triggers the same
  redeploy flow as a git push.
- The agent receives back the resulting URL and live log stream.

## Out of Scope

- CI gates (test/lint before deploy) — explicitly deferred per user.
- Multiple instances per project / blue-green / canary — single instance only.
- Per-branch preview environments — only the primary repo's default branch.
- DNS provisioning — the operator sets up the wildcard `A`/`CNAME` for the
  base domain manually; we only document what's required.
- Authentication in front of web services and dev previews — both rely on
  domain knowledge / unguessable subdomains; private previews behind SSO are
  a future task.
