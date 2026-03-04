# Design: Fix Org-less Project URLs

## Overview

Implement a redirect guard for org-less project routes (`/projects/:projectId/*`) that resolves the project's organization and redirects to the correct org-scoped URL (`/org/:orgSlug/projects/:projectId/*`).

## Technical Approach

### Option Chosen: Route-Level Redirect Component

Add a `ProjectOrgRedirect` wrapper component that intercepts org-less project routes before rendering the actual page component.

### Why This Approach

1. **Single point of fix** — handles all `/projects/:projectId/*` routes in one place
2. **Uses existing patterns** — leverages `useGetProject` and `useGetOrgById` hooks already in codebase
3. **Clean separation** — redirect logic doesn't pollute individual page components
4. **Minimal changes** — only modifies `router.tsx` to wrap the 6 affected routes

### Architecture

```
User visits /projects/prj_xxx/settings
            ↓
    ProjectOrgRedirect component
            ↓
    Fetch project via useGetProject(projectId)
            ↓
    Extract organization_id from project
            ↓
    Fetch org via useGetOrgById(organization_id)
            ↓
    Redirect to /org/{org.name}/projects/prj_xxx/settings
            (or homepage if project/org not found)
```

## Implementation Details

### New Component: `ProjectOrgRedirect.tsx`

Location: `frontend/src/components/routing/ProjectOrgRedirect.tsx`

```typescript
// Pseudocode
function ProjectOrgRedirect({ children }) {
  const { params, name } = useRouter()
  const projectId = params.id
  
  // If already on org-prefixed route, render children
  if (name.startsWith('org_')) return children
  
  // Fetch project to get organization_id
  const { data: project, isLoading, error } = useGetProject(projectId)
  
  // Wait for loading
  if (isLoading) return <LoadingSpinner />
  
  // No project found → redirect to homepage
  if (error || !project) {
    navigateReplace('projects')
    return null
  }
  
  // Personal project (no org) → render as-is
  if (!project.organization_id) return children
  
  // Fetch org to get slug
  const { data: org } = useGetOrgById(project.organization_id)
  
  // Redirect to org-scoped route
  if (org?.name) {
    const orgRouteName = `org_${name}`
    navigateReplace(orgRouteName, { ...params, org_id: org.name })
    return null
  }
  
  return children
}
```

### Router Changes

Wrap the 6 org-less project routes in `getOrgRoutes()` with `ProjectOrgRedirect`:

- `project-specs` → `/projects/:id/specs`
- `project-task-detail` → `/projects/:id/tasks/:taskId`
- `project-task-review` → `/projects/:id/tasks/:taskId/review/:reviewId`
- `project-team-desktop` → `/projects/:id/desktop/:sessionId`
- `project-settings` → `/projects/:id/settings`
- `project-session` → `/projects/:id/session/:session_id`

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| Project not found (404) | Redirect to `/` (projects list) |
| Org lookup fails | Redirect to `/` |
| Personal project (no org_id) | Render page normally (no redirect needed) |
| Already on org route | Pass through to children |
| Network error during fetch | Show loading briefly, then redirect to `/` on error |

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Infinite redirect loop | Check route name before redirecting; only redirect from non-org routes |
| Flash of loading state | Use small spinner, fetches are typically <100ms |
| Breaking existing navigation | Only affects routes without `/org/` prefix; org-prefixed routes unchanged |

## Files to Modify

1. **Create**: `frontend/src/components/routing/ProjectOrgRedirect.tsx`
2. **Modify**: `frontend/src/router.tsx` — wrap project routes with redirect component
3. **Modify**: `frontend/src/services/index.ts` — export `useGetOrgById` if not already exported