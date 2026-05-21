# Implementation Tasks: Restore Kanban Board Scroll Position on Return Navigation

## Change 1 — Kanban scroll restoration (frontend)

- [x] Create `frontend/src/components/tasks/kanbanScrollMemory.ts` exporting `getKanbanScrollState`, `saveKanbanHorizontalScroll`, `saveKanbanColumnScroll`, `clearKanbanScrollState` backed by a module-scoped `Map<string, { horizontal: number; columns: Record<string, number> }>` keyed by `projectId`.
- [x] In `SpecTaskKanbanBoard.tsx`, add a `useRef<HTMLDivElement>` for the outer column-strip `<Box>` and attach it.
- [x] In `SpecTaskKanbanBoard.tsx`, add a `useRef<Map<string, HTMLDivElement>>` registry and a callback-ref factory `getColumnRefSetter(columnId)` with stable identity.
- [x] In `DroppableColumn`, accept `columnBodyRef` + `onColumnScroll` props; replaced the existing no-op `setNodeRef` stub.
- [x] Add `onScroll` handlers (synchronous save) on the outer strip and each column body; skip horizontal on mobile.
- [x] Add `hasRestoredRef`, `userHasScrolledRef`, `isRestoringRef`; reset restoration guards when `projectId` changes.
- [x] Add a `useLayoutEffect` that restores positions; re-runs across renders until satisfied; clamps when columns shrink; distinguishes "data not loaded" from "column shrunk" via `columns.some(c => c.tasks.length > 0)`.
- [x] Verified mobile single-column path.
- [x] `npx tsc --noEmit` clean.
- [x] Manually tested all paths in the inner Helix (horizontal restore, per-column vertical restore, live-save of latest scroll value, clamp after archive). Screenshots `02-kanban-after-restore.png`, `03-kanban-restored-with-tasks.png`.

## Change 2 — Backend re-syncs PR description from helix-specs (platform fix)

Added after the kanban work because the very PR opened for this task (`https://github.com/helixml/helix/pull/2450`) had the wrong body — the original task prompt instead of `pull_request_helix.md` content. Root cause: the backend templated PR descriptions only at creation time, so a `pull_request_*.md` written *after* the first feature-branch push was silently ignored.

- [x] In `api/pkg/services/git_http_server.go`, in `processDesignDocsForBranch`, after the per-task status switch: if the pushed branch is `helix-specs` and the commit touched `pull_request*.md` in that task's design-doc directory, fire `syncOpenPRDescriptions` as a goroutine for that task.
- [x] Add `syncOpenPRDescriptions(ctx, task, repo, repoPath)`: looks up the open PR for this repo, re-reads `pull_request_<repo-name>.md` (or generic `pull_request.md`) from helix-specs, rebuilds the same footer, calls `GitRepositoryService.UpdatePullRequest`. Bails if file missing/unparseable (don't stomp manually-authored bodies).
- [x] Add `pullRequestFileChangedForTask(files, designDocPath)` helper that detects whether the commit touched `pull_request*.md` directly in the task dir (subdirs like `screenshots/` ignored).
- [x] Add unit test `TestPullRequestFileChangedForTask` (10 cases) in `api/pkg/services/git_http_server_test.go`. All pass.
- [x] `go build ./api/pkg/services/` clean.
- [x] Patch PR #2450 body directly via GitHub MCP to reflect both changes (the platform code change can't update an already-open PR until it's deployed; manual patch covers the existing case).
- [ ] **Limitation: end-to-end sync test from inside this inner Helix is not possible.** PR #2450 was created by the *outer* Helix; the inner Helix this task runs in has no link to that PR (its DB has no row for `spt_01krav8qpvfrj1tj8a6r5jmz9z`). The new code path will run for real when the outer Helix picks up this PR after merge.

## Notes

- The Helix backend creates the GitHub PR on the *first* push to the feature branch and templates the body from the spec-task prompt (with a "🔗 Open in Helix / 🚀 Built with Helix" footer). It then never updated the body again — that's the bug Change 2 fixes.
- `git_http_server.go` and `spec_task_workflow_handlers.go` each have their own near-duplicate `getPullRequestContent` / `parsePullRequestMarkdown` / `buildPRFooter`. Out of scope to dedupe here; Change 2 stays in `git_http_server.go` to match the push path.
- Existing comment in `ensurePullRequest` ("Do not update the PR title/description here — the user may have renamed the PR") deliberately avoids updates on every feature-branch push. Change 2 doesn't relax that guard; it adds a *new* path that only fires on helix-specs pushes that touched `pull_request*.md` — making the trigger explicit (user edited the description on purpose).
- Discovered that `docker-compose.dev.yaml` added a `./helix-org:/app/helix-org` mount in a recent main commit; the running api container needs `docker compose up -d api` after pulling main, otherwise the new code referencing `helix-org/*` packages fails to build.
