# Requirements: Azure DevOps PAT Connection Persistence Bug

## Problem Statement

When connecting to Azure DevOps via Personal Access Token (PAT), the dialog promises to save the PAT for future use, but never actually offers to reuse it. Users are prompted to enter their PAT every time they want to browse Azure DevOps repositories.

## User Stories

### US-1: Saved PAT Connection Recognition
**As a** user who has previously saved an Azure DevOps PAT connection  
**I want** the system to recognize my saved connection  
**So that** I don't have to re-enter my PAT every time I browse repositories

### US-2: Direct Repository Browsing
**As a** user with a saved Azure DevOps connection  
**I want** to click "Azure DevOps" and immediately browse my repositories  
**So that** the experience is as smooth as other providers with OAuth

## Acceptance Criteria

1. When a user has a saved Azure DevOps PAT connection:
   - The provider list should show "Connected as [username]" 
   - Clicking "Use Personal Access Token" should skip the PAT entry form
   - The system should directly browse repositories using the saved connection

2. The saved connection must persist across:
   - Dialog close/reopen
   - Page refresh
   - Browser session restart

3. Users should be able to:
   - Delete a saved connection and enter a new PAT
   - See which Azure DevOps organization they're connected to

## Root Cause Analysis

The `getPatConnectionForProvider()` function in `BrowseProvidersDialog.tsx` correctly matches both `'azure-devops'` and `'ado'` provider types. The backend stores the type as `'ado'` (as defined in `ExternalRepositoryTypeADO`).

**Suspected issue**: The `patConnections` data from `useGitProviderConnections()` may not be loading correctly, or the query may be failing silently. The matching logic appears correct, so the bug is likely in:
1. The API call not being made
2. The response not being parsed correctly
3. A race condition where the UI renders before connections are loaded

## Out of Scope

- OAuth integration for Azure DevOps (separate feature)
- Service Principal authentication (enterprise feature)
- Multi-organization support