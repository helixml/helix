# Design: Azure DevOps PAT Connection Persistence Bug

## Overview

Fix the bug where Azure DevOps PAT connections are saved but not recognized when the user returns to the Connect to Azure DevOps dialog.

## Key Findings

1. **GitHub PAT reuse works correctly, but Azure DevOps does not.** This narrows the issue to something specific to Azure DevOps handling.

2. **Important context**: GitHub has OAuth configured, Azure DevOps does not. This may be relevant because:
   - GitHub users likely connected via OAuth first (so they have an `oauthConnection`)
   - GitHub PAT reuse may not actually be tested if users are using OAuth instead
   - Azure DevOps users MUST use PAT (no OAuth option), exposing the PAT save bug

## Current Architecture

### Data Flow

1. User enters PAT in `BrowseProvidersDialog.tsx`
2. Frontend calls `createPatConnection.mutateAsync()` → `POST /api/v1/git-provider-connections`
3. Backend validates token via `validateAndFetchUserInfo()` then stores encrypted PAT in `git_provider_connections` table
4. On next dialog open, `useGitProviderConnections()` → `GET /api/v1/git-provider-connections`
5. `getPatConnectionForProvider('azure-devops')` should match connections with `provider_type = 'ado'`

### Key Components

- **Frontend**: `BrowseProvidersDialog.tsx` - UI for provider selection and PAT entry
- **Service**: `gitProviderConnectionService.ts` - React Query hooks for connections
- **Backend**: `git_provider_connection_handlers.go` - API endpoints
- **Store**: `store_git_provider_connection.go` - Database operations

## Root Cause Analysis

### Silent Error Swallowing (MOST LIKELY)

In `handlePatSubmit()` (lines 314-327), errors when saving the connection are silently caught:

```typescript
if (saveConnection) {
  try {
    await createPatConnection.mutateAsync({...})
    snackbar.success('Connection saved for future use')
  } catch (err) {
    // Don't fail the flow if saving fails
    console.error('Failed to save connection:', err)  // USER NEVER SEES THIS
  }
}
```

If the Azure DevOps validation fails on the backend, the user:
1. Still gets to browse repos (because `fetchReposWithPat` uses a different API endpoint)
2. Never sees an error about the failed save
3. Thinks the connection was saved, but it wasn't

### Azure DevOps-Specific Validation

The backend has extra validation for Azure DevOps at `git_provider_connection_handlers.go:324-326`:

```go
case types.ExternalRepositoryTypeADO:
    if orgURL == "" {
        return nil, fmt.Errorf("organization URL is required for Azure DevOps")
    }
    client := azuredevops.NewAzureDevOpsClient(orgURL, token)
    profile, err := client.GetUserProfile(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to validate Azure DevOps token: %w", err)
    }
```

The `GetUserProfile` call may be failing (token scope, API compatibility, org URL format) causing the save to fail silently.

### Provider Type Matching (Verified Correct)

The matching logic correctly handles both `'azure-devops'` (frontend ID) and `'ado'` (backend type):

```typescript
const getPatConnectionForProvider = (providerType: ProviderType) => {
  return patConnections?.find(conn => {
    const connType = conn.provider_type?.toLowerCase()
    if (providerType === 'azure-devops') {
      return connType === 'azure-devops' || connType === 'ado'  // Correct
    }
    return connType === providerType
  })
}
```

## Solution

### Fix 1: Show Save Errors to User

Change the error handling to inform the user when save fails:

```typescript
if (saveConnection) {
  try {
    await createPatConnection.mutateAsync({...})
    snackbar.success('Connection saved for future use')
  } catch (err: any) {
    const message = err?.response?.data || err?.message || 'Unknown error'
    snackbar.error(`Failed to save connection: ${message}`)
    console.error('Failed to save connection:', err)
  }
}
```

### Fix 2: Investigate Azure DevOps Token Validation

If `GetUserProfile` is failing, determine why:
- Token scope issues (PAT may not have profile read permission)
- Organization URL format issues (trailing slash, etc.)
- API endpoint compatibility (cloud vs on-prem)

Consider making profile validation optional or catching the error gracefully.

## Testing

### To Verify the Bug
1. Open browser DevTools console before testing
2. Enter Azure DevOps PAT and check "Save connection"
3. Look for any `console.error` about "Failed to save connection"
4. Check Network tab for `POST /api/v1/git-provider-connections` - look for 400/500 errors
5. If error found, check response body for specific validation failure

### To Verify GitHub PAT (not OAuth)
1. Disconnect any existing GitHub OAuth connection
2. Try connecting to GitHub using PAT only
3. Verify PAT is saved and reused on next dialog open

## Risk Assessment

- **Low risk**: This is a frontend UI bug with no data loss potential
- **Rollback**: No database changes required