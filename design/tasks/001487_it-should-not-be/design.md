# Design: Prevent Personal Agents on Organization Projects

## Context

Every `App` (agent) has an `organization_id` field. Org-scoped agents have this set to the org's ID; personal agents (a leftover from the removed "personal org" feature) have it empty. Projects always belong to an organization. When a personal agent is attached to an org project, downstream operations fail silently because provider resolution, authorization checks, and config lookups are scoped to the org but the agent has no org context.

## Key Codebase Findings

### How agents are listed

- **Frontend**: `AppsContext` (`frontend/src/contexts/apps.tsx`) fetches `/api/v1/apps?organization_id=<org_id>`. When the org param is set, the backend calls `listOrganizationApps()` which queries `WHERE organization_id = ?`. This correctly returns only org agents.
- **Backend**: `listApps` in `api/pkg/server/app_handlers.go` has two paths — org-filtered (correct) and personal+global (the no-org path). The frontend always passes the org ID when in an org context, so the listing itself isn't the bug.

### Where the bug lives

The bug is in the **write path** — no validation on project create/update:

- `updateProject` (`api/pkg/server/project_handlers.go` ~L498–502): blindly sets `DefaultHelixAppID`, `ProjectManagerHelixAppID`, `PullRequestReviewerHelixAppID` without checking the app's org matches the project's org.
- `createProject` (~L257–261): fetches the app but only to check it exists, not that its `organization_id` matches.
- Spec task create/update: `helix_app_id` is set without org validation.

The frontend `AgentDropdown` component (`frontend/src/components/agent/AgentDropdown.tsx`) renders whatever `agents` array it receives — it does no filtering itself. The `agents` prop comes from `AppsContext.apps`, which *should* only contain org agents. But there's a timing/state issue: the `AppsContext` can contain stale personal apps if the user navigated from a non-org context, and the `sortedApps` memo in `ProjectSettings.tsx` doesn't filter by org — it only sorts by agent type.

## Solution

### Backend: Validate org match on project write

Add a helper function in `api/pkg/server/project_handlers.go`:

```go
func (s *HelixAPIServer) validateAppBelongsToOrg(ctx context.Context, appID, orgID string) error {
    app, err := s.Store.GetApp(ctx, appID)
    if err != nil {
        return fmt.Errorf("agent not found: %w", err)
    }
    if app.OrganizationID != orgID {
        return fmt.Errorf("agent %s does not belong to organization %s", appID, orgID)
    }
    return nil
}
```

Call this in:
1. `createProject` — validate `req.DefaultHelixAppID` against `req.OrganizationID`
2. `updateProject` — validate `DefaultHelixAppID`, `ProjectManagerHelixAppID`, and `PullRequestReviewerHelixAppID` (when non-empty) against `project.OrganizationID`
3. `createSpecTask` / `updateSpecTask` — validate `HelixAppID` (when non-empty) against the parent project's `OrganizationID`

Return HTTP 400 with a message like: `"agent app_01xxx does not belong to this organization"`.

### Frontend: Filter agents in dropdowns

In `ProjectSettings.tsx`, the `sortedApps` memo should filter out any app whose `organization_id` doesn't match the project's org. This is a defense-in-depth measure (the context *should* already be org-scoped, but this prevents edge cases):

```typescript
const sortedApps = useMemo(() => {
  if (!apps || !project?.organization_id) return [];
  const orgApps = apps.filter(app => app.organization_id === project.organization_id);
  // ... existing zed_external sorting logic on orgApps ...
}, [apps, project?.organization_id]);
```

Apply the same filter in `SpecTaskDetailContent.tsx`.

### Frontend: Show warning for invalid existing assignments

When the currently selected `default_helix_app_id` doesn't match any agent in the filtered list and is non-empty, show an inline warning: *"Current agent is not available in this organization. Please select a new agent."*

This uses existing data — no new API call needed. Just check `selectedAgentId && !sortedApps.find(a => a.id === selectedAgentId)`.

## What We're NOT Doing

- **No data migration**: We don't bulk-fix existing project→personal-agent references. Users see a warning and can pick a valid agent.
- **No changes to `listApps` API**: The endpoint already filters correctly by org when the param is provided.
- **No removal of personal apps from the data model**: They still exist for non-project use cases (chat, etc.). We just prevent them from being attached to org projects.
- **No new API endpoints**: All changes are validation logic in existing handlers and frontend filtering.

## Risks

- **Spec tasks with personal agents already in-progress**: If a running spec task references a personal agent, this change won't retroactively break it further (it's already broken). The task can be updated to a valid agent.
- **API consumers**: External API users who are setting personal app IDs on org projects will start getting 400 errors. This is correct behavior — they were getting silent failures before.