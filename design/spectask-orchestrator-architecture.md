# SpecTask Orchestrator Architecture

## Vision

Transform Helix into a manager-facing AI development platform where:
- Multiple agents work in parallel on SpecTasks
- Each agent progresses through design → approval → implementation workflow
- Agents work across multiple Helix sessions while maintaining context via git worktrees
- Managers see real-time visual progress of each agent's current task

## Core Concepts

### 1. SpecTask Scheduler/Orchestrator

**Purpose**: Autonomous system that pushes agents through the complete workflow

**Responsibilities**:
- Monitor SpecTask states continuously
- Automatically transition tasks through workflow phases
- Reuse external agent sessions across multiple Helix sessions
- Manage agent lifecycle and resource allocation
- Coordinate parallel agent work
- Handle failures and retries

**Workflow States**:
```
backlog → spec_generation → spec_review → [spec_revision →] spec_approved →
implementation_queued → implementation → implementation_review → done
```

### 2. helix-design-docs Branch/Worktree System

**Purpose**: Forward-only design document tracking that survives git branch switches

**Architecture**:
```
/workspace/repos/{repo-name}/
├── .git/                     # Main repo git directory
├── src/                      # Working code (may switch branches)
├── ...
└── .git-worktrees/
    └── helix-design-docs/    # Worktree for design docs branch
        ├── design.md         # Current design document
        ├── progress.md       # Task checklist and progress
        └── sessions/         # Per-session notes
```

**Key Properties**:
- **Forward-only**: Never rolled back, only appended to
- **Survives branch switches**: Main repo can switch branches, design docs persist
- **Git worktree**: Separate working directory for helix-design-docs branch
- **Shared history**: Same git repo, different branch

**Branch Creation**:
```bash
# When SpecTask starts:
cd /workspace/repos/{repo-name}
git branch helix-design-docs
git worktree add .git-worktrees/helix-design-docs helix-design-docs
```

### 3. Fleet Dashboard with Live Task Visualization

**Purpose**: Manager view showing which specific tasks agents are working on RIGHT NOW

**Visual Design**:
```
┌─────────────────────────────────────┐
│ Agent 1: Implementing authentication│
│                                     │
│ ┌───────────────────────────────┐   │
│ │ ○ Setup database schema       │ ← Faded (completed)
│ │ ○ Create user model          │ ← Faded (completed)
│ │ ● Add password hashing       │ ← HIGHLIGHTED (current task)
│ │ □ Implement login endpoint   │ ← Normal (next)
│ │ □ Add JWT token generation   │ ← Faded (future)
│ └───────────────────────────────┘   │
└─────────────────────────────────────┘
```

**Features**:
- Parse `- [ ]` / `- [x]` task lists from markdown design docs
- Highlight current task agent is working on
- Show few tasks before (faded, completed) and after (faded, upcoming)
- Live updates as agents commit progress to helix-design-docs
- Multiple agent cards showing parallel work

### 4. Design Doc Communication Protocol

**Agent ↔ Design Docs Flow**:

1. **Read current state**:
```bash
cd /workspace/repos/{repo}/.git-worktrees/helix-design-docs
cat progress.md
```

2. **Update progress**:
```bash
# Agent marks task as in-progress
sed -i 's/- \[ \] Add password hashing/- \[~\] Add password hashing/' progress.md
git add progress.md
git commit -m "🤖 Agent: Started password hashing implementation"
```

3. **Mark complete and move to next**:
```bash
sed -i 's/- \[~\] Add password hashing/- \[x\] Add password hashing/' progress.md
git add progress.md
git commit -m "🤖 Agent: Completed password hashing"
```

**Design Doc Format** (progress.md):
```markdown
# Implementation Progress

## Phase: Implementation

### Current Sprint
- [x] Setup database schema
- [x] Create user model
- [~] Add password hashing           ← Agent currently here
- [ ] Implement login endpoint
- [ ] Add JWT token generation
- [ ] Create refresh token logic

### Status
**Current Task**: Add password hashing
**Agent Session**: ses_abc123
**Started**: 2025-10-08 10:30 UTC
**Estimated Completion**: ~15 min
```

