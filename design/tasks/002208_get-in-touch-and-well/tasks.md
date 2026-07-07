# Implementation Tasks: Self-Serve ACME Challenge Record for Proxied Custom Domains

- [x] Add `VHostACMEChallengeTarget` env-config field (`HELIX_VHOST_ACME_CHALLENGE_TARGET`) to the `WebServer` struct in `api/pkg/config/config.go`, next to `VHostCNAMETarget`.
- [x] Add `ACMEChallengeTarget string json:"acme_challenge_target,omitempty"` to `ProjectWebServiceResponse` in `api/pkg/server/project_web_service_handlers.go`.
- [x] Populate `ACMEChallengeTarget` from `s.Cfg.WebServer.VHostACMEChallengeTarget` (trimmed) in `getProjectWebService`.
- [x] Run `./stack update_openapi` to regenerate `frontend/src/api/api.ts` and swagger docs with the new field.
- [x] In `WebServiceTab.tsx`, read `acmeChallengeTarget` from the query response next to `cnameTarget`.
- [x] In `WebServiceTab.tsx`, when `acmeChallengeTarget` is set, replace the "get in touch" paragraph with a self-serve record block (Name `_acme-challenge.app.yourcompany.com`, Type `CNAME`, Value `{acmeChallengeTarget}`) with copy buttons, styled like the existing direct-CNAME block.
- [x] ~~Keep the "get in touch" paragraph as the fallback when empty.~~ **Superseded by user feedback:** eliminate "get in touch" entirely — when `acmeChallengeTarget` is empty, omit the whole proxy/delegation section (direct-CNAME steps remain). Re-verified both states E2E.
- [x] Build check: `go build ./api/pkg/server/ ./api/pkg/config/` passes; frontend `tsc --noEmit` passes (full `yarn build` blocked only by root-owned `dist` bind-mount, not code).
- [x] E2E-test both states in the inner Helix (env unset = fallback; env set = record block) and save before/after screenshots to `screenshots/`.
- [x] Commit (conventional format), merge latest main, push feature branch `feature/002208-self-serve-acme`. (Drone CI runs once the platform opens the GitHub PR — not creatable from here.) — PR #2813, CI green.
- [x] **Follow-up (user feedback):** drop the dedicated env var; gate the record on `VHostACMEDNSProvider == "cloudflare"` and derive the target from the CNAME target's registrable domain (`publicsuffix`), keeping `HELIX_VHOST_ACME_CHALLENGE_TARGET` as an optional override. Added unit tests; re-verified both states E2E.
