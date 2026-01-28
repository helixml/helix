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
PublicDesignDocs bool `json:"public_design_docs" gorm:"default:false"` // Allow viewing design docs without token
```

## API Changes

### Modified Endpoint: GET /spec-tasks/{id}/view

**Current behavior**: Requires `token` query parameter with valid JWT

**New behavior**:
1. If `task.PublicDesignDocs == true` → render design docs (no auth needed)
2. Else if valid `token` provided → render design docs (existing behavior)
3. Else → return user-friendly "This spec task is private" HTML page

### Modified Endpoint: PATCH /api/v1/spec-tasks/{id}

Allow updating `public_design_docs` field. Only task owner or admin can modify.

Add to `SpecTaskUpdateRequest`:
```go
PublicDesignDocs *bool `json:"public_design_docs,omitempty"`
```

## Frontend Changes

### SpecTaskReviewPanel.tsx

Add toggle switch above "Get Shareable Link" button:

```
┌─────────────────────────────────────┐
│ 📱 View on Any Device               │
│                                     │
│ ☐ Make design docs public           │
│   Anyone with the link can view     │
│                                     │
│ [Copy Public Link] (when enabled)   │
│ ─────────────── or ───────────────  │
│ [Get Temporary Link] (7 day token)  │
└─────────────────────────────────────┘
```

- Toggle calls PATCH `/api/v1/spec-tasks/{id}` with `{ public_design_docs: true/false }`
- When public, show simple "Copy Link" that copies `{baseURL}/spec-tasks/{id}/view`
- Keep existing "Get Shareable Link" as fallback for temporary token-based access

## Security Considerations

1. **Explicit opt-in**: Default is `false` (private)
2. **Owner-only control**: Only task creator or admin can enable public access
3. **Design docs only**: Public access shows requirements/design/implementation plan - no session data, no credentials
4. **Reversible**: Owner can disable public access at any time

## Migration

GORM AutoMigrate handles adding the new boolean column with default `false`. No data migration needed.