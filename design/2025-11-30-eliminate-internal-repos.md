# Eliminate Internal Repos - Store Config in CODE Repo

**Date:** 2025-11-30
**Status:** Design
**Branch:** feat/sandbox-disk-monitoring (will need new branch)

## Overview

Currently, each Helix project creates a separate `-internal` bare git repo to store:
- Startup script at `.helix/startup.sh`
- Project config at `.helix/project.json` (not currently read from anywhere)

This design proposes eliminating the internal repo entirely and storing this configuration directly in the project's primary CODE repository at `.helix/`.

## Current Architecture

### Internal Repo Creation

1. **Project Creation** (`project_handlers.go:188-240`):
   - Calls `projectInternalRepoService.InitializeProjectRepo()`
   - Creates bare git repo at `{filestore}/projects/{project-id}/repo`
   - Initializes with `.helix/startup.sh` and `.helix/project.json`
   - Creates GitRepository entry with `id: {project-id}-internal`, `repo_type: internal`
   - Stores path in `Project.InternalRepoPath`

2. **Sample Project Fork** (`simple_sample_projects.go:766-830`):
   - Same pattern: creates internal repo + code repo(s)
   - Internal repo ID: `{project-id}-internal`
   - Code repo created separately with sample files

### Startup Script Flow

1. **Storage** (`project_internal_repo_service.go:212-261`):
   - `SaveStartupScript()`: Clones bare repo, writes `.helix/startup.sh`, commits, pushes
   - `LoadStartupScript()`: Opens bare repo, reads from git tree (no clone needed)

2. **Loading at Agent Startup** (`start-zed-helix.sh:260-408`):
   - Internal repo cloned to `~/work/.helix-project/`
   - Pulls latest changes before running
   - Executes `.helix-project/.helix/startup.sh`

3. **Frontend Display** (`ProjectSettings.tsx`):
   - Loads `project.startup_script` from API (transient field)
   - Updates via `updateProjectMutation`
   - Separates repos: `internalRepo = repos.find(r => r.id.endsWith('-internal'))`

### Code Locations

| File | References |
|------|------------|
| `api/pkg/types/project.go:28` | `InternalRepoPath` field |
| `api/pkg/types/git_repositories.go:11` | `GitRepositoryTypeInternal` constant |
| `api/pkg/server/project_handlers.go` | Creates internal repo, loads/saves startup script |
| `api/pkg/server/simple_sample_projects.go` | Forks create internal repos |
| `api/pkg/services/project_internal_repo_service.go` | All internal repo operations |
| `wolf/sway-config/start-zed-helix.sh:101-103` | Clones internal repo to `.helix-project` |
| `frontend/src/pages/ProjectSettings.tsx:88-89` | Filters out internal repo from display |
| `frontend/src/components/project/ProjectRepositoriesList.tsx` | Handles internal repo display |

## Proposed Changes

### 1. Require Primary Repository on Project Creation

**UX Change:**
- Project creation MUST select a primary CODE repository
- Cannot create a project without a repo attached
- Sample projects already work this way (create code repo automatically)

**Frontend Changes:**
- `ProjectSettings.tsx`: Remove ability to create project without primary repo
- Add validation: project must have `default_repo_id` set
- New project dialog: require repo selection/creation

### 2. Store Config in Primary Repo at `.helix/`

**File Structure in Code Repo:**
```
{code-repo}/
├── .helix/
│   ├── startup.sh       # Startup script (executable)
│   └── project.yml      # Project config (future use)
├── src/
└── ...
```

**API Changes:**
- `SaveStartupScript()`: Write to primary code repo instead of internal repo
- `LoadStartupScript()`: Read from primary code repo
- `GetStartupScriptHistory()`: Get git history from primary repo

### 3. Remove Internal Repo Creation

**Backend Changes:**
- Remove `InitializeProjectRepo()` calls from `createProject()` and `forkSimpleProject()`
- Remove internal repo GitRepository entry creation
- Remove `InternalRepoPath` updates

**Database Migration:**
- `InternalRepoPath` field can be deprecated (leave for backwards compatibility)
- Don't delete existing internal repos (they still work)

### 4. Update Agent Startup Script

**`start-zed-helix.sh` Changes:**
- Don't look for `.helix-project/.helix/startup.sh`
- Instead, look for startup script in primary repo: `$PRIMARY_REPO_DIR/.helix/startup.sh`
- This is simpler: no separate clone needed

### 5. Migrate Existing Projects (Optional)

For existing projects with internal repos:
- On first access, copy `.helix/startup.sh` from internal to primary repo
- Update project to use primary repo for storage
- Keep internal repo for history but don't write to it

## Implementation Plan

### Phase 1: Backend Changes

1. **New startup script service methods** (`project_internal_repo_service.go`):
   - `LoadStartupScriptFromCodeRepo(projectID string) (string, error)`
   - `SaveStartupScriptToCodeRepo(projectID, script string) error`
   - These use the primary code repo, not internal repo

2. **Update project handlers** (`project_handlers.go`):
   - `getProject()`: Load startup script from primary code repo
   - `updateProject()`: Save startup script to primary code repo
   - `createProject()`: Validate `default_repo_id` is set, skip internal repo creation

3. **Update sample project fork** (`simple_sample_projects.go`):
   - Skip internal repo creation
   - Write startup script to code repo's `.helix/startup.sh`

### Phase 2: Agent Startup Changes

1. **Update `start-zed-helix.sh`**:
   - Remove `.helix-project` cloning for internal repo
   - Look for startup script in primary repo: `$PRIMARY_REPO_DIR/.helix/startup.sh`

### Phase 3: Frontend Changes

1. **Project creation validation**:
   - Require primary repository selection
   - Cannot save project without repo

2. **Remove internal repo filtering**:
   - `ProjectSettings.tsx`: No longer need to filter out `-internal` repos
   - No need to show internal repo section

### Phase 4: Cleanup (Future)

1. Deprecate `InternalRepoPath` field (keep for backwards compat)
2. Consider migration tool to copy startup scripts to code repos
3. Eventually remove internal repo code paths

## Edge Cases

### Empty/New Projects
- If creating a new project without code:
  - Must create a code repo first (even if empty)
  - Sample projects handle this correctly already

### Projects Without Primary Repo
- Existing projects without `default_repo_id`:
  - Continue using internal repo (backwards compat)
  - Show UI prompt to select a primary repo

### Git Commits for Config
- Startup script changes will create commits in the code repo
- User can see history in git log
- Consider: should these be in a separate branch?

## Benefits

1. **Simpler architecture**: No separate internal repo to manage
2. **One fewer git clone** at agent startup
3. **Config lives with code**: `.helix/` directory is natural for project config
4. **Better visibility**: Users can see/edit config in their IDE
5. **Version control**: Changes to startup script are in code repo history

## Risks

1. **Backwards compatibility**: Existing projects with internal repos
   - Mitigated: Keep `InternalRepoPath` field, fall back if present

2. **Code repo pollution**: `.helix/` directory in user's code
   - Mitigated: Small footprint, `.gitignore` if user wants

3. **Permission model**: Startup script access = code repo access
   - This is already true since both are cloned to same workspace

## Open Questions

1. Should `.helix/project.yml` be YAML or JSON?
   - YAML is more human-readable
   - JSON is already used in internal repo

2. Should we use a separate branch for `.helix/` config?
   - Pro: Doesn't pollute main branch history
   - Con: More complex to manage

3. How to handle projects created from sample projects?
   - Currently: Sample startup script → internal repo → cloned to workspace
   - Proposed: Sample startup script → code repo `.helix/startup.sh`
