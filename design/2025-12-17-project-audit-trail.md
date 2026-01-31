# Project Audit Trail Feature

**Date:** 2025-12-17
**Status:** Proposed
**Author:** Claude

## Overview

Add an audit trail feature to track all user prompts and agent interactions within a project. This provides visibility into what work has been requested, by whom, and what the outcomes were.

## Motivation

Currently there's no centralized view of all prompts and interactions that have occurred within a project. This makes it difficult to:
- Understand the history of work requested on a project
- Track which users are making requests
- Audit what prompts were sent to agents
- Correlate prompts with resulting PRs and task outcomes

## User Experience

### UI Toggle

The Kanban board page will have a toggle button next to the "Archived" button:

```
[Kanban] [Audit Trail]  |  [Show Archived]
```

When "Audit Trail" is selected, the entire view switches from the Kanban board to a full-screen paginated table.

### Audit Trail Table

| Timestamp | User | Event Type | Prompt/Description | Task | Session | PR | Actions |
|-----------|------|------------|-------------------|------|---------|-----|---------|
| 2025-12-17 14:32 | luke@example.com | Task Created | "Add dark mode toggle to settings" | #00001 | - | - | [Open] |
| 2025-12-17 14:33 | luke@example.com | Agent Prompt | "Start implementing the dark mode..." | #00001 | [View] | - | [Open] |
| 2025-12-17 15:10 | luke@example.com | User Message | "Can you also add a system preference option?" | #00001 | [View] | - | [Open] |
| 2025-12-17 16:45 | luke@example.com | PR Created | - | #00001 | - | PR #42 | [Open] |

The **Session** column shows a "View" link when a session ID is present. Clicking it opens the Helix Session view (`/session/{sessionId}`) and scrolls to the specific interaction if an `interaction_id` is recorded.

#### Columns

1. **Timestamp** - When the event occurred
2. **User** - Email/name of the user who triggered the event
3. **Event Type** - One of:
   - `Task Created` - New spec task created
   - `Agent Prompt` - Prompt sent from Helix UI to agent (via `SendUserPromptToAgent`)
   - `User Message` - Message sent by user inside the agent (via WebSocket sync)
   - `PR Created` - Pull request was created/linked
   - `Task Approved` - Spec was approved
   - `Task Completed` - Task moved to completed status
