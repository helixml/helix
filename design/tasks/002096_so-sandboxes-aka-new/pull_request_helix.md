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

Operator-driven path for hosting a real website on a sandbox:

1. Operator runs Caddy in front of Helix with a wildcard DNS-01 cert
   covering `*.<DEV_SUBDOMAIN base>` (e.g. `*.dev.helix.example.com`).
2. Project owner: `PUT /api/v1/projects/:id/web-service {enabled: true}`
   → default subdomain `<slug>.<base>` is allocated automatically with
   collision suffixing.
3. Project owner (optional): `POST .../web-service/domains
   {hostname: "app.customer.com"}` → row inserted unverified; once the
   operator's DNS points at us and the `/.well-known/helix-domain-verify/:token`
   endpoint round-trips, the row is verified and usable.
4. Operator provisions a sandbox container holding the web app.
5. `POST .../web-service/active-sandbox {sandbox_id: "sbx_..."}` →
   vhost router cuts over; live HTTPS traffic on the configured
   domains is proxied via RevDial through hydra into the container.

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
NEW api/pkg/server/project_web_service_handlers.go      - PUT/GET/active-sandbox/domains
NEW api/pkg/server/session_preview_handlers.go          - preview-tokens CRUD
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

## What's deferred to follow-up PRs

- **Frontend `WebServiceTab` + `<SharePreviewSection>`.** Endpoints are
  callable via curl today; swagger annotations are in place so
  `./stack update_openapi` regenerates the typed client cleanly.
- **Auto-deploy on push.** The webhook trigger, the `web-service`
  sandbox runtime, the runner-side workload supervisor (clone +
  `.helix/startup.sh` + restart), and the redeploy state machine.
  All call the same `POST .../active-sandbox` primitive shipping here.
- **certmagic `auto` TLS mode** (passthrough ships now).
- **Sandbox `sbx_*` preview tokens** (sessions cover spec tasks,
  agents, desktops — the high-value cases).
- **DNS verifier cron** (verifier endpoint ships now; automated
  poller is the cleanup).

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
