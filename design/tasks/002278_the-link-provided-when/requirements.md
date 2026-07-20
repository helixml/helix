# Requirements: Fix Public Share Link for Spec Task Design Docs

## Problem

The "Share Design Docs" link copied from a spec task's details panel is a broken URL.
Even after toggling **Share Design Docs** on and copying the link, opening it forces an
OIDC login instead of showing the docs publicly.

**Root cause (verified in code):** the copied link is
`${window.location.origin}/spec-tasks/${taskId}/view`
(`frontend/src/components/tasks/SpecTaskDetailContent.tsx:405`). It is missing the
`/api/v1` prefix, so the browser hits the **React SPA** at a route that does not exist in
`router.tsx`. The SPA loads, the client-side auth guard
(`frontend/src/contexts/account.tsx:416`) sees no logged-in user, and redirects to
`/login` (OIDC).

A fully-working **unauthenticated** viewer already exists at
`GET /api/v1/spec-tasks/{id}/view` (`api/pkg/server/spec_task_share_handlers.go:14`,
registered on the unauthenticated `subRouter` at `server.go:1711`). It self-gates on
`task.PublicDesignDocs`, renders the requirements / technical design / implementation plan
as a styled, mobile-friendly HTML page, and requires no login. The only bug is that the
frontend never points people at it.

The review page's top-right share button has the same class of bug: it builds
`/design-doc/{specTaskId}/{reviewId}` (`DesignReviewContent.tsx:399`), a real SPA route
that also forces login (its route name is not in the public allowlist and its data API is
auth-gated).

## User Stories

### 1. Share link that actually works unauthenticated
As a spec task owner, when I turn on public sharing and copy the link, I want anyone —
logged in or not — to open it and see the design docs, so I can share specs like a Google
Doc without forcing recipients to sign in.

**Acceptance criteria**
1. With **Share Design Docs** ON, the copied/shown link points to the working
   unauthenticated endpoint `${origin}/api/v1/spec-tasks/${taskId}/view`.
2. Opening that link in a fresh browser session (no cookie/token) renders the design docs
   and does **not** redirect to OIDC login.
3. With sharing OFF, opening the link shows the existing "This spec task is private" page
   (already implemented by `renderPrivateTaskPage`) — not a broken page or a login loop.
4. The top-right share control on the review page produces the same working URL (no more
   `/design-doc/...` link that forces login).

### 2. Google-Docs-style share dialog
As a spec task owner, when I click **Share**, I want a small share dialog/popover (like
Google Docs / Google Maps) that shows the link and lets me toggle public access, so
sharing is obvious and one click.

**Acceptance criteria**
1. A **Share** button on the spec task details panel opens a share dialog/popover.
2. The dialog contains a toggle labelled so it clearly means "anyone on the internet with
   the link can view" (drives the existing `public_design_docs` flag).
3. When the toggle is ON, the dialog shows the share URL in a read-only field that is
   clickable (opens the docs in a new tab) and has a one-click **Copy** button that
   confirms the copy.
4. When the toggle is OFF, the URL is hidden or clearly disabled, with a short note that
   only people with project access can view.
5. Toggling persists to the backend (the existing `public_design_docs` update) and the UI
   reflects the saved state.

## Scope

- **In scope:** correcting the share URL in the frontend to the working
  `/api/v1/spec-tasks/{id}/view` endpoint; a share dialog/popover UX in the spec task
  details panel; making the review-page share button use the same working URL.
- **Out of scope:** changing the backend viewer (it already works); adding an unguessable
  share token; making the React `DesignDocPage` (`/design-doc/...`) public; per-person
  invites/roles.

## Notes / Learnings

- Backend sharing model: a plain boolean `PublicDesignDocs`
  (`api/pkg/types/simple_spec_task.go:226`); the URL uses the raw task ID — there is no
  share token/slug. This matches the user's "anyone with the link" ask.
- `subRouter` = unauthenticated (extract user if present, no `requireUser`); `authRouter`
  adds `requireUser`. Public routes are chosen by which subrouter, not a per-route flag.
- The toggle currently persists via raw `api.put('/api/v1/spec-tasks/${taskId}', ...)`
  (`SpecTaskDetailContent.tsx:413`), which violates the repo rule "always use the generated
  API client." Worth switching while we're here.
- Reusable pieces: `frontend/src/components/common/CopyButton.tsx` (clipboard + confirm).
  No `DarkDialog` component exists — use a standard MUI `Dialog`/`Popover`.
- `SpecTaskReviewPanel.tsx:56` has the same broken URL but does not appear to be mounted by
  any active page (likely dead code) — fix or delete for consistency.

## Open Questions

1. **Which viewer should the link open?** This spec recommends the existing
   server-rendered `/api/v1/spec-tasks/{id}/view` page (works today, zero backend change).
   The alternative — making the richer React `DesignDocPage` public — needs a new public
   API endpoint plus allowlisting the route, and is deliberately out of scope. Confirm the
   server-rendered page is acceptable as the shared experience.
2. **Share token?** Access is by raw task ID with no unguessable token. The request says
   "anyone with the link," so a token seems unnecessary — confirm you don't want one.
3. **Share button placement:** the details panel currently has an inline toggle + copy row
   (lines 1679–1733). Should the new Share **button** live at the top-right of the details
   panel (matching "copy the URL from the top right"), replacing the inline row? Assumed
   yes.
4. Should `SpecTaskReviewPanel.tsx` (apparently unused) be fixed or deleted? Assumed fixed
   or left; deletion needs your ok.
