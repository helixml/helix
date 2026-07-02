# Requirements: Self-Serve ACME Challenge Record for Proxied Custom Domains

## Background

When a user adds a custom domain to a project web service (Project → Web
Service tab), the UI (`frontend/src/components/project/WebServiceTab.tsx`)
shows a "How to add a custom domain" panel. The last paragraph handles the
case where the domain sits behind the customer's own proxy/CDN (e.g.
Cloudflare orange-cloud), which hides the origin from Let's Encrypt so the
cert must be issued via a DNS-01 **ACME challenge delegation** — a one-time
`_acme-challenge.<host>` CNAME into the Helix zone.

Today that paragraph says:

> …get in touch and we'll give you the exact `_acme-challenge` record to add.

This is a manual, human-in-the-loop step, and it is unnecessary: the record
is fully deterministic. The name is always `_acme-challenge.<their-host>` and
the value is a single, fixed delegation target in the Helix-controlled DNS
zone (the same for every customer domain). Helix already knows this value —
it can display the exact record and let the user self-serve, exactly like the
existing CNAME instructions do (name / type / value + copy button).

The prod TLS design doc already anticipated this: `design/2026-06-25-prod-tls-sni-split.md`
Stage 2 lists as remaining work "a small 'add this `_acme-challenge` CNAME'
hint UI for that [orange proxy] case." This task delivers that.

## User Stories

**US-1 — Self-serve the ACME challenge record.**
As a user putting my custom domain behind Cloudflare (or any proxy/CDN), I
want the UI to show me the exact `_acme-challenge` CNAME record to add, so I
can enable HTTPS myself without emailing support.

**US-2 — Copy the record without transcription errors.**
As that same user, I want copy buttons for the record name and value, so I
don't mistype the delegation target and silently break cert issuance.

**US-3 — Graceful fallback when unconfigured.**
As an operator of a Helix instance that has NOT configured a delegation
target, I want the panel to keep the existing "get in touch" wording rather
than showing a blank/incorrect record.

## Acceptance Criteria

- [ ] The web-service API response exposes the ACME challenge delegation
  target (a new field alongside the existing `cname_target`), populated from
  operator config. Empty when not configured.
- [ ] A new operator config env var sets the delegation target host (a name
  in the Helix Cloudflare-controlled zone, e.g. `_acme-challenge.helix.ml`).
- [ ] When the delegation target is configured, the proxy paragraph in the
  "How to add a custom domain" panel is replaced with a concrete record block
  showing: **Name** `_acme-challenge.app.yourcompany.com`, **Type** `CNAME`,
  **Value** `<delegation target>`, with copy buttons on name and value —
  mirroring the styling of the existing direct-CNAME block.
- [ ] When the delegation target is NOT configured, the paragraph keeps the
  current "get in touch and we'll give you the exact `_acme-challenge` record"
  wording (no regression, no empty value shown).
- [ ] Copy still notes that domains pointed **directly** at the CNAME target
  (no proxy) need none of this.
- [ ] `./stack update_openapi` regenerated; frontend uses the generated API
  client type (no hand-edited `api.ts`).
- [ ] `cd frontend && yarn build` and `go build ./api/pkg/server/ ./api/pkg/config/` succeed.

## Out of Scope

- Changing the actual certmagic DNS-01 / CNAME-delegation issuance logic
  (`vhost_tls.go`, `vhost_tls_dns.go`) — that already works (Stage 2 shipped).
  This task only surfaces the known record; it does not alter how certs issue.
- Automatically creating the customer's DNS record for them (we don't control
  their zone).
- Per-domain unique delegation targets (see design.md — rejected).
