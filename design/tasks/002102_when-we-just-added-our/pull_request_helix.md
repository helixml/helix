# Add Cloudflare DNS-01 ACME challenge for Let's Encrypt

## Summary

`HELIX_VHOST_TLS_MODE=auto` already terminates TLS in the API server
using certmagic + Let's Encrypt, but it uses the HTTP-01 and TLS-ALPN-01
challenges — both of which require Let's Encrypt to open a direct TCP
connection to the origin. **Behind a Cloudflare proxy (orange-cloud
DNS) neither challenge can reach Helix:** CF intercepts :80, and CF
terminates TLS at the edge so the `acme-tls/1` ALPN negotiation never
reaches the origin.

This PR adds **DNS-01** support via Cloudflare's API. When configured,
certmagic provisions/validates certs by writing a TXT record through
the Cloudflare API instead of talking to the origin — so it works
regardless of whether the origin is publicly reachable.

## Configuration

Two new env vars on top of the existing `HELIX_VHOST_TLS_MODE=auto`
and `HELIX_VHOST_LETSENCRYPT_EMAIL`:

| Env var | Meaning |
|---|---|
| `HELIX_VHOST_ACME_DNS_PROVIDER` | DNS-01 provider selector. Empty (default) keeps the existing HTTP-01 + TLS-ALPN-01 behaviour. Set to `cloudflare` to use DNS-01 via the Cloudflare API. |
| `HELIX_VHOST_CLOUDFLARE_API_TOKEN` | Cloudflare API token (required when provider=cloudflare). |

### Required Cloudflare token permissions

Create a scoped token at <https://dash.cloudflare.com/profile/api-tokens>:

- **`Zone:Zone:Read`** — for the zones Helix issues certs for
- **`Zone:DNS:Edit`** — for the same zones

**Must be an API token, not the legacy global API key.** `libdns/cloudflare`
rejects keys.

### What ports Helix binds in each mode

| Mode | :80 | :443 | Outbound to CF API |
|---|---|---|---|
| `HELIX_VHOST_TLS_MODE=off` | not used | not used | n/a |
| `auto` (HTTP-01, default) | required (LE validation + redirect) | required | n/a |
| `auto` (DNS-01 via cloudflare) | **not bound** | required (traffic) | required (TXT writes) |

DNS-01 mode does not bind :80 at all — there's no HTTP-01 challenge to
serve, and behind Cloudflare a :80 redirect handler wouldn't be reached
by visitors anyway (CF handles HTTP→HTTPS at the edge).

## Changes

- `api/pkg/config/config.go` — two new `WebServer` fields
  (`VHostACMEDNSProvider`, `VHostCloudflareAPIToken`).
- `api/pkg/server/vhost_tls_dns.go` (new) — `buildACMEChallengeSolver`
  builds a `*certmagic.DNS01Solver` from the configured provider and
  validates credentials at startup. Returns nil (HTTP-01 fallback)
  when no provider is set. Errors loudly on unsupported provider /
  missing token rather than failing at first cert issue.
- `api/pkg/server/vhost_tls.go` — wires the solver into the existing
  `ACMEIssuer`. Setting `ACMEIssuer.DNS01Solver` disables HTTP-01 and
  TLS-ALPN-01 for that issuer (per certmagic docs) — DNS-01 is used
  exclusively. The `:80` goroutine is skipped when DNS-01 is on.
- `api/pkg/server/vhost_tls_dns_test.go` (new) — exercises the
  full validation matrix: empty/cloudflare/unsupported provider,
  with/without token, case-insensitivity, whitespace handling.
- `charts/helix-controlplane/values-example.yaml` — commented
  examples of all four env vars in the `extraEnv` block.
- `go.mod` / `go.sum` — adds `github.com/libdns/cloudflare v0.2.2`
  as a direct dependency. `libdns/libdns` was already pulled in
  indirectly by the existing certmagic auto-mode code.

## Validation matrix (enforced at startup)

| `TLS_MODE` | `DNS_PROVIDER` | `CF_TOKEN` | Outcome |
|---|---|---|---|
| `off` | (anything) | (anything) | TLS disabled. Stray DNS vars cause a startup warning. |
| `auto` | `""` | `""` | Existing behaviour: HTTP-01 + TLS-ALPN-01. |
| `auto` | `""` | set | Warn that the token is ignored. Falls back to HTTP-01. |
| `auto` | `cloudflare` | `""` | **Fail at startup**: token required. |
| `auto` | `cloudflare` | set | DNS-01 via Cloudflare. No :80 listener. |
| `auto` | other | (any) | **Fail at startup**: unsupported provider. |

## Notes for the docs agent

End-user docs in the `docs` repo aren't touched in this PR — a separate
agent will pick up the changes. The information they need to surface is
in the "Configuration" and "What ports Helix binds" sections above,
plus the validation matrix.

Suggested doc placement: extend whatever page documents
`HELIX_VHOST_TLS_MODE` today with a "Behind Cloudflare" subsection
covering token permissions and the env vars, and update the "ports
to open" diagram if there is one.

## Out of scope (deferred, not regressions)

- Other DNS providers (Route53, Google Cloud DNS, etc.). The
  `…_DNS_PROVIDER` enum shape lets them slot in without further
  config-surface churn — just a new `case` in
  `buildACMEChallengeSolver`.
- Dual-token Cloudflare configuration (`ZoneToken` + `APIToken`
  in `libdns/cloudflare`). Single-token mode is the common path;
  dual-token is a one-field follow-up.
- Wildcard certs for `*.<DEV_SUBDOMAIN base>`. DNS-01 enables them,
  but the existing per-hostname on-demand issuance still works fine
  and the rate-limit concern is hypothetical at current scale.
- Token hot-reload. Changing the token still requires an API server
  restart.

## Test plan

- [x] `CGO_ENABLED=0 go build ./api/pkg/server/ ./api/pkg/config/` passes.
- [x] `go test -run TestBuildACMEChallengeSolver ./api/pkg/server/` passes.
- [ ] Manual smoke test on a real deploy behind Cloudflare: set the
  two env vars, restart the API, trigger cert issuance for a hostname
  in `vhost_routes`, watch the log for the `dns-01 via cloudflare`
  line and a successful certificate.
