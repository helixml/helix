# Projects Architecture & Sample Projects Design

**Date:** 2025-11-01
**Status:** Planning
**Author:** Claude (based on user requirements)

## Executive Summary

This document outlines the architectural changes needed to elevate Projects to a first-class concept in Helix, replacing the current hardcoded "default project" approach with a proper project management system. This includes project-scoped repositories, per-project startup scripts, sample projects with pre-loaded backlog tasks, and exploratory agent sessions.

## Current State

### What Works
- Projects exist in the database (`spec_projects` table)
- SpecTasks are linked to projects via `project_id`
- Git repositories can be created and attached to projects/tasks
- Agents can be launched with workspace and repository access

### Pain Points
1. **UI hardcodes default project** - No way to select/view multiple projects
2. **Kanban board is global** - Not scoped to a specific project
3. **No project-level repository management** - Repos attached at task level only
4. **No project startup scripts** - Each agent starts with generic workspace
5. **No exploratory sessions** - Can't browse a project without starting a task
6. **No sample projects** - New users face empty backlog

## Proposed Architecture

### 1. Projects as Top-Level Navigation

```
Before:
  Home → Specs (Kanban with all tasks from default project)

After:
  Home → Projects (list of projects) → [Select Project] → Kanban (project-scoped)
```

**UI Changes:**
- `/projects` - List all projects user has access to
- `/projects/{projectId}/specs` - Kanban board for specific project
- `/projects/{projectId}/settings` - Project configuration
  - Name, description
  - Default primary repository
  - Attached repositories list
  - Startup script editor
  - Member access (if using Teams/Orgs)

### 2. Project-Level Repository Management

**Database Schema (Already Exists in `git_repositories` table):**
```sql
-- GitRepository already has project_id field
CREATE TABLE git_repositories (
  id VARCHAR(255) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  project_id VARCHAR(255),  -- Attach repos at project level
  spec_task_id VARCHAR(255), -- Can also attach at task level (override)
  clone_url TEXT NOT NULL,
  ...
);
```

**Implementation:**
- Projects can have multiple repositories attached
- One repository marked as "primary" (default opened in Zed)
- Tasks inherit project's repositories by default
- Tasks can override primary repository (e.g., "Work in auth-service repo for this task")
- When agent starts:
  1. Clone all project repositories
  2. Open primary repository in Zed by default
  3. All repos available in workspace at `{workspace}/{repo-name}/`

**API Endpoints:**
- `GET /api/v1/projects/{id}/repositories` - List project repos
- `POST /api/v1/projects/{id}/repositories` - Attach repo to project
- `PUT /api/v1/projects/{id}/repositories/{repoId}/primary` - Set primary repo
- `DELETE /api/v1/projects/{id}/repositories/{repoId}` - Detach repo

### 3. Per-Project Startup Scripts

**Purpose:**
- Install dependencies (npm install, pip install requirements.txt, etc.)
- Start development servers (webpack dev server, Django runserver, etc.)
- Set up databases (migrations, seed data, etc.)
- Configure environment (create .env files, etc.)

**Storage:**
- Stored in project metadata (`spec_projects.metadata JSON`)
- Alternatively: store in `.helix/startup.sh` in project's config repository

**Execution Flow:**
```
1. Agent container starts (Sway + Zed binary loaded)
2. Clone all project repositories
3. Execute project startup script in terminal (visible to user)
4. Launch Zed when startup completes
```

**Implementation:**
```go
// In types.go
type SpecProject struct {
    ID              string                 `gorm:"type:varchar(255);primaryKey"`
    Name            string                 `gorm:"type:varchar(255);not null"`
    Description     string                 `gorm:"type:text"`
    OwnerID         string                 `gorm:"type:varchar(255);not null;index"`
    DefaultRepoID   string                 `gorm:"type:varchar(255)"` // Primary repository
    StartupScript   string                 `gorm:"type:text"`         // Bash script to run on agent start
    Metadata        datatypes.JSON         `gorm:"serializer:json"`
    CreatedAt       time.Time              `gorm:"autoCreateTime"`
    UpdatedAt       time.Time              `gorm:"autoUpdateTime"`
}
```

**Startup Script Example:**
```bash
#!/bin/bash
# Helix Project Startup Script
echo "🚀 Starting Helix project: my-web-app"

# Install dependencies
cd /workspace/my-web-app
echo "📦 Installing npm dependencies..."
npm install

# Start dev server in background
echo "🌐 Starting webpack dev server on port 3000..."
npm run dev &

# Run database migrations
echo "🗄️ Running database migrations..."
npm run db:migrate

echo "✅ Project startup complete! Open http://localhost:3000 to view app."
```

**UI:**
- Project settings page with startup script editor
- Script runs in visible terminal inside Zed/PDE
- Agent won't launch Zed until script completes (or times out after 5min)

