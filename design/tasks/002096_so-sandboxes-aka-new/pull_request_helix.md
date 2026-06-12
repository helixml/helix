# feat(api): name-based virtual hosting for project web services and session previews

## Summary

Lays the data-plane foundation for hosting customer websites and web
applications on Helix sandboxes via name-based virtual hosting. Replaces
the unused `p{port}-{session_id}` URL scheme with a single share-token +
project-web-service dispatcher that resolves hostnames through a new
`vhost_routes` table. Operators can now point a project at a sandbox via
HTTP API, register custom domains, and route HTTPS traffic through Caddy
(passthrough mode) into the container.

## What works end-to-end after this PR

A project owner / CI pipeline / operator can host a real customer
website with a single API call once setup is done:

1. **Operator setup (one time):** Caddy in front of Helix with a wildcard
   DNS-01 cert covering `*.<DEV_SUBDOMAIN base>`. Set `DEV_SUBDOMAIN`
   env var on the API.
2. **Project owner enables:** `PUT /api/v1/projects/:id/web-service
   {enabled: true, container_port: 8080}` → default subdomain
   `<slug>.<base>` allocated.
3. **Optional custom domain:** `POST .../web-service/domains
   {hostname: "app.customer.com"}`, point CNAME at the Caddy front,
   `/.well-known/helix-domain-verify/:token` round-trip flips
   `verified_at` and the row goes live.
4. **Deploy:** `POST /api/v1/projects/:id/web-service/deploy
   {commit_sha: "abc123"}` (sha optional). Helix:
   - Provisions a fresh headless sandbox in the project's org.
   - Polls until the sandbox is running.
   - Execs a bootstrap inside it: `git clone <primary repo>`, checkout
     the requested SHA, `bash .helix/startup.sh` (which becomes the
     long-running web server process).
   - Atomically points routing at the new sandbox.
   - Stops the previous sandbox.
   Returns the in-flight deploy row immediately (status=pending /
   building); the whole flow runs asynchronously and the row's
   `status` advances to `live` or `failed` so the UI can poll.
5. **Live traffic:** any HTTPS request to a verified domain proxies
   through Caddy → vhost middleware → RevDial → hydra → container
   port `container_port`.

Sessions also get a "Share preview" path: mint a random
`share-<adj>-<noun>-<8hex>.<base>` token bound to a session + port; the
URL works until the token is revoked or the session is deleted.

## Architectural changes

- **Delete** `session_expose_handlers.go`, `subdomain_proxy.go`,
  `subdomain_proxy_test.go` and the `/api/v1/sessions/{id}/proxy/{port}`
  + `/sessions/{id}/expose` routes — the `p{port}-{session_id}` scheme
  had no consumers per user direction.
- **Add** `api/pkg/vhost/` (share-token generator, anti-hijack
  `ReserveHostname`, default-subdomain allocator).
- **Add** `vhost_middleware.go` + `vhost_proxy.go`. Dispatch order:
  canonical hostname (from `SERVER_URL`) → main mux; `share-*.<base>`
  → preview lookup; any other host → project web service lookup; else
  → main mux 404.
- **Add** polymorphic `vhost_routes` table + per-project
  `project_web_service_states` + `web_service_deploys` history.
- **Reuse** existing env vars only (`SERVER_URL`, `DEV_SUBDOMAIN`).
  No new env vars introduced for hostname/base; reserved-hostname
  policy uses what's already there.
- **Add** `api/pkg/webservice/Controller`. Composes existing
  `sandbox.Controller.Create` + `hydra.RevDialClient.RunSandboxCommand`
  + the new store primitives. No new sandbox runtime, no new
  runner-side code — the user's `.helix/startup.sh` becomes the
  long-running web server process.

## Files

```
NEW api/pkg/types/vhost.go                              - 3 types + status enum
NEW api/pkg/store/vhost.go                              - 12 store methods
NEW api/pkg/vhost/sharetoken.go                         - random share-* generator
NEW api/pkg/vhost/reserve.go                            - anti-hijack policy
NEW api/pkg/vhost/slug.go                               - default-subdomain allocator
NEW api/pkg/vhost/vhost_test.go                         - unit tests
NEW api/pkg/server/vhost_middleware.go                  - new dispatch middleware
NEW api/pkg/server/vhost_middleware_test.go             - parser/strip tests
NEW api/pkg/server/vhost_proxy.go                       - shared revdial proxy fn
NEW api/pkg/server/project_web_service_handlers.go      - PUT/GET/deploy/active-sandbox/domains
NEW api/pkg/server/session_preview_handlers.go          - preview-tokens CRUD
NEW api/pkg/webservice/controller.go                    - provision → bootstrap → cutover
NEW api/pkg/webservice/controller_test.go               - shellEscape safety
MOD api/pkg/store/postgres.go                           - AutoMigrate registrations
MOD api/pkg/store/store.go                              - Store interface methods
MOD api/pkg/store/store_mocks.go                        - regenerated
MOD api/pkg/system/uuid.go                              - VHostRoute/WebServiceDeploy IDs
MOD api/pkg/server/server.go                            - swap middleware + register routes
MOD api/pkg/server/session_handlers.go                  - cleanup hook on session delete
DEL api/pkg/server/session_expose_handlers.go           - 807 lines removed
DEL api/pkg/server/subdomain_proxy.go                   - 195 lines removed
DEL api/pkg/server/subdomain_proxy_test.go              - 247 lines removed
```

