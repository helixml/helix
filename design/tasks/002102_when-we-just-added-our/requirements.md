# Requirements: Cloudflare DNS-01 ACME Challenge for Let's Encrypt

## Background

In spec task 002096 we added embedded Let's Encrypt termination to the Helix
API server (`HELIX_VHOST_TLS_MODE=auto`). The implementation uses certmagic
with the HTTP-01 and TLS-ALPN-01 challenges, which both rely on Let's
Encrypt being able to open a TCP connection directly to the API server on
port 80 (HTTP-01) or port 443 (TLS-ALPN-01).

That assumption breaks when the operator runs Helix behind a Cloudflare
proxy (orange-cloud DNS records):

- HTTP-01 fails: LE's validator hits Cloudflare's edge on :80, not Helix.
- TLS-ALPN-01 fails: Cloudflare terminates TLS at the edge with its own
  cert and re-originates a new upstream connection. The `acme-tls/1` ALPN
  negotiation never reaches Helix, and the cert presented to LE is
  Cloudflare's, not the challenge cert.

The only ACME challenge that works behind an orange-cloud Cloudflare
record is **DNS-01**, because the validator checks a TXT record on the
domain instead of talking to the origin. DNS-01 requires API access to
the DNS provider — for Cloudflare, a scoped API token.

This task adds Cloudflare DNS-01 support to `HELIX_VHOST_TLS_MODE=auto`
so operators behind Cloudflare can get automatic Let's Encrypt certs.

## User Stories

### As an operator running Helix behind Cloudflare's proxy, I want Helix to issue Let's Encrypt certs successfully

So that my project web services and sandbox previews serve real HTTPS
without me running a separate reverse proxy.

**Acceptance:**
- Setting `HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare` plus
  `HELIX_VHOST_CLOUDFLARE_API_TOKEN=<token>` in `auto` mode switches
  certmagic from HTTP-01/TLS-ALPN-01 to DNS-01 via Cloudflare's API.
- The `:80` ACME challenge listener is not started in DNS-01 mode (it
  serves no purpose behind Cloudflare and only confuses operators about
  what ports need to be open).
- The `:443` listener still binds and serves traffic (Cloudflare's
  upstream connection terminates here).
- Cert issuance works for: the canonical hostname (`SERVER_URL`),
  default project subdomains under `DEV_SUBDOMAIN`'s base, verified
  custom domains, and `share-*` sandbox preview tokens. Same allow-list
  as HTTP-01 mode (`vhostShouldIssueCert`).
- A clear startup log line states the chosen challenge type
  (`acme: dns-01 via cloudflare` vs the existing
  `acme: http-01 + tls-alpn-01`).

### As an operator, I want clear configuration errors when DNS-01 is misconfigured

So that I find out at startup, not when my first cert fails to issue.

**Acceptance:**
- `HELIX_VHOST_ACME_DNS_PROVIDER` set to anything other than `cloudflare`
  (or the empty default) fails startup with a message listing supported
  providers.
- `HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare` without
  `HELIX_VHOST_CLOUDFLARE_API_TOKEN` fails startup with a clear error.
- `HELIX_VHOST_CLOUDFLARE_API_TOKEN` set without
  `HELIX_VHOST_ACME_DNS_PROVIDER` triggers a startup warning (token is
  ignored) so operators don't think DNS-01 is on when it isn't.
- Setting DNS-01 vars while `HELIX_VHOST_TLS_MODE != auto` triggers a
  startup warning (ignored, but operator clearly told).

### As an operator, I want documentation of exactly which Cloudflare token permissions are required

So that I can scope the token tightly and don't have to guess.

**Acceptance:**
- Docs in the helix repo describe the minimum permission set:
  `Zone:Zone:Read` + `Zone:DNS:Edit` scoped to the specific zone(s)
  covering hostnames Helix will issue certs for.
- Docs note the single-token vs dual-token (`ZoneToken` +
  `APIToken`) options offered by `github.com/libdns/cloudflare`. v1
  ships single-token only; dual-token is a follow-up if anyone asks.
- Docs explicitly call out: API **tokens** only, not legacy global
  API **keys**. (libdns/cloudflare rejects keys.)

## Out of Scope

- Other DNS providers (Route53, Google Cloud DNS, DigitalOcean, etc.).
  The `HELIX_VHOST_ACME_DNS_PROVIDER` env var is shaped as an enum so
  more providers slot in later without further config-surface churn,
  but only `cloudflare` ships in this task.
- acme-dns / CNAME delegation (an alternate way to use DNS-01 without
  granting Helix a Cloudflare token directly). Deferred — adds an
  external service dependency and isn't requested.
- Cloudflare Origin Certificates (CF's free 15-year origin-only certs).
  Operators wanting this should leave `HELIX_VHOST_TLS_MODE=off` and
  install the origin cert into their own reverse proxy — no Helix
  feature needed.
- Wildcard certs as a default. DNS-01 enables them, but the existing
  per-hostname on-demand model in `vhostShouldIssueCert` keeps cert
  issuance gated on the hostname existing in `vhost_routes`. Wildcards
  for `*.<DEV_SUBDOMAIN base>` are a separate optimisation (fewer ACME
  orders) tracked as a follow-up — not blocking.
- UI exposure of the API token. It's an operator-level secret like
  `HELIX_VHOST_LETSENCRYPT_EMAIL`; lives in env / Helm values only.
