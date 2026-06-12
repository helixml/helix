# Requirements: Project Web Service Hosting on Sandboxes

## Background

Sandboxes (runners) already host two workload classes: LLM inference and agent
workspaces (desktop or headless dev containers). This adds a third class:
long-running **web services** — a single headless container per project that
runs a web app, a static site, or a full `docker compose` stack. Public traffic
reaches it via name-based virtual hosting through the Helix API server, which
also handles TLS. Deploys are triggered automatically by pushes to the primary
repo's default branch — a minimal continuous-delivery loop with no CI gates.

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
- A single env var (`HELIX_WEB_SERVICE_TLS_MODE = auto | passthrough | off`)
  selects: terminate TLS in Helix (Let's Encrypt), trust an upstream reverse
  proxy (Caddy, Cloudflare, etc.), or disable web-service hosting entirely.
- In `passthrough` mode Helix listens HTTP only on its existing port and
  trusts `X-Forwarded-Proto` / `X-Forwarded-Host`.
- In `auto` mode Helix listens on `:443` and `:80` for ACME challenges.

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
- Authentication in front of web services — services are public; private
  previews are a future task.
