# Implementation Tasks: Restore Projects as First Nav Item and Default Landing Page

- [ ] In `frontend/src/components/orgs/UserOrgSelector.tsx`, move the Projects entry (icon `Kanban`, label "Projects", `onClick: handleProjectsClick`) to the first position of the `baseButtons` array, above the alpha-gated Org Chart block.
- [ ] Update the Org Chart comment in the same file — it says "Leads the rail as the primary org-level surface", which is no longer accurate; reword to note it sits under Projects.
- [ ] Verify the resulting rail order is Projects → Org Chart (alpha) → Agents → Chat → Tasks → Sandbox → Settings, with no other entries added or removed.
- [ ] In `frontend/src/utils/organizations.ts`, make `orgLandingRoute()` return `'org_projects'` unconditionally and update its doc comment; keep the `user` parameter so the four call sites stay unchanged.
- [ ] In `UserOrgSelector.tsx`, replace the inline `account.user?.alpha_features?.includes('helix-org') ?? false` with `isHelixOrgEnabled(account.user)` so the helper does not become dead code (add the import alongside the existing `orgLandingRoute` import).
- [ ] Confirm no remaining reference points `helix_org_chart` as a landing destination: `grep -rn "helix_org_chart" frontend/src` should only show the rail button, the `helix_org_root` redirect, and route definitions.
- [ ] Run the frontend typecheck/lint (`yarn tsc --noEmit` or the repo's configured task) and confirm it passes.
- [ ] Manual check: app loads with Projects as the top rail icon; `/` with a stored org lands on Projects; `/` with no stored org still redirects to `/orgs`.
- [ ] Manual check as a `helix-org` alpha user: org switch, org create, and org-card select all land on Projects; the Org Chart icon still renders and navigates correctly.
- [ ] Optional: add a unit test for `orgLandingRoute()` asserting `'org_projects'` for both alpha and non-alpha users.
