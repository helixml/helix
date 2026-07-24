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
