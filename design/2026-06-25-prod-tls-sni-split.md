# Prod TLS: Helix certmagic terminates all Helix domains; nginx keeps legacy; Cloudflare stays on

Date: 2026-06-25 (revised 2026-06-29). Decision (Luke): **Helix terminates TLS (certmagic) for all
its own domains — `app.helix.ml`, `*.apps.helix.ml`, and customer custom domains — while nginx keeps
terminating the legacy/infra domains (registry, get, demos, kodit, marketing, docs, posthog, k8s…)
on their existing certbot certs.** This makes custom-domain TLS self-serve and consolidates Helix TLS
under certmagic, matching how `meta.helix.ml` already runs.

## Key constraints (verified against live prod + code, 2026-06-29)
- **Cloudflare proxy stays ON (orange) for all Helix domains, incl. custom ones.** `app.helix.ml` is
  already orange; `*.apps.helix.ml` is currently grey and moves to orange.
- **Behind CF orange, LE network challenges can't reach the origin** → certmagic must use **DNS-01
  via Cloudflare** for `*.helix.ml`.
- **`app.helix.ml` is path-coupled to Keycloak.** The OIDC issuer is
  `https://app.helix.ml/auth/realms/helix`; nginx currently routes `/auth/*` → the Keycloak
  container. An SNI router can't path-split, and Helix doesn't proxy `/auth`. So Helix must proxy
  `/auth/*` → Keycloak itself when it terminates `app.helix.ml`.
- **certmagic binds `:443`/`:80` inside the container** (`vhost_tls.go`, hardcoded). Keep it; use a
  docker host **loopback** port so nginx can own `:443` as an SNI router. Overlay port is
  env-configurable (`${VHOST_HTTPS_PORT:-443}:443`).
- **certmagic shares the same router** as the plain HTTP listener, so the `/auth` proxy + vhost
  routing work identically under TLS. The cert gate (`vhostShouldIssueCert`) already allows canonical
  hostnames + any `vhost_routes` row.

## Architecture
```
nginx :443 → stream { ssl_preread on; map $ssl_preread_server_name → backend }
  legacy SNIs → 127.0.0.1:8443   (existing nginx http server blocks, certbot, unchanged)
  app.helix.ml, *.apps.helix.ml, default → 127.0.0.1:8444   (Helix api container :443, certmagic)
nginx :80 → keep for legacy certbot renew + http→https redirects
Helix api: HELIX_VHOST_TLS_MODE=auto, certs in filestore, generic proxy /auth/ → keycloak:8080
custom domains → CF for SaaS edge (optional) → origin → Helix (LE cert it issued) → route by Host
```

## Generic reverse proxy (NOT Keycloak-specific) — shipped
A single configurable reverse proxy mounts when both `HELIX_PROXY_PATH_PREFIX` and
`HELIX_PROXY_UPSTREAM` are set: requests under the prefix are forwarded to the upstream, **Host
preserved** + `X-Forwarded-*` set. Mounted on the bare router (no auth/CSRF) before the SPA
catch-all. Empty = disabled (meta uses Google OIDC and leaves it empty). On prod:
`HELIX_PROXY_PATH_PREFIX=/auth/`, `HELIX_PROXY_UPSTREAM=http://keycloak-config-keycloak-1:8080`,
which keeps the issuer URL intact. Code: `api/pkg/server/path_proxy.go`, mounted in
`registerRoutes` (`server.go`); config in `api/pkg/config/config.go`.

## Custom-domain certs: Helix issues its own (no Cloudflare required)
Custom domains must work **with or without** Cloudflare. To get a valid cert on the CF→origin hop
(or directly), Helix issues the cert itself. Challenge selection is **per-name** (a Stage-2 code
item — the deployed single global DNS-01 solver disables HTTP-01/ALPN for all names):
- `*.helix.ml` (behind CF orange) → **DNS-01 via Cloudflare** (our zone, our token).
- custom domain pointed **directly** at us (grey/no CF) → **HTTP-01 / TLS-ALPN-01** — the customer
  just points DNS at us; that A/CNAME is the proof of control. **No extra record, no UI needed.**
- custom domain behind **their** Cloudflare (orange) → **DNS-01 CNAME delegation**: a one-time
  `_acme-challenge.<host>` CNAME into `helix.ml` (certmagic v0.25.3 follows cross-zone CNAMEs; add
  explicit public `Resolvers` for reliability). A small "add this record" UI helps here only.

The cert gate already allows any `vhost_routes` hostname; custom-domain verification exists
(`/.well-known/helix-domain-verify`). Cloudflare-for-SaaS is only the *edge* option, not required.

## Stages
- **Stage 0 (code, this PR):** generic configurable reverse proxy + tests. (Per-name challenge
  selection + `Resolvers` come in Stage 2.) Remove dead `PORT_NOXY` (prod `.env`).
- **Stage 1 (prod):** enable certmagic DNS-01 on loopback `127.0.0.1:8444`; convert nginx `:443` to
  `stream`+`ssl_preread` (legacy→`8443` certbot, app+apps+default→`8444`); flip `*.apps.helix.ml` to
  CF orange. Pre-warm certs; cut over with an `nginx reload`; instant rollback via config backup.
  Set prod `.env`: `HELIX_VHOST_TLS_MODE=auto`, `HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare`,
  `HELIX_VHOST_CLOUDFLARE_API_TOKEN`, `HELIX_VHOST_LETSENCRYPT_EMAIL`, `HELIX_PROXY_PATH_PREFIX=/auth/`,
  `HELIX_PROXY_UPSTREAM=http://keycloak-config-keycloak-1:8080`.
- **Stage 2 (custom domains) — code SHIPPED:** per-name challenge selection via certmagic issuer
  fallback in `startCertMagicListener` (`vhost_tls.go`): issuer 1 = DNS-01 (Cloudflare) for
  `helix.ml` names + custom domains that CNAME `_acme-challenge` into our zone; issuer 2 =
  TLS-ALPN-01 (over `:443`, no `:80` needed) for custom domains pointed directly at us. The DNS
  solver gets public `Resolvers` (`vhost_tls_dns.go`) so CNAME delegation resolves from inside the
  container. Net effect: a custom domain works **with or without** Cloudflare, and the direct case
  needs **no DNS-record UI** (the A/CNAME to us is the proof). Remaining optional: CF-for-SaaS edge
  for orange custom domains; a small "add this `_acme-challenge` CNAME" hint UI for that case.

## Risk
High blast radius — `:443` carries registry image pulls, the `app.helix.ml` console+auth, and demos.
Pre-warm certs + loopback-first + nginx config backup keep the cutover to a reload with instant
rollback. Ship Stage 0, then Stage 1, then Stage 2 as separate verified steps.
