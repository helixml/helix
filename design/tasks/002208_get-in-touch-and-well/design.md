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
- Gate the **entire** proxy/ACME-delegation section on `acmeChallengeTarget`:
  - **If `acmeChallengeTarget` is set:** render the intro sentence ("Behind
    Cloudflare or another proxy/CDN? …one-time ACME challenge delegation — add
    this second record alongside the CNAME above:"), then a record block styled
    like the existing direct-CNAME block: **Name**
    `_acme-challenge.app.yourcompany.com`, **Type** `CNAME`, **Value**
    `{acmeChallengeTarget}`, each with a `ContentCopyIcon` copy button that
    calls `navigator.clipboard.writeText(...)` + `snackbar.success(...)`.
    Close with the "Domains pointed directly at `{cnameTarget}` (no proxy)
    need none of this." line.
  - **If `acmeChallengeTarget` is empty:** render **nothing** — the whole
    proxy section is omitted. There is NO "get in touch" fallback (see the
    decision below). The direct-CNAME steps (1–3) still render.

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
- **Config-driven; unconfigured = omit the section (no "get in touch").** The
  target is deployment-specific (the operator's DNS zone), so it stays config-
  driven. When it is empty we hide the proxy/delegation section entirely rather
  than fall back to "get in touch". Two reasons: (1) "get in touch" is
  meaningless on a self-hosted instance with no Helix support desk; (2) it
  would be misleading — an orange-proxied custom domain's cert genuinely
  cannot issue without the delegation record (network challenges can't reach
  the hidden origin), so the surrounding "point the proxy at us and it still
  works" claim is only true once the record exists. Showing the section only
  when self-serve is actually possible keeps the panel honest.
- **Additive only.** Rides the existing response and endpoint; no new routes,
  no store or migration changes, no change to issuance code.

## Implementation Notes (verified 2026-07-03)

- **Files changed (helix repo):**
  - `api/pkg/config/config.go` — added `VHostACMEChallengeTarget` (`HELIX_VHOST_ACME_CHALLENGE_TARGET`).
  - `api/pkg/server/project_web_service_handlers.go` — added `ACMEChallengeTarget` to `ProjectWebServiceResponse`, populated from config (trimmed).
  - `frontend/src/components/project/WebServiceTab.tsx` — read `acme_challenge_target`; conditional record block vs. "get in touch" fallback.
  - Regenerated: `frontend/src/api/api.ts`, `api/pkg/server/swagger.json`, `swagger.yaml`, `docs.go`.
- **`./stack update_openapi` gotcha:** `swag` installs to `$(go env GOPATH)/bin` which is NOT on PATH by default here. Run with `export PATH="$PATH:$(go env GOPATH)/bin"` first, otherwise it fails with `swag: command not found` (and misleadingly exits 0).
- **Frontend build gotcha:** `yarn build` fails at `prepare-out-dir` because `frontend/dist` is a root-owned bind-mount (production-frontend-mode artifact). This is an environment permission issue, NOT a code error — the Vite transform completes (21654 modules) and `npx tsc --noEmit` passes clean. Dev stack uses Vite HMR (port 8081), so source edits are live without a build.
- **E2E verified in inner Helix** (`localhost:8080`, dev mode): registered, onboarded (testorg → testproj), enabled the project web service, opened the "How to add a custom domain" panel.
  - Env unset → the panel ends at step 3; the "Behind Cloudflare…" / "get in touch" section is entirely absent (screenshot `01-unconfigured-no-proxy-section.png`).
  - Set `HELIX_VHOST_ACME_CHALLENGE_TARGET=_acme-challenge.helix.ml`, recreated the api container (`docker compose ... up -d api` — `restart` does NOT reload `.env`), reloaded → record block shows Name `_acme-challenge.app.yourcompany.com`, Type `CNAME`, Value `_acme-challenge.helix.ml` with copy buttons, and no "get in touch" text (screenshot `02-self-serve-record.png`).
  - Restored `.env` to default afterwards.
- **Follow-up (user feedback): eliminated "get in touch" entirely.** The original design kept a "get in touch" fallback when unconfigured; the user rejected it as nonsensical. Changed the frontend to omit the whole proxy section when `acmeChallengeTarget` is empty (commit `fix(frontend): drop meaningless get-in-touch ACME fallback`). Re-verified both states E2E after an environment/DB reset (had to re-onboard).
- `cnameTarget` falls back to the SERVER_URL host, which is `localhost` in dev — that's why the panel shows `localhost` as the direct CNAME value.

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
