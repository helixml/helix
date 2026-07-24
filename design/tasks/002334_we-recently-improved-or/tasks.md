# Implementation Tasks: Fix Empty Documents in Shared Spec Task Design Doc Links

- [ ] In `api/pkg/server/spec_task_share_handlers.go`, change `viewDesignDocsPublic` to fetch the current design-review record for the task (prefer latest non-superseded, fall back to latest via `store.ListSpecTaskDesignReviews`).
- [ ] Update `renderDesignDocsPage` to render `review.RequirementsSpec / review.TechnicalDesign / review.ImplementationPlan` through blackfriday instead of the empty `task.*` columns; keep `TaskName/Status/OriginalPrompt/UpdatedAt` from the task.
- [ ] Add a git-backfill fallback: when no review row (or empty content) exists but `task.DesignDocsPushedAt` is set, reuse `backfillDesignReviewFromGit` before rendering.
- [ ] Add an empty-content guard: if all three docs are still empty after resolving the review + backfill, render a clear "documents not available yet" message rather than blank sections.
- [ ] Add Go unit tests in `api/pkg/server` (gomock store): public task with populated review renders content; empty review + git triggers backfill; non-public renders private page; backlog/spec_generation returns 404.
- [ ] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` passes.
- [ ] End-to-end verify in inner Helix (`localhost:8080`): create task, generate + push docs, toggle public, open `/api/v1/spec-tasks/{id}/view` unauthenticated, confirm the three sections show real content matching the in-app review.
- [ ] (Optional / separate PR) Flag dead `HandleSpecGenerationComplete` and the unused `SpecTask` doc columns for cleanup — confirm with the user first.