### 5. Demo Setup with Internal Git Hosting

**Purpose**: Quick demo using internal git hosting and sample repos

**Sample Repos Available**:
- `nodejs-todo` - Node.js + Express todo app
- `python-api` - FastAPI microservice
- `react-dashboard` - React admin dashboard
- `linkedin-outreach` - Multi-session campaign example
- `helix-blog-posts` - Multi-session content generation

**Setup Flow**:
1. User creates SpecTask, selects demo repo
2. System clones repo to `/opt/helix/filestore/git-repositories/{user}/{repo-name}`
3. Creates helix-design-docs branch and worktree
4. Agent starts with access to both code and design docs
5. All work happens in user's namespace (isolated)

## Implementation Components

### Backend

#### 1. SpecTaskOrchestrator Service

```go
type SpecTaskOrchestrator struct {
    store              store.Store
    controller         *controller.Controller
    gitService         *GitRepositoryService
    specTaskService    *SpecDrivenTaskService
    agentPool          *ExternalAgentPool
    runningTasks       map[string]*OrchestratedTask
    mutex              sync.RWMutex
}

type OrchestratedTask struct {
    SpecTask           *types.SpecTask
    CurrentAgentID     string
    CurrentSessionID   string
    DesignDocsPath     string
    RepoPath           string
    CurrentTaskIndex   int
    TaskList           []TaskItem
    LastUpdate         time.Time
}

type TaskItem struct {
    Index       int
    Description string
    Status      TaskStatus // pending, in_progress, completed
    StartedAt   *time.Time
    CompletedAt *time.Time
}
```

**Key Methods**:
- `Start(ctx)` - Main orchestrator loop
- `processTask(task)` - State machine for single task
- `transitionToNextPhase(task)` - Workflow transitions
- `reuseOrCreateAgent(task)` - Agent session management
- `parseProgressFromGit(task)` - Read current state from worktree
- `updateLiveProgress(task)` - Broadcast current task to dashboard

#### 2. DesignDocsWorktreeManager

```go
type DesignDocsWorktreeManager struct {
    gitService *GitRepositoryService
}

func (m *DesignDocsWorktreeManager) SetupWorktree(repoPath string) error {
    // 1. Create helix-design-docs branch if not exists
    // 2. Create worktree at .git-worktrees/helix-design-docs
    // 3. Initialize with template design doc structure
    // 4. Return worktree path
}

func (m *DesignDocsWorktreeManager) GetCurrentTask(worktreePath string) (*TaskItem, error) {
    // Parse progress.md
    // Find task with [~] marker (in progress)
    // Return TaskItem
}

func (m *DesignDocsWorktreeManager) MarkTaskInProgress(worktreePath string, taskIndex int) error {
    // Update [ ] to [~] for task
    // Commit to helix-design-docs branch
}

func (m *DesignDocsWorktreeManager) MarkTaskComplete(worktreePath string, taskIndex int) error {
    // Update [~] to [x] for task
    // Commit to helix-design-docs branch
}
```

#### 3. ExternalAgentPool

```go
type ExternalAgentPool struct {
    agents     map[string]*ExternalAgentInstance
    mutex      sync.RWMutex
}

type ExternalAgentInstance struct {
    InstanceID      string
    ZedInstanceID   string
    CurrentTaskID   string
    HelixSessions   []string  // Multiple Helix sessions can share this agent
    WorkingDir      string
    DesignDocsPath  string
    LastActivity    time.Time
    Status          AgentStatus
}

func (p *ExternalAgentPool) GetOrCreateForTask(task *types.SpecTask) (*ExternalAgentInstance, error) {
    // Check if task already has assigned agent
    // If yes, reuse (might be working across multiple sessions)
    // If no, allocate from pool or create new
}

func (p *ExternalAgentPool) TransitionToNewSession(agentID, newSessionID string) error {
    // Agent finishes one Helix session, starts another
    // Maintains workspace and context
    // Updates tracking
}
```

