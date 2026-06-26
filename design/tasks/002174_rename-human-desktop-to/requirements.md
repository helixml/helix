# Requirements: Rename "Human Desktop" to "Project Desktop"

## Background

The Helix UI refers to the per-project exploratory chat/desktop session as the
**"Human Desktop"** — the shared desktop where a person explores the codebase and
tests the app, as opposed to the per-task **"Agent Desktop"** driven by an AI agent.

The term "Human Desktop" is being renamed to **"Project Desktop"** to better
describe what it is (one desktop per project) and read more naturally to users.

This is a **user-facing label rename**. No functional identifiers change — the
route name (`org_project-team-desktop`), component (`TeamDesktopPage`), and API
fields stay as-is. Only the displayed text changes.

## User Stories

### US-1: Consistent "Project Desktop" naming in the UI
As a Helix user, I want every place that currently says "Human Desktop" to say
"Project Desktop" instead, so the product uses one consistent, clear term.

**Acceptance Criteria:**
- The page title / breadcrumb for the desktop page reads "Project Desktop".
- The browser/router title for the desktop route reads "Project Desktop".
- Buttons in the spec tasks page read "Open Project Desktop", "Resume Project
  Desktop", and "View Project Desktop".
- Snackbar/toast messages read "Project Desktop started", "Project Desktop
  resumed", "Failed to start Project Desktop", "Failed to resume Project Desktop".
- Tab titles and the desktop list item in the workspace tabs view default to
  "Project Desktop".
- The kanban board tooltip explaining the desktop concept refers to "Project
  Desktop".
- The org worker detail page copy referring to the session uses "Project Desktop".
- The error thrown when a worker desktop session fails to open reads "failed to
  open Project Desktop session".

### US-2: No leftover "Human Desktop" strings
As a maintainer, I want no user-facing "Human Desktop" string to remain, so the
rename is complete and searchable.

**Acceptance Criteria:**
- `grep -rn "Human Desktop"` over `frontend/src` returns no user-facing string
  literals (only, optionally, internal code comments — see below).
- The frontend builds cleanly (`yarn build`).

## Out of Scope / Decisions

- **Functional identifiers unchanged.** Route names, component names, API field
  names, query param names (e.g. `desktopTitle`, `exploratorySessionId`) are NOT
  renamed — they are not user-visible and renaming them risks breaking links.
- **Backend code comments and Go test names** that mention "Human Desktop" (e.g.
  in `api/pkg/server/*_test.go`, `api/pkg/org/...`) are internal documentation.
  Updating them is OPTIONAL consistency cleanup; doing so keeps the codebase
  coherent but is not required for the user-facing feature. Recommended: update
  comments, leave test function names alone unless trivial.
- No database, API, or behavioural changes.
