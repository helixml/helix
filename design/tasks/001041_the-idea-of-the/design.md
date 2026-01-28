# Design: Opt-in Public Shareable Spec Task Links

## Overview

Add an opt-in `PublicDesignDocs` boolean field to SpecTask that allows the design documents to be viewed without authentication when enabled.

## Architecture Decision

**Approach**: Add a simple boolean field to SpecTask, modify the existing public view handler to check it.

This follows the existing `Global` pattern used by `App` and `Tool` types in the codebase for similar public/private visibility control.

## Data Model Changes

### SpecTask (types/simple_spec_task.go)

Add new field:

```go
// Public sharing
PublicDesignDocs bool `json:"public_design_docs" gorm:"default:false"` // Allow viewing design docs without login
```

## API Changes

### Modified Endpoint: GET /spec-tasks/{id}/view

**Current behavior**: Requires `token` query parameter with valid JWT

**New behavior**:
1. If `task.PublicDesignDocs == true` → render design docs (no auth needed)
2. Else → return user-friendly "This spec task is private" HTML page with link to login

### Modified Endpoint: PATCH /api/v1/spec-tasks/{id}

Allow updating `public_design_docs` field. Only task owner or admin can modify.

Add to `SpecTaskUpdateRequest`:
```go
PublicDesignDocs *bool `json:"public_design_docs,omitempty"`
```

## Frontend Changes

### SpecTaskReviewPanel.tsx

Replace the existing "Get Shareable Link" section with a simpler public toggle:

```
┌─────────────────────────────────────┐
│ 🔗 Share Design Docs                │
│                                     │
│ ☐ Make publicly viewable            │
│   Anyone with the link can view     │
│                                     │
│ [Copy Link] (shown when enabled)    │
└─────────────────────────────────────┘
```

- Toggle calls PATCH `/api/v1/spec-tasks/{id}` with `{ public_design_docs: true/false }`
- When public, show "Copy Link" button that copies `{baseURL}/spec-tasks/{id}/view`
- Remove existing token-based share link generation

## Security Considerations

1. **Explicit opt-in**: Default is `false` (private)
2. **Owner-only control**: Only task creator or admin can enable public access
3. **Design docs only**: Public access shows requirements/design/implementation plan - no session data, no credentials
4. **Reversible**: Owner can disable public access at any time

## Migration

GORM AutoMigrate handles adding the new boolean column with default `false`. No data migration needed.

## Cleanup

Remove the token-based share link feature:
- Delete `generateDesignDocsShareLink` handler
- Delete `DesignDocsShareTokenClaims` and `DesignDocsShareLinkResponse` types
- Remove POST `/api/v1/spec-tasks/{id}/design-docs/share` route

## Implementation Notes

### Files Modified

**Backend:**
- `api/pkg/types/simple_spec_task.go` - Added `PublicDesignDocs` field to `SpecTask` and `SpecTaskUpdateRequest`
- `api/pkg/server/spec_task_share_handlers.go` - Rewrote `viewDesignDocsPublic` to check `PublicDesignDocs`, added private task template, removed token-based logic
- `api/pkg/server/spec_driven_task_handlers.go` - Added handling for `PublicDesignDocs` in `updateSpecTask` handler
- `api/pkg/server/server.go` - Removed the POST `/api/v1/spec-tasks/{id}/design-docs/share` route

**Frontend:**
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` - Added public toggle in details view (above Archive button)
- `frontend/src/components/tasks/SpecTaskReviewPanel.tsx` - Replaced token-based share with public toggle

### Key Decisions

1. **Placed toggle in SpecTaskDetailContent.tsx** - This is the main task details view that users see when viewing a task. The toggle is in a "Share Design Docs" section above the Archive button.

2. **Uses existing PUT endpoint** - No new API endpoint needed. The `PUT /api/v1/spec-tasks/{taskId}` endpoint already handles updates, we just added `public_design_docs` to the request schema.

3. **Private page shows login link** - When a task is not public, visitors see a friendly message with a link to login rather than a raw 401 error.

### Patterns Used

- Followed the existing `Global` pattern used by `App` and `Tool` types for public/private visibility
- Used MUI `Switch` component with `FormControlLabel` for the toggle UI (consistent with other toggles in the codebase)
- Used `api.put()` for updates (matches existing patterns in `SpecTaskDetailContent.tsx`)