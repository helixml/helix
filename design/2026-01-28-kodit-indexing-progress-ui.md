# Kodit Indexing Progress in Repository List UI

**Date:** 2026-01-28
**Status:** Proposed

## Problem

When code intelligence is enabled on a repository, users have no visibility into the indexing progress from the repository list view. They must navigate to the individual repository's "Code Intelligence" tab to see the status.

The Kodit logs show detailed progress (e.g., "create_summary_enrichment at 45%"), but this information is not exposed in the UI.

## Current State

### Backend (Already Implemented)
- **Endpoint:** `GET /api/v1/git/repositories/{id}/kodit-status`
- **Handler:** `api/pkg/server/kodit_handlers.go:getRepositoryIndexingStatus`
- **Response:** `KoditIndexingStatus` with status values: `completed`, `indexing`, `in_progress`, `queued`, `pending`, `failed`

### Frontend (Partially Implemented)
- **`KoditStatusPill`** component exists at `frontend/src/components/git/KoditStatusPill.tsx`
  - Shows colored status pill with appropriate icons
  - Handles all status states
- **`useKoditStatus`** hook exists at `frontend/src/services/koditService.ts`
  - Polls every 10 seconds
  - Conditionally enabled
- **Code Intelligence tab** uses `KoditStatusPill` - works correctly
- **Repository list view** shows static "Code Intelligence" chip (no status)

## Proposed Solution

Replace the static chip in `RepositoriesListView.tsx` with the existing `KoditStatusPill` component.

### File to Modify

`frontend/src/components/project/RepositoriesListView.tsx`

### Current Code (lines 184-192)

```tsx
{repo.kodit_indexing && (
  <Chip
    label="Code Intelligence"
    size="small"
    icon={<BrainIcon fontSize="small" />}
    color="success"
    variant="outlined"
  />
)}
```

### Proposed Code

```tsx
{repo.kodit_indexing && (
  <KoditStatusPill repoId={repo.id} />
)}
```

### Performance Consideration

Each repository with `kodit_indexing = true` will trigger a status API call. For lists with many repos, this could cause many requests.

**Mitigation options (if needed):**
1. Lazy-load status only for visible rows
2. Add a batch endpoint to fetch status for multiple repos at once
3. Only show status pill on hover or click

For now, start with the simple approach - most users won't have hundreds of repos with code intelligence enabled.

## Implementation Steps

1. Import `KoditStatusPill` in `RepositoriesListView.tsx`
2. Replace the static `Chip` with `<KoditStatusPill repoId={repo.id} />`
3. Test with repos in various states (indexing, completed, failed)
4. Run `cd frontend && yarn test && yarn build`

## Out of Scope

- Detailed progress percentage (Kodit status endpoint doesn't expose this)
- Bulk status endpoint
- Real-time WebSocket updates

## Testing

1. Enable code intelligence on a repo
2. Verify status shows "Indexing" or similar in the repo list
3. Wait for completion, verify shows "Completed"
4. Test error state by simulating a Kodit failure
