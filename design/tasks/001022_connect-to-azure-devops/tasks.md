# Implementation Tasks

## Investigation

- [ ] Add console.log in `BrowseProvidersDialog.tsx` to log `patConnections` data when loaded
- [ ] Add console.log to log result of `getPatConnectionForProvider('azure-devops')` 
- [ ] Check browser Network tab to verify `GET /api/v1/git-provider-connections` is being called
- [ ] Verify API response includes connections with `provider_type: "ado"`
- [ ] Check if `patConnectionsLoading` is stuck as `true`

## Potential Fixes

- [ ] Verify `useGitProviderConnections()` hook is returning data correctly
- [ ] Check if React Query cache is properly invalidated after `createPatConnection` mutation
- [ ] Ensure dialog doesn't close before query cache invalidation completes
- [ ] Verify the `provider_type` field in TypeScript interface matches API response casing

## Testing

- [ ] Test creating new Azure DevOps PAT connection with "Save connection" checked
- [ ] Verify connection appears in database: `SELECT * FROM git_provider_connections WHERE provider_type = 'ado'`
- [ ] Close and reopen dialog - verify "Connected as [user]" shows
- [ ] Click Azure DevOps - verify it skips PAT entry form and browses repos directly
- [ ] Refresh page - verify saved connection is still recognized
- [ ] Test deleting saved connection and adding a new one