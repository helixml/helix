# Implementation Tasks

- [ ] Add `HasProviders bool` field to `ServerConfigForFrontend` in `api/pkg/types/types.go`
- [ ] In `api/pkg/server/handlers.go` `getConfig()`, query global provider endpoints and set `HasProviders` to true if any have enabled chat models
- [ ] Regenerate OpenAPI client: `./stack update_openapi`
- [ ] In `frontend/src/pages/Onboarding.tsx`, extend `visibleSteps` filter to exclude "provider" step when `serverConfig.has_providers` is true
- [ ] Verify `cd frontend && yarn build` succeeds
- [ ] Test: deploy with global providers configured → onboarding should skip provider step
- [ ] Test: deploy with no global providers → onboarding should show provider step as before
