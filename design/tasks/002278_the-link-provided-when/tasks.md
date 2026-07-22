# Implementation Tasks: Fix Public Share Link for Spec Task Design Docs

- [x] Fix the share URL in `SpecTaskDetailContent.tsx:405` to
      `${window.location.origin}/api/v1/spec-tasks/${taskId}/view` (add the `/api/v1` prefix)
- [x] Fix the review-page share URL in `DesignReviewContent.tsx:399` (`handleShareLink`) to
      the same `/api/v1/spec-tasks/${specTaskId}/view` endpoint
- [x] Fix the duplicate broken URL in `SpecTaskReviewPanel.tsx:56` (unused component; fixed
      URL rather than deleting without explicit approval)
- [x] Add a **Share** button to the top-right toolbar of the spec task details panel (and
      an info-column Share button) that opens the share dialog
- [x] Build the share dialog (`SpecTaskShareDialog.tsx`, MUI `Dialog`) with: public-access
      toggle ("Anyone with the link"), and when ON a clickable link field + open + copy
- [x] Wire the toggle to the existing `public_design_docs` update; hide the link when
      sharing is OFF
- [x] Replace the raw `api.put('/api/v1/spec-tasks/${taskId}', ...)` toggle call with the
      generated client `v1SpecTasksUpdate`
- [x] Remove the now-redundant inline toggle + copy row (replaced by the Share button/dialog)
- [x] Typecheck + build: `yarn tsc` passes clean; `vite build` transforms all modules (final
      write to root-owned `dist/` is env-blocked, not a code error)
- [x] Test end-to-end in the inner Helix: registered/onboarded, seeded a public spec task,
      opened the share URL in a fresh no-cookie browser context → docs render, NO OIDC
      redirect; toggled OFF → "This spec task is private" page; dialog toggle persists to
      the DB via the generated client. Screenshots in `screenshots/`.
