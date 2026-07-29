# Fix empty documents in shared spec task design doc links

## Summary

Shared spec-task links opened to a page with three **empty** document sections.
The previous fix (#2878, `002278-fix-public-share-link`) corrected the link's
*URL* so it stopped forcing an OIDC login, but the page it lands on was reading
the wrong data source, so the content was still blank.

The public viewer rendered from the `SpecTask` columns
`requirements_spec / technical_design / implementation_plan`. Those columns are
only ever written by `HandleSpecGenerationComplete`, a legacy spec-generation
entry point that **has no callers**. The live pipeline writes design-doc content
to the `SpecTaskDesignReview` record on every git push
(`git_http_server.go:1866`) — which is exactly what the authenticated in-app
review view reads, and why that view always worked.

This points the public viewer at the same source of truth, so the shared page
and the in-app view can no longer disagree.

## Changes

- `viewDesignDocsPublic` now resolves content from the current design review —
  latest **non-superseded**, matching what `createDesignReviewForPush` picks
  when it writes. (`reviews[0]` alone is wrong: a superseded review can sort
  first under `created_at DESC`.)
- Added a git-backfill fallback that reuses the existing
  `backfillDesignReviewFromGit`, mirroring the authenticated `listDesignReviews`
  self-healing path — both viewers now behave identically.
- Added an empty-content guard: when no content exists anywhere, render an
  explicit "Design documents not available yet" page (404) instead of silently
  blank sections.
- Task-level metadata (name, status, original prompt, updated-at) still comes
  from the task row; the private-task and "specs not yet generated" guards are
  unchanged.
- New `PublicShareViewerSuite` (6 tests) covering render-from-review,
  superseded-review preference, the empty guard, the private page, and the
  status guard.

Deliberately **not** done here: the dead `HandleSpecGenerationComplete` and the
now-unused `SpecTask` doc columns are left in place — those columns are still
read by the clone path (`spec_task_clone_handlers.go:199`) and the agent get-tool
(`spec_task_get_tool.go:158`), so removing them is a wider change. Flagged as a
follow-up.

## Testing

- `CGO_ENABLED=1 go test -run TestPublicShareViewerSuite ./pkg/server/` — 6/6 pass.
- `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` — clean.
- **End-to-end** against the running stack (real API + Postgres, request made
  unauthenticated), with a task seeded in the exact production state — task doc
  columns empty, content only on the review:

  | Scenario | Result |
  |---|---|
  | Public task, content on review | 200, all three docs render |
  | Superseded review sorting first | Stale content not shown; current wins |
  | Public task, no content anywhere | 404 + "not available yet" page |
  | `public_design_docs = false` | 200 + private-task page |
  | Status `backlog` | 404 "specifications not yet generated" |

- The bug was **reproduced and then re-fixed on the same data**: temporarily
  restoring the pre-fix handler reproduced the three empty sections; restoring
  the fix brought the content back. Both screenshots below were taken in an
  isolated, logged-out browser context.

## Screenshots

Before — the reported bug (headers render, all three bodies empty):

![Before](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002334_we-recently-improved-or/screenshots/01-share-link-before-fix.png)

After — same task, same data, content renders:

![After](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002334_we-recently-improved-or/screenshots/02-share-link-after-fix.png)
