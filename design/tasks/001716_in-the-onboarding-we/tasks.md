# Implementation Tasks

- [ ] Add `HasProviders bool` field to `ServerConfigForFrontend` in `api/pkg/types/types.go`
- [ ] In `api/pkg/server/handlers.go` `getConfig()`, query global provider endpoints and set `HasProviders` to true if any have enabled chat models
- [ ] Regenerate OpenAPI client: `./stack update_openapi`
- [ ] In `frontend/src/pages/Onboarding.tsx`, extend `visibleSteps` filter to exclude "provider" step when `serverConfig.has_providers` is true
- [ ] Verify `cd frontend && yarn build` succeeds
- [ ] Browser test: register new user with global providers configured → onboarding should not show provider step at all, step numbering correct, full flow completes
- [ ] Browser test: register new user with no global providers → provider step appears as before, can complete or skip
