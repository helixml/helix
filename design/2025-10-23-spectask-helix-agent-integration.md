# SpecTask + Helix Agent Integration Design

**Date**: 2025-10-23
**Status**: Implementation In Progress
**Authors**: Claude Code
**Last Updated**: 2025-10-23 17:30 UTC

## Implementation Progress

### Completed
- ✅ **Database Schema** - Added new columns to SpecTask, created new tables
  - `SpecTask.HelixAppID` - Single agent for entire workflow
  - `SpecTask.AttachedRepositories` - Multiple git repo attachments (JSONB)
  - `SpecTask.PrimaryRepositoryID` - Primary repo for helix-design-docs
  - `SpecTask.ExternalAgentID` - Links to external agent
  - `spec_task_external_agents` table - Per-SpecTask agent tracking
  - `external_agent_activity` table - Idle detection per-agent
  - GORM hooks for JSON serialization
  - Tables registered in AutoMigrate

- ✅ **Startup Script** - Complete state persistence symlinks
  - `~/.config/zed` → `work/.zed-state/config`
  - `~/.local/share/zed` → `work/.zed-state/local-share`
  - `~/.cache/zed` → `work/.zed-state/cache`
  - `~/.claude` → `work/.claude-state`
  - File: `wolf/sway-config/start-zed-helix.sh`

- ✅ **SpecTaskOrchestrator** - Added Wolf executor integration
  - Added `WolfExecutorInterface` to orchestrator struct
  - Updated constructor to accept wolf executor
  - Updated `server.go` to pass wolf executor

### In Progress
- None currently

### TODO (Remaining Steps)
- [ ] Implement `handleBacklog()` with Wolf executor and multi-repo cloning
- [ ] Implement `buildPlanningPrompt()` with helix-design-docs worktree setup
- [ ] Implement `handleImplementationQueued()` with external agent reuse
- [ ] Add store methods for SpecTaskExternalAgent CRUD
- [ ] Add store methods for ExternalAgentActivity tracking
- [ ] Implement idle cleanup loop in Wolf executor
- [ ] Frontend: Simplified creation form
- [ ] Frontend: Repository dropdown and multi-attach
- [ ] Frontend: Helix Agent selection
- [ ] Frontend: Idle warning banners
- [ ] Frontend: Basic git repository UI

---

> **Note**: Throughout this document, we refer to "Helix Agents" - these are configured AI agents with specific capabilities. In the codebase, they are still often called "Apps" (the `App` type in `api/pkg/types/types.go`), as they were recently renamed from "Helix Apps" to "Helix Agents". The terms are used interchangeably in code.

## Executive Summary

This design refactors SpecTask triggers to use Helix Agents (configured AI agents with specific tools, knowledge, and models) instead of the old NATS infrastructure. Each SpecTask creates a **single long-lived external agent** (Wolf container with Zed) that spans multiple phases through Zed's multi-threading capability.

**Key architectural decisions**:
1. **Wolf apps** (not Wolf lobbies) for session management
2. **Per-SpecTask workspace** (not per-session) - shared across planning, implementation, testing phases
3. **Single external agent per SpecTask** - hosts multiple Zed threads (one per Helix session)
4. **Zed thread ↔ Helix session 1:1 mapping** - each phase creates new thread in SAME Zed instance
5. **Forward-only helix-design-docs branch** - pushed to Helix git server for UI sync
6. **Complete state persistence** - symlink all Zed/Claude state to workspace
7. **30-minute idle cleanup** - terminates external agent (frees GPU), workspace persists

**Complexity justification**: We need per-SpecTask workspaces because:
- Planning phase writes to helix-design-docs worktree
- Implementation phase must read those same design docs
- All phases share git repositories
- Zed threads (planning + implementation) need to be visible together

## Current State Analysis

### SpecTask Architecture

**Current workflow:**
1. User creates SpecTask with name, description, original prompt
2. SpecTask has fields for `SpecAgent` and `ImplementationAgent` (strings)
3. SpecTask orchestrator manages workflow through phases:
   - `backlog` → `spec_generation` → `spec_review` → `spec_approved` → `implementation_queued` → `implementation`
4. Trigger infrastructure uses NATS to launch external agents

**Current trigger implementation** (`api/pkg/trigger/agent_work_queue/agent_work_queue_trigger.go`):
- Creates `AgentWorkItem` from trigger payload
- Calls `controller.CreateSession()` to create Helix session
- Calls `controller.LaunchExternalAgent()` to start agent via NATS
- Uses `AgentType` string (e.g., "zed", "vscode")

### Helix Agent Architecture

**Helix Agents** (represented as `App` type in `api/pkg/types/types.go`):
- Configured AI agents with specific capabilities
- Have `AppConfig` with assistants, tools, knowledge sources
- Currently used for PDEs (Personal Dev Environments)
- Can be used for SpecTask planning and implementation phases

