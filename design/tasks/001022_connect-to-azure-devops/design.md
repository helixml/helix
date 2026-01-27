# Design: Azure DevOps PAT Connection Persistence Bug

## Overview

Fix the bug where Azure DevOps PAT connections are saved but not recognized when the user returns to the Connect to Azure DevOps dialog.

## Current Architecture

### Data Flow

1. User enters PAT in `BrowseProvidersDialog.tsx`
2. Frontend calls `createPatConnection.mutateAsync()` → `POST /api/v1/git-provider-connections`
3. Backend stores encrypted PAT in `git_provider_connections` table
4. On next dialog open, `useGitProviderConnections()` → `GET /api/v1/git-provider-connections`
5. `getPatConnectionForProvider('azure-devops')` should match connections with `provider_type = 'ado'`

### Key Components

- **Frontend**: `BrowseProvidersDialog.tsx` - UI for provider selection and PAT entry
- **Service**: `gitProviderConnectionService.ts` - React Query hooks for connections
- **Backend**: `git_provider_connection_handlers.go` - API endpoints
- **Store**: `store_git_provider_connection.go` - Database operations

## Investigation Points

### 1. Provider Type Matching (LIKELY ISSUE)

The frontend uses `'azure-devops'` as the provider ID, but the API stores `'ado'`:

```typescript
// Frontend maps 'azure-devops' → 'ado' when creating
const mapProviderType = (provider: ProviderType): TypesExternalRepositoryType => {
  switch (provider) {
    case 'azure-devops':
      return 'ado' as TypesExternalRepositoryType
    // ...
  }
}

// Frontend matching looks for both (this is correct)
const getPatConnectionForProvider = (providerType: ProviderType) => {
  return patConnections?.find(conn => {
    const connType = conn.provider_type?.toLowerCase()
    if (providerType === 'azure-devops') {
      return connType === 'azure-devops' || connType === 'ado'  // Handles both
    }
    // ...
  })
}
```

### 2. Query Loading State

The `patConnections` data comes from `useGitProviderConnections()`. If this query is:
- Not enabled
- Failing silently
- Not refetching after mutation

...then `patConnections` would be empty/undefined.

### 3. Query Cache Invalidation

After creating a connection, the mutation invalidates the cache:
```typescript
onSuccess: () => {
  queryClient.invalidateQueries({ queryKey: CONNECTIONS_KEY })
}
```

But if the dialog closes before the refetch completes, the next open might still have stale data.

## Solution Approach

### Step 1: Add Debug Logging

Add console.log statements to trace the data flow:
- Log `patConnections` when it loads
- Log result of `getPatConnectionForProvider('azure-devops')`
- Log the create mutation response

### Step 2: Verify API Response

Check that the API returns the correct `provider_type` field:
- Should be `"ado"` (lowercase string)
- Verify field name matches TypeScript interface

### Step 3: Fix Identified Issue

Based on debugging, apply the fix:
- If type mismatch: update matching logic
- If query not loading: check `enabled` condition
- If cache issue: ensure proper invalidation timing

## Testing

1. Clear existing connections from database
2. Open BrowseProvidersDialog
3. Select Azure DevOps → Enter PAT → Check "Save connection"
4. Submit and browse repos
5. Close dialog
6. Reopen dialog
7. Azure DevOps should show "Connected as [username]"
8. Click Azure DevOps → Should skip PAT entry and browse directly

## Risk Assessment

- **Low risk**: This is a frontend UI bug with no data loss potential
- **Rollback**: No database changes required