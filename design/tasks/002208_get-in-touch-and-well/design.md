# Design: Self-Serve ACME Challenge Record for Proxied Custom Domains

## Summary

Surface the deterministic `_acme-challenge` CNAME delegation record in the UI
instead of telling the user to "get in touch". This is a thin, additive change:
one new operator config value, one new field on the existing web-service
response, and a rewrite of one paragraph in `WebServiceTab.tsx`. No change to
cert issuance logic.

## Why the record is deterministic (key finding)

For a proxied custom domain, certmagic issues the cert via DNS-01 with
**cross-zone CNAME delegation** (already shipped — see
`design/2026-06-25-prod-tls-sni-split.md` Stage 2 and
`api/pkg/server/vhost_tls_dns.go`). The customer adds:

```
Name:  _acme-challenge.app.yourcompany.com
Type:  CNAME
Value: <delegation target in the Helix Cloudflare zone>
```

certmagic (holding the Cloudflare token for the Helix zone, with public
`Resolvers` set for reliability) follows that CNAME and writes the ACME TXT
challenge at the resolved target. The **value is a single fixed host** — the
same for every customer domain — because certmagic follows the CNAME to that
target and appends its per-challenge TXT value there. Only the record **name**
varies (it is `_acme-challenge.` + the customer's own hostname).

Because the value is fixed and Helix already controls it, there is nothing to
"get in touch" about — Helix can just print it.

## Backend changes

### 1. Config (`api/pkg/config/config.go`)

Add one field to the `WebServer` struct, next to the existing
`VHostCNAMETarget` (line ~909):

```go
// VHostACMEChallengeTarget is the fixed delegation host that customers
// point "_acme-challenge.<their-domain>" at (via CNAME) when their custom
// domain sits behind a proxy/CDN that hides the origin from Let's Encrypt.
// It must be a name in the DNS zone Helix's Cloudflare token controls, so
// certmagic can follow the CNAME and place the ACME TXT record there.
// Empty = the self-serve record UI is hidden and the "get in touch"
// fallback is shown instead.
VHostACMEChallengeTarget string `envconfig:"HELIX_VHOST_ACME_CHALLENGE_TARGET" description:"Delegation host customers CNAME '_acme-challenge.<domain>' at for proxied custom domains (e.g. '_acme-challenge.helix.ml'). Must live in the Helix Cloudflare zone. Empty = hide the self-serve record."`
```

### 2. API response (`api/pkg/server/project_web_service_handlers.go`)

Add a field to `ProjectWebServiceResponse` (next to `CNAMETarget`, line ~44)
and populate it in `getProjectWebService` (next to the `cnameTarget` block,
line ~108):

```go
// ACMEChallengeTarget is the fixed CNAME value customers point
// "_acme-challenge.<their-domain>" at when the domain is behind a
// proxy/CDN. Empty when the operator has not configured delegation.
ACMEChallengeTarget string `json:"acme_challenge_target,omitempty"`
```

```go
ACMEChallengeTarget: strings.TrimSpace(s.Cfg.WebServer.VHostACMEChallengeTarget),
```

No new endpoint — this rides the existing `GET /api/v1/projects/{id}/web-service`.

### 3. Regenerate client

Run `./stack update_openapi` so `frontend/src/api/api.ts` and the swagger
docs pick up `acme_challenge_target`. Do not hand-edit generated files.

## Frontend changes (`frontend/src/components/project/WebServiceTab.tsx`)

- Read the new field: `const acmeChallengeTarget = data?.acme_challenge_target ?? ''`
  (next to the existing `cnameTarget` read, line ~62).
- Replace the proxy paragraph (lines ~320-330) with a conditional:
  - **If `acmeChallengeTarget` is set:** keep the intro sentence ("Behind
    Cloudflare or another proxy/CDN? …one-time ACME challenge delegation"),
    then render a record block styled like the existing direct-CNAME block
    (the monospace `Box` at lines ~281-313): **Name**
    `_acme-challenge.app.yourcompany.com`, **Type** `CNAME`, **Value**
    `{acmeChallengeTarget}`, each with a `ContentCopyIcon` copy button that
    calls `navigator.clipboard.writeText(...)` + `snackbar.success(...)`
    (reuse the existing pattern). Close with the existing "Domains pointed
    directly at `{cnameTarget}` (no proxy) need none of this." line.
  - **If `acmeChallengeTarget` is empty:** render the current "get in touch
    and we'll give you the exact `_acme-challenge` record to add" paragraph
    unchanged (fallback, no regression).

The example host in the Name (`app.yourcompany.com`) matches the placeholder
already used in the panel's step 1, keeping the instructions consistent. This
static instructional panel is shown once (not per-domain), matching the
existing direct-CNAME instructions.

## Key decisions

- **Single fixed delegation target, not per-domain.** certmagic appends its
  challenge TXT at the shared target and removes only its own value, so
  concurrent issuance across domains is safe. A per-domain unique target would
  add backend record-derivation logic and a per-domain UI with zero benefit —
  rejected. The value is one string, displayed the same way `cname_target`
  already is.
- **Config-driven, empty-safe.** The target is not hardcoded (it is
  deployment-specific and belongs to the operator's DNS zone). Empty config
  keeps today's behaviour, so no instance regresses.
- **Additive only.** Rides the existing response and endpoint; no new routes,
  no store or migration changes, no change to issuance code.

## Testing

- Backend: `go build ./api/pkg/server/ ./api/pkg/config/`. Optionally a table
  case in the existing `project_web_service_handlers` test confirming the
  field is passed through from config.
- Frontend: `cd frontend && yarn build`.
- E2E in inner Helix (`localhost:8080`): register/onboard, open a project's
  Web Service tab. With `HELIX_VHOST_ACME_CHALLENGE_TARGET` unset, confirm the
  "get in touch" fallback still shows. Set the env var, restart api, reload,
  and confirm the record block renders with the correct value and copy
  buttons. Screenshot both states into this task's `screenshots/`.
