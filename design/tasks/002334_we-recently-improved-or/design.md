# Design: Fix Empty Documents in Shared Spec Task Design Doc Links

## Overview

Change the public, server-rendered design-docs viewer to read document content
from the **`SpecTaskDesignReview`** record (the same source the authenticated
in-app view uses) instead of the empty `SpecTask` columns. This is a small,
backend-only fix. The frontend, routing and auth toggle from `002278` are
correct and unchanged.

## Root Cause Recap

Two divergent storage locations exist for the design-doc markdown:

| Path | Reads from | Populated by current flow? |
|------|-----------|----------------------------|
| Authenticated in-app review (works) | `SpecTaskDesignReview.{RequirementsSpec,TechnicalDesign,ImplementationPlan}` | ✅ yes — `git_http_server.go:1866` on every git push, plus git backfill |
| Public `/view` share link (empty) | `SpecTask.{RequirementsSpec,TechnicalDesign,ImplementationPlan}` | ❌ no — only `HandleSpecGenerationComplete` (dead, no callers) writes these |

The push pipeline (`createDesignReviewForPush`) writes doc content only to the
design-review record, never back to the task columns, so the public viewer
renders blank sections.

## The Fix

In `api/pkg/server/spec_task_share_handlers.go`, `viewDesignDocsPublic` /
`renderDesignDocsPage`:

1. After confirming the task is public and its specs are generated, **fetch the
   current design-review record** for the task rather than reading the task
   columns.
2. Resolve the review the same way the rest of the system does: prefer the
   latest **non-superseded** review; fall back to the latest review
   (`store.ListSpecTaskDesignReviews` is ordered `created_at DESC`; the git-push
   handler picks the first non-superseded entry — mirror that logic).
3. Render `review.RequirementsSpec / review.TechnicalDesign /
   review.ImplementationPlan` through blackfriday into the existing template
   slots. Keep `TaskName`, `Status`, `OriginalPrompt`, `UpdatedAt` from the task.

### Git backfill fallback (parity with authenticated view)

The authenticated `listDesignReviews` self-heals: when no review row exists it
calls `backfillDesignReviewFromGit` (`spec_task_design_review_handlers.go:1579`)
which `git show helix-specs:<taskDir>/{requirements,design,tasks}.md` and creates
a review row. To guarantee the public viewer never shows empty content when docs
exist in git, apply the same fallback: if no review record (or a review with
empty content) is found but `task.DesignDocsPushedAt` is set, backfill from git,
then render. Reuse the existing backfill helper rather than duplicating the git
read.

### Empty-content guard

If, after resolving the review and attempting git backfill, all three documents
are still empty, render a clear "documents not available yet" message instead of
three blank sections (better UX and easier to diagnose than silent blanks).

## Key Decisions

- **Read from the design-review record, not re-populate the task columns.**
  The review record + git are already the single source of truth used
  everywhere else. Mirroring content back into the `SpecTask` columns would
  reintroduce a second copy that must be kept in sync — exactly the drift that
  caused this bug. (Rejected alternative: make `createDesignReviewForPush` also
  write `task.RequirementsSpec` etc.)
- **Reuse `backfillDesignReviewFromGit`** rather than writing a fresh git read
  in the share handler — keeps the two viewers consistent and DRY.
- **Leave the dead `HandleSpecGenerationComplete` and the task columns in
  place** for this task (they are read by clone/get-tool paths); flag them for a
  separate cleanup. See Open Question 2 in requirements.

## Affected Files

- `api/pkg/server/spec_task_share_handlers.go` — `viewDesignDocsPublic` /
  `renderDesignDocsPage`: source content from the design review + git fallback.
- (Reference, unchanged) `api/pkg/server/spec_task_design_review_handlers.go` —
  `backfillDesignReviewFromGit`, the reusable git-read helper.
- (Reference, unchanged) `api/pkg/store/spec_task_design_review_store.go` —
  `ListSpecTaskDesignReviews` / `GetLatestDesignReview`.

