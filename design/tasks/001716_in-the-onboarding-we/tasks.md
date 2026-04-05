# Implementation Tasks

- [x] Add `HasProviders bool` field to `ServerConfigForFrontend` in `api/pkg/types/types.go`
- [x] In `api/pkg/server/handlers.go` `getConfig()`, query global provider endpoints and set `HasProviders` to true if any exist
- [x] Regenerate OpenAPI client: `./stack update_openapi`
- [x] In `frontend/src/pages/Onboarding.tsx`, extend `visibleSteps` filter to exclude "provider" step when `serverConfig.has_providers` is true
- [x] Verify `cd frontend && yarn build` succeeds
- [x] Browser test: register new user with global providers configured → onboarding should not show provider step at all, step numbering correct, full flow completes
- [x] Browser test: register new user with no global providers → provider step appears as before, can complete or skip
