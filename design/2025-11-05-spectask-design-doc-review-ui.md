# SpecTask Design Document Review UI

**Date:** 2025-11-05
**Status:** Planned (not implemented)

## Context

SpecTasks have a planning phase where a Zed agent generates design documents and commits them to a `helix-design-docs` orphan branch in the project's primary code repository. Currently, there's no inline UI for reviewing these docs.

## Current State

**What Works:**
- ✅ Design docs ARE committed to `helix-design-docs` branch via git worktree (wolf_executor.go:2414-2530)
- ✅ Shareable links for mobile review (`POST /spec-tasks/{id}/design-docs/share`)
- ✅ Public viewer endpoint (`/spec-tasks/{id}/view`)
- ✅ Approval workflow UI (SpecTaskReviewPanel.tsx) with "Approve" and "Request Changes" buttons
- ✅ Interactive feedback - user can chat with planning agent to refine specs

**What's Missing:**
- ❌ No inline design doc viewer in the UI
- ❌ `getSpecTaskDesignDocs` endpoint is a stub (returns empty strings)
- ❌ Users can't see the actual markdown files before approving

**Current Workarounds:**
- Generate shareable link and view externally
- Open planning session to read agent chat messages
- Manually browse git branch (not practical)

## Proposed Implementation

**When:** After spec generation completes and task moves to "Review" column in Kanban board

**Where:** Kanban board task card in "Review" column

**UI Flow:**
1. Task card shows "View Design Docs" button when status = `spec_review`
2. Click opens modal/drawer with markdown viewer
3. Display files from `helix-design-docs` branch (fetched from git worktree)
4. User reads design docs inline
5. User clicks "Approve" or "Request Changes" from the viewer

**Backend Changes Needed:**
1. Implement `getSpecTaskDesignDocs` to read from worktree (currently returns empty strings at spec_task_orchestrator_handlers.go:183-189)
2. Return list of markdown files with content
3. Use DesignDocsWorktreeManager to access the git worktree

**Frontend Changes Needed:**
1. Add design doc viewer component (markdown renderer)
2. Add "View Design Docs" button to Kanban task card in review column
3. Integrate with existing SpecTaskReviewPanel approval workflow

## Technical Details

**Design Docs Location:**
- Branch: `helix-design-docs` (orphan branch, separate history)
- Worktree path: `{repoPath}/.git-worktrees/helix-design-docs/`
- Created during primary repo clone (wolf_executor.go:2300, 2403)

**Existing Services:**
- `DesignDocsWorktreeManager` - already exists for managing worktrees
- `getSpecTaskDesignDocs` endpoint - exists but not implemented

## Notes

- Keep it simple: just display markdown files inline
- Reuse existing approval buttons from SpecTaskReviewPanel
- Design docs should be human-readable markdown (Zed agent already generates these)
- Mobile shareable links still useful for on-the-go review
