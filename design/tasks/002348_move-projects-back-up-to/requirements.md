# Requirements: Restore Projects as First Nav Item and Default Landing Page

## Background

Two earlier changes demoted Projects in the Helix frontend:

1. `8bf905644` — *"feat(org): reorder org nav rail, rename Org->Org Chart, drop Providers tab"* (2026-07-02)
   moved **Projects** from position 1 down to position 4 in the left icon rail
   (`frontend/src/components/orgs/UserOrgSelector.tsx`). Current order is
   Org Chart → Agents → Chat → Projects → Tasks → Sandbox → Settings.
2. `7320af3e6` / `344b02e74` (2026-07-06) introduced `orgLandingRoute()` in
   `frontend/src/utils/organizations.ts`, which lands `helix-org` alpha users on
   the **Org Chart** instead of Projects after login, org creation, or org switch.

The user wants Projects back as the top nav item and back as the default home page.

## User Stories

### US-1: Projects is the first item in the left nav rail
**As a** Helix user
**I want** Projects to be the first icon in the left navigation rail
**So that** the surface I use most is where I expect it, at the top.

Acceptance criteria:
- [ ] In the left icon rail, **Projects** renders above every other navigation item.
- [ ] The resulting order is: Projects → Org Chart (only when the `helix-org` alpha
      feature is enabled) → Agents → Chat → Tasks → Sandbox → Settings (only when an
      org is selected).
- [ ] Projects keeps its existing icon (`Kanban`), tooltip ("View projects"), click
      handler (`handleProjectsClick` → `org_projects`) and active-state matching
      (`spec-tasks`, `projects`, `project`).
- [ ] Nothing else about the rail changes: "Org Chart" keeps its name, Providers stays
      off the rail, Q&A stays in the Settings sub-nav, Files stays commented out.

### US-2: Projects is the default landing page
**As a** Helix user
**I want** to land on Projects after logging in, selecting an org, creating an org, or
visiting `/`
**So that** the default home page is Projects again, regardless of alpha flags.

Acceptance criteria:
- [ ] `orgLandingRoute()` returns `org_projects` for **all** users, including users with
      the `helix-org` alpha feature.
- [ ] Visiting `/` with a stored org in `localStorage.selected_org` lands on
      `org_projects` (already true — must stay true).
- [ ] Visiting `/` with no stored org still redirects to the `orgs` picker (unchanged).
- [ ] Switching orgs via the org dropdown lands on Projects for that org.
- [ ] Selecting an org from the orgs table lands on Projects for that org.
- [ ] Creating a new org lands on Projects for the new org.
- [ ] Auto-select-first-org on fresh login lands on Projects, unless the user is already
      on an `org_*` route (existing behaviour: current route is preserved).

### US-3: Org Chart remains reachable
**As a** `helix-org` alpha user
**I want** the Org Chart to still be one click away
**So that** demoting it from the landing page does not hide it.

Acceptance criteria:
- [ ] The Org Chart rail button is still rendered for `helix-org` alpha users and still
      navigates to `helix_org_chart`.
- [ ] Its active state still highlights for any `helix_org*` route.
- [ ] The `helix_org_root` → `helix_org_chart` redirect in `router.tsx` is unchanged.

## Out of Scope

- Reverting the "Org" → "Org Chart" rename.
- Restoring Providers to the rail or Q&A as a top-level rail entry.
- Changing the onboarding completion flow (which navigates to `project-specs` for the
  newly created project, not to Projects).
- Backend/API changes — this is frontend-only.

## Open Questions

1. **Org Chart position** — the spec assumes Projects goes to position 1 and Org Chart
   slots in at position 2 (i.e. exactly the pre-`8bf905644` layout, minus the rename).
   Alternative reading: Projects first, Org Chart left where it is relative to the
   others (after Chat). Assumed the former; say if you want Org Chart lower.
2. **Alpha users and the landing page** — the spec makes `orgLandingRoute()` return
   `org_projects` unconditionally, which removes the only remaining caller of
   `isHelixOrgEnabled()`. Assumed you want the alpha branch gone entirely rather than
   flipped behind a new preference. If Org Chart should stay the landing page for some
   subset of users, that changes the design.
3. **`isHelixOrgEnabled()` retention** — the design keeps the helper and reuses it in
   `UserOrgSelector` (replacing an inline `alpha_features.includes('helix-org')` check)
   so it does not become dead code. Deleting it instead is also fine — confirm the
   preference.