4. **Prompt/Description** - The actual prompt text (truncated in table, full text on hover/click)
5. **Task** - Link to the spec task (task number with link, e.g., #00001)
6. **Branch** - Git branch name for the task
7. **Session** - Link to Helix Session view (scrolls to specific interaction if available)
8. **PR** - Pull request link if one exists (internal or external like Azure DevOps)
9. **Spec** - Button to view spec at point-in-time (if spec hashes are captured)
10. **Actions** - Button to open the spec task detail panel

#### Features

- **Pagination**: 50 entries per page with page controls
- **Filtering**: Filter by event type, user, date range
- **Search**: Full-text search on prompt content
- **Click to expand**: Click a row to see full prompt text
- **Open Task**: "Open" button opens the SpecTaskDetailDialog for that task

## Database Schema

### New Table: `project_audit_logs`

```sql
CREATE TABLE project_audit_logs (
    id VARCHAR(255) PRIMARY KEY,
    project_id VARCHAR(255) NOT NULL REFERENCES projects(id),
    spec_task_id VARCHAR(255) REFERENCES spec_tasks(id),
    user_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(255),
    event_type VARCHAR(50) NOT NULL,
    prompt_text TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    INDEX idx_project_audit_project_id (project_id),
    INDEX idx_project_audit_created_at (created_at),
    INDEX idx_project_audit_spec_task_id (spec_task_id),
    INDEX idx_project_audit_event_type (event_type)
);
```

### Go Type

```go
// api/pkg/types/project_audit_log.go

type AuditEventType string

const (
    AuditEventTaskCreated    AuditEventType = "task_created"
    AuditEventAgentPrompt    AuditEventType = "agent_prompt"
    AuditEventUserMessage    AuditEventType = "user_message"
    AuditEventPRCreated      AuditEventType = "pr_created"
    AuditEventTaskApproved   AuditEventType = "task_approved"
    AuditEventTaskCompleted  AuditEventType = "task_completed"
)

type ProjectAuditLog struct {
    ID          string          `json:"id" gorm:"primaryKey;size:255"`
    ProjectID   string          `json:"project_id" gorm:"size:255;index;not null"`
    SpecTaskID  string          `json:"spec_task_id,omitempty" gorm:"size:255;index"`
    UserID      string          `json:"user_id" gorm:"size:255;not null"`
    UserEmail   string          `json:"user_email,omitempty" gorm:"size:255"`
    EventType   AuditEventType  `json:"event_type" gorm:"size:50;index;not null"`
    PromptText  string          `json:"prompt_text,omitempty" gorm:"type:text"`
    Metadata    AuditMetadata   `json:"metadata,omitempty" gorm:"type:jsonb;serializer:json"`
    CreatedAt   time.Time       `json:"created_at"`
}

type AuditMetadata struct {
    TaskNumber     int    `json:"task_number,omitempty"`
    TaskName       string `json:"task_name,omitempty"`
    PullRequestID  string `json:"pull_request_id,omitempty"`
    PullRequestURL string `json:"pull_request_url,omitempty"`
    SessionID      string `json:"session_id,omitempty"`
    InteractionID  string `json:"interaction_id,omitempty"`  // For scrolling to specific interaction
}

type ProjectAuditLogFilters struct {
    ProjectID  string         `json:"project_id"`
    EventType  AuditEventType `json:"event_type,omitempty"`
    UserID     string         `json:"user_id,omitempty"`
    SpecTaskID string         `json:"spec_task_id,omitempty"`
    StartDate  *time.Time     `json:"start_date,omitempty"`
    EndDate    *time.Time     `json:"end_date,omitempty"`
    Search     string         `json:"search,omitempty"`
    Limit      int            `json:"limit,omitempty"`
    Offset     int            `json:"offset,omitempty"`
}
```

## API Endpoints

### List Audit Logs

```
GET /api/v1/projects/{projectId}/audit-logs
```

Query parameters:
- `event_type` - Filter by event type
- `user_id` - Filter by user
- `spec_task_id` - Filter by task
- `start_date` - Filter by date range start
- `end_date` - Filter by date range end
- `search` - Full-text search on prompt_text
- `limit` - Page size (default 50, max 100)
- `offset` - Pagination offset

Response:
```json
{
  "logs": [...],
  "total": 1234,
  "limit": 50,
  "offset": 0
}
```

## Integration Points

### 1. Task Creation

In `spec_driven_task_service.go` - `CreateTaskFromPrompt`:

```go
// After task is created
s.auditLogService.LogEvent(ctx, &types.ProjectAuditLog{
    ProjectID:  task.ProjectID,
    SpecTaskID: task.ID,
    UserID:     task.CreatedBy,
    EventType:  types.AuditEventTaskCreated,
    PromptText: task.OriginalPrompt,
    Metadata: types.AuditMetadata{
        TaskNumber: task.TaskNumber,
        TaskName:   task.Name,
    },
})
```

### 2. Agent Prompts from Helix UI

In `spec_driven_task_service.go` - `SendUserPromptToAgent`:

```go
// When sending prompt to agent
s.auditLogService.LogEvent(ctx, &types.ProjectAuditLog{
    ProjectID:  task.ProjectID,
    SpecTaskID: task.ID,
    UserID:     userID,
    EventType:  types.AuditEventAgentPrompt,
    PromptText: prompt,
    Metadata: types.AuditMetadata{
        SessionID: sessionID,
    },
})
```

### 3. User Messages via WebSocket

In the WebSocket message handler (when user sends message from inside agent):

```go
// When receiving user message from agent WebSocket
auditLogService.LogEvent(ctx, &types.ProjectAuditLog{
    ProjectID:  projectID,
    SpecTaskID: taskID,
    UserID:     userID,
    EventType:  types.AuditEventUserMessage,
    PromptText: messageContent,
    Metadata: types.AuditMetadata{
        SessionID: sessionID,
    },
})
```

### 4. PR Creation

In PR detection/linking code:

```go
// When PR is created or linked
auditLogService.LogEvent(ctx, &types.ProjectAuditLog{
    ProjectID:  task.ProjectID,
    SpecTaskID: task.ID,
    UserID:     userID,
    EventType:  types.AuditEventPRCreated,
    Metadata: types.AuditMetadata{
        PullRequestID:  prID,
        PullRequestURL: prURL,
    },
})
```

### 5. Task Approval

In `ApproveSpecs`:

```go
auditLogService.LogEvent(ctx, &types.ProjectAuditLog{
    ProjectID:  task.ProjectID,
    SpecTaskID: task.ID,
    UserID:     approverID,
    EventType:  types.AuditEventTaskApproved,
})
```

## Frontend Components

### New Files

1. **`frontend/src/components/tasks/ProjectAuditTrail.tsx`**
   - Main audit trail table component
   - Pagination controls
   - Filter controls
   - Row click handler to expand/view full prompt

2. **`frontend/src/services/projectAuditService.ts`**
   - React Query hooks for fetching audit logs
   - `useProjectAuditLogs(projectId, filters)`

### Modified Files

1. **`frontend/src/pages/SpecTasksPage.tsx`** or **`SpecTaskKanbanBoard.tsx`**
   - Add toggle between Kanban and Audit Trail views
   - Render `ProjectAuditTrail` when audit view is active

## Implementation Plan

1. **Database & Types**
   - Create `ProjectAuditLog` type
   - Add GORM model and auto-migrate
   - Create store methods (Create, List with filters)

2. **Audit Log Service**
   - Create `AuditLogService` with `LogEvent` method
   - Fire-and-forget logging (don't block main operations)
   - Consider async channel for high-volume logging

3. **API Handlers**
   - Add `GET /api/v1/projects/{projectId}/audit-logs` endpoint
   - Add Swagger annotations
   - Update OpenAPI spec

4. **Integration Points**
   - Add audit logging calls to:
     - `CreateTaskFromPrompt`
     - `SendUserPromptToAgent`
     - WebSocket message handler
     - PR creation/linking
     - `ApproveSpecs`
     - Task completion

5. **Frontend**
   - Create React Query service
   - Create `ProjectAuditTrail` component
   - Add view toggle to Kanban page
   - Style the table with proper pagination

## Performance Considerations

1. **Write-Heavy Table**: Audit logs are append-only, optimize for writes
2. **Async Logging**: Use goroutines to avoid blocking main operations
3. **Pagination**: Always paginate, never load all logs at once
4. **Indexes**: Proper indexes on project_id, created_at, event_type
5. **Retention**: Consider adding data retention policy (e.g., archive after 1 year)

## Security Considerations

1. **Authorization**: Only users with project access can view audit logs
2. **Sensitive Data**: Prompt text may contain sensitive information - consider who can view
3. **User Privacy**: Email addresses visible - may need to restrict to project admins

## Future Enhancements

1. **Export**: CSV/JSON export of audit logs
2. **Webhooks**: Notify external systems of audit events
3. **Analytics**: Dashboard showing prompt statistics over time
4. **Retention Policies**: Auto-archive or delete old logs
5. **Search**: Full-text search with highlighting

### Developer Leaderboard (Potential Feature)

Aggregate audit data could power a developer leaderboard showing:

| Metric | Description |
|--------|-------------|
| Tasks Completed | Number of tasks moved to completed/merged |
| Avg Task Duration | Average time from task creation to PR merge |
| Prompts Sent | Total prompts sent to agents |
| First-Time Success Rate | % of tasks completed without restart/retry |
| PR Merge Rate | % of PRs that get merged vs abandoned |

**Considerations:**
- Could be useful for managers/execs to understand team velocity
- Could create unhealthy competition or gaming metrics
- Should be opt-in per organization
- Focus on celebrating efficiency, not just volume
- Consider team-level stats rather than individual to avoid singling out

### Background Code Quality Agents (Potential Feature)

Audit data could trigger background agents that actively counteract negative effects of vibe coding:

| Agent Type | Purpose |
|------------|---------|
| Duplication Detector | Analyze prompts and code changes for copy-paste patterns |
| Documentation Generator | Auto-generate docs for code that lacks comments |
| Maintenance Tracker | Flag technical debt accumulation trends |
| Test Coverage Monitor | Identify untested code paths from audit patterns |
| Refactoring Suggester | Propose improvements based on repeated patterns |

**Workflow:**
1. Audit log captures all prompts and code changes
2. Background agents analyze patterns asynchronously
3. Generate improvement suggestions as new spec tasks
4. Create "maintenance" or "cleanup" task recommendations
5. Surface in project dashboard or weekly digest

**Benefits:**
- Proactive code quality improvement
- Catches issues before they compound
- Learns from project-specific patterns
- Non-blocking - runs in background
