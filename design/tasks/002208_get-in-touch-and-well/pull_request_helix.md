# Self-serve ACME challenge record for proxied custom domains

## Summary

When a user adds a custom domain that sits behind their own proxy/CDN (e.g.
Cloudflare orange-cloud), the "How to add a custom domain" panel used to tell
them to *"get in touch and we'll give you the exact `_acme-challenge` record to
add."* That manual, human-in-the-loop step is unnecessary: the DNS-01
delegation record is deterministic — the name is always
`_acme-challenge.<their-host>` and the value is a single fixed delegation host
in the Helix Cloudflare zone.

This surfaces that record in the UI (name / type / value + copy buttons),
exactly like the existing direct-CNAME instructions, so users can self-serve.
The old "get in touch" wording is removed entirely — the proxy section is shown
**only when DNS-01 delegation can actually work** (the Cloudflare DNS-01
provider is enabled), and the delegation value is **derived automatically**
from the CNAME target's registrable domain (`ingress.helix.ml` →
`_acme-challenge.helix.ml`). No change to how certs are actually issued
(`vhost_tls*.go` — already shipped).

## Changes

- **`api/pkg/server/project_web_service_handlers.go`** — expose
  `acme_challenge_target` on the existing `GET .../web-service` response,
  computed by a new `acmeChallengeTarget()` helper: empty unless the Cloudflare
  DNS-01 provider is enabled, then an explicit override or a value derived from
  the CNAME target's registrable domain (via `golang.org/x/net/publicsuffix`).
- **`api/pkg/config/config.go`** — `HELIX_VHOST_ACME_CHALLENGE_TARGET` is now an
  optional **override** (empty by default; only needed when the ACME zone
  differs from the CNAME target's domain).
- **`frontend/src/components/project/WebServiceTab.tsx`** — when the target is
  present, render the concrete `_acme-challenge` CNAME record with copy buttons;
  otherwise omit the proxy/delegation section entirely (no "get in touch"). Also
  added bottom padding under "Recent deploys" so the settings modal scrolls to a
  clean bottom.
- **`api/pkg/server/project_web_service_acme_test.go`** — unit tests for
  derivation, override precedence, provider gating, multi-level TLDs, and the
  no-registrable-domain case.
- Regenerated OpenAPI client + swagger docs.

## Testing

- `go build ./api/pkg/server/ ./api/pkg/config/` passes; `go test -run
  TestACMEChallengeTarget ./pkg/server/` passes; `tsc --noEmit` clean.
- Verified end-to-end in a dev Helix: with the Cloudflare DNS-01 provider +
  `HELIX_VHOST_CNAME_TARGET=ingress.helix.ml` (no challenge-target var) the
  panel shows the derived record (Value `_acme-challenge.helix.ml`) with working
  copy buttons; removing the provider hides the whole proxy section even with a
  valid CNAME target.

## Screenshots

Unconfigured (env unset) — no proxy/"get in touch" section:

![Unconfigured](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002208_get-in-touch-and-well/screenshots/01-unconfigured-no-proxy-section.png)

Self-serve record (env set):

![Self-serve record](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002208_get-in-touch-and-well/screenshots/02-self-serve-record.png)
