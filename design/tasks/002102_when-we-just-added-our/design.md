# Design: Cloudflare DNS-01 ACME Challenge for Let's Encrypt

## Context

The existing `HELIX_VHOST_TLS_MODE=auto` implementation lives in
`api/pkg/server/vhost_tls.go` (~150 lines) and uses certmagic's default
challenge flow: HTTP-01 on :80 and TLS-ALPN-01 on :443. Both require LE's
validator to reach the Helix process directly over TCP. Behind a
Cloudflare proxy (orange-cloud), neither does — see requirements.md
Background for the network-path explanation.

certmagic supports DNS-01 via `certmagic.DNS01Solver`, which takes any
`github.com/libdns/<provider>` implementation. Helix already has
`github.com/caddyserver/certmagic v0.25.3` and `github.com/libdns/libdns
v1.1.1` in `go.mod` (both indirect — pulled in by the existing certmagic
auto-mode code). Adding `github.com/libdns/cloudflare` is the only new
direct dependency.

Per certmagic's docs: **setting `DNS01Solver` on an `ACMEIssuer`
disables HTTP-01 and TLS-ALPN-01 for that issuer.** That's the right
behaviour for us — if an operator wires up Cloudflare credentials, they
want DNS-01 exclusively; the other challenges would fail noisily behind
CF and aren't worth keeping as fallbacks.

## Key Decisions

### 1. New env vars: provider enum + provider-specific credentials

```
HELIX_VHOST_ACME_DNS_PROVIDER   (optional)  enum: "" | "cloudflare"
HELIX_VHOST_CLOUDFLARE_API_TOKEN (required when provider=cloudflare)
```

Shape rationale:

- A separate `…_DNS_PROVIDER` enum keeps the `…_TLS_MODE` field clean
  (it stays `off | auto` — same two values, same meaning) and lets
  future providers slot in without another mode value. The pattern is
  the same as how the `Sandboxes` config takes a `Runtimes` map: an
  enum dispatch with provider-specific creds elsewhere.
- Credentials live in dedicated, provider-prefixed env vars
  (`HELIX_VHOST_CLOUDFLARE_API_TOKEN`) rather than a generic
  `HELIX_VHOST_DNS_CREDENTIALS` blob, so each provider's required
  fields surface in `envconfig` introspection and the Helm
  `values-example.yaml` documents them by name. When Route53 lands it
  gets `HELIX_VHOST_AWS_ACCESS_KEY_ID` / `_SECRET_ACCESS_KEY` (or an
  IAM role hint), not a stringly-typed bag.

We do **not** add `HELIX_VHOST_CLOUDFLARE_ZONE_TOKEN` in this task —
the dual-token (separate Zone:Read token) feature offered by
`libdns/cloudflare` is a small marginal improvement when an operator
manages many zones and wants the DNS:Write token scoped to one. For v1
we ship single-token only and document it; dual-token is a one-field
follow-up when someone asks.

### 2. Wire DNS01Solver into the existing certmagic config

`startCertMagicListener` in `api/pkg/server/vhost_tls.go` already builds
the `certmagic.Config` and the `ACMEIssuer`. The DNS-01 wiring is a
single new branch after the existing `NewACMEIssuer` call:

```go
issuerTmpl := certmagic.ACMEIssuer{
    CA:     certmagic.LetsEncryptProductionCA,
    Email:  email,
    Agreed: true,
}
solver, challengeDesc, err := buildACMEChallengeSolver(apiServer.Cfg.WebServer)
if err != nil {
    return err
}
if solver != nil {
    issuerTmpl.DNS01Solver = solver
}
magicACME := certmagic.NewACMEIssuer(cfg, issuerTmpl)
cfg.Issuers = []certmagic.Issuer{magicACME}

log.Info().
    Str("email", email).
    Str("challenge", challengeDesc).
    Msg("vhost TLS auto mode enabled (certmagic + Let's Encrypt)")
```

A new file `api/pkg/server/vhost_tls_dns.go` holds `buildACMEChallengeSolver`:

