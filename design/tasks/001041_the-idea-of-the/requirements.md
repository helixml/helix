# Requirements: Opt-in Public Shareable Spec Task Links

## Problem Statement

Currently, shareable spec task links use JWT tokens that expire after 7 days. The original intent was that these links shouldn't require login, but users must explicitly generate a token-based link each time. 

Users want the ability to make a spec task's design documents permanently publicly accessible without requiring a token, while maintaining security by making this an explicit opt-in choice.

## User Stories

### US1: Make Spec Task Public
**As a** spec task owner  
**I want to** mark my spec task as publicly viewable  
**So that** anyone with the link can view the design documents without needing a token or login

**Acceptance Criteria:**
- [ ] Toggle/checkbox in spec task UI to enable "Public Link"
- [ ] When enabled, `/spec-tasks/{id}/view` works without a token parameter
- [ ] Only the task owner (or admin) can toggle public visibility
- [ ] Default is private (requires token)

### US2: View Public Spec Task
**As a** anyone with a link  
**I want to** view a public spec task's design documents  
**So that** I can review specs without creating an account or requesting access

**Acceptance Criteria:**
- [ ] Public spec tasks render the same mobile-friendly HTML view
- [ ] No login prompt or authentication required
- [ ] If spec task is not public, show "This spec task is private" message (not 401)

### US3: Copy Public Link
**As a** spec task owner  
**I want to** easily copy the public link after enabling public access  
**So that** I can share it immediately

**Acceptance Criteria:**
- [ ] "Copy Link" button appears when public access is enabled
- [ ] Link format is simple: `{baseURL}/spec-tasks/{id}/view` (no token needed)
- [ ] Confirmation shown when link is copied

## Out of Scope

- Revoking individual token-based links (tokens still expire after 7 days)
- Analytics on public link views
- Password-protected public links
- Expiring public access (once enabled, stays enabled until disabled)