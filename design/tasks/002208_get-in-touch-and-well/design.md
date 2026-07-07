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

`VHostACMEChallengeTarget` is now an **optional override** (see "Follow-up 2"
in the implementation notes). It is normally left empty; the delegation target
is derived automatically. It's kept only for deployments whose ACME zone
differs from the CNAME target's registrable domain.

### 2. API response (`api/pkg/server/project_web_service_handlers.go`)

Add a field to `ProjectWebServiceResponse` (next to `CNAMETarget`) and populate
it in `getProjectWebService` via a helper:

```go
// ACMEChallengeTarget is the CNAME value customers point
// "_acme-challenge.<their-domain>" at when the domain is behind a
// proxy/CDN. Empty when DNS-01 delegation isn't available (hides the record).
ACMEChallengeTarget string `json:"acme_challenge_target,omitempty"`
```

```go
ACMEChallengeTarget: s.acmeChallengeTarget(cnameTarget),
```

`acmeChallengeTarget` returns `""` unless the Cloudflare DNS-01 provider is
enabled (so the UI shows the record only when delegation can actually work),
then returns the override if set, else derives
`"_acme-challenge." + publicsuffix.EffectiveTLDPlusOne(cnameTarget)`.

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
- **Gate on the real capability; derive the value; no "get in touch".** The
  self-serve record is shown only when the Cloudflare DNS-01 provider is
  enabled (`VHostACMEDNSProvider == "cloudflare"`) — the exact condition under
  which CNAME delegation can succeed — and the target value is *derived* from
  the CNAME target's registrable domain rather than requiring a dedicated env
  var. This replaced two earlier iterations: (a) a "get in touch" fallback,
  rejected as meaningless on a self-hosted instance and misleading (an
  orange-proxied domain's cert can't issue without the record, so "point the
  proxy at us and it still works" is only true once it exists); and (b) gating
  on a standalone `HELIX_VHOST_ACME_CHALLENGE_TARGET`, which could be set or
  forgotten independently of the solver. Tying the UI to the capability and
  deriving the value keeps the standard deployment zero-config and the panel
  honest. The env var survives only as an override for the rare mismatched-zone
  case.
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
- **Follow-up 2 (user feedback): the dedicated env var was mostly redundant — derive it instead.** `HELIX_VHOST_ACME_CHALLENGE_TARGET` is no longer required. The handler now computes the field in a new `acmeChallengeTarget(cnameTarget)` method (commit `refactor(api): derive ACME challenge target, gate on DNS-01 provider`):
  - Returns `""` unless `VHostACMEDNSProvider == "cloudflare"` — i.e. the self-serve record shows *only when DNS-01 delegation can actually work*, instead of keying off a standalone flag that could be set/forgotten independently of the solver.
  - Otherwise derives `"_acme-challenge." + publicsuffix.EffectiveTLDPlusOne(cnameTarget)` (e.g. `ingress.helix.ml` → `_acme-challenge.helix.ml`). Uses `golang.org/x/net/publicsuffix` (was already an indirect dep; now direct).
  - `HELIX_VHOST_ACME_CHALLENGE_TARGET` remains only as an optional override for the edge case where the ACME zone differs from the CNAME target's registrable domain.
  - Edge cases covered by `project_web_service_acme_test.go`: derivation, override precedence, case-insensitive provider, multi-level TLD (`helix.co.uk`), trailing dot, and no-registrable-domain (`localhost` → `""`).
  - **E2E re-verified:** `HELIX_VHOST_ACME_DNS_PROVIDER=cloudflare` + `HELIX_VHOST_CNAME_TARGET=ingress.helix.ml` (no challenge-target var) → record shows `_acme-challenge.helix.ml` (derived). Removing the provider → section hidden even though the CNAME target has a valid registrable domain. Screenshots refreshed.
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
