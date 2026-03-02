# Implementation Tasks

## Frontend Fix (Primary)

- [ ] Update `frontend/src/pages/Onboarding.tsx` to pass `createdOrg?.id` to `AddProviderDialog` instead of empty string
  - Line ~2024: Change `orgId=""` to `orgId={createdOrg?.id || ''}`

## Backend Fix (Fallback)

- [ ] Update `api/pkg/server/anthropic_api_proxy_handler.go` in `getProviderEndpoint()` to fall back to user-level provider lookup when org-level lookup fails
  - After org-level query fails, add query with `Owner: user.ID`
  - Ensure this only happens when `user.OrganizationID != ""` (already in org context)

## Testing

- [ ] Add unit test for `getProviderEndpoint` fallback behavior in `anthropic_api_proxy_handler_test.go`
- [ ] Manual test: Complete onboarding with Anthropic API key, create spec task, verify LLM calls succeed
- [ ] Verify existing org-level providers still take precedence over user-level