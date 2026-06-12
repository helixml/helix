# Implementation Tasks: Project Web Service Hosting on Sandboxes

## Backend — data model & types

- [ ] Add `SandboxRuntimeWebService` and `Purpose` field to `api/pkg/types/sandbox.go`.
- [ ] Add `web-service` runtime entry to the default `RuntimeRegistry` in `api/pkg/sandbox/runtimes.go` (image `ubuntu:22.04`, headless, persistent, no idle TTL).
- [ ] Add migrations for `project_web_service_state`, `project_web_service_domains`, `web_service_deploys` tables with FKs and the `(project_id, purpose=web-service)` uniqueness constraint on sandboxes.
- [ ] Add CRUD store methods in `api/pkg/store/` for the three new tables.

## Backend — vhost router & TLS

- [ ] New package `api/pkg/webservice/` with a `Router` that resolves `Host` → projectID → active sandbox via the new tables (in-memory cache, pubsub invalidation on writes).
- [ ] HTTP middleware in the API server that, when `Host` matches a known web-service domain, proxies through `connman.Dial(deviceID)` + `ResilientProxy` to the sandbox container port. All other hostnames fall through to the existing API/UI handlers.
- [ ] Add `HELIX_WEB_SERVICE_TLS_MODE`, `HELIX_WEB_SERVICE_BASE_DOMAIN`, `HELIX_WEB_SERVICE_LETSENCRYPT_EMAIL` to `api/pkg/config/`.
- [ ] In `auto` mode wire `github.com/caddyserver/certmagic` with on-demand TLS gated on the domain table (`OnDemand.DecisionFunc` checks the hostname exists & is verified). Bind `:443` and `:80`.
- [ ] In `passthrough` mode trust `X-Forwarded-Proto` / `X-Forwarded-Host`; skip cert mgr; keep existing listener.

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

## Frontend — Project Settings

- [ ] Add `web-service` to the tab conditional block in `frontend/src/pages/ProjectSettings.tsx:1958-1964`.
- [ ] New `<WebServiceTab>` component: enable toggle, default URL display, custom-domain list with status badges, container-port input, deploys table with "Deploy now" button.
- [ ] Mutations via existing `updateProjectMutation` pattern plus new domain/deploy endpoints; invalidate queries after each.

## Docs & ops

- [ ] Update `helix` docs: new env vars, required DNS setup (wildcard `A`/`CNAME` for base domain), Caddy snippet for `passthrough` mode.
- [ ] Add a sample project showing a tiny `.helix/startup.sh` that boots a web server on `:8080`.

## Tests

- [ ] Unit: runtime registry, vhost router (cache + invalidation), webhook ref filter, redeploy state machine (success / health-fail / cutover-fail).
- [ ] Integration: enable feature → push to primary repo → assert sandbox provisioned, domain resolves, `/` returns 200 via the API proxy.
- [ ] Integration: `passthrough` mode behind a stub upstream proxy passes `X-Forwarded-*` correctly.