## Implementation Notes

**What was built** (all in `api/pkg/server/spec_task_share_handlers.go`):

- `resolvePublicDesignDocs(ctx, task)` — resolves doc content: reads the current
  design review, and if empty *and* `task.DesignDocsPushedAt != nil`, looks up
  the project → default repo and calls the existing
  `backfillDesignReviewFromGit`, then re-reads.
- `designDocsFromReviews(ctx, taskID)` — picks the review: iterates
  `ListSpecTaskDesignReviews` (ordered `created_at DESC`) and takes the first
  **non-superseded** entry, defaulting to `reviews[0]`.
- `publicDesignDocs` struct + `empty()` helper — small value type so the
  empty-guard is a single readable check.
- `renderDocsUnavailablePage` + `docsUnavailableTemplate` — 404 page shown when
  no content exists anywhere. Styled to match the existing private-task page.

**Answers to the requirements' Open Questions** (resolved during implementation):

1. *Which review is authoritative?* Latest **non-superseded**, matching what
   `createDesignReviewForPush` in `git_http_server.go` picks when it writes. A
   superseded review can legitimately sort first under `created_at DESC`, so
   taking `reviews[0]` blindly would show stale content — covered by a test.
2. *Remove dead code?* Left in place. `HandleSpecGenerationComplete` has no
   callers, but the `SpecTask` doc columns are still read by the clone path
   (`spec_task_clone_handlers.go:199`) and the agent get-tool
   (`spec_task_get_tool.go:158`), so removing the columns is a wider change.
   Flagged as a follow-up rather than bundled here.
3. *Git fallback needed?* Yes, implemented — it mirrors the authenticated
   `listDesignReviews` self-healing path, so both viewers behave identically.

**Gotchas discovered:**

- Go tests in `pkg/server` need CGo for tree-sitter. `gcc` is not installed by
  default in this sandbox: `sudo apt-get install -y gcc libc6-dev`, then
  `CGO_ENABLED=1 go test ...` (documented in CLAUDE.md, easy to miss).
- `docker exec ... psql` does not accept a heredoc on stdin the way you'd
  expect — the SQL silently does nothing. Use `psql -c "..."` per statement.
- `psql -c` with multiple `;`-separated statements only prints the last
  result, which makes seeding look like it failed when it didn't.

## Testing

- **Unit (Go, `pkg/server`)**: with a gomock store, a public task whose latest
  design review has non-empty content renders that content into the HTML (assert
  the response body contains the requirements/design/plan text). A public task
  with an empty/absent review + git docs triggers backfill. Non-public task still
  renders the private page; backlog/spec_generation still returns 404.
- **End-to-end (inner Helix at `localhost:8080`)**: register/onboard, create a
  spec task, let it generate + push design docs, toggle "public", open
  `GET /api/v1/spec-tasks/{id}/view` in a fresh/incognito context (no auth) and
  confirm the three sections show real content matching the in-app review.

### Verification performed (results)

Go unit tests — all 6 pass (`CGO_ENABLED=1 go test -run TestPublicShareViewerSuite
./pkg/server/`). `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` clean.

End-to-end against the running inner Helix (real API + real Postgres, request
made **unauthenticated**), seeding a task in the exact production state — task
doc columns empty (`req_len=0`), content only on the design review:

| Scenario | Result |
|---|---|
| Public task, content on review | HTTP 200, all three docs render as HTML |
| Superseded review sorting first | Stale content NOT shown; current review wins |
| Public task, no content anywhere | HTTP 404 + "Design documents not available yet" |
| `public_design_docs = false` | HTTP 200 + "This spec task is private" |
| Status `backlog` | HTTP 404 "specifications not yet generated" |

The bug was also **reproduced then re-fixed on the same data**: temporarily
restoring the pre-fix handler made the page render three empty sections
(`screenshots/01-share-link-before-fix.png`); restoring the fix brought the
content back (`screenshots/02-share-link-after-fix.png`). Both screenshots were
taken in an isolated, logged-out browser context.
