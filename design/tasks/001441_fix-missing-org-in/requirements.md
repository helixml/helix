# Requirements: Fix Org-less Project URLs

## Problem Statement

Users can land on project URLs missing the `/org/:org_id` segment (e.g., `/projects/prj_xxx/settings` instead of `/org/helix/projects/prj_xxx/settings`). This causes broken UI: missing agent lists, failed settings saves, and unavailable org-scoped data.

## User Stories

1. **As a user** clicking an old or malformed bookmark, I want to be redirected to the correct org-scoped URL so the page works properly.

2. **As a user** following a shared link without org context, I want to either land on the correct page or see a clear error—not a broken UI.

## Acceptance Criteria

- [ ] Navigating to any `/projects/:projectId/*` URL without org prefix redirects to `/org/:orgSlug/projects/:projectId/*`
- [ ] Full sub-path is preserved (e.g., `/projects/:id/settings` → `/org/:slug/projects/:id/settings`)
- [ ] Agents list loads correctly after redirect
- [ ] Project settings save successfully after redirect
- [ ] No infinite redirect loops occur
- [ ] If project not found or org cannot be resolved, redirect to homepage (not broken page)
- [ ] Works for all project routes: `/settings`, `/specs`, `/tasks/:taskId`, `/session/:sessionId`, `/desktop/:sessionId`

## Out of Scope

- Fixing other non-project org-less routes (separate task if needed)
- Deep URL rewriting at server level (this is a frontend fix)