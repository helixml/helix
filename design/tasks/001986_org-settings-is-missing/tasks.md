# Implementation Tasks

- [x] In `frontend/src/components/orgs/OrgSidebar.tsx`, add `Plug` to the existing `lucide-react` import on line 3.
- [x] In the same file, insert a new sidebar item between the `api_keys` and `settings` entries: `{ id: 'providers', label: 'Providers', icon: <Plug size={20} />, isActive: currentRouteName === 'org_providers', onClick: () => handleNavigationClick('org_providers') }`.
- [x] Run `cd frontend && yarn build` and confirm there are no TypeScript errors. (Built inside `helix-frontend-1` container; Vite build succeeds.)
- [x] Manually verify in the browser at `http://localhost:8080`: log in, open any org, confirm the new "Providers" item appears in the sidebar, click it, confirm it navigates to `/orgs/:org_id/providers` and shows as active. (Verified — sidebar shows People / Teams / Billing / API Keys / Providers / Settings; clicking "Providers" navigates to `/orgs/test-org/providers` and renders the Providers page. Screenshots saved.)
- [x] Push the feature branch to origin so the Helix platform can open the PR. (PR creation is handled by the platform when the user clicks "Open PR" in the UI — agent must NOT run `gh pr create`.)