### 4. Sample Projects with Pre-loaded Backlog

**Concept:**
Replace current "examples/demos" with full sample projects that include:
- Complete codebase (cloneable git repository)
- Pre-populated backlog tasks (ready to work on)
- Startup script (gets project running immediately)
- README with project overview

**Sample Project Examples:**

**a) Todo App (React + FastAPI)**
- Repository: `https://github.com/helixml/sample-todo-app`
- Backlog tasks:
  - "Add user authentication"
  - "Implement dark mode toggle"
  - "Add task categories"
  - "Export tasks to CSV"
- Startup script: `npm install && npm run dev` + `pip install -r requirements.txt && uvicorn main:app`

**b) E-commerce Store (Next.js + Stripe)**
- Repository: `https://github.com/helixml/sample-ecommerce`
- Backlog tasks:
  - "Add product search"
  - "Implement shopping cart"
  - "Integrate Stripe checkout"
  - "Add order history"

**c) Chat Application (Node.js + Socket.io)**
- Repository: `https://github.com/helixml/sample-chat-app`
- Backlog tasks:
  - "Add private messaging"
  - "Implement read receipts"
  - "Add file uploads"
  - "Create user presence indicators"

**Implementation:**
```sql
CREATE TABLE sample_projects (
  id VARCHAR(255) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  category VARCHAR(100), -- 'web', 'mobile', 'api', 'ml', etc.
  difficulty VARCHAR(50), -- 'beginner', 'intermediate', 'advanced'
  repository_url TEXT NOT NULL,
  startup_script TEXT,
  thumbnail_url TEXT,
  sample_tasks JSON, -- Pre-defined backlog tasks
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**API Endpoints:**
- `GET /api/v1/sample-projects` - List available samples
- `POST /api/v1/sample-projects/{id}/instantiate` - Clone sample as new project
  - Creates project
  - Attaches repository
  - Creates backlog tasks
  - Saves startup script

**UI Flow:**
```
1. User clicks "New Project"
2. Options: "Blank Project" or "Start from Sample"
3. If sample: Show gallery of sample projects
4. User selects sample (e.g., "Todo App")
5. Backend creates project with:
   - Cloned repository
   - Pre-populated backlog
   - Startup script
6. User sees project Kanban with ready-to-work tasks
```

### 5. Exploratory Agent Sessions

**Purpose:**
Allow users to explore a project's codebase without starting a specific task. Perfect for:
- New users exploring sample projects
- Understanding project structure before planning
- Testing if project runs correctly
- Learning from example code

**Implementation:**
- New button on project page: "Start Exploratory Session"
- Creates external agent session WITHOUT a SpecTask
- Agent launches with:
  - All project repositories cloned
  - Startup script executed
  - Zed opens in primary repository
  - No specific task/goal - user can explore freely
- User can browse code, run commands, test application
- When ready: Click "Start Planning" to create a task from this session

**API:**
- `POST /api/v1/projects/{id}/exploratory-session`
  - Creates temporary session
  - Launches agent with project context
  - Returns session ID for streaming

**Workspace Scoping:**
- Exploratory sessions: `/opt/helix/filestore/workspaces/exploratory/{session_id}`
- NOT task-scoped (temporary, can be deleted after session ends)

### 6. Project Configuration Repository

**Optional Enhancement:**
Some projects may want to store project-level configuration in version control:

```
project-config/
  .helix/
    startup.sh          # Startup script
    environment.env     # Template environment variables
    README.md           # Project documentation
    tasks/              # Backlog task templates (Markdown files)
      001-authentication.md
      002-dark-mode.md
