# Design: Fix Public Share Link for Spec Task Design Docs

## Approach

The fix is almost entirely frontend. A working, unauthenticated, self-gating viewer already
exists on the backend — the frontend just points at the wrong URL. So:

1. **Correct the URL** everywhere it is built: use the working endpoint
   `${window.location.origin}/api/v1/spec-tasks/${taskId}/view` (note the `/api/v1`
   prefix). This alone fixes "invalid URL" + "forces OIDC login."
2. **Redesign the share UX** in the spec task details panel into a Google-Docs-style share
   dialog/popover: a public-access toggle + a clickable, copyable link.

No backend changes are required. The endpoint `GET /api/v1/spec-tasks/{id}/view` already
returns the docs when `PublicDesignDocs` is true and a "private" page otherwise.

## Why the current link fails (mechanism)

- `/spec-tasks/{id}/view` (no `/api/v1`) → matched by the SPA catch-all
  (`router.PathPrefix("/")`) → React app boots → `account.tsx` auth guard sees no `user`
  → `router.navigateReplace('login')` → OIDC. The route isn't even defined in `router.tsx`.
- `/api/v1/spec-tasks/{id}/view` → matched by `subRouter` (unauthenticated) →
  `viewDesignDocsPublic` → server-rendered HTML, no login. This is the target.

## Key Files

Frontend (all under `frontend/src/`):
- `components/tasks/SpecTaskDetailContent.tsx` — **primary change.** Line 405 builds the
  broken `publicLink`; lines 1679–1733 are the inline toggle + copy row to replace with the
  share dialog; lines 392–434 hold the toggle state + `handlePublicToggle` + `copyPublicLink`.
- `components/spec-tasks/DesignReviewContent.tsx` — line 399 `handleShareLink`; repoint to
  the `/api/v1/.../view` URL so the review page's top-right share button works too.
- `components/tasks/SpecTaskReviewPanel.tsx` — line 56 same broken URL (apparently unused;
  fix or delete).
- `components/common/CopyButton.tsx` — reuse for the copy control.

Backend (reference only — no changes expected):
- `api/pkg/server/spec_task_share_handlers.go` — `viewDesignDocsPublic`, private-page and
  docs templates.
- `api/pkg/server/server.go:1711` — route registration on the unauthenticated `subRouter`.

## Share Dialog Design

A `SpecTaskShareDialog` (new component, or an inline MUI `Dialog`/`Popover` in
`SpecTaskDetailContent`). Contents:

- **Title:** "Share design docs".
- **Toggle:** `Switch` bound to `isPublicDesignDocs`; label "Anyone with the link can view"
  + helper text. `onChange` → `handlePublicToggle` (persists `public_design_docs`).
- **When ON:** a read-only text field showing the URL, rendered as a clickable link
  (`<a href={shareUrl} target="_blank" rel="noopener noreferrer">`) plus a `CopyButton`.
- **When OFF:** URL hidden/disabled with note "Only people with access to this project can
  view."
- **Trigger:** a "Share" button at the top-right of the details panel.

```ts
const shareUrl = `${window.location.origin}/api/v1/spec-tasks/${taskId}/view`;
```

## Key Decisions

- **Server-rendered viewer over React viewer.** The existing `/api/v1/.../view` page works
  unauthenticated today and needs zero backend work. Making the React `DesignDocPage`
  (`/design-doc/...`) public would require a new public API endpoint on `subRouter`
  (self-gated on `PublicDesignDocs`) plus adding the route to the `publicRoutes` allowlist
  in `account.tsx` — more surface area and risk for no user-visible gain. Chosen: reuse the
  working page. (Documented as an open question in requirements.md.)
- **No share token.** Access stays by raw task ID gated on the `PublicDesignDocs` boolean,
  matching the "anyone with the link" request. Task IDs are already high-entropy IDs.
- **Use the generated API client** for the toggle PUT instead of raw `api.put`, per repo
  convention (`public_design_docs` already exists on the generated request type). Low-cost
  cleanup while touching this code.

## Implementation Notes (as built)

- **URL fix (the actual bug):** added the missing `/api/v1` prefix in all three builders —
  `SpecTaskDetailContent.tsx`, `DesignReviewContent.tsx` (review page top-right share), and
  the unused `SpecTaskReviewPanel.tsx`. This is what makes the link resolve to the working
  unauthenticated server-rendered viewer instead of the SPA login redirect.
- **Share dialog:** new component `frontend/src/components/tasks/SpecTaskShareDialog.tsx`.
  MUI `Dialog` with a public-access toggle (Globe/Lock icon + "Anyone with the link" /
  "Restricted" copy) and, when public, a read-only URL field + open-in-new-tab + copy
  button (own copied-state, no external CopyButton needed — CopyButton is absolutely
  positioned, which didn't fit an inline row).
- **Triggers:** a `Share` button replaces the old inline toggle/copy row in the info
  column, AND a `Share` icon button in the detail panel's top-right toolbar (next to Clone),
  both gated on `task.design_docs_pushed_at` (docs exist) and both opening the same dialog.
- **Generated client:** the toggle now calls
  `api.getApiClient().v1SpecTasksUpdate(taskId, { public_design_docs })` instead of raw
  `api.put`, per repo convention. `TypesSpecTaskUpdateRequest` already has the field.
- **Kept `SpecTaskReviewPanel.tsx`** (unused/dead) rather than deleting it without explicit
  approval — only fixed its URL. Flagged in requirements Open Question 4.
- Removed the now-unused `copyPublicLink` helper and the `Copy` lucide import from
  `SpecTaskDetailContent.tsx`.

## Verification (done)

Tested end-to-end in the inner Helix (`localhost:8080`):

- **Unauthenticated access works.** `curl` (no auth) and a fresh no-cookie browser context
  both load `GET /api/v1/spec-tasks/{id}/view` → HTTP 200, full docs HTML, zero redirects.
- **Old URL reproduced the bug.** `GET /spec-tasks/{id}/view` (no prefix) returns the SPA
  shell (`<title>Helix</title>`), which is what triggered the client-side OIDC redirect.
- **Private path.** With `public_design_docs=false`, the endpoint returns the "This spec
  task is private" page (HTTP 200, not a login loop).
- **Share dialog.** Opens from both the info-column "Share" button and the top-right
  toolbar Share icon; shows the correct `/api/v1/...` URL, open-in-new-tab link, and copy
  button; toggle OFF hides the link and flips `public_design_docs` to false in the DB via
  the generated client; toggle back ON re-shows the link.

Screenshots: `screenshots/01-task-detail-share-button.png`,
`02-share-dialog-public.png`, `03-share-dialog-restricted.png`,
`04-public-view-unauthenticated.png`.

Note: could not exercise real spec-generation (needs the agent/sandbox); used a seeded
spec task row with generated spec fields + `design_docs_pushed_at` set (test row deleted
after verification).

## Risks / Gotchas

- The share link is an API URL, so clicking it navigates the whole tab to server-rendered
  HTML (not an SPA route). That is intended and matches the existing public viewer.
- Confirm `PublicDesignDocs` is actually persisted/migrated (the toggle already works today,
  so it is), and that flipping OFF immediately makes `/view` return the private page.
- Verify end-to-end in the inner Helix: register/login, create a spec task with generated
  specs, toggle ON, open the copied URL in an incognito/logged-out context and confirm no
  OIDC redirect; toggle OFF and confirm the private page.
