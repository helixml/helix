# Restore kanban scroll position on return navigation (+ platform fix for late PR descriptions)

## Summary

Originally a single-issue PR for restoring Kanban board scroll position;
hijacked to also include a small platform fix uncovered while writing the
first feature — the Helix backend was templating PR descriptions only at
creation time, so `pull_request_*.md` files written after the first push to
the feature branch were silently ignored. This PR's own original description
was a victim of that bug.

## Change 1 — Restore kanban scroll position on return navigation

When users click a task on the Kanban board and then navigate back, both the
horizontal column-strip scroll and each column's vertical scroll position are
now restored to where they left off — instead of resetting to `(0, 0)`.

State is scoped per-project and lives in memory only (no `localStorage`), so
it survives in-app navigation but is wiped on full reload. It preserves "the
last place you were within this SPA session", not a permanent preference.

**Frontend changes** (`frontend/src/components/tasks/`):

- New `kanbanScrollMemory.ts` — module-scoped
  `Map<projectId, { horizontal, columns: { [columnId]: scrollTop } }>` with
  get / save helpers.
- `SpecTaskKanbanBoard.tsx`:
  - Outer column-strip `<Box>` gets a ref + synchronous `onScroll` that
    persists `scrollLeft` per `projectId` (desktop only).
  - `DroppableColumn` gains `columnBodyRef` + `onColumnScroll` props; the
    parent passes per-column ref-setters and scroll handlers so each
    column's `scrollTop` is saved under its `column.id`. Replaces the
    pre-existing no-op `setNodeRef` stub.
  - A `useLayoutEffect` restores both surfaces once data has loaded.
    Bails if the user has already scrolled, runs at most once successful
    pass per mount, and clamps any saved value that exceeds the current
    `scrollWidth` / `scrollHeight` (e.g. tasks archived since the previous
    visit). Re-runs across renders until satisfied — needed because on a
    warm react-query cache the local `tasks` state is briefly empty on the
    first render after remount.

## Change 2 — Backend re-syncs PR description from `helix-specs` on push

`api/pkg/services/git_http_server.go`:

- `processDesignDocsForBranch` now notices when a push to `helix-specs`
  includes `pull_request*.md` inside a task's design-doc directory, and
  fires a new helper `syncOpenPRDescriptions` per affected task.
- `syncOpenPRDescriptions` re-reads `pull_request_<repo-name>.md` (or
  generic `pull_request.md`) from `helix-specs`, builds the same
  footer template used at PR creation, and calls
  `GitRepositoryService.UpdatePullRequest` to patch the title + body of
  the matching open PR in that repo.
- New `pullRequestFileChangedForTask` helper detects whether the commit
  touched a `pull_request*.md` directly in the task dir (subdirs like
  `screenshots/` are ignored). 10 unit tests cover its edge cases:
  `git_http_server_test.go::TestPullRequestFileChangedForTask`.
- Scope: only updates PRs in the repo that received the helix-specs push.
  Multi-repo coordination is unchanged — agents already need to push
  `helix-specs` to each repo to keep their copies in sync.
- Behaviour unchanged when `pull_request_*.md` is missing or unparseable:
  we leave the existing PR body alone rather than overwriting with a
  task-name fallback.

## Why bundle these together

The kanban-scroll PR is the only artifact a reviewer of this spec task is
expecting to see, and the platform fix exists *because* this PR was the
first to surface the bug end-to-end. Splitting them out as a separate
follow-up PR would just add review overhead without changing the diff.

## Screenshots (kanban change)

![Kanban after restore on empty board](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krav8qpvfrj1tj8a6r5jmz9z/screenshots/02-kanban-after-restore.png)

![Kanban restored with backlog scrolled](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krav8qpvfrj1tj8a6r5jmz9z/screenshots/03-kanban-restored-with-tasks.png)

## Test plan

**Kanban scroll restoration** — verified end-to-end in the inner Helix:
- [x] Scroll horizontal + vertical on a populated board, click away, click
  back — both positions restored.
- [x] Live-save: scroll, navigate, scroll again before navigating back —
  latest value is restored.
- [x] Clamp: scroll to bottom of a column, archive most of its tasks,
  navigate away and back — scrollTop lands at the new max without errors.
- [x] `npx tsc --noEmit` is clean.

**Backend PR-description sync**:
- [x] Unit tests for `pullRequestFileChangedForTask` (10 cases) pass.
- [x] `go build ./api/pkg/services/` is clean.
- [ ] CI green.
