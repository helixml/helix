# Implementation Tasks

## Setup

- [ ] Create new directory `frontend/src/components/routing/`

## Core Implementation

- [ ] Create `ProjectOrgRedirect.tsx` component with:
  - [ ] Hook into `useRouter()` to get current route name and params
  - [ ] Extract `projectId` from `params.id`
  - [ ] Early return children if route name starts with `org_`
  - [ ] Use `useGetProject(projectId)` to fetch project data
  - [ ] Handle loading state with minimal spinner
  - [ ] Handle error/not-found by redirecting to homepage
  - [ ] Check `project.organization_id` — if null, render children (personal project)
  - [ ] Use `useGetOrgById(organization_id)` to fetch org data
  - [ ] Build org-scoped route name (`org_${routeName}`) and redirect with `navigateReplace`

## Router Integration

- [ ] Modify `router.tsx` — import `ProjectOrgRedirect` component
- [ ] Wrap these routes' render functions with `<ProjectOrgRedirect>`:
  - [ ] `project-specs`
  - [ ] `project-task-detail`
  - [ ] `project-task-review`
  - [ ] `project-team-desktop`
  - [ ] `project-settings`
  - [ ] `project-session`

## Exports

- [ ] Ensure `useGetOrgById` is exported from `services/index.ts` (check if already exported)

## Testing

- [ ] Manual test: Visit `/projects/prj_xxx/settings` → should redirect to `/org/orgname/projects/prj_xxx/settings`
- [ ] Manual test: Visit `/projects/prj_xxx/specs` → should redirect correctly
- [ ] Manual test: Visit `/projects/invalid-id/settings` → should redirect to homepage
- [ ] Manual test: Visit `/org/helix/projects/prj_xxx/settings` → should NOT redirect (already correct)
- [ ] Manual test: Visit personal project URL (no org) → should render without redirect
- [ ] Verify no infinite redirect loops occur
- [ ] Verify agents list loads after redirect
- [ ] Verify settings save works after redirect

## Cleanup

- [ ] Run `yarn build` in frontend to verify no TypeScript errors
- [ ] Remove any console.log statements used during development