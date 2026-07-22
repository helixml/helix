# Fix spec task design-docs share link (works unauthenticated) + share dialog

## Summary

The "Share Design Docs" link copied from a spec task forced an OIDC login and never showed
the docs. Root cause: the copied URL was `${origin}/spec-tasks/${taskId}/view` — **missing
the `/api/v1` prefix** — so it hit the React SPA at a nonexistent route and the client-side
auth guard redirected anonymous users to login. A fully-working, unauthenticated,
server-rendered viewer already existed at `GET /api/v1/spec-tasks/{id}/view` (self-gated on
`public_design_docs`); the frontend just never pointed at it.

This PR is frontend-only — no backend/API changes were needed.

## Changes

- **Fix the share URL** to `${origin}/api/v1/spec-tasks/${taskId}/view` in all three
  builders: `SpecTaskDetailContent.tsx`, `DesignReviewContent.tsx` (review-page top-right
  share button, which previously built a `/design-doc/...` link that also forced login),
  and the unused `SpecTaskReviewPanel.tsx`.
- **New Google-Docs-style share dialog** (`SpecTaskShareDialog.tsx`): a public-access
  toggle ("Anyone with the link" / "Restricted") plus, when public, a read-only URL field,
  an open-in-new-tab link, and a copy button.
- **Two triggers**, both gated on `design_docs_pushed_at` and opening the same dialog: a
  "Share" button in the details info column (replacing the old inline toggle/copy row) and a
  Share icon in the details top-right toolbar.
- **Use the generated API client** (`v1SpecTasksUpdate`) for the public toggle instead of a
  raw `api.put`, per repo convention. Removed the now-unused `copyPublicLink` helper and
  `Copy` import.

## Verification

Tested end-to-end in the inner Helix:

- Unauthenticated `curl` and a fresh no-cookie browser context both load
  `/api/v1/spec-tasks/{id}/view` → HTTP 200, full docs, no redirect.
- The old prefix-less URL returns the SPA shell (reproducing the login-redirect bug).
- With sharing off, the endpoint returns the "This spec task is private" page.
- The dialog toggle persists `public_design_docs` to the DB via the generated client;
  ON→OFF→ON all behave correctly.
- `yarn tsc` passes clean; `vite build` transforms all modules.

## Screenshots

![Details view with Share button](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002278_the-link-provided-when/screenshots/01-task-detail-share-button.png)
![Share dialog — public](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002278_the-link-provided-when/screenshots/02-share-dialog-public.png)
![Share dialog — restricted](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002278_the-link-provided-when/screenshots/03-share-dialog-restricted.png)
![Public view, unauthenticated](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002278_the-link-provided-when/screenshots/04-public-view-unauthenticated.png)
