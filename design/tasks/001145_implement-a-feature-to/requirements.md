# Requirements: Move Project to Organization

## Overview

Allow users to move a project from their personal workspace into an organization, enabling RBAC roles and team sharing.

## User Stories

### US-1: Move Personal Project to Organization
**As a** user with projects in my personal workspace  
**I want to** move a project into an organization I belong to  
**So that** I can share the project with my team and apply RBAC roles

### US-2: Authorization Check
**As a** system administrator  
**I want** the move operation to verify the user has appropriate permissions  
**So that** users can only move projects to organizations they're members of

### US-3: UI for Moving Projects
**As a** user viewing my personal project settings  
**I want to** see an option to move the project to an organization  
**So that** I can easily share my work with my team

## Acceptance Criteria

### AC-1: Basic Move Operation
- [ ] User can move a project from personal workspace to an organization
- [ ] User must be the owner of the project (project.UserID matches user.ID)
- [ ] User must be a member of the target organization
- [ ] Project's `OrganizationID` field is updated to the target org ID
- [ ] Project remains associated with the same UserID (original owner)

### AC-2: Associated Resources
- [ ] Git repositories linked to the project have their `OrganizationID` updated
- [ ] ProjectRepository junction table entries have their `OrganizationID` updated
- [ ] Existing sessions associated with the project continue to work

### AC-3: Error Handling
- [ ] Return 403 if user is not the project owner
- [ ] Return 403 if user is not a member of the target organization
- [ ] Return 400 if target organization ID is invalid or empty
- [ ] Return 404 if project doesn't exist

### AC-4: Audit Trail
- [ ] Log the project move operation for audit purposes

### AC-5: UI in Danger Zone (Personal Projects Only)
- [ ] Show "Move to Organization" section in Danger Zone for personal projects only
- [ ] Hide the section when project is already in an organization
- [ ] Display dropdown to select target organization (user's memberships)
- [ ] Show confirmation dialog before moving
- [ ] Explain that this is a one-way operation (cannot move back to personal)
- [ ] Refresh project data after successful move

## Out of Scope (v1)

- Moving projects back from organization to personal workspace (one-way move only)
- Transferring project ownership to another user
- Bulk move operations (multiple projects at once)