### Frontend

#### 1. LiveAgentFleetDashboard Component

```typescript
interface AgentTaskProgress {
  agent_id: string
  task_id: string
  task_name: string
  current_task: {
    index: number
    description: string
    started_at: string
  }
  task_list: TaskItem[]
  tasks_before: TaskItem[]  // 2-3 completed tasks
  tasks_after: TaskItem[]   // 2-3 upcoming tasks
  last_update: string
}

const LiveAgentFleetDashboard: FC = () => {
  const [agentProgress, setAgentProgress] = useState<AgentTaskProgress[]>([])

  // WebSocket or polling for live updates
  useEffect(() => {
    const interval = setInterval(() => {
      api.getAgentFleetProgress().then(setAgentProgress)
    }, 5000) // Update every 5 seconds

    return () => clearInterval(interval)
  }, [])

  return (
    <Grid container spacing={3}>
      {agentProgress.map(agent => (
        <Grid item xs={12} md={6} lg={4}>
          <AgentTaskCard agent={agent} />
        </Grid>
      ))}
    </Grid>
  )
}
```

#### 2. AgentTaskCard Component

```typescript
const AgentTaskCard: FC<{agent: AgentTaskProgress}> = ({ agent }) => {
  return (
    <Card>
      <CardHeader
        title={`Agent: ${agent.task_name}`}
        subheader={`Session: ${agent.agent_id.slice(0, 8)}...`}
      />
      <CardContent>
        {/* Faded completed tasks */}
        {agent.tasks_before.map(task => (
          <TaskListItem task={task} fade={0.3} completed />
        ))}

        {/* HIGHLIGHTED current task */}
        <TaskListItem
          task={agent.current_task}
          highlight
          pulse
        />

        {/* Faded upcoming tasks */}
        {agent.tasks_after.map(task => (
          <TaskListItem task={task} fade={0.5} />
        ))}
      </CardContent>
    </Card>
  )
}
```

#### 3. TaskListItem Component

```typescript
const TaskListItem: FC<{
  task: TaskItem
  highlight?: boolean
  completed?: boolean
  fade?: number
  pulse?: boolean
}> = ({ task, highlight, completed, fade = 1, pulse }) => {
  return (
    <Box
      sx={{
        opacity: fade,
        backgroundColor: highlight ? 'warning.light' : 'transparent',
        animation: pulse ? 'pulse 2s infinite' : 'none',
        borderLeft: highlight ? '4px solid' : 'none',
        borderColor: 'warning.main',
        p: 1,
        mb: 0.5,
        borderRadius: 1,
        transition: 'all 0.3s ease'
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        {completed ? (
          <CheckCircle color="success" fontSize="small" />
        ) : highlight ? (
          <CircularProgress size={16} />
        ) : (
          <RadioButtonUnchecked fontSize="small" />
        )}
        <Typography variant="body2" fontWeight={highlight ? 'bold' : 'normal'}>
          {task.description}
        </Typography>
      </Box>
    </Box>
  )
}
```

### API Endpoints

```go
// Get live fleet progress for dashboard
GET /api/v1/agents/fleet/live-progress
Response: {
  agents: [
    {
      agent_id: "ext_agent_123",
      task_id: "spec_task_456",
      task_name: "User authentication",
      current_task: {
        index: 2,
        description: "Add password hashing",
        started_at: "2025-10-08T10:30:00Z"
      },
      tasks_before: [...],
      tasks_after: [...]
    }
  ]
}

// Create SpecTask with demo repo
POST /api/v1/spec-tasks/from-demo
Request: {
  prompt: "Add user authentication",
  demo_repo: "nodejs-todo",
  priority: "high"
}

// Get design docs for task (from worktree)
GET /api/v1/spec-tasks/{id}/design-docs
Response: {
  progress_md: "...",
  design_md: "...",
  current_task_index: 2
}
```

