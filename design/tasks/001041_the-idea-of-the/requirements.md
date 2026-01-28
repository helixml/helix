# Requirements: Opt-in Public Shareable Spec Task Links

## Problem Statement

Users want the ability to share spec task design documents publicly without requiring the viewer to log in. This should be an explicit opt-in choice for security.

## User Stories

### US1: Make Spec Task Public
**As a** spec task owner  
**I want to** mark my spec task as publicly viewable  
**So that** anyone with the link can view the design documents without logging in

**Acceptance Criteria:**
- [ ] Toggle/checkbox in spec task UI to enable "Public Link"
- [ ] When enabled, `/spec-tasks/{id}/view` works without authentication
- [ ] Only the task owner (or admin) can toggle public visibility
- [ ] Default is private (login required)

### US2: View Public Spec Task
**As a** anyone with a link  
**I want to** view a public spec task's design documents  
**So that** I can review specs without creating an account

**Acceptance Criteria:**
- [ ] Public spec tasks render the same mobile-friendly HTML view
- [ ] No login prompt or authentication required
- [ ] If spec task is not public, redirect to login or show "This spec task is private" message

### US3: Copy Public Link
**As a** spec task owner  
**I want to** easily copy the public link after enabling public access  
**So that** I can share it immediately

**Acceptance Criteria:**
- [ ] "Copy Link" button appears when public access is enabled
- [ ] Link format is simple: `{baseURL}/spec-tasks/{id}/view`
- [ ] Confirmation shown when link is copied

## Out of Scope

- Analytics on public link views
- Password-protected public links
- Expiring public access (once enabled, stays enabled until disabled)