```go
// buildACMEChallengeSolver inspects HELIX_VHOST_ACME_DNS_PROVIDER and
// returns either (nil, "http-01 + tls-alpn-01", nil) — meaning let
// certmagic fall back to its built-in HTTP/TLS-ALPN challenges — or
// (solver, "dns-01 via <provider>", nil) configured for the named
// provider. Unknown providers and missing creds return an error.
func buildACMEChallengeSolver(ws config.WebServer) (acmez.Solver, string, error) {
    provider := strings.ToLower(strings.TrimSpace(ws.VHostACMEDNSProvider))
    switch provider {
    case "":
        // Warn if a CF token is set but the provider isn't — operator
        // probably forgot to flip the switch and is wondering why
        // DNS-01 isn't kicking in.
        if strings.TrimSpace(ws.VHostCloudflareAPIToken) != "" {
            log.Warn().Msg("HELIX_VHOST_CLOUDFLARE_API_TOKEN is set but HELIX_VHOST_ACME_DNS_PROVIDER is not — ignoring token, using HTTP-01")
        }
        return nil, "http-01 + tls-alpn-01", nil
    case "cloudflare":
        token := strings.TrimSpace(ws.VHostCloudflareAPIToken)
        if token == "" {
            return nil, "", errors.New("HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare requires HELIX_VHOST_CLOUDFLARE_API_TOKEN")
        }
        return &certmagic.DNS01Solver{
            DNSManager: certmagic.DNSManager{
                DNSProvider: &cloudflare.Provider{APIToken: token},
            },
        }, "dns-01 via cloudflare", nil
    default:
        return nil, "", fmt.Errorf("HELIX_VHOST_ACME_DNS_PROVIDER=%q is not supported (supported: cloudflare)", provider)
    }
}
```

### 3. Skip the :80 listener when DNS-01 is active

The existing code unconditionally starts a goroutine on `:80` for
HTTP-01 challenges + HTTP→HTTPS redirects. In DNS-01 mode the HTTP-01
challenge is dead — the listener serves only as a redirect helper.
Behind Cloudflare, traffic on :80 doesn't reach Helix anyway (CF
handles HTTP→HTTPS via its own "Always Use HTTPS" page rule). Starting
a :80 listener that does nothing useful is misleading and risks
conflicting with whatever else operators put on the box.

Decision: when `DNS01Solver != nil`, skip the :80 goroutine entirely.
The :443 listener still starts and serves all production traffic.
Operators who want a :80 redirect for direct-to-origin testing can run
one themselves; it's not Helix's job once we're not using HTTP-01.

### 4. Cert allow-list (`vhostShouldIssueCert`) is unchanged

The DNS-01 challenge change is purely about *how* the challenge is
performed; *whether* certmagic should attempt to issue a cert for a
given hostname is still gated by `vhostShouldIssueCert`. That function
checks the canonical hostname allow-list and looks up
`vhost_routes` — both work identically regardless of challenge type.
No change there.

This means: even with DNS-01 wired up, certmagic only requests certs
for hostnames Helix actually serves. The Cloudflare token therefore
only ever sees TXT-record writes for legitimate vhosts; a leaked or
overscoped row in `vhost_routes` does not let a stranger drive
arbitrary `_acme-challenge.*` records via Helix.

### 5. Configuration shape (full picture)

```
# Mode (existing)
HELIX_VHOST_TLS_MODE=auto                          # off (default) | auto
HELIX_VHOST_LETSENCRYPT_EMAIL=ops@example.com      # required in auto mode

# DNS-01 (new)
HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare           # "" (default = HTTP-01) | cloudflare
HELIX_VHOST_CLOUDFLARE_API_TOKEN=<token>           # required when provider=cloudflare
```

Validation matrix at startup:

