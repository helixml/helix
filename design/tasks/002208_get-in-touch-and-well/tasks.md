# Implementation Tasks: Self-Serve ACME Challenge Record for Proxied Custom Domains

- [x] Add `VHostACMEChallengeTarget` env-config field (`HELIX_VHOST_ACME_CHALLENGE_TARGET`) to the `WebServer` struct in `api/pkg/config/config.go`, next to `VHostCNAMETarget`.
- [x] Add `ACMEChallengeTarget string json:"acme_challenge_target,omitempty"` to `ProjectWebServiceResponse` in `api/pkg/server/project_web_service_handlers.go`.
- [x] Populate `ACMEChallengeTarget` from `s.Cfg.WebServer.VHostACMEChallengeTarget` (trimmed) in `getProjectWebService`.
- [x] Run `./stack update_openapi` to regenerate `frontend/src/api/api.ts` and swagger docs with the new field.
- [x] In `WebServiceTab.tsx`, read `acmeChallengeTarget` from the query response next to `cnameTarget`.
- [x] In `WebServiceTab.tsx`, when `acmeChallengeTarget` is set, replace the "get in touch" paragraph with a self-serve record block (Name `_acme-challenge.app.yourcompany.com`, Type `CNAME`, Value `{acmeChallengeTarget}`) with copy buttons, styled like the existing direct-CNAME block.
- [x] In `WebServiceTab.tsx`, keep the existing "get in touch" paragraph as the fallback when `acmeChallengeTarget` is empty.
- [x] Build check: `go build ./api/pkg/server/ ./api/pkg/config/` passes; frontend `tsc --noEmit` passes (full `yarn build` blocked only by root-owned `dist` bind-mount, not code).
- [ ] E2E-test both states in the inner Helix (env unset = fallback; env set = record block) and save before/after screenshots to `screenshots/`.
- [ ] Commit (conventional format) and open PR; verify Drone CI is green.
