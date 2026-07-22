# Implementation Tasks: Fix Public Share Link for Spec Task Design Docs

- [x] Fix the share URL in `SpecTaskDetailContent.tsx:405` to
      `${window.location.origin}/api/v1/spec-tasks/${taskId}/view` (add the `/api/v1` prefix)
- [x] Fix the review-page share URL in `DesignReviewContent.tsx:399` (`handleShareLink`) to
      the same `/api/v1/spec-tasks/${specTaskId}/view` endpoint
- [x] Fix the duplicate broken URL in `SpecTaskReviewPanel.tsx:56` (unused component; fixed
      URL rather than deleting without explicit approval)
- [~] Add a **Share** button to the top-right of the spec task details panel that opens a
      share dialog/popover
- [~] Build the share dialog (MUI `Dialog`) with: public-access toggle ("Anyone
      with the link can view"), and when ON a clickable link field + copy button
- [~] Wire the toggle to the existing `public_design_docs` update; hide/disable the link
      when sharing is OFF
- [~] Replace the raw `api.put('/api/v1/spec-tasks/${taskId}', ...)` toggle call with the
      generated API client method (repo convention)
- [~] Remove the now-redundant inline toggle + copy row (`SpecTaskDetailContent.tsx`
      1679–1733) once the dialog replaces it
- [ ] Test end-to-end in the inner Helix: toggle ON → open copied link logged-out (no OIDC
      redirect, docs render); toggle OFF → private page shown
- [ ] `cd frontend && yarn build` to confirm the frontend compiles