| TLS_MODE | DNS_PROVIDER | CF_TOKEN  | Outcome |
|---|---|---|---|
| off      | (anything)   | (anything)| TLS disabled. If DNS_PROVIDER or CF_TOKEN set: warn (ignored). |
| auto     | ""           | ""        | HTTP-01 + TLS-ALPN-01 (existing behaviour). |
| auto     | ""           | set       | Warn that token is ignored. HTTP-01 + TLS-ALPN-01. |
| auto     | cloudflare   | ""        | **Fail** at startup: token required. |
| auto     | cloudflare   | set       | DNS-01 via Cloudflare. No :80 listener. |
| auto     | other        | (any)     | **Fail** at startup: unsupported provider. |

### 6. Test scope

- Unit tests in `vhost_tls_dns_test.go` cover the
  `buildACMEChallengeSolver` matrix above. No live Cloudflare API call.
- Integration test verifying actual cert issuance is **not** in scope
  — it requires a real Cloudflare zone, a valid token, and either the
  LE staging CA or the production CA. Manual verification by the
  operator on first deploy is the validation path.
- A small smoke check: assert that `startCertMagicListener` does not
  spawn the :80 goroutine when DNS-01 is configured. This can be done
  by injecting a fake `net.Listen` or by extracting the listener-start
  logic into a function that returns the listeners it would start
  (without actually binding) — pick whichever is shorter.

### 7. Documentation

Two doc surfaces update:

1. The Helm chart's `values-example.yaml` gains commented examples for
   the two new env vars under the existing web-server section, with a
   brief note pointing operators behind Cloudflare to DNS-01.
2. `docs/learn/helm-install.mdx` (or wherever LE setup is documented
   today — `grep -li letsencrypt /home/retro/work/docs/` came up
   empty, so this may be a fresh page under `docs/learn/` describing
   the whole `HELIX_VHOST_TLS_MODE` flow). Tasks.md lists writing the
   docs page as its own item; whoever picks it up should grep first
   and either extend an existing operator-deployment page or add a
   `docs/learn/vhost-tls.mdx`.

## Data Model

No schema changes. DNS-01 is purely a runtime configuration change;
the existing `vhost_routes`, `project_web_service_state`,
`web_service_deploys` tables are untouched.

## Operational Notes

### Token permissions (for the docs page)

The Cloudflare token needs, at minimum:

- `Zone:Zone:Read` — All zones (or just the zones Helix serves)
- `Zone:DNS:Edit` — All zones (or just the zones Helix serves)

Create at https://dash.cloudflare.com/profile/api-tokens → Create
Custom Token. Use API **tokens**, not the legacy global API **key** —
libdns/cloudflare rejects keys.

### What ports need to be open

| Mode | :80 inbound | :443 inbound | Outbound to CF API |
|---|---|---|---|
| `off` | not used by Helix | not used by Helix | n/a |
| `auto` (HTTP-01) | required (LE validation + redirect) | required | n/a |
| `auto` (DNS-01)  | not used by Helix | required (traffic) | required (TXT writes) |

Behind Cloudflare proxy, :80 isn't reachable from LE anyway. Behind a
plain firewall (no CF), DNS-01 is still a valid choice if the operator
wants to avoid exposing :80 publicly — there's no requirement that
DNS-01 only be used with Cloudflare proxying. The mode just needs the
DNS provider's API to work.

## Open Questions

- **LE rate limits when many default subdomains spin up?** Cert
  issuance is per-hostname on demand today. If a project enables web
  hosting and immediately gets traffic on its default subdomain,
  that's one cert. If hundreds of projects do so at once, we may hit
  LE's per-domain rate limit on the base domain. Mitigation is a
  wildcard cert for `*.<DEV_SUBDOMAIN base>` — exactly what DNS-01
  unlocks — but that's a separate optimisation. Track as follow-up;
  don't block on it.
- **Token rotation.** Hot-reload of the token isn't implemented —
  changing `HELIX_VHOST_CLOUDFLARE_API_TOKEN` requires an API server
  restart. Acceptable for an operator-level secret with multi-year
  validity; revisit if anyone runs short-lived CF tokens.