## Tests

- `api/pkg/vhost/vhost_test.go`: share-token format, 5k uniqueness
  pass, ReserveHostname table for canonical/alias/apex/reserved
  label/sub-of-reserved/operator-extra/`share-` prefix, AllowSharePrefix
  opt-in, slug normalisation.
- `api/pkg/server/vhost_middleware_test.go`: `parseVHostConfig` matrix
  (DEV_SUBDOMAIN unset / prefix / full domain × SERVER_URL set/unset),
  stripPort IPv6 handling.
- `go build ./api/...` green.

## What ALSO landed (originally planned as follow-ups, all shipped)

- **`WebServiceTab` in Project Settings** — sidebar item with globe
  icon, full enable/port/domains/deploy/active-sandbox/deploys UI.
  Uses the regenerated typed API client; React Query for cache +
  invalidation. Visually verified in helix-in-helix
  (`screenshots/03-web-service-tab-enabled.png`).
- **Auto-deploy on push** — `GitHTTPServer.SetOnDefaultBranchPush`
  hook fires after every successful push to a primary repo's
  default branch. The hook calls `webservice.Controller.Redeploy`
  on every project whose primary repository is that repo AND has
  web service enabled. New store method
  `ListEnabledWebServiceProjectsByRepo` joins
  `projects + project_web_service_states`.
- **DNS verifier cron** — `webservice.DomainVerifier` runs every
  60s, polls pending vhost_routes rows, makes
  `GET http://<host>/.well-known/helix-domain-verify/<token>`, and
  marks `verified_at` when the response body echoes the token.
  No redirect following (defends against bogus echo services).
- **Real readiness check before cutover** —
  `hydra.RevDialClient.ProbeDevContainerPort` makes a HEAD request
  through the existing dev-container proxy until any HTTP response
  comes back, or 90s deadline. Replaces the previous fixed 10s
  sleep. Treats 4xx/5xx as success (listener bound is what we care
  about, not the response shape).
- **`./stack update_openapi` regenerated** — frontend gets typed
  methods for every new endpoint plus typed result shapes.

## What's deferred to follow-up PRs

- **certmagic `auto` TLS mode.** Passthrough mode ships now —
  operator runs Caddy with a wildcard DNS-01 cert. Embedded
  certmagic with on-demand TLS gated on `vhost.ReserveHostname`
  is the cleanup.
- **Sandbox `sbx_*` preview tokens.** Sessions cover spec tasks,
  agents, and desktops — the high-value cases. Sandbox-API
  containers are unblocked in the middleware (the `sbx_` branch
  works now that the device-key fix is in); mirror endpoints
  under `/api/v1/sandboxes/:id/preview-tokens` is a small follow-up.
- **`<SharePreviewSection>` in session detail UI.** Preview-token
  endpoints are callable today.
- **External-webhook `TriggerKind = "web-service-deploy"`** —
  GitHub Actions / GitLab CI call `POST /web-service/deploy`
  directly today. The trigger config is the configurable wrapper.

## Screenshots

![Web Service tab in Project Settings](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002096_so-sandboxes-aka-new/screenshots/03-web-service-tab-enabled.png)

`screenshots/test-results.md` contains the full curl-level
validation log: vhost dispatch by Host header, fall-through to
main app, reserved-hostname rejection, DNS verification round-trip,
unverified→503 / verified→proxy.

## Operator setup

```caddyfile
*.dev.helix.example.com, dev.helix.example.com {
    tls { dns cloudflare {env.CF_API_TOKEN} }
    reverse_proxy helix-api:80
}
helix.example.com, customer.example.com {
    reverse_proxy helix-api:80
}
```

Set `DEV_SUBDOMAIN=dev.helix.example.com` (or just `dev`) in Helix env
and the vhost middleware activates.