**Wolf Executor** (`api/pkg/external-agent/wolf_executor.go`):
- **Uses Wolf apps** for external agent sessions (current stable approach)
- Creates Wolf app per session, containers managed by Wolf
- Containers run Zed in Sway compositor
- Support both PDEs and External Agent sessions
- Note: We are NOT using Wolf lobbies for this implementation (apps are what's working)

### Problems with Current Approach

1. **NATS Dependency**: Trigger system uses NATS to launch agents, but we should use Wolf executor directly
2. **String-based Agent Types**: SpecTask stores agent types as strings, not references to configured Helix Agents
3. **No Agent Configuration**: Can't specify which tools, knowledge, or settings the agent should use
4. **Missing Default Agent**: No mechanism to create default "Zed" Helix Agent if none exists
5. **Resource Leaks**: External agent sessions run indefinitely, consuming GPU resources
6. **No Idle Detection**: No tracking of when sessions are inactive
7. **No Cleanup Warnings**: Users don't know when their sessions will be terminated

## Proposed Architecture

### Phase 1: Helix Agent Selection for SpecTasks

#### Database Schema Changes

Update `SpecTask` struct to simplify creation and use Helix Agent IDs:

```go
type SpecTask struct {
    ID          string `json:"id" gorm:"primaryKey"`
    ProjectID   string `json:"project_id" gorm:"index"`

    // SIMPLIFIED: Single field for all task context
    // User dumps everything they know here - AI extracts requirements/design/plan
    OriginalPrompt string `json:"original_prompt" gorm:"type:text"`

    // AI-generated artifacts (created during planning phase)
    Name                string `json:"name"`                                     // AI extracts from prompt
    Description         string `json:"description" gorm:"type:text"`            // AI extracts from prompt
    Type                string `json:"type"`                                     // AI infers: "feature", "bug", "refactor"
    RequirementsSpec    string `json:"requirements_spec" gorm:"type:text"`      // AI generates
    TechnicalDesign     string `json:"technical_design" gorm:"type:text"`       // AI generates
    ImplementationPlan  string `json:"implementation_plan" gorm:"type:text"`    // AI generates

    Priority    string `json:"priority"` // User selects: "low", "medium", "high", "critical"
    Status      string `json:"status"`   // Workflow status

    // OLD (remove these):
    // SpecAgent               string
    // ImplementationAgent     string

    // NEW: Reference to single Helix Agent (App type in code) for entire workflow
    HelixAppID string `json:"helix_app_id,omitempty" gorm:"size:255;index"`

    // Git repository attachments (MULTIPLE repos can be attached)
    AttachedRepositories datatypes.JSON `json:"attached_repositories,omitempty" gorm:"type:jsonb"`
    // Primary repository for design docs (first in attached list, or explicitly set)
    PrimaryRepositoryID string `json:"primary_repository_id,omitempty" gorm:"size:255;index"`

    // Track active sessions for each phase (same agent, different sessions)
    PlanningSessionID        string `json:"planning_session_id,omitempty" gorm:"size:255;index"`
    ImplementationSessionID  string `json:"implementation_session_id,omitempty" gorm:"size:255;index"`

    // ... existing fields (CreatedBy, CreatedAt, etc.) ...
}
```

**Key Schema Changes**:
1. `OriginalPrompt` is the ONLY user-provided field (big text dump)
2. `Name`, `Description`, `Type` are AI-extracted during planning phase
3. Priority is still user-selected (defaults to "medium")
4. Type field kept for filtering/display but NOT user input
5. **Single `HelixAppID`** - same agent for planning AND implementation (simplified)
6. **Multiple repository attachments** - `AttachedRepositories` JSON array
7. **Primary repository** - `PrimaryRepositoryID` for design docs (first attachment by default)

#### Frontend Changes

**Simplified SpecTask Creation Form**:

The form should be dramatically simplified to reduce friction:

```
┌─────────────────────────────────────────────────────────────┐
│  New SpecTask                    [From Example Tasks ▼]    │
├─────────────────────────────────────────────────────────────┤
│                                              Priority: [High ▼] │
│  ┌───────────────────────────────────────────────────────┐ │
│  │ Describe what you want to get done                    │ │
│  │                                                        │ │
│  │ Dump everything you know here - the AI will parse     │ │
│  │ this into requirements, design, and implementation     │ │
│  │ plan.                                                  │ │
│  │                                                        │ │
│  │ Examples:                                              │ │
│  │ - "Add dark mode toggle to settings page"            │ │
│  │ - "Fix the user registration bug where emails aren't  │ │
│  │    validated properly"                                │ │
│  │ - "Refactor the payment processing to use Stripe      │ │
│  │    instead of PayPal"                                 │ │
│  │                                                        │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                             │
│  Primary Repository: [my-project ▼]          [New Repo]    │
│  Additional Repos: [+ Add Repository]                      │
│    └─ [my-backend ▼] [Remove]                              │
│    └─ [shared-library ▼] [Remove]                          │
│                                                             │
│  Agent: [Zed Agent ▼]                                      │
│                                                             │
│                                     [Cancel]  [Create Task] │
└─────────────────────────────────────────────────────────────┘
```

**Key Changes**:
1. **Single text field** for task description (no separate name/description/requirements)
2. **Priority in top right** (High/Medium/Low dropdown)
3. **No "Type" field** - AI infers from description (feature/bug/refactor)
4. **"From Example Tasks" dropdown** - moved to top-level button next to "New SpecTask"
5. **Multiple repository attachments** - Select primary + additional Helix-hosted repos
6. **New Repo button** - Create new repository inline if none exist
7. **Single agent selection** - Same agent used for both planning and implementation
8. **Auto-create default agent** if none exist

**"From Example Tasks" Dropdown** (separate from creation form):
- Moved out of the main creation flow
- Becomes a separate button: `[New SpecTask]  [From Example Tasks ▼]`
- Selecting an example pre-fills the description field with example text
- Keeps the actual work creation clean and uncluttered

**Default "Zed" Helix Agent**:
```json
{
  "name": "Zed Agent",
  "description": "Default Zed editor for autonomous coding",
  "config": {
    "helix": {
      "assistants": [{
        "name": "Zed Agent",
        "system_prompt": "You are an autonomous coding agent with access to Zed editor. Work methodically through tasks, make atomic commits, and ask for help when blocked.",
        "model": "claude-sonnet-4"
      }]
    }
  }
}
```

### Phase 2: Wolf Apps-Based Session Triggering

#### Remove NATS Infrastructure

**Old flow (NATS-based)**:
```
SpecTask → agent_work_queue_trigger → controller.LaunchExternalAgent() → NATS → agent runner
```

**New flow (Wolf apps-based)**:
```
SpecTask → SpecTaskOrchestrator → WolfExecutor.StartZedAgent() → Wolf app → Zed container
```

**Note**: We use **Wolf apps**, not Wolf lobbies. Each external agent session gets its own Wolf app created dynamically.

#### SpecTaskOrchestrator Updates

Update `handleImplementationQueued` to use Wolf executor:

```go
func (o *SpecTaskOrchestrator) handleImplementationQueued(ctx context.Context, task *types.SpecTask) error {
    // Get Helix Agent configuration (same agent used for planning)
    app, err := o.store.GetApp(ctx, task.HelixAppID)
    if err != nil {
        return fmt.Errorf("failed to get agent: %w", err)
    }

    // Create Helix session for implementation
    session, err := o.createImplementationSession(ctx, task, app)
    if err != nil {
        return fmt.Errorf("failed to create session: %w", err)
    }

    // Build system prompt from task spec + agent config
    systemPrompt := o.buildImplementationPrompt(task, app)

    // Create Wolf app-based external agent session
    agentReq := &types.ZedAgent{
        SessionID:          session.ID,
        HelixSessionID:     session.ID, // The actual Helix session this agent serves
        UserID:             task.CreatedBy,
        Input:              systemPrompt,
        ProjectPath:        task.ProjectPath,
        WorkDir:            fmt.Sprintf("/workspace/spectasks/%s", task.ID),
        DisplayWidth:       2560,
        DisplayHeight:      1600,
        DisplayRefreshRate: 60,
    }

    // This creates a Wolf app for this session (apps mode, not lobbies)
    agentResp, err := o.wolfExecutor.StartZedAgent(ctx, agentReq)
    if err != nil {
        return fmt.Errorf("failed to start Wolf agent: %w", err)
    }

    // Store session details
    task.ImplementationSessionID = session.ID
    task.Status = types.TaskStatusImplementation
    return o.store.UpdateSpecTask(ctx, task)
}
```

#### System Prompt Construction

Combine SpecTask spec documents with Helix Agent configuration:

```go
func (o *SpecTaskOrchestrator) buildImplementationPrompt(task *types.SpecTask, app *types.App) string {
    basePrompt := app.Config.Helix.Assistants[0].SystemPrompt

    return fmt.Sprintf(`%s

## Current SpecTask Context

**Task**: %s
**Description**: %s

**Requirements Specification**:
%s

**Technical Design**:
%s

**Implementation Plan**:
%s

You are working on this SpecTask. Follow the implementation plan, make atomic commits, and mark tasks complete as you go. Use the work/ directory for persistence across sessions.
`, basePrompt, task.Name, task.Description, task.RequirementsSpec, task.TechnicalDesign, task.ImplementationPlan)
}
```

### Phase 3: Planning Phase Support

Planning phase uses external agents to parse user prompt and generate specs:

```go
func (o *SpecTaskOrchestrator) handleBacklog(ctx context.Context, task *types.SpecTask) error {
    // Get Helix Agent configuration (same agent for entire workflow)
    app, err := o.store.GetApp(ctx, task.HelixAppID)
    if err != nil {
        return fmt.Errorf("failed to get agent: %w", err)
    }

    // Create session + launch agent for spec generation
    session, err := o.createPlanningSession(ctx, task, app)
    if err != nil {
        return fmt.Errorf("failed to create planning session: %w", err)
    }

    // Launch planning agent (creates Wolf app for this session)
    agentReq := &types.ZedAgent{
        SessionID:      session.ID,
        HelixSessionID: session.ID,
        UserID:         task.CreatedBy,
        Input:          o.buildPlanningPrompt(task, app),
        ProjectPath:    task.ProjectPath,
        WorkDir:        fmt.Sprintf("/workspace/spectasks/%s/planning", task.ID),
    }

    agentResp, err := o.wolfExecutor.StartZedAgent(ctx, agentReq)
    if err != nil {
        return fmt.Errorf("failed to start planning agent: %w", err)
    }

    task.PlanningSessionID = session.ID
    task.Status = types.TaskStatusSpecGeneration
    return o.store.UpdateSpecTask(ctx, task)
}

func (o *SpecTaskOrchestrator) buildPlanningPrompt(task *types.SpecTask, app *types.App) string {
    basePrompt := app.Config.Helix.Assistants[0].SystemPrompt

    // Generate task directory name: {YYYY-MM-DD}_{branch-name}_{task_id}
    dateStr := time.Now().Format("2006-01-02")
    branchName := sanitizeForBranchName(task.OriginalPrompt[:min(50, len(task.OriginalPrompt))])
    taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, branchName, task.ID)

    return fmt.Sprintf(`%s

## Task: Generate Specifications from User Request

You are running in a full external agent session with access to Zed editor and a git repository.

**Repository Information**:
- Clone URL: %s
- Work Directory: /home/retro/work/repo
- Design Docs Worktree: /home/retro/work/repo/.git-worktrees/helix-design-docs

The user has provided the following task description:

---
%s
---

**Your job is to:**

1. Clone the repository to /home/retro/work/repo
2. Setup helix-design-docs branch and worktree (if not exists):
   ` + "```bash" + `
   cd /home/retro/work/repo
   git clone %s .
   git branch helix-design-docs 2>/dev/null || true
   git worktree add .git-worktrees/helix-design-docs helix-design-docs
   ` + "```" + `

3. Write design documents in organized task directory structure:
   ` + "```bash" + `
   cd .git-worktrees/helix-design-docs/tasks/%s
   ` + "```" + `

4. Create the following markdown files (following spec-driven development):
   - **requirements.md**: User stories + EARS acceptance criteria
   - **design.md**: Architecture, sequence diagrams, data models, API contracts
   - **tasks.md**: Discrete implementation tasks with [ ]/[~]/[x] markers
   - **task-metadata.json**: Extracted task name, description, type (feature/bug/refactor)

5. Make atomic commits to helix-design-docs branch:
   ` + "```bash" + `
   cd .git-worktrees/helix-design-docs
   git add tasks/%s/requirements.md
   git commit -m "Add requirements specification for %s"
   git add tasks/%s/design.md
   git commit -m "Add technical design for %s"
   git add tasks/%s/tasks.md
   git commit -m "Add implementation plan for %s"
   git add tasks/%s/task-metadata.json
   git commit -m "Add task metadata for %s"
   ` + "```" + `

6. **Push helix-design-docs branch** to Helix git server:
   ` + "```bash" + `
   git push -u origin helix-design-docs
   ` + "```" + `

**CRITICAL**:
- The helix-design-docs branch is **forward-only** and never rolled back
- Push to Helix git server (`http://api:8080/git/{repo_id}`) so UI can read design docs
- This branch survives git operations on main code (branch switches, rebases, etc.)

The user will review these design documents before implementation begins.
`, basePrompt, task.RepositoryCloneURL, task.OriginalPrompt, task.RepositoryCloneURL, taskDirName, taskDirName, task.ID, taskDirName, task.ID, taskDirName, task.ID)
}
```

### Phase 4: Git Repository Integration with helix-design-docs Worktree

> **Reference**: This phase builds on the existing architecture documented in:
> - `design/spectask-orchestrator-architecture.md` - helix-design-docs branch/worktree system
> - `design/spectask-interactive-review-enhancement.md` - Git-based design docs
> - `api/pkg/services/design_docs_worktree_manager.go` - Existing implementation

#### helix-design-docs Branch Architecture (Existing)

The infrastructure is **already implemented**:

**Key Properties**:
- **Branch name**: `helix-design-docs` (forward-only, never rolled back)
- **Worktree location**: `.git-worktrees/helix-design-docs/`
- **Survives branch switches**: Main repo can switch branches, design docs persist
- **Task organization**: `tasks/{YYYY-MM-DD}_{branch-name}_{task_id}/`

**Directory Structure** (existing):
```
/home/retro/work/repo/
├── .git/                              # Main repository
├── src/                               # Working code (can switch branches)
├── .git-worktrees/
│   └── helix-design-docs/            # Worktree for design docs branch
│       ├── README.md                  # Structure documentation
│       ├── tasks/                     # Organized by date + task
│       │   └── 2025-10-23_add-auth_spec_abc123/
│       │       ├── requirements.md    # EARS acceptance criteria
│       │       ├── design.md          # Architecture + diagrams
│       │       ├── tasks.md           # Implementation tasks with [ ]/[~]/[x]
│       │       ├── task-metadata.json # Extracted metadata
│       │       └── sessions/          # Per-session notes
│       └── archive/                   # Completed tasks
```

**Existing Services**:
- `DesignDocsWorktreeManager` - Already handles worktree setup, task parsing, commit tracking
- `GitRepositoryService` - Already handles repository creation and management

#### Existing Git Infrastructure

Helix already has full git repository support:
- **GitRepositoryService** (`api/pkg/services/git_repository_service.go`)
  - Creates and manages git repositories in filestore
  - Supports SpecTask-specific repositories
  - Provides HTTP clone URLs for agent access
  - Full go-git integration for commits, branches, worktrees

- **Git HTTP Server** (`api/pkg/services/git_http_server.go`)
  - Serves repositories over HTTP (accessible from Wolf containers)
  - Clone URL format: `http://api:8080/git/{repo_id}`

- **Repository Types**:
  - `GitRepositoryTypeProject` - User project repositories
  - `GitRepositoryTypeSpecTask` - SpecTask-specific repositories
  - `GitRepositoryTypeSample` - Sample/demo repositories
  - `GitRepositoryTypeTemplate` - Template repositories

#### SpecTask Multiple Repository Attachment

When creating a SpecTask, user can attach **multiple** Helix-hosted repositories:

```go
// AttachedRepository represents a git repository attached to a SpecTask
type AttachedRepository struct {
    RepositoryID string `json:"repository_id"`
    CloneURL     string `json:"clone_url"`
    LocalPath    string `json:"local_path"`    // Where to clone in workspace (e.g., "backend", "frontend")
    IsPrimary    bool   `json:"is_primary"`    // Primary repo hosts helix-design-docs branch
}
```

**Stored in SpecTask**:
```json
{
  "attached_repositories": [
    {
      "repository_id": "project-my-backend-123",
      "clone_url": "http://api:8080/git/project-my-backend-123",
      "local_path": "backend",
      "is_primary": true
    },
    {
      "repository_id": "project-my-frontend-456",
      "clone_url": "http://api:8080/git/project-my-frontend-456",
      "local_path": "frontend",
      "is_primary": false
    },
    {
      "repository_id": "project-shared-lib-789",
      "clone_url": "http://api:8080/git/project-shared-lib-789",
      "local_path": "shared",
      "is_primary": false
    }
  ],
  "primary_repository_id": "project-my-backend-123"
}
```

**Creation Flow**:
1. User selects primary repository (required)
2. User optionally adds additional repositories
3. Specifies local path for each repo (where to clone in workspace)
4. Primary repo hosts the helix-design-docs branch
5. External agent sessions automatically clone ALL attached repos into workspace

**Dynamic Repository Management**:
- User can add/remove repositories at any phase
- Updates `AttachedRepositories` JSON array
- Next agent session picks up new repository configuration
- Useful when work expands to additional codebases

**SpecTask Detail View** includes repository management:
```
Attached Repositories
├── my-backend (primary) [Edit] [Remove]
├── my-frontend [Edit] [Remove]
└── [+ Add Repository]
```

#### Planning Phase Git Workflow (Using Existing Architecture)

**Planning agent responsibilities**:
1. Clone repository from Helix git server: `git clone http://api:8080/git/{repo_id} /home/retro/work/repo`
2. Setup helix-design-docs branch and worktree (uses existing `DesignDocsWorktreeManager`)
3. Create task directory: `.git-worktrees/helix-design-docs/tasks/{YYYY-MM-DD}_{branch-name}_{task_id}/`
4. Write markdown files following existing template structure:
   - `requirements.md` - User stories + EARS acceptance criteria (existing template)
   - `design.md` - Architecture, sequence diagrams, data models (existing template)
   - `tasks.md` - Implementation tasks with `[ ]`/`[~]`/`[x]` markers (existing format)
   - `task-metadata.json` - Extracted task name/description/type
5. Commit each file to helix-design-docs branch (forward-only)
6. **Push helix-design-docs branch** to Helix git server: `git push -u origin helix-design-docs`

**CRITICAL**: Pushing to Helix git server enables:
- ✅ Helix UI can read design docs from git (sync mechanism)
- ✅ Design docs visible in frontend without database duplication
- ✅ Version history preserved in git
- ✅ Multiple agents can see same design docs

**User review**:
- Helix UI fetches design documents from git repository (reads helix-design-docs branch)
- User views rendered markdown in UI
- Can comment/request changes via Helix session
- Approves or requests revision

#### Implementation Phase Git Workflow

**Implementation agent responsibilities**:
1. Clone repository from Helix git server (or reuse existing checkout)
2. Fetch latest helix-design-docs branch: `git fetch origin helix-design-docs`
3. Read design documents from `.git-worktrees/helix-design-docs/tasks/{task_dir}/`
4. Create feature branch: `feature/{spectask_id}-{task_name_slug}`
5. Implement according to `tasks.md` checklist
6. Mark tasks in progress/complete in helix-design-docs worktree:
   - Update `tasks.md`: `[ ]` → `[~]` → `[x]`
   - Commit to helix-design-docs branch (progress tracking)
   - **Push progress updates**: `git push origin helix-design-docs`
7. Make implementation commits on feature branch
8. Push feature branch to Helix git server
9. Optionally create pull request

**CRITICAL**: Implementation agent pushes progress updates to helix-design-docs so Helix UI can show live task progress!

#### Minimal Git UI (Future Enhancement)

**Top-level "Repositories" section** (new UI area):
```
Repositories
├── List View
│   ├── Repository cards (name, description, last activity)
│   ├── Filter by type (project/spectask/sample)
│   └── Create new repository button
└── Repository Detail View
    ├── README.md display
    ├── File browser (tree view)
    ├── File content viewer (read-only for now)
    ├── Branch selector
    ├── Commit history (basic)
    └── Clone command display
```

**SpecTask Creation Integration**:
- Dropdown: "Attach to Repository"
- Shows list of user's repositories
- Option to create new repository inline
- Displays clone URL once selected

**Implementation Priority**:
- Phase 1: Backend already exists (working git service)
- Phase 2: SpecTask repository attachment (schema + backend)
- Phase 3: Basic repo list UI
- Phase 4: File browser UI (future - nice to have)

For now, agents work with repositories via git commands only (clone/commit/push). UI file browsing is a future enhancement.

### Phase 5: Multi-Session External Agent Architecture

**ARCHITECTURAL DECISION**: Lean into the complexity of agents that span multiple Helix sessions.

#### Why Single External Agent Per SpecTask

**Problem we can't avoid**:
- Workspace must be per-SpecTask (not per-session)
- Planning writes helix-design-docs → Implementation reads it
- All phases need same git repositories
- Design docs must be accessible across phases

**Solution**: One external agent (Wolf container) serves entire SpecTask lifecycle

**Architecture**:
```go
type SpecTaskExternalAgent struct {
    ID              string    `json:"id"`                // zed-spectask-{spectask_id}
    SpecTaskID      string    `json:"spec_task_id"`      // Parent SpecTask
    WolfAppID       string    `json:"wolf_app_id"`       // Wolf app managing this agent
    WorkspaceDir    string    `json:"workspace_dir"`     // /workspaces/spectasks/{id}/work/
    HelixSessionIDs []string  `json:"helix_session_ids"` // All sessions using this agent
    ZedThreadIDs    []string  `json:"zed_thread_ids"`    // Zed threads (1:1 with sessions)
    Status          string    `json:"status"`            // running, terminated
    Created         time.Time `json:"created"`
    LastActivity    time.Time `json:"last_activity"`
}
```

#### External Agent Lifecycle

**Creation** (when SpecTask moves to planning phase):
```go
func (o *SpecTaskOrchestrator) handleBacklog(ctx context.Context, task *types.SpecTask) error {
    // Create external agent for this SpecTask
    externalAgent := &SpecTaskExternalAgent{
        ID:           fmt.Sprintf("zed-spectask-%s", task.ID),
        SpecTaskID:   task.ID,
        WorkspaceDir: fmt.Sprintf("/workspaces/spectasks/%s/work", task.ID),
        Status:       "creating",
    }

    // Create Wolf app with per-SpecTask workspace
    agentReq := &types.ZedAgent{
        SessionID:      generateAgentSessionID(task.ID), // Agent-level ID
        WorkDir:        externalAgent.WorkspaceDir,
        ProjectPath:    "backend", // Primary repo
    }

    agentResp, err := o.wolfExecutor.StartZedAgent(ctx, agentReq)
    // Wolf container starts, Zed launches, connects to Helix WebSocket

    externalAgent.WolfAppID = agentResp.WolfAppID
    externalAgent.Status = "running"
    o.store.CreateSpecTaskExternalAgent(ctx, externalAgent)

    // Now create first Helix session for planning phase
    planningSession := o.createPlanningSession(ctx, task)

    // This will trigger WebSocket message to Zed → creates thread #1
    // Maps: ses_planning_001 ←→ zed-thread-001

    externalAgent.HelixSessionIDs = append(externalAgent.HelixSessionIDs, planningSession.ID)
    o.store.UpdateSpecTaskExternalAgent(ctx, externalAgent)
}
```

**Phase Transition** (planning → implementation):
```go
func (o *SpecTaskOrchestrator) handleImplementationQueued(ctx context.Context, task *types.SpecTask) error {
    // Get EXISTING external agent (already running from planning phase!)
    externalAgent, err := o.store.GetSpecTaskExternalAgent(ctx, task.ID)

    if externalAgent.Status != "running" {
        // Agent was terminated, resurrect it
        agentResp, err := o.wolfExecutor.StartZedAgent(ctx, &types.ZedAgent{
            WorkDir: externalAgent.WorkspaceDir, // SAME workspace
        })
        externalAgent.WolfAppID = agentResp.WolfAppID
        externalAgent.Status = "running"
    }

    // Create NEW Helix session for implementation (in SAME Zed instance)
    implSession := o.createImplementationSession(ctx, task)

    // WebSocket message to Zed → creates thread #2 in existing Zed
    // Maps: ses_impl_002 ←→ zed-thread-002
    // Both thread #1 (planning) and thread #2 (implementation) now visible in Zed!

    externalAgent.HelixSessionIDs = append(externalAgent.HelixSessionIDs, implSession.ID)
    o.store.UpdateSpecTaskExternalAgent(ctx, externalAgent)
}
```

**Key Insight**: We're not avoiding complexity - we're **embracing** multi-session agents as a powerful feature!

### Phase 6: Complete Git-Based Workflow

#### SpecTask Lifecycle with Git Integration

```
User Creates SpecTask
    ↓
Selects Repository (or creates new one)
    ↓
Planning Session Starts (External Agent in Wolf app)
    ├─ Agent clones repository
    ├─ Creates branch: design-docs/{spectask_id}
    ├─ Writes markdown files:
    │  ├─ design/requirements.md
    │  ├─ design/technical-design.md
    │  ├─ design/implementation-plan.md
    │  └─ design/task-metadata.json
    ├─ Commits each file
    └─ Pushes design-docs branch
    ↓
User Reviews Design Docs
    ├─ Views markdown files in Helix UI
    ├─ Approves OR requests changes
    └─ If changes: Agent revises and re-commits
    ↓
Implementation Session Starts (External Agent in Wolf app)
    ├─ Agent clones repository
    ├─ Checks out design-docs branch (reads specs)
    ├─ Creates feature branch: feature/{spectask_id}
    ├─ Implements according to plan
    ├─ Makes atomic commits
    └─ Pushes feature branch
    ↓
User Reviews Code
    ├─ Views commits in Helix UI (basic) or GitHub/external tool (advanced)
    ├─ Tests implementation
    └─ Merges or requests changes
```

#### Complete Git Sync Flow

**Agent → Helix Git Server → Helix UI**

```
Planning Agent (Wolf container)
    ↓
Writes design docs to worktree
    ↓
Commits to helix-design-docs branch (local)
    ↓
Pushes to Helix git server: git push origin helix-design-docs
    ↓
Helix Git Server (/opt/helix/filestore/git-repositories/{repo_id}/)
Updates helix-design-docs branch in bare repository
    ↓
Helix UI (Frontend)
Fetches helix-design-docs branch via API
    ↓
Reads design docs from git (not database!)
    ↓
Renders markdown in UI for user review
```

**Key Benefits**:
- ✅ Git is source of truth (not database)
- ✅ Design docs versioned with full git history
- ✅ Multiple sessions can read same design docs
- ✅ UI polls git for updates (live progress)
- ✅ Forward-only branch preserves all design iterations

#### Git Repository Storage

**Existing Infrastructure** (already working):
- Repositories stored in `/opt/helix/filestore/git-repositories/`
- Each repo gets unique ID: `{repo_type}-{name}-{timestamp}`
- Served via HTTP: `http://api:8080/git/{repo_id}`
- Accessible from Wolf containers (on helix_default network)

**Example**:
```
/opt/helix/filestore/git-repositories/
├── project-my-app-1729700000/     # User project repository
│   ├── .git/
│   │   └── refs/heads/
│   │       ├── main               # Main development branch
│   │       └── helix-design-docs  # Design docs branch (forward-only)
│   ├── src/                       # Source code
│   └── README.md
```

**helix-design-docs branch structure** (in git):
```
helix-design-docs branch:
├── README.md
├── tasks/
│   └── 2025-10-23_add-auth_spec_abc123/
│       ├── requirements.md
│       ├── design.md
│       ├── tasks.md
│       └── task-metadata.json
└── archive/
```

#### Agent Workspace: Per-SpecTask (Not Per-Session!)

**CRITICAL**: Workspace is **per-SpecTask**, not per-session!

**Why**: Single SpecTask spans multiple Helix sessions (planning, implementation, testing). All sessions share the same workspace, git repos, and Zed state.

**External agent workspace structure** (per-SpecTask):
```
/opt/helix/filestore/workspaces/spectasks/spec_abc123/
└── work/                                    # SHARED across ALL sessions in this SpecTask
    ├── .zed-sessions/                       # Zed session persistence
    ├── backend/                             # Primary repository checkout
    │   ├── .git/
    │   ├── .git-worktrees/
    │   │   └── helix-design-docs/          # Design docs worktree
    │   │       └── tasks/
    │   │           └── 2025-10-23_add-auth_spec_abc123/
    │   │               ├── requirements.md
    │   │               ├── design.md
    │   │               └── tasks.md
    │   └── src/
    ├── frontend/                            # Additional repository checkout
    │   ├── .git/
    │   └── src/
    ├── shared/                              # Another repository checkout
    │   ├── .git/
    │   └── lib/
    └── README.md                            # Workspace welcome file
```

**CRITICAL - Per-SpecTask Workspace Benefits**:
- ✅ **Shared across phases**: Planning and implementation use SAME workspace
- ✅ **All Zed threads in one place**: Planning thread, implementation thread, etc. all visible
- ✅ **Design docs accessible**: Implementation reads planning's helix-design-docs
- ✅ **No re-cloning**: Git repos cloned once, used across all phases
- ✅ **Single Wolf container**: One Zed instance for entire SpecTask lifecycle
- ✅ **Continuous git history**: All phases contribute to same git state
- ✅ **Survives idle termination**: Wolf app deleted, workspace untouched in filestore

**External Agent Creation** (updated):
```go
// Create Wolf agent with per-SpecTask workspace
workspaceDir := filepath.Join(w.workspaceBasePath, "spectasks", specTaskID)

agentReq := &types.ZedAgent{
    SessionID:      helixSessionID,     // Current session (planning or implementation)
    HelixSessionID: helixSessionID,     // Maps to this Helix session
    WorkDir:        workspaceDir,       // PER-SPECTASK workspace (shared across sessions!)
    ProjectPath:    "backend",          // Primary repo checkout
}
```

When external agent is terminated (idle), the Wolf app is removed but **workspace remains** for next session.

### Phase 6: Automatic Session Teardown

#### Idle Detection Strategy

**Definition of "idle"** (updated for per-SpecTask agents):
- No new interactions in **ANY** Helix session for a SpecTask for >30 minutes
- PDEs are EXEMPT (user's personal workspace)
- Applies to SpecTask external agents (which may serve multiple sessions)

**Tracking mechanism** (per-external-agent, not per-session):
```go
type ExternalAgentActivity struct {
    ExternalAgentID string    `json:"external_agent_id" gorm:"primaryKey"` // e.g., "zed-spectask-abc123"
    SpecTaskID      string    `json:"spec_task_id" gorm:"index"`           // Parent SpecTask
    LastInteraction time.Time `json:"last_interaction" gorm:"index"`
    AgentType       string    `json:"agent_type"` // "spectask", "pde", "adhoc"
    WolfAppID       string    `json:"wolf_app_id"` // Wolf app ID for termination
    WorkspaceDir    string    `json:"workspace_dir"` // Persistent workspace path
    ActiveSessions  []string  `json:"active_sessions" gorm:"-"` // All Helix sessions using this agent
    UserID          string    `json:"user_id"`
}
```

Update on EVERY interaction from ANY session in the SpecTask:
```go
func (s *HelixAPIServer) handleSessionInteraction(sessionID string) {
    // Find external agent for this session
    agent := s.findExternalAgentForSession(sessionID)
    if agent != nil {
        activity := &ExternalAgentActivity{
            ExternalAgentID: agent.ID,
            SpecTaskID:      agent.SpecTaskID,
            LastInteraction: time.Now(),
        }
        s.store.UpsertExternalAgentActivity(ctx, activity)
    }
}
```

#### Cleanup Loop

Background goroutine in Wolf executor:

```go
func (w *WolfExecutor) idleSessionCleanupLoop(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.cleanupIdleSessions(ctx)
        }
    }
}

func (w *WolfExecutor) cleanupIdleExternalAgents(ctx context.Context) {
    cutoff := time.Now().Add(-30 * time.Minute)

    // Get idle external agents (not individual sessions - entire agents)
    idleAgents, err := w.store.GetIdleExternalAgents(ctx, cutoff, []string{"spectask"})
    if err != nil {
        log.Error().Err(err).Msg("Failed to get idle external agents")
        return
    }

    for _, activity := range idleAgents {
        log.Info().
            Str("external_agent_id", activity.ExternalAgentID).
            Str("spectask_id", activity.SpecTaskID).
            Str("wolf_app_id", activity.WolfAppID).
            Time("last_interaction", activity.LastInteraction).
            Int("active_sessions", len(activity.ActiveSessions)).
            Msg("Terminating idle SpecTask external agent")

        // Stop Wolf app (terminates Zed container)
        err := w.wolfClient.RemoveApp(ctx, activity.WolfAppID)
        if err != nil {
            log.Error().Err(err).Str("wolf_app_id", activity.WolfAppID).Msg("Failed to remove idle Wolf app")
        }

        // Update ALL affected Helix sessions to mark external agent as terminated
        for _, sessionID := range activity.ActiveSessions {
            session, err := w.store.GetSession(ctx, sessionID)
            if err == nil {
                session.Metadata.ExternalAgentStatus = "terminated_idle"
                w.store.UpdateSession(ctx, session.ID, session)
            }
        }

        // Delete activity record
        w.store.DeleteExternalAgentActivity(ctx, activity.ExternalAgentID)

        log.Info().
            Str("external_agent_id", activity.ExternalAgentID).
            Str("workspace_dir", activity.WorkspaceDir).
            Msg("External agent terminated, workspace preserved in filestore")
    }

    if len(idleAgents) > 0 {
        log.Info().Int("count", len(idleAgents)).Msg("Terminated idle SpecTask external agents")
    }
}
```

#### Frontend Warnings

**Warning Banner** (show when session is >25 minutes idle):
```typescript
{idleMinutes >= 25 && sessionType !== 'pde' && (
  <Alert severity="warning" sx={{ mb: 2 }}>
    <AlertTitle>Idle Session Warning</AlertTitle>
    This external agent session has been idle for {idleMinutes} minutes.
    Sessions are automatically terminated after 30 minutes of inactivity to free GPU resources.
    Send a message to keep the session alive.
  </Alert>
)}
```

**Session Activity Tracking**:
```typescript
const { data: sessionActivity } = useQuery({
  queryKey: ['session-activity', sessionId],
  queryFn: () => apiClient.v1SessionActivityGet(sessionId),
  refetchInterval: 60000, // Check every minute
  enabled: !!sessionId && sessionType !== 'pde'
})

const idleMinutes = sessionActivity
  ? Math.floor((Date.now() - new Date(sessionActivity.last_interaction).getTime()) / 60000)
  : 0
```

### Phase 7: Workspace Persistence - Complete State Relocation

#### Critical State Directories

Based on analysis of Zed on this host, Zed stores state in multiple directories:

**Zed State Locations** (default):
- `~/.config/zed/` - Settings, themes (settings.json)
- `~/.local/share/zed/` - Database, extensions, languages, logs, server state, **threads**, conversations
  - `db/` - IndexedDB-like storage
  - `threads/` - **CRITICAL**: Thread state and history
  - `conversations/` - Chat history
  - `extensions/` - Installed extensions
  - `languages/` - Language servers
  - `server_state/` - LSP and other server state
  - `logs/` - Application logs
- `~/.cache/zed/` - Temporary files, crash handlers (less critical)

**Claude Code State Locations** (default):
- `~/.claude/` - **ALL state in one directory**
  - `.credentials.json` - Authentication credentials
  - `history.jsonl` - Command history
  - `settings.json` - User settings
  - `projects/` - Project-specific state
  - `session-env/` - Session environment data
  - `file-history/` - File operation history
  - `todos/` - Todo list state
  - `debug/` - Debug logs
  - `shell-snapshots/` - Shell state snapshots
  - `statsig/` - Analytics/telemetry

#### Complete Symlink Strategy

**Updated startup script** (`wolf/sway-config/start-zed-helix.sh`):
```bash
#!/bin/bash
set -e

# CRITICAL: Relocate ALL Zed state to persistent workspace
# This ensures state survives container termination (30min idle cleanup)

WORK_DIR=/home/retro/work
ZED_STATE_DIR=$WORK_DIR/.zed-state

# Create persistent state directory structure
mkdir -p $ZED_STATE_DIR/config
mkdir -p $ZED_STATE_DIR/local-share
mkdir -p $ZED_STATE_DIR/cache

# Symlink entire state directories to persistent workspace
# This captures EVERYTHING (settings, db, threads, conversations, extensions, etc.)

# ~/.config/zed → work/.zed-state/config
rm -rf ~/.config/zed
mkdir -p ~/.config
ln -sf $ZED_STATE_DIR/config ~/.config/zed

# ~/.local/share/zed → work/.zed-state/local-share
rm -rf ~/.local/share/zed
mkdir -p ~/.local/share
ln -sf $ZED_STATE_DIR/local-share ~/.local/share/zed

# ~/.cache/zed → work/.zed-state/cache (less critical but include for completeness)
rm -rf ~/.cache/zed
mkdir -p ~/.cache
ln -sf $ZED_STATE_DIR/cache ~/.cache/zed

# ~/.claude → work/.claude-state (if Claude Code is installed)
# Claude stores ALL state in one directory, making this simpler
CLAUDE_STATE_DIR=$WORK_DIR/.claude-state
if command -v claude &> /dev/null; then
    mkdir -p $CLAUDE_STATE_DIR
    rm -rf ~/.claude
    ln -sf $CLAUDE_STATE_DIR ~/.claude
    echo "   Claude: ~/.claude → $CLAUDE_STATE_DIR"
fi

echo "✅ All editor state directories symlinked to persistent workspace"
echo "   Zed Config: ~/.config/zed → $ZED_STATE_DIR/config"
echo "   Zed Data: ~/.local/share/zed → $ZED_STATE_DIR/local-share"
echo "   Zed Cache: ~/.cache/zed → $ZED_STATE_DIR/cache"

# Continue with existing startup logic...
# (SSH agent, git config, settings sync wait, Zed launch)
```

#### Benefits of Complete State Relocation

- ✅ **Zed settings persist** (`~/.config/zed/settings.json`)
- ✅ **Thread history persists** (`~/.local/share/zed/threads/`)
- ✅ **Conversations persist** (`~/.local/share/zed/conversations/`)
- ✅ **Extension state persists** (`~/.local/share/zed/extensions/`)
- ✅ **Database persists** (`~/.local/share/zed/db/`)
- ✅ **Language servers persist** (`~/.local/share/zed/languages/`)
- ✅ **No re-initialization needed** on container restart
- ✅ **User customizations preserved** across sessions
- ✅ **Git repository clones persist** (no need to re-clone)

#### Complete Work Directory Structure

```
/opt/helix/filestore/workspaces/external-agents/ses_abc123/
└── work/                                    # ENTIRE DIR MOUNTED TO CONTAINER
    ├── .zed-state/                          # ALL Zed state relocated here
    │   ├── config/                          # ~/.config/zed → symlinked here
    │   │   ├── settings.json               # Zed settings (persists!)
    │   │   └── themes/                      # User themes
    │   ├── local-share/                     # ~/.local/share/zed → symlinked here
    │   │   ├── db/                         # Zed database
    │   │   ├── threads/                    # Thread state (CRITICAL!)
    │   │   ├── conversations/              # Chat history
    │   │   ├── extensions/                 # Installed extensions
    │   │   ├── languages/                  # Language servers
    │   │   ├── server_state/               # LSP state
    │   │   └── logs/                       # Application logs
    │   └── cache/                           # ~/.cache/zed → symlinked here
    ├── .claude-state/                       # ALL Claude Code state (if installed)
    │   ├── .credentials.json               # Auth credentials
    │   ├── history.jsonl                   # Command history
    │   ├── settings.json                   # Settings
    │   ├── projects/                       # Project state
    │   ├── session-env/                    # Session environment
    │   ├── file-history/                   # File operations
    │   ├── todos/                          # Todo lists
    │   └── debug/                          # Debug logs
    ├── backend/                             # Primary git repository
    │   ├── .git/
    │   ├── .git-worktrees/
    │   │   └── helix-design-docs/          # Design docs worktree
    │   │       └── tasks/
    │   │           └── 2025-10-23_add-auth_spec_abc123/
    │   │               ├── requirements.md
    │   │               ├── design.md
    │   │               └── tasks.md
    │   └── src/
    ├── frontend/                            # Additional repository
    │   └── .git/
    ├── shared/                              # Another repository
    │   └── .git/
    └── README.md                            # Workspace welcome file
```

**Symlinks Inside Container** (created at startup):
```bash
~/.config/zed       → /home/retro/work/.zed-state/config
~/.local/share/zed  → /home/retro/work/.zed-state/local-share
~/.cache/zed        → /home/retro/work/.zed-state/cache
~/.claude           → /home/retro/work/.claude-state (if Claude Code installed)
```

**Key Benefits**:
- ✅ **ALL Zed state persists** (config, db, threads, conversations, extensions)
- ✅ **ALL Claude Code state persists** (history, todos, projects, credentials)
- ✅ **Git repositories persist** (no re-clone needed)
- ✅ **User customizations preserved** (themes, settings, extensions)
- ✅ **Thread history intact** (no lost work)
- ✅ **Language servers don't re-download** (faster startup)
- ✅ **Survives idle termination** (Wolf app deleted, work/ directory untouched)
- ✅ **Resume exactly where you left off** (same threads, same git state, same everything)

## Implementation Plan

### Step 1: Database Migration
- [ ] Simplify SpecTask schema: `OriginalPrompt` is primary user input
- [ ] Add `HelixAppID` column to `spec_tasks` table (single agent for entire workflow)
- [ ] Add `AttachedRepositories` JSONB column for multiple repo attachments
- [ ] Add `PrimaryRepositoryID` column for design docs host repository
- [ ] Add `PlanningSessionID`, `ImplementationSessionID` columns
- [ ] **Create `spec_task_external_agents` table** for per-SpecTask agent tracking
  - Tracks external agent ID, Wolf app ID, workspace dir
  - Links to multiple Helix sessions
  - Tracks Zed threads
- [ ] Create `external_agent_activity` table for idle tracking (per-agent, not per-session!)
- [ ] No migration needed - no existing data

### Step 2: Frontend - Simplified Creation Form
- [ ] Replace multi-field form with single large text box for task description
- [ ] Move priority to top-right dropdown (High/Medium/Low)
- [ ] Remove "Type" field (AI infers from description)
- [ ] Move "From Example Tasks" to separate dropdown button next to "New SpecTask"
- [ ] Add primary repository dropdown (list user's Helix-hosted repos)
- [ ] Add "Additional Repositories" section with [+ Add Repository] button
- [ ] Each additional repo has local path input and [Remove] button
- [ ] Add "New Repo" button for inline repository creation
- [ ] Add single Helix Agent dropdown at bottom
- [ ] Implement "Create Default Zed Agent" if none exist
- [ ] Update SpecTask creation API call with simplified payload
- [ ] Add repository management to SpecTask detail view (edit attached repos at any phase)

### Step 3: Backend - Wolf Apps Integration
- [ ] Update `SpecTaskOrchestrator.handleBacklog()` to use Wolf executor (apps mode)
- [ ] Implement `buildPlanningPrompt()` with multi-repo git workflow
- [ ] Include clone commands for ALL attached repositories in prompt
- [ ] Specify primary repo for helix-design-docs branch
- [ ] Update planning phase to read task-metadata.json and populate Name/Description/Type
- [ ] Update `SpecTaskOrchestrator.handleImplementationQueued()` to use Wolf executor (apps mode)
- [ ] Implement `buildImplementationPrompt()` with multi-repo git workflow
- [ ] Remove NATS trigger code from agent work queue
- [ ] Ensure Wolf executor uses apps mode (not lobbies) for session creation
- [ ] Pass ALL attached repository clone URLs to external agent workspace

### Step 4: Idle Session Detection
- [ ] Create `SessionActivity` model and table
- [ ] Add `handleSessionInteraction()` hook on every API call
- [ ] Implement `GetIdleSessions()` store method
- [ ] Add session activity tracking to WebSocket sync

### Step 5: Automatic Cleanup
- [ ] Implement `idleSessionCleanupLoop()` in Wolf executor
- [ ] Add `cleanupIdleSessions()` with Wolf app removal (not lobby - we use apps mode)
- [ ] Update session metadata with termination reason
- [ ] Add logging and metrics for cleanup actions

### Step 6: Frontend Warnings
- [ ] Add session activity API endpoint
- [ ] Implement idle warning banner component
- [ ] Add countdown timer showing time until termination
- [ ] Test warning appears at 25-minute mark

### Step 7: Complete Zed State Persistence
- [ ] Update `start-zed-helix.sh` to symlink ALL Zed state directories to work/.zed-state/
- [ ] Symlink `~/.config/zed` → `work/.zed-state/config`
- [ ] Symlink `~/.local/share/zed` → `work/.zed-state/local-share`
- [ ] Symlink `~/.cache/zed` → `work/.zed-state/cache`
- [ ] Test Zed settings persist across container restarts
- [ ] Test thread history persists across restarts
- [ ] Test extensions persist across restarts
- [ ] Verify work is preserved when idle session terminates
- [ ] Document complete workspace directory structure
- [ ] Add similar symlinks for Claude Code if/when supported

### Step 8: Git Repository UI (Basic)
- [ ] Create top-level "Repositories" navigation item
- [ ] Implement repository list view (cards with name, description, last activity)
- [ ] Add repository creation modal from repositories page
- [ ] Filter repositories by type (project/spectask/sample)
- [ ] Display clone URL and clone command
- [ ] Basic repository detail view (README.md display)
- [ ] File browser UI (OPTIONAL - future enhancement)

### Step 9: Testing & Validation
- [ ] Test SpecTask creation with Helix Agent selection
- [ ] Test SpecTask creation with repository attachment
- [ ] Test planning phase agent launch (Wolf apps mode)
- [ ] Verify planning agent clones repo and creates design-docs branch
- [ ] Verify design documents are committed to git
- [ ] Test implementation phase agent launch (Wolf apps mode)
- [ ] Verify implementation agent creates feature branch
- [ ] Test idle detection triggers at 30 minutes
- [ ] Test warning appears at 25 minutes
- [ ] Test session resurrection after termination
- [ ] Test workspace persistence across restarts
- [ ] Verify Wolf apps are created (not lobbies)

## Migration Strategy

### Backward Compatibility

**No migration needed** - there is no existing SpecTask data to migrate.

**Trigger System**:
- Keep NATS infrastructure temporarily for non-SpecTask triggers
- Gradually migrate all triggers to Wolf executor
- Remove NATS completely in future release

### Rollout Plan

1. **Phase 1 (Week 1)**: Database schema + default Helix Agent creation + git repository attachment
2. **Phase 2 (Week 1)**: Frontend simplified form + repo selection + backend Wolf apps integration
3. **Phase 3 (Week 2)**: Planning phase git workflow (clone, design-docs branch, markdown files)
4. **Phase 4 (Week 2)**: Implementation phase git workflow (clone, feature branch, implement)
5. **Phase 5 (Week 3)**: Idle detection + cleanup loop + frontend warnings
6. **Phase 6 (Week 3)**: Zed persistence + basic git UI + testing
7. **Phase 7 (Week 4)**: Advanced git UI (file browser) - OPTIONAL future work

## Open Questions

1. ~~**Should planning and implementation share the same agent?**~~ **RESOLVED**: Use single agent for entire workflow (simpler UX)

2. **What about long-running tasks that genuinely need >30 minutes?**
   - Any user interaction resets the idle timer
   - Agent can send periodic status updates to keep alive
   - PDEs are exempt from idle timeout (long-lived workspaces)

3. **How do we handle cleanup failures?**
   - Log errors and retry on next cleanup loop
   - Manual cleanup via admin endpoint if needed
   - Alert if cleanup fails repeatedly

4. **Should we allow users to configure idle timeout?**
   - Start with hard-coded 30 minutes
   - Add per-app configuration later if needed

5. **What happens to workspace data after termination?**
   - Workspace persists in filestore (not deleted)
   - User can create new session in same workspace
   - Add workspace cleanup policy later (e.g., 7 days after last use)

## Success Metrics

- ✅ SpecTasks can select Helix Agents for planning/implementation
- ✅ External agent sessions launch via Wolf apps executor (no NATS)
- ✅ Idle sessions terminate automatically after 30 minutes
- ✅ Users receive warning at 25-minute idle mark
- ✅ Work persists in workspace even after termination
- ✅ GPU resources freed up promptly (no leaked sessions)
- ✅ No NATS infrastructure dependencies remaining
- ✅ Wolf apps mode used throughout (not lobbies)

## Session Lifecycle and Multi-Threading

### Critical Architecture: Helix Sessions vs External Agent vs Zed Threads

> **Reference**: This builds on existing architecture in:
> - `design/simplified_integration_architecture.md` - "Each Zed thread maps 1:1 to a Helix session"
> - `design/ZED_HELIX_INTEGRATION_ARCHITECTURE.md` - WebSocket protocol for thread creation
> - `design/2025-09-04-COMPLETE_IMPLEMENTATION_WITH_UI.md` - Automatic session creation from Zed threads

#### Three Separate Concepts

**1. Helix Session** (`ses_abc123`):
- Chat thread in Helix database
- Contains conversation history, interactions
- **Persists indefinitely** (never auto-terminated)
- Created by user OR by Zed when it creates a new thread

**2. External Agent** (Wolf container):
- Single Zed instance running in Wolf container
- GPU-allocated environment
- Can host **MULTIPLE Zed threads** simultaneously
- **Terminated after 30min idle** (frees GPU)
- Workspace persists in filestore

**3. Zed Thread** (ACP thread in Zed UI):
- Individual conversation thread in Zed
- Maps 1:1 to a Helix session
- Multiple threads can exist in one Zed instance
- When user creates thread in Zed → automatically creates Helix session

#### Key Architectural Insight: Thread Spawning

**Zed threads are 1:1 with Helix sessions** (existing architecture):

```
Single External Agent (Wolf container with Zed)
    ├─ Thread 1 (zed-ctx-001) ←→ Helix Session ses_001 (planning)
    ├─ Thread 2 (zed-ctx-002) ←→ Helix Session ses_002 (implementation)
    ├─ Thread 3 (zed-ctx-003) ←→ Helix Session ses_003 (testing)
    └─ Thread 4 (zed-ctx-004) ←→ Helix Session ses_004 (docs)
```

**SpecTask with single external agent can span multiple phases**:
- Planning phase → Creates Helix session → Creates Zed thread #1
- Implementation phase → Creates Helix session → Creates Zed thread #2 (SAME Zed instance)
- Both threads visible in Zed UI
- Agent can switch between threads for context
- All work happens in same workspace (shared git repos)

### Idle Termination and Resurrection Flow

#### Scenario: External Agent with Multiple Threads

**T0: Active External Agent (Multiple Threads)**
```
External Agent: zed-spectask-abc123 (Wolf container)
    ├─ Zed Thread 1 ←→ Helix Session ses_001 (planning)
    ├─ Zed Thread 2 ←→ Helix Session ses_002 (implementation)
    └─ Zed Thread 3 ←→ Helix Session ses_003 (testing)
    ↓
Workspace: work/ directory (filestore mount)
    ├─ .zed-state/ (all Zed threads, conversations, state)
    ├─ backend/ (git repos with helix-design-docs)
    └─ frontend/
```

**T+30min: Idle Cleanup Triggers (ALL sessions idle)**
```
✅ Helix Sessions: ses_001, ses_002, ses_003 (STILL EXIST in database)
✅ Helix threads preserved (conversation history intact)
❌ External Agent: Wolf app TERMINATED (GPU freed, container deleted)
✅ Workspace: work/ directory (UNTOUCHED in filestore)
✅ Zed state: .zed-state/ with all 3 threads (preserved)
```

**T+35min: User Sends New Message to ANY Session**
```
User sends message to ses_002 (implementation session)
    ↓
Helix API detects: External agent for ses_002 is terminated
    ↓
Helix creates NEW external agent (NEW Wolf container)
    ↓
NEW Wolf app starts, mounts SAME work/ directory
    ↓
Zed launches, reads state from .zed-state/ symlinks
    ↓
Zed loads ALL 3 threads from persisted state
    ├─ Thread 1 (ses_001) - planning session
    ├─ Thread 2 (ses_002) - implementation session ← ACTIVE
    └─ Thread 3 (ses_003) - testing session
    ↓
User continues conversation in ses_002
ALL previous threads still visible and accessible
Agent picks up exactly where it left off
```

**Key Insights**:
1. **One external agent** (Wolf container) can serve **multiple Helix sessions** (via Zed threads)
2. **Idle timeout** affects the external agent (container), not individual sessions
3. **Resurrection** creates new Wolf container with SAME workspace → all threads restored
4. **"New session"** in resurrection context means **new external agent container**, not new Helix session

## Architecture Verification

Before implementing, please verify this design aligns with existing architecture:

### Questions for Review

1. **helix-design-docs branch naming**: Is `helix-design-docs` the correct name? (Found in existing code)
   - ✅ Confirmed in `design_docs_worktree_manager.go:64`

2. **Task directory structure**: Is `tasks/{YYYY-MM-DD}_{branch-name}_{task_id}/` correct?
   - ✅ Confirmed in `design_docs_worktree_manager.go:293`

3. **File naming**: Are `requirements.md`, `design.md`, `tasks.md` the right names?
   - ✅ Confirmed in `design_docs_worktree_manager.go:304-428`

4. **Task markers**: Do we use `[ ]`, `[~]`, `[x]` for pending/in-progress/completed?
   - ✅ Confirmed in `design_docs_worktree_manager.go:493-495`

5. **Forward-only commits**: Design docs are never rolled back, only appended?
   - ✅ Confirmed in existing design docs

6. **Worktree vs push**: Do we push helix-design-docs or keep it local?
   - ✅ **CONFIRMED**: Push to Helix git server for UI sync

### Alignment with Existing Code

✅ **DesignDocsWorktreeManager exists** - No need to rebuild, just integrate
✅ **GitRepositoryService exists** - Full git hosting already working
✅ **Task parsing exists** - Can read `[ ]`/`[~]`/`[x]` markers
✅ **Worktree setup exists** - `SetupWorktree()` method ready
✅ **Task organization exists** - Date-prefixed directories implemented

### What's New in This Design

1. **SpecTask → Helix Agent integration** (not yet connected)
2. **Wolf apps triggering** (replace NATS)
3. **Repository attachment** (SpecTask schema addition)
4. **30-minute idle cleanup** (new feature)
5. **Simplified creation form** (UX improvement)

## References

- **Existing Architecture**:
  - `design/spectask-orchestrator-architecture.md` - helix-design-docs worktree system
  - `design/spectask-interactive-review-enhancement.md` - Git-based design doc workflow
  - `api/pkg/services/design_docs_worktree_manager.go` - Worktree manager implementation
  - `api/pkg/services/git_repository_service.go` - Git repository management

- **Current SpecTask Implementation**:
  - `api/pkg/types/simple_spec_task.go` - SpecTask data model
  - `api/pkg/services/spec_task_orchestrator.go` - Orchestrator (needs Wolf integration)
  - `api/pkg/external-agent/wolf_executor.go` - Wolf apps executor
  - `api/pkg/trigger/agent_work_queue/agent_work_queue_trigger.go` - NATS trigger (to be replaced)