## Workflow Example

### 1. User Creates Task

```
POST /api/v1/spec-tasks/from-demo
{
  "prompt": "Add user authentication with JWT",
  "demo_repo": "nodejs-todo",
  "priority": "high"
}
```

### 2. Orchestrator Initializes

```
1. Clone demo repo to user namespace
2. Create helix-design-docs branch
3. Setup worktree at .git-worktrees/helix-design-docs
4. Initialize progress.md with empty task list
5. Start spec generation phase
```

### 3. Spec Generation Phase

```
1. Create planning session (Helix agent)
2. Agent generates:
   - requirements.md
   - design.md
   - progress.md with task checklist:
     - [ ] Setup authentication schema
     - [ ] Implement password hashing
     - [ ] Create login endpoint
     - [ ] Add JWT generation
     - [ ] Implement refresh tokens
3. Commit all to helix-design-docs branch
4. Transition to spec_review
```

### 4. Human Approval

```
1. Dashboard shows spec ready for review
2. User reviews design docs via UI
3. User approves or requests changes
4. If approved → transition to implementation_queued
```

### 5. Implementation Phase

```
1. Allocate external agent (Zed) from pool
2. Agent reads design docs from worktree:
   cd .git-worktrees/helix-design-docs
   cat progress.md
3. Agent starts first task:
   - Mark "Setup authentication schema" as [~]
   - Commit to helix-design-docs
4. Agent creates Helix session for implementation
5. Completes task, marks [x], commits
6. Moves to next task automatically
7. Orchestrator watches commits, updates dashboard
8. Cycle continues through all tasks
```

### 6. Dashboard Shows Live Progress

```
Manager sees:
┌─────────────────────────────────────┐
│ Agent ext_123: nodejs-todo auth    │
│                                     │
│ ○ Setup authentication schema      │ ← Completed (faded)
│ ● Implement password hashing       │ ← CURRENT (highlighted, pulsing)
│ □ Create login endpoint            │ ← Next (normal)
│ □ Add JWT generation               │ ← Future (faded)
└─────────────────────────────────────┘

Updates live as agent commits to helix-design-docs
```

## Benefits

1. **Parallel Agent Work**: Multiple agents on different tasks simultaneously
2. **Cross-Session Context**: Agents maintain context across Helix session boundaries
3. **Visual Progress**: Managers see exactly what each agent is doing RIGHT NOW
4. **Git-Based State**: Design docs survive any git operations on main code
5. **Forward-Only History**: Never lose progress, always append
6. **Demo-Ready**: Works with sample repos for instant demos
7. **Real-Time Updates**: Live dashboard updates from git commits

## Implementation Checklist

### Backend
- [ ] SpecTaskOrchestrator service with state machine
- [ ] DesignDocsWorktreeManager for git worktree management
- [ ] ExternalAgentPool for agent session reuse
- [ ] API endpoints for live progress
- [ ] Markdown task list parser
- [ ] Auto-transition logic for workflow states
- [ ] Demo repo integration with git service

### Frontend
- [ ] LiveAgentFleetDashboard component
- [ ] AgentTaskCard with task list visualization
- [ ] TaskListItem with highlighting and fading
- [ ] Live progress polling/WebSocket
- [ ] Demo repo selector in create task flow

### Integration
- [ ] Connect orchestrator to existing SpecDrivenTaskService
- [ ] Wire up external agent pool to Zed instances
- [ ] Setup git worktree on task creation
- [ ] Parse and broadcast task progress
- [ ] Handle agent failures and retries

## Success Criteria

✅ Multiple agents work in parallel on different SpecTasks
✅ Each agent progresses through design → approval → implementation
✅ Dashboard shows current task for each agent with context
✅ Design docs persist in helix-design-docs branch/worktree
✅ Agents reuse sessions across multiple Helix interactions
✅ Demo works with sample repos out of the box
✅ Live updates show real-time agent progress
