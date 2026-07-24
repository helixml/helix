# Requirements: Fix Empty Documents in Shared Spec Task Design Doc Links

## Background

A spec task can be shared via a public link so that people without a Helix
login can read its design documents (requirements, technical design,
implementation plan). A recent change (`002278-fix-public-share-link`)
repointed the share link at the server-rendered, unauthenticated viewer
(`GET /api/v1/spec-tasks/{id}/view`) so it no longer forces an OIDC login.

That change fixed the *auth/routing* problem, but the shared page now renders
with **empty document sections** — the header, badge and original prompt show,
but the Requirements Specification, Technical Design and Implementation Plan
bodies are blank.

## Root Cause (confirmed)

The public viewer renders document content from the **`SpecTask` DB columns**
`RequirementsSpec`, `TechnicalDesign`, `ImplementationPlan`:

- `api/pkg/server/spec_task_share_handlers.go:71` (`renderDesignDocsPage`) reads
  `task.RequirementsSpec` / `task.TechnicalDesign` / `task.ImplementationPlan`.

Those columns are only ever written by `HandleSpecGenerationComplete`
(`api/pkg/services/spec_driven_task_service.go:1124`), which has **no callers**
— it is dead code from an older spec-generation flow.

In the current pipeline the design-doc content lives in the `helix-specs` git
repo and is mirrored onto the **`SpecTaskDesignReview`** record on every push
(`api/pkg/services/git_http_server.go:1866`, fields `RequirementsSpec`,
`TechnicalDesign`, `ImplementationPlan`). The authenticated in-app review UI
reads from that review record via `listDesignReviews`/`getDesignReview`.

So `task.RequirementsSpec` etc. are empty strings for every task created by the
current flow, and the public viewer renders three empty sections.

## User Stories

### US-1: Shared link shows the actual documents
**As** someone given a public spec-task share link,
**I want** the design documents to render with their real content,
**so that** I can read the requirements, design and plan without a login.

**Acceptance Criteria:**
- Opening `GET /api/v1/spec-tasks/{id}/view` for a task with
  `public_design_docs = true` renders the current Requirements, Technical
  Design and Implementation Plan content (not blank sections).
- The content matches what an authenticated user sees in the in-app design
  review for the same task.
- If the task has design docs but no populated content is available anywhere,
  the page fails gracefully (clear message), not silent blank sections.

### US-2: Private / not-yet-generated tasks unchanged
**As** the task owner,
**I want** existing behaviour preserved for private tasks and tasks without
generated specs,
**so that** the fix does not leak content or break the current guard rails.

**Acceptance Criteria:**
- A task with `public_design_docs = false` still shows the private-task page.
- A task in `backlog`/`spec_generation` still returns "specifications not yet
  generated".
- Task name, status, original request and last-updated still render as today.

## Out of Scope

- Redesign of the public viewer HTML/styling.
- The share dialog / link-generation frontend (already handled by 002278).
- Reworking how design docs are stored (git + design-review record stays the
  source of truth).

## Open Questions

1. **Which review record is authoritative?** `git_http_server.go` updates the
   first *non-superseded* review, while `GetLatestDesignReview` returns the most
   recent by `created_at`. Assumption: render the latest non-superseded review
   (falling back to latest). Is that the intended "current" version to share?
2. **Remove dead code?** `HandleSpecGenerationComplete` and the task-level
   `RequirementsSpec/TechnicalDesign/ImplementationPlan` columns appear unused
   for content. Assumption: leave the columns in place (other code reads them,
   e.g. clone/get-tool paths) but stop relying on them for the public viewer.
   Should the dead `HandleSpecGenerationComplete` be deleted in this task, or
   left for a separate cleanup?
3. **Git fallback:** if no design-review record exists but docs are in git,
   should the viewer read directly from git (as the backfill path does) as a
   fallback, or is the review record guaranteed to exist whenever
   `design_docs_pushed_at` is set?
