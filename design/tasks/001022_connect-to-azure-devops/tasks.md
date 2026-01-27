# Implementation Tasks

## Investigation (Do First)

- [ ] Open browser DevTools console before testing Azure DevOps connection
- [ ] Enter Azure DevOps PAT with "Save connection" checked, submit form
- [ ] Check console for `Failed to save connection:` error message
- [ ] Check Network tab for `POST /api/v1/git-provider-connections` - look for 400/500 errors
- [ ] If error found, check response body for specific validation failure message
- [ ] Compare with GitHub PAT flow - does GitHub show the same error or succeed?

## Likely Fix: Show Save Errors to User

- [ ] In `BrowseProvidersDialog.tsx` `handlePatSubmit()`, change silent `console.error` to `snackbar.error()`
- [ ] Extract error message from response: `err?.response?.data || err?.message`
- [ ] Test that user now sees feedback when save fails

## If Azure DevOps Validation is Failing

- [ ] Check `GetUserProfile()` in `api/pkg/agent/skill/azure_devops/client.go`
- [ ] Verify the Azure DevOps profile API URL is correct for the user's org type (cloud vs server)
- [ ] Check if PAT requires specific scopes to access user profile
- [ ] Test with a known-working Azure DevOps PAT and org URL

## Verification

- [ ] Create new Azure DevOps PAT connection - verify "Connection saved" message appears
- [ ] Close and reopen dialog - verify "Connected as [user]" shows for Azure DevOps
- [ ] Click Azure DevOps - verify it skips PAT entry and browses repos directly
- [ ] Query database: `SELECT * FROM git_provider_connections WHERE provider_type = 'ado'`
- [ ] Test GitHub PAT flow still works correctly (regression test)