```

**Benefits:**
- Version-controlled project configuration
- Easy to clone projects with full setup
- Share project templates across teams

## Implementation Plan

### Phase 1: Projects UI & Navigation (2-3 days)
**Goal:** Users can view and select projects

**Tasks:**
- [ ] Create `/projects` list page (frontend)
- [ ] Create `/projects/{id}/specs` Kanban page (move existing Kanban, filter by project)
- [ ] Create `/projects/{id}/settings` page
- [ ] Update navigation to show Projects as top-level
- [ ] Add "New Project" button

**API Changes:**
- [ ] `GET /api/v1/projects` - List projects
- [ ] `POST /api/v1/projects` - Create project
- [ ] `GET /api/v1/projects/{id}` - Get project details
- [ ] `PUT /api/v1/projects/{id}` - Update project
- [ ] `DELETE /api/v1/projects/{id}` - Delete project

**Database:**
- Already exists (`spec_projects` table)
- May need to add `default_repo_id` field

### Phase 2: Project-Level Repository Management (2-3 days)
**Goal:** Attach multiple repos to projects

**Tasks:**
- [ ] Add UI to attach/detach repositories on project settings page
- [ ] Add "Set as Primary" button for repositories
- [ ] Update `StartSpecGeneration` to load ALL project repositories (not just task primary)
- [ ] Display repository list on project settings

**API Changes:**
- [ ] `GET /api/v1/projects/{id}/repositories` - List project repos
- [ ] `POST /api/v1/projects/{id}/repositories` - Attach repo
- [ ] `PUT /api/v1/projects/{id}/repositories/{repoId}/primary` - Set primary
- [ ] `DELETE /api/v1/projects/{id}/repositories/{repoId}` - Detach

**Database:**
- `git_repositories.project_id` already exists
- Add `spec_projects.default_repo_id` field

### Phase 3: Per-Project Startup Scripts (2-3 days)
**Goal:** Projects can define custom startup scripts

**Tasks:**
- [ ] Add `startup_script` field to `spec_projects` table
- [ ] Create startup script editor in project settings UI
- [ ] Modify Sway container startup to:
  1. Clone repos
  2. Run startup script in visible terminal
  3. Launch Zed when script completes
- [ ] Add timeout handling (5min max for startup script)
- [ ] Show startup script output in agent UI

**Implementation Location:**
- `wolf/sway-config/start-zed-helix.sh` - Modify to check for project startup script
- Execute script BEFORE launching Zed

### Phase 4: Sample Projects (3-4 days)
**Goal:** Users can instantiate pre-built sample projects

**Tasks:**
- [ ] Create 3 sample repositories:
  - Todo App (React + FastAPI)
  - Chat App (Node.js + Socket.io)
  - Blog (Next.js + Markdown)
- [ ] Create `sample_projects` table
- [ ] Seed sample project definitions in database
- [ ] Build sample project gallery UI
- [ ] Implement "Instantiate Sample" API
  - Creates project
  - Attaches repository
  - Creates backlog tasks from template
  - Saves startup script

**API Changes:**
- [ ] `GET /api/v1/sample-projects` - List samples
- [ ] `GET /api/v1/sample-projects/{id}` - Get sample details
- [ ] `POST /api/v1/sample-projects/{id}/instantiate` - Clone as new project

### Phase 5: Exploratory Sessions (2 days)
**Goal:** Users can explore projects without starting tasks

**Tasks:**
- [ ] Add "Start Exploratory Session" button on project page
- [ ] Create API endpoint to launch exploratory session
- [ ] Implement exploratory workspace scoping
- [ ] Add UI to transition from exploratory → task creation

**API Changes:**
- [ ] `POST /api/v1/projects/{id}/exploratory-session` - Start exploration

## Data Model Summary

```sql
-- spec_projects (already exists, needs updates)
ALTER TABLE spec_projects
  ADD COLUMN default_repo_id VARCHAR(255),
  ADD COLUMN startup_script TEXT;

-- git_repositories (already exists, already has project_id)
-- No changes needed

-- sample_projects (new table)
CREATE TABLE sample_projects (
  id VARCHAR(255) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  category VARCHAR(100),
  difficulty VARCHAR(50),
  repository_url TEXT NOT NULL,
  startup_script TEXT,
  thumbnail_url TEXT,
  sample_tasks JSON, -- Array of {title, description, priority, type}
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Open Questions

1. **Project Templates:** Should we support custom project templates beyond samples?
2. **Project Cloning:** Should users be able to clone their own projects (duplicating backlog)?
3. **Multi-Repo Workspaces:** How do we handle dependencies between repos in a project?
4. **Startup Script Security:** Do we need sandboxing/restrictions on what scripts can do?
5. **Exploratory Session Limits:** How long can exploratory sessions run? Auto-cleanup?

## Success Metrics

- Users create multiple projects (not just default)
- Sample project instantiation rate
- Exploratory sessions before first task creation
- Time-to-first-contribution in sample projects
- Startup script usage rate

## Migration Plan

**Backwards Compatibility:**
- Existing SpecTasks already have `project_id` - will work with new UI
- Default project can remain as fallback for orphaned tasks
- Kanban continues to work at `/specs` (redirects to default project)

**Migration Steps:**
1. Deploy Phase 1 (Projects UI) - existing data still works
2. Migrate default project tasks to have proper project assignments
3. Deploy remaining phases incrementally
4. Sunset hardcoded default project logic

## Next Steps

1. **User Validation:** Confirm this architecture meets user needs
2. **Prioritize Phases:** Determine which phases deliver most value first
3. **Sample Repo Creation:** Build the first sample project repository
4. **Begin Implementation:** Start with Phase 1 (Projects UI)

## References

- Current SpecTask implementation: `api/pkg/services/spec_driven_task_service.go`
- Git repository service: `api/pkg/services/git_repository_service.go`
- Workspace management: `api/pkg/external-agent/wolf_executor.go`
- Sway container startup: `wolf/sway-config/start-zed-helix.sh`
