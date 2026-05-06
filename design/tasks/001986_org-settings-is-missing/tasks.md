# Implementation Tasks

- [x] In `frontend/src/components/orgs/OrgSidebar.tsx`, add `Plug` to the existing `lucide-react` import on line 3.
- [x] In the same file, insert a new sidebar item between the `api_keys` and `settings` entries: `{ id: 'providers', label: 'Providers', icon: <Plug size={20} />, isActive: currentRouteName === 'org_providers', onClick: () => handleNavigationClick('org_providers') }`.
- [x] Run `cd frontend && yarn build` and confirm there are no TypeScript errors. (Built inside `helix-frontend-1` container; Vite build succeeds.)
- [~] Manually verify in the browser at `http://localhost:8080`: log in, open any org, confirm the new "Providers" item appears in the sidebar, click it, confirm it navigates to `/orgs/:org_id/providers` and shows as active.
- [ ] Open a PR against `helixml/helix`, link to this spec task, and check Drone CI is green before requesting review.
