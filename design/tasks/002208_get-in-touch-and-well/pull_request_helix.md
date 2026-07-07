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
The old "get in touch" wording is removed entirely — on an instance that
hasn't configured a delegation target the proxy section is simply omitted
(it's meaningless to tell a self-hosted user to contact support, and an
orange-proxied domain's cert can't issue without the record anyway). No change
to how certs are actually issued (`vhost_tls*.go` — already shipped).

## Changes

- **`api/pkg/config/config.go`** — new operator config
  `HELIX_VHOST_ACME_CHALLENGE_TARGET` (the fixed delegation host, e.g.
  `_acme-challenge.helix.ml`). Empty = feature hidden.
- **`api/pkg/server/project_web_service_handlers.go`** — expose the value as
  `acme_challenge_target` on the existing `GET .../web-service` response.
- **`frontend/src/components/project/WebServiceTab.tsx`** — when configured,
  render the concrete `_acme-challenge` CNAME record with copy buttons; when
  not configured, omit the proxy/delegation section entirely (no "get in
  touch").
- Regenerated OpenAPI client + swagger docs.

## Testing

- `go build ./api/pkg/server/ ./api/pkg/config/` passes; `tsc --noEmit` clean.
- Verified end-to-end in a dev Helix: with the env var unset the panel ends at
  step 3 with no proxy section at all; with it set to `_acme-challenge.helix.ml`
  the panel shows the record block (Name `_acme-challenge.app.yourcompany.com`,
  Type `CNAME`, Value `_acme-challenge.helix.ml`) with working copy buttons.

## Screenshots

Unconfigured (env unset) — no proxy/"get in touch" section:

![Unconfigured](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002208_get-in-touch-and-well/screenshots/01-unconfigured-no-proxy-section.png)

Self-serve record (env set):

![Self-serve record](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002208_get-in-touch-and-well/screenshots/02-self-serve-record.png)
