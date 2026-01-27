# Requirements: Azure DevOps PAT Connection Persistence Bug

## Problem Statement

When connecting to Azure DevOps via Personal Access Token (PAT), the dialog promises to save the PAT for future use, but never actually offers to reuse it. Users are prompted to enter their PAT every time they want to browse Azure DevOps repositories.

**Key observation:** GitHub PAT connections work correctly and are reused. Azure DevOps connections are not. This indicates the issue is specific to Azure DevOps handling, not a general PAT storage problem.

## User Stories

### US-1: Saved PAT Connection Recognition
**As a** user who has previously saved an Azure DevOps PAT connection  
**I want** the system to recognize my saved connection  
**So that** I don't have to re-enter my PAT every time I browse repositories

### US-2: Direct Repository Browsing
**As a** user with a saved Azure DevOps connection  
**I want** to click "Azure DevOps" and immediately browse my repositories  
**So that** the experience is as smooth as GitHub with a saved PAT

### US-3: Visible Save Errors
**As a** user attempting to save an Azure DevOps connection  
**I want** to see an error message if the save fails  
**So that** I know whether my connection was actually saved

## Acceptance Criteria

1. When a user has a saved Azure DevOps PAT connection:
   - The provider list should show "Connected as [username]" 
   - Clicking "Use Personal Access Token" should skip the PAT entry form
   - The system should directly browse repositories using the saved connection

2. When saving a connection fails:
   - User should see an error notification (not silent failure)
   - Error message should explain why the save failed

3. The saved connection must persist across:
   - Dialog close/reopen
   - Page refresh
   - Browser session restart

4. Users should be able to:
   - Delete a saved connection and enter a new PAT
   - See which Azure DevOps organization they're connected to

## Root Cause Hypothesis

The `handlePatSubmit()` function silently catches and logs errors when `createPatConnection.mutateAsync()` fails. If Azure DevOps token validation fails on the backend (e.g., `GetUserProfile()` returns an error), the user:
1. Can still browse repos (because `fetchReposWithPat` uses a different API)
2. Never sees the save failure
3. Returns later expecting the connection to be saved, but it wasn't

## Out of Scope

- OAuth integration for Azure DevOps (separate feature)
- Service Principal authentication (enterprise feature)
- Multi-organization support