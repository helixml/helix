# Skip "Connect an AI provider" onboarding step when global providers exist

## Summary
When a Helix deployment already has global AI providers configured, the "Connect an AI provider" onboarding step is now hidden entirely — users go straight from org setup to project creation.

## Changes
- Added `has_providers` field to `ServerConfigForFrontend` (Go type + API response)
- In `getConfig()`, query for global provider endpoints to set the flag
- Extended `visibleSteps` filter in `Onboarding.tsx` to exclude the provider step when `has_providers` is true (same pattern as existing `billing_enabled` / subscription step filter)
- Regenerated OpenAPI client

## Test plan
- Verified with global provider in DB: provider step hidden, stepper shows 4 steps, flow proceeds org → project correctly
- Verified without global providers: provider step appears as before
- Frontend build passes
- Go build passes
