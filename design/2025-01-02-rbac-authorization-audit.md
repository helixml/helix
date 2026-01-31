# RBAC Authorization Audit

**Date:** 2025-01-02
**Status:** Review Complete - Fixes Needed

## Executive Summary

An audit of the Helix API authorization patterns reveals systemic issues with organization-aware RBAC checks in repository and spec task handlers. The core problem is a broken authorization pattern where org membership checks and owner checks are performed sequentially without proper branching, causing org members who aren't owners to be incorrectly rejected.

## Issues Found

### Critical: Git Repository Handlers (9 handlers)

**File:** `api/pkg/server/git_repository_handlers.go`

**Broken Pattern:**
```go
// WRONG: These checks run sequentially, not as alternatives
if repository.OrganizationID != "" {
    _, err := s.authorizeOrgMember(r.Context(), user, repository.OrganizationID)
    if err != nil {
        return 403  // Org membership check
    }
}

if repository.OwnerID != user.ID {
    return 403  // ALWAYS runs, even for valid org members!
}
```

**Affected Handlers:**
1. `getGitRepository` (line 116-127)
2. `updateGitRepository` (line 171-182)
3. `deleteGitRepository` (line 223-234)
4. `createOrUpdateGitRepositoryFileContents` (line 822-833)
5. `pushPullGitRepository` (line 929-940)
6. `listGitRepositoryCommits` (line 1013-1024)
7. `createGitRepositoryBranch` (line 1096-1107)
8. `listGitRepositoryPullRequests` (line 1176-1187)
9. `createGitRepositoryPullRequest` (line 1232-1243)

**Fix:** Create `authorizeUserToRepository` function (similar to `authorizeUserToProject`) that:
1. For personal repos: check owner only
2. For org repos: check org membership → owner check → org owner check → RBAC

### Critical: Spec Task Workflow Handlers (3 handlers)

**Files:**
- `api/pkg/server/spec_task_workflow_handlers.go`
- `api/pkg/server/spec_task_implementation_handlers.go`

**Issues:**

1. **`approveImplementation`** - Uses manual ownership check for personal projects but calls `authorizeUserToResource` directly for org projects (bypasses owner/org-owner checks)

2. **`stopAgentSession`** - Same issue as above

3. **`startImplementation`** - Calls `authorizeUserToResource` with **empty string** for orgID:
   ```go
   s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, ...)
   ```
   This will never find any access grants since they're all tied to an org.

**Fix:** Use `authorizeUserToProject` or `authorizeUserToProjectByID` instead.

### Medium: Project Handler Repository Attachment

**File:** `api/pkg/server/project_handlers.go`

**Issue (line 627):**
```go
// Only checks owner, not org membership
if repo.OwnerID != user.ID {
    return nil, system.NewHTTPError404("repository not found")
}
```

When attaching a repository to a project, only owner is checked. For org repos, org members with RBAC grants should also be able to attach.

**Fix:** Use `authorizeUserToRepository` function.

### OK: Handlers with Correct Patterns

The following use proper authorization patterns and are NOT broken:

1. **`authz.go`** - Core authorization functions are correct:
   - `authorizeUserToApp` - proper owner → org membership → org owner → RBAC chain
   - `authorizeUserToProject` - proper owner → org membership → org owner → RBAC chain

2. **`spec_driven_task_handlers.go`** - Uses `authorizeUserToProjectByID` consistently

3. **`project_handlers.go`** (most handlers) - Uses `authorizeUserToProject` correctly

4. **`app_handlers.go`** (most handlers) - Uses `authorizeUserToApp` correctly

5. **`llm_calls_handlers.go`** - Properly branches with `if/else` for org vs personal:
   ```go
   if app.OrganizationID != "" {
       _, err := s.authorizeOrgMember(...)
   } else {
       if app.Owner != user.ID && !isAdmin(user) {
           return 403
       }
   }
   ```

### OK: User-Level Resources (No Org Association)

These resources don't have organization association, so simple ownership checks are correct:

1. **SSH Keys** (`ssh_key_handlers.go`) - `key.UserID != user.ID`
2. **Agent Work Items** (`agent_dashboard_handlers.go`) - `workItem.UserID != user.ID`
3. **OAuth Connections** (`oauth.go`) - `connection.UserID != user.ID` (with admin bypass)

## Recommended Fixes

### 1. Create `authorizeUserToRepository` Function

Add to `authz.go`:
```go
func (apiServer *HelixAPIServer) authorizeUserToRepository(
    ctx context.Context,
    user *types.User,
    repository *types.GitRepository,
    action types.Action,
) error {
    // Personal repos: owner only
    if repository.OrganizationID == "" {
        if user.ID == repository.OwnerID {
            return nil
        }
        return fmt.Errorf("user is not the owner of the repository")
    }

    // Org repos: membership required
    orgMembership, err := apiServer.authorizeOrgMember(ctx, user, repository.OrganizationID)
    if err != nil {
        return err
    }

    // Owner with membership: allowed
    if user.ID == repository.OwnerID {
        return nil
    }

    // Org owner: allowed
    if orgMembership.Role == types.OrganizationRoleOwner {
        return nil
    }

    // Fall back to RBAC
    return apiServer.authorizeUserToResource(
        ctx, user, repository.OrganizationID, repository.ID,
        types.ResourceGitRepository, action,
    )
}
```

### 2. Fix All Git Repository Handlers

Replace all 9 broken patterns with:
```go
if err := s.authorizeUserToRepository(r.Context(), user, repository, types.ActionXxx); err != nil {
    writeErrResponse(w, err, http.StatusForbidden)
    return
}
```

### 3. Fix Spec Task Workflow Handlers

Replace direct `authorizeUserToResource` calls with `authorizeUserToProject`:
```go
// Instead of manual checks + authorizeUserToResource
if err := s.authorizeUserToProject(ctx, user, project, types.ActionUpdate); err != nil {
    http.Error(w, "Not authorized", http.StatusForbidden)
    return
}
```

### 4. Fix Repository Attachment in Project Handlers

Use `authorizeUserToRepository` for repo ownership check when attaching.

## Testing Recommendations

After fixes, test the following scenarios:

1. **Personal repo/project:** Owner can access, others cannot
2. **Org repo/project with owner:** Owner (who is org member) can access
3. **Org repo/project with org owner:** Org owner can access
4. **Org repo/project with RBAC grant:** User with explicit grant can access
5. **Org repo/project without access:** Org member without grant cannot access (unless org owner)

## Priority

1. **P0 (Critical):** Git repository handlers - users currently cannot access org repos they have RBAC grants for
2. **P0 (Critical):** Spec task workflow handlers - users cannot approve implementations in org projects
3. **P1 (High):** Repository attachment - org members cannot attach org repos to projects
