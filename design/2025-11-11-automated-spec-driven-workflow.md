# Automated Spec-Driven Workflow

**Date:** 2025-11-11
**Status:** 📋 PLANNED
**Effort:** Large (20-30 hours)
**Priority:** High - Core product workflow automation

## Problem Statement

Currently, the spec-driven task workflow requires too much manual intervention:

1. **Spec Approval → Implementation**: User clicks "Approve" but agent doesn't automatically start implementing
2. **Implementation → Review**: No automatic detection when agent finishes (pushes code to branch)
3. **Review Experience**: No integrated way to review implementation - no agent dialog, no browser preview
4. **Implementation Approval**: No "Approve Implementation" button that tells agent to merge their branch
5. **Completion Detection**: No automatic move to completed after merge
6. **Testing**: No clear path for user to test the merged feature on main branch

**User Impact:**
- Manual status changes cause delays
- Unclear what state each task is in
- Can't easily review implementation work
- No automated merge workflow
- Fragmented testing experience

## Current Workflow (Manual)

```
backlog
  ↓ [User clicks "Start Planning"]
spec_generation (agent generates specs)
  ↓ [Agent completes]
spec_review (user reviews in Design Review UI)
  ↓ [User clicks "Approve" in review dialog]
spec_approved
  ↓ [User manually starts implementation somehow?]
implementation_queued
  ↓ [???]
implementation (agent codes)
  ↓ [Agent pushes - how do we detect this?]
implementation_review
  ↓ [User reviews - where? how?]
done
  ↓ [How does user test the merged feature?]
```

## Proposed Workflow (Automated)

```
backlog
  ↓ [User clicks "Start Planning" on card]
spec_generation (agent generates specs, auto-commits to helix-specs branch)
  ↓ [Agent completes, creates design review]
spec_review (user reviews in Design Review UI)
  ↓ [User clicks "Approve Design" button in review dialog]
  ↓ → Backend: Updates status to implementation
  ↓ → Backend: Sends message to agent: "Create branch {branch_name} and start implementing"
  ↓ → Backend: Opens agent session if not already open
  ↓ → Frontend: Card auto-moves to "Implementing" column
  ↓ → Frontend: Card shows agent screenshot + "View Session" button
implementation (agent creates branch, codes, commits)
  ↓ [Agent completes, pushes branch to origin]
  ↓ → Backend: Git webhook OR polling detects push
  ↓ → Backend: Updates status to implementation_review
  ↓ → Frontend: Card auto-moves to "Review" column
  ↓ [User clicks "Review Implementation" button on card]
  ↓ → Opens agent dialog (SpecTaskDetailDialog)
  ↓ → Sends message to agent: "Start the web app for manual testing"
  ↓ → Agent starts dev server, outputs URL
  ↓ → User manually opens URL in browser
  ↓ → User tests implementation, gives feedback in chat
  ↓ [User clicks "Approve Implementation" button on card or in dialog]
  ↓ → Sends message to agent: "Merge your branch to main and push"
  ↓ → Agent: git checkout main && git merge {branch_name} && git push origin main
  ↓ [Backend detects merge to main (webhook or polling)]
  ↓ → Backend: Updates status to done
  ↓ → Frontend: Card auto-moves to "Completed" column
  ↓ → Card shows: "Test on main: [Start Exploratory Session] button"
done
  ↓ [User can start exploratory session to test merged feature]
```

## Implementation Plan

### Phase 1: Backend Foundation (8-10 hours)

#### 1.1: Extend SpecTask Schema (1h)
**File**: `api/pkg/types/simple_spec_task.go`

Add fields for tracking git state:
```go
type SpecTask struct {
    // ... existing fields ...

    // Git tracking
    BranchName              string     `json:"branch_name,omitempty"`              // Feature branch name
    LastPushCommitHash      string     `json:"last_push_commit_hash,omitempty"`    // Last commit hash pushed
    LastPushAt              *time.Time `json:"last_push_at,omitempty"`             // When branch was last pushed
    MergedToMain            bool       `json:"merged_to_main" gorm:"default:false"` // Whether branch was merged
    MergedAt                *time.Time `json:"merged_at,omitempty"`                // When merge happened
    MergeCommitHash         string     `json:"merge_commit_hash,omitempty"`        // Merge commit hash

    // Implementation approval tracking
    ImplementationApprovedBy string     `json:"implementation_approved_by,omitempty"` // User who approved implementation
    ImplementationApprovedAt *time.Time `json:"implementation_approved_at,omitempty"`
}
```

Run: `db.AutoMigrate(&SpecTask{})` in `api/pkg/store/store.go`

#### 1.2: Create Git Event Detection Service (3-4h)
**File**: `api/pkg/services/git_event_service.go`

Options for detecting git pushes/merges:
1. **Polling** (simpler, works immediately)
   - Every 30 seconds, check tasks in `implementation` or `implementation_review` status
   - For each task, run `git fetch` and check if branch exists remotely
   - Compare remote commit hash with `last_push_commit_hash`
   - If different → push detected

2. **Git Webhooks** (better, requires setup)
   - For internal repos: Helix can expose `/api/v1/git/webhooks/push` endpoint
   - For GitHub/GitLab: Configure webhook in repo settings
   - On push event, update relevant spec task

**Implementation** (polling for MVP):
```go
type GitEventService struct {
    store      store.Store
    pollInterval time.Duration
    gitBasePath  string
}

func (s *GitEventService) Start(ctx context.Context) {
    ticker := time.NewTicker(s.pollInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.checkForGitEvents(ctx)
        }
    }
}

func (s *GitEventService) checkForGitEvents(ctx context.Context) {
    // Get all tasks in implementation or implementation_review
    tasks, err := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
        Status: "", // Get all, filter in code
    })

    for _, task := range tasks {
        if task.Status != "implementation" && task.Status != "implementation_review" {
            continue
        }

        // Check for push
        if task.Status == "implementation" {
            s.checkForBranchPush(ctx, &task)
        }

        // Check for merge
        if task.Status == "implementation_review" {
            s.checkForMergeToMain(ctx, &task)
        }
    }
}

func (s *GitEventService) checkForBranchPush(ctx context.Context, task *types.SpecTask) error {
    // Get project and repo
    project, _ := s.store.GetProject(ctx, task.ProjectID)
    repo, _ := s.store.GetGitRepository(ctx, project.DefaultRepoID)

    // Fetch latest from remote
    repoPath := repo.LocalPath
    cmd := exec.Command("git", "-C", repoPath, "fetch", "origin", task.BranchName)
    cmd.Run()

    // Get remote commit hash
    cmd = exec.Command("git", "-C", repoPath, "rev-parse", "origin/"+task.BranchName)
    output, err := cmd.Output()
    if err != nil {
        return nil // Branch doesn't exist remotely yet
    }

    remoteHash := strings.TrimSpace(string(output))

    // Compare with stored hash
    if remoteHash != task.LastPushCommitHash {
        log.Info().
            Str("task_id", task.ID).
            Str("branch", task.BranchName).
            Str("commit", remoteHash).
            Msg("Detected push to feature branch")

        // Update task
        task.LastPushCommitHash = remoteHash
        now := time.Now()
        task.LastPushAt = &now
        task.Status = types.TaskStatusImplementationReview
        s.store.UpdateSpecTask(ctx, task)
    }

    return nil
}

func (s *GitEventService) checkForMergeToMain(ctx context.Context, task *types.SpecTask) error {
    // Get project and repo
    project, _ := s.store.GetProject(ctx, task.ProjectID)
    repo, _ := s.store.GetGitRepository(ctx, project.DefaultRepoID)

    repoPath := repo.LocalPath

    // Fetch latest main
    exec.Command("git", "-C", repoPath, "fetch", "origin", repo.DefaultBranch).Run()

    // Check if branch is merged into main
    // git branch --merged origin/main | grep branch_name
    cmd := exec.Command("git", "-C", repoPath, "branch", "-r", "--merged", "origin/"+repo.DefaultBranch)
    output, err := cmd.Output()
    if err != nil {
        return err
    }

    if strings.Contains(string(output), "origin/"+task.BranchName) {
        log.Info().
            Str("task_id", task.ID).
            Str("branch", task.BranchName).
            Msg("Detected merge to main branch")

        // Get merge commit hash
        cmd = exec.Command("git", "-C", repoPath, "rev-parse", "origin/"+repo.DefaultBranch)
        hashOutput, _ := cmd.Output()

        // Update task
        task.MergedToMain = true
        now := time.Now()
        task.MergedAt = &now
        task.MergeCommitHash = strings.TrimSpace(string(hashOutput))
        task.Status = types.TaskStatusDone
        task.CompletedAt = &now
        s.store.UpdateSpecTask(ctx, task)
    }

    return nil
}
```

Start service in `api/pkg/server/server.go`:
```go
gitEventService := services.NewGitEventService(s.Store, 30*time.Second, "/workspace/git")
go gitEventService.Start(ctx)
```

#### 1.3: Agent Messaging Service (2-3h)
**File**: `api/pkg/services/agent_instruction_service.go`

Service for sending automated messages to agent sessions:
```go
type AgentInstructionService struct {
    controller *controller.Controller
}

func (s *AgentInstructionService) SendApprovalInstruction(
    ctx context.Context,
    sessionID string,
    branchName string,
    baseBranch string,
) error {
    message := fmt.Sprintf(`# Design Approved! 🎉

Your design has been approved. Please begin implementation:

**Steps:**
1. Create and checkout feature branch: \`git checkout -b %s\`
2. Implement the features according to the approved design
3. Write tests for all new functionality
4. Commit your work with clear messages
5. When ready, push: \`git push origin %s\`

I'll be watching for your push and will notify you when it's time for review.
`, branchName, branchName)

    return s.sendMessage(ctx, sessionID, message)
}

func (s *AgentInstructionService) SendImplementationReviewRequest(
    ctx context.Context,
    sessionID string,
    branchName string,
) error {
    message := fmt.Sprintf(`# Implementation Review 🔍

Great work pushing your changes! The implementation is now ready for review.

The user will test your work. If this is a web application, please:

1. Start the development server
2. Provide the URL where the user can test
3. Answer any questions about your implementation

Branch: \`%s\`
`, branchName)

    return s.sendMessage(ctx, sessionID, message)
}

func (s *AgentInstructionService) SendMergeInstruction(
    ctx context.Context,
    sessionID string,
    branchName string,
    baseBranch string,
) error {
    message := fmt.Sprintf(`# Implementation Approved! ✅

Your implementation has been approved. Please merge to main:

**Steps:**
1. \`git checkout %s\`
2. \`git pull origin %s\` (ensure up to date)
3. \`git merge %s\`
4. \`git push origin %s\`

Let me know once the merge is complete!
`, baseBranch, baseBranch, branchName, baseBranch)

    return s.sendMessage(ctx, sessionID, message)
}

func (s *AgentInstructionService) sendMessage(ctx context.Context, sessionID string, message string) error {
    // Use controller to send interaction
    interaction := &types.Interaction{
        ID:           system.GenerateUUID(),
        Created:      time.Now(),
        Updated:      time.Now(),
        Scheduled:    time.Now(),
        Completed:    time.Now(),
        Creator:      types.CreatorTypeSystem,
        Mode:         types.SessionModeInference,
        Message:      message,
        State:        types.InteractionStateComplete,
    }

    session, err := s.controller.GetSession(ctx, sessionID)
    if err != nil {
        return err
    }

    session.Interactions = append(session.Interactions, interaction)
    return s.controller.UpdateSession(ctx, *session)
}
```

#### 1.4: API Endpoints (2-3h)

**File**: `api/pkg/server/spec_task_workflow_handlers.go`

```go
// approveDesignAndStartImplementation - called when user approves design
// @Summary Approve design and start implementation
// @Description Approve the design review and automatically start implementation phase
// @Tags spec-tasks
// @Param spec_task_id path string true "SpecTask ID"
// @Param review_id path string true "Review ID"
// @Success 200 {object} types.SpecTaskWorkflowResponse
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/approve-and-implement [post]
// @Security BearerAuth
func (s *HelixAPIServer) approveDesignAndStartImplementation(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    user := getRequestUser(r)
    vars := mux.Vars(r)
    specTaskID := vars["spec_task_id"]
    reviewID := vars["review_id"]

    // Get spec task
    specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Authorize
    if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, types.ActionUpdate); err != nil {
        http.Error(w, "Not authorized", http.StatusForbidden)
        return
    }

    // Mark review as approved
    review, err := s.Store.GetSpecTaskDesignReview(ctx, reviewID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    review.Status = "approved"
    review.ReviewedBy = user.ID
    now := time.Now()
    review.ReviewedAt = &now
    s.Store.UpdateSpecTaskDesignReview(ctx, review)

    // Update spec task
    project, _ := s.Store.GetProject(ctx, specTask.ProjectID)
    repo, _ := s.Store.GetGitRepository(ctx, project.DefaultRepoID)

    branchName := generateFeatureBranchName(specTask)
    specTask.Status = types.TaskStatusImplementation
    specTask.BranchName = branchName
    specTask.SpecApprovedBy = user.ID
    specTask.SpecApprovedAt = &now
    specTask.StartedAt = &now

    if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Send instruction to agent
    if specTask.ImplementationSessionID != "" {
        s.AgentInstructionService.SendApprovalInstruction(
            ctx,
            specTask.ImplementationSessionID,
            branchName,
            repo.DefaultBranch,
        )
    } else if specTask.PlanningSessionID != "" {
        // Use planning session if implementation session not created yet
        s.AgentInstructionService.SendApprovalInstruction(
            ctx,
            specTask.PlanningSessionID,
            branchName,
            repo.DefaultBranch,
        )
    }

    // Return response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(&types.SpecTaskWorkflowResponse{
        TaskID:     specTaskID,
        Status:     specTask.Status,
        BranchName: branchName,
        Message:    "Design approved. Agent has been instructed to start implementation.",
    })
}

// approveImplementationAndMerge - called when user approves implementation
// @Summary Approve implementation and merge to main
// @Description Approve the implementation and instruct agent to merge to main branch
// @Tags spec-tasks
// @Param spec_task_id path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskWorkflowResponse
// @Router /api/v1/spec-tasks/{spec_task_id}/approve-implementation [post]
// @Security BearerAuth
func (s *HelixAPIServer) approveImplementationAndMerge(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    user := getRequestUser(r)
    vars := mux.Vars(r)
    specTaskID := vars["spec_task_id"]

    // Get spec task
    specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Authorize
    if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, types.ActionUpdate); err != nil {
        http.Error(w, "Not authorized", http.StatusForbidden)
        return
    }

    // Verify status
    if specTask.Status != types.TaskStatusImplementationReview {
        http.Error(w, "Task must be in implementation_review status", http.StatusBadRequest)
        return
    }

    // Update task
    now := time.Now()
    specTask.ImplementationApprovedBy = user.ID
    specTask.ImplementationApprovedAt = &now
    // Keep in implementation_review - will move to done when merge detected

    if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Get repo info
    project, _ := s.Store.GetProject(ctx, specTask.ProjectID)
    repo, _ := s.Store.GetGitRepository(ctx, project.DefaultRepoID)

    // Send merge instruction to agent
    if specTask.ImplementationSessionID != "" {
        s.AgentInstructionService.SendMergeInstruction(
            ctx,
            specTask.ImplementationSessionID,
            specTask.BranchName,
            repo.DefaultBranch,
        )
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(&types.SpecTaskWorkflowResponse{
        TaskID:     specTaskID,
        Status:     specTask.Status,
        BranchName: specTask.BranchName,
        Message:    "Implementation approved. Agent has been instructed to merge to main.",
    })
}

// stopAgentSession - stop the agent session for a spec task
// @Summary Stop agent session
// @Description Stop the running agent session for a spec task
// @Tags spec-tasks
// @Param spec_task_id path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskWorkflowResponse
// @Router /api/v1/spec-tasks/{spec_task_id}/stop-agent [post]
// @Security BearerAuth
func (s *HelixAPIServer) stopAgentSession(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    user := getRequestUser(r)
    vars := mux.Vars(r)
    specTaskID := vars["spec_task_id"]

    specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, types.ActionUpdate); err != nil {
        http.Error(w, "Not authorized", http.StatusForbidden)
        return
    }

    // Stop external agent if exists
    if specTask.ExternalAgentID != "" {
        // TODO: Call wolf executor to stop the agent
        log.Info().Str("external_agent_id", specTask.ExternalAgentID).Msg("Stopping external agent")
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(&types.SpecTaskWorkflowResponse{
        TaskID:  specTaskID,
        Message: "Agent session stopped",
    })
}
```

Register routes in `api/pkg/server/server.go`:
```go
// Spec task workflow routes
specTasksSubRouter.HandleFunc("/{spec_task_id}/design-reviews/{review_id}/approve-and-implement", s.middleware(s.approveDesignAndStartImplementation, true)).Methods("POST")
specTasksSubRouter.HandleFunc("/{spec_task_id}/approve-implementation", s.middleware(s.approveImplementationAndMerge, true)).Methods("POST")
specTasksSubRouter.HandleFunc("/{spec_task_id}/stop-agent", s.middleware(s.stopAgentSession, true)).Methods("POST")
```

**Types** (`api/pkg/types/spec_task_workflow.go`):
```go
type SpecTaskWorkflowResponse struct {
    TaskID     string `json:"task_id"`
    Status     string `json:"status"`
    BranchName string `json:"branch_name,omitempty"`
    Message    string `json:"message"`
}
```

### Phase 2: Frontend Integration (8-12 hours)

#### 2.1: Update Generated TypeScript Client (15 min)
```bash
./stack update_openapi
```

#### 2.2: Create React Query Hooks (1h)
**File**: `frontend/src/services/specTaskWorkflowService.ts`

```typescript
import { useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'
import { specTaskQueryKey } from './specTaskService'

export function useApproveDesignAndImplement(specTaskId: string, reviewId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  const snackbar = useSnackbar()

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1SpecTasksDesignReviewsApproveAndImplementCreate(
        specTaskId,
        reviewId
      )
      return response.data
    },
    onSuccess: (data) => {
      snackbar.success('Design approved! Agent starting implementation...')
      // Invalidate queries to refetch task
      queryClient.invalidateQueries({ queryKey: specTaskQueryKey(specTaskId) })
      queryClient.invalidateQueries({ queryKey: ['spec-tasks'] })
    },
    onError: (error: any) => {
      snackbar.error(error?.response?.data?.message || 'Failed to approve design')
    },
  })
}

export function useApproveImplementation(specTaskId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  const snackbar = useSnackbar()

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1SpecTasksApproveImplementationCreate(specTaskId)
      return response.data
    },
    onSuccess: () => {
      snackbar.success('Implementation approved! Agent will merge to main...')
      queryClient.invalidateQueries({ queryKey: specTaskQueryKey(specTaskId) })
      queryClient.invalidateQueries({ queryKey: ['spec-tasks'] })
    },
    onError: (error: any) => {
      snackbar.error(error?.response?.data?.message || 'Failed to approve implementation')
    },
  })
}

export function useStopAgent(specTaskId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  const snackbar = useSnackbar()

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1SpecTasksStopAgentCreate(specTaskId)
      return response.data
    },
    onSuccess: () => {
      snackbar.success('Agent session stopped')
      queryClient.invalidateQueries({ queryKey: specTaskQueryKey(specTaskId) })
    },
    onError: (error: any) => {
      snackbar.error(error?.response?.data?.message || 'Failed to stop agent')
    },
  })
}
```

#### 2.3: Update DesignReviewViewer "Approve" Button (30 min)
**File**: `frontend/src/components/spec-tasks/DesignReviewViewer.tsx`

Replace the current submit review logic with the new workflow:

```typescript
import { useApproveDesignAndImplement } from '../../services/specTaskWorkflowService'

export default function DesignReviewViewer({ ... }) {
  const approveAndImplementMutation = useApproveDesignAndImplement(specTaskId, reviewId)

  const handleApprove = async () => {
    try {
      await approveAndImplementMutation.mutateAsync()
      onClose() // Close the review dialog
      // Card will auto-update via query invalidation
    } catch (err) {
      console.error('Approval failed:', err)
    }
  }

  // Update Approve button
  return (
    <Button
      variant="contained"
      color="success"
      startIcon={<CheckCircleIcon />}
      onClick={handleApprove}
      disabled={approveAndImplementMutation.isPending}
    >
      {approveAndImplementMutation.isPending ? 'Approving...' : 'Approve Design & Start Implementation'}
    </Button>
  )
}
```

#### 2.4: Update Kanban Card UI (3-4h)
**File**: `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`

Add new buttons to cards based on status:

```typescript
const TaskCard: React.FC<{...}> = ({ task, ... }) => {
  const [reviewDialogOpen, setReviewDialogOpen] = useState(false)
  const approveImplementationMutation = useApproveImplementation(task.id)
  const stopAgentMutation = useStopAgent(task.id)

  const renderActionButtons = () => {
    switch (task.status) {
      case 'backlog':
        return (
          <Button
            size="small"
            variant="contained"
            startIcon={<PlayIcon />}
            onClick={handleStartPlanning}
            disabled={isPlanningFull}
          >
            Start Planning
          </Button>
        )

      case 'spec_review':
        return (
          <Button
            size="small"
            variant="contained"
            color="info"
            startIcon={<ViewIcon />}
            onClick={() => {
              // Open design review viewer
              setDocViewerOpen(true)
            }}
          >
            Review Documents
          </Button>
        )

      case 'implementation':
        return (
          <Box sx={{ display: 'flex', gap: 1, flexDirection: 'column' }}>
            <Button
              size="small"
              variant="outlined"
              startIcon={<ViewIcon />}
              onClick={() => setReviewDialogOpen(true)}
            >
              View Agent Session
            </Button>
            <Button
              size="small"
              variant="outlined"
              color="error"
              onClick={() => stopAgentMutation.mutate()}
            >
              Stop Agent
            </Button>
          </Box>
        )

      case 'implementation_review':
        return (
          <Box sx={{ display: 'flex', gap: 1, flexDirection: 'column' }}>
            <Button
              size="small"
              variant="contained"
              color="primary"
              startIcon={<ViewIcon />}
              onClick={() => setReviewDialogOpen(true)}
            >
              Review Implementation
            </Button>
            <Button
              size="small"
              variant="contained"
              color="success"
              startIcon={<CheckCircleIcon />}
              onClick={() => approveImplementationMutation.mutate()}
              disabled={approveImplementationMutation.isPending}
            >
              Approve Implementation
            </Button>
            <Button
              size="small"
              variant="outlined"
              color="error"
              onClick={() => stopAgentMutation.mutate()}
            >
              Stop Agent
            </Button>
          </Box>
        )

      case 'done':
        return (
          <Alert severity="success" sx={{ mt: 1 }}>
            <Typography variant="caption">
              Merged to main! Test the feature:
            </Typography>
            <Button
              size="small"
              variant="outlined"
              sx={{ mt: 0.5 }}
              onClick={() => {
                // TODO: Start exploratory session on main branch
              }}
            >
              Start Exploratory Session
            </Button>
          </Alert>
        )

      default:
        return null
    }
  }

  return (
    <Card>
      {/* ... existing card content ... */}

      <CardActions>
        {renderActionButtons()}
      </CardActions>

      {/* Review Implementation Dialog */}
      {reviewDialogOpen && (
        <SpecTaskDetailDialog
          task={task}
          open={reviewDialogOpen}
          onClose={() => setReviewDialogOpen(false)}
        />
      )}
    </Card>
  )
}
```

#### 2.5: Update SpecTaskDetailDialog for Review (2-3h)
**File**: `frontend/src/components/tasks/SpecTaskDetailDialog.tsx`

When opened for implementation review, automatically send a message to the agent:

```typescript
const SpecTaskDetailDialog: FC<SpecTaskDetailDialogProps> = ({
  task,
  open,
  onClose,
}) => {
  const [initialMessageSent, setInitialMessageSent] = useState(false)
  const streaming = useStreaming()

  // Auto-send review request message when dialog opens for implementation_review
  useEffect(() => {
    if (
      open &&
      !initialMessageSent &&
      task?.status === 'implementation_review' &&
      task?.implementation_session_id
    ) {
      const reviewMessage = `I'm here to review your implementation.

If this is a web application, please start the development server and provide the URL where I can test it.

I'll give you feedback and we can iterate on any changes needed.`

      streaming.NewInference({
        type: SESSION_TYPE_TEXT,
        message: reviewMessage,
        sessionId: task.implementation_session_id,
      }).then(() => {
        setInitialMessageSent(true)
      })
    }
  }, [open, initialMessageSent, task?.status, task?.implementation_session_id])

  return (
    // ... rest of component
  )
}
```

#### 2.6: Column Definitions Update (30 min)
**File**: `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`

Update column definitions to match new statuses:

```typescript
const getPhaseFromStatus = (status: string): SpecTaskPhase => {
  switch (status) {
    case 'backlog':
      return 'backlog'
    case 'spec_generation':
      return 'planning'
    case 'spec_review':
    case 'spec_revision':
      return 'review'
    case 'spec_approved':
    case 'implementation':
    case 'implementation_review':
      return 'implementation'
    case 'done':
      return 'completed'
    default:
      return 'backlog'
  }
}

const columns: KanbanColumn[] = [
  {
    id: 'backlog',
    title: 'Backlog',
    description: 'Tasks waiting to start',
    // ...
  },
  {
    id: 'planning',
    title: 'Planning',
    description: 'Agent generating specs',
    // ...
  },
  {
    id: 'review',
    title: 'Design Review',
    description: 'Awaiting design approval',
    // ...
  },
  {
    id: 'implementation',
    title: 'Implementing',
    description: 'Agent coding + Review ready',
    // Shows both implementation and implementation_review
    // ...
  },
  {
    id: 'completed',
    title: 'Completed',
    description: 'Merged to main',
    // ...
  },
]
```

### Phase 3: Polish & Testing (4-8 hours)

#### 3.1: Add Status Indicators (1-2h)
Show substatus on cards for clarity:

```typescript
const getSubstatus = (task: SpecTask): string | null => {
  switch (task.status) {
    case 'spec_generation':
      return 'Generating specs...'
    case 'spec_review':
      return 'Awaiting design approval'
    case 'implementation':
      return task.last_push_at
        ? 'Push detected - ready for review'
        : 'Agent coding...'
    case 'implementation_review':
      return task.implementation_approved_at
        ? 'Merging to main...'
        : 'Awaiting approval'
    case 'done':
      return task.merged_at
        ? `Merged ${formatDistanceToNow(new Date(task.merged_at))} ago`
        : 'Completed'
    default:
      return null
  }
}
```

#### 3.2: Real-time Updates via Polling (2-3h)
Add polling for tasks in active states:

```typescript
const { data: tasks } = useQuery({
  queryKey: ['spec-tasks', projectId],
  queryFn: () => apiClient.v1SpecTasksList({ project_id: projectId }),
  refetchInterval: (data) => {
    // Poll every 5s if any tasks are in active states
    const hasActiveTasks = data?.some(task =>
      ['spec_generation', 'implementation', 'implementation_review'].includes(task.status)
    )
    return hasActiveTasks ? 5000 : false
  },
})
```

#### 3.3: Archive Button (1h)
Add archive button to all cards:

```typescript
<IconButton
  size="small"
  onClick={(e) => {
    e.stopPropagation()
    onArchiveTask?.(task, !task.archived)
  }}
  title={task.archived ? "Restore" : "Archive"}
>
  {task.archived ? <RestoreIcon /> : <DeleteIcon />}
</IconButton>
```

#### 3.4: Integration Testing (2-4h)
1. Test complete flow end-to-end:
   - Start planning → generates specs
   - Approve design → agent starts implementing
   - Agent pushes → card moves to review
   - Review implementation → test in browser
   - Approve → agent merges
   - Detect merge → card moves to done

2. Test error cases:
   - Agent fails during implementation
   - Merge conflicts
   - Network errors during git operations

3. Test UI responsiveness:
   - Cards update quickly after status changes
   - No UI flicker or jumps
   - Buttons disabled during operations

## Technical Decisions

### Git Detection: Polling vs Webhooks
**Decision**: Start with **polling** (30s interval)
**Rationale**:
- Simpler implementation
- Works immediately without webhook setup
- Only polls tasks in relevant statuses (low overhead)
- Can add webhooks later for real-time updates

### Agent Communication
**Decision**: Send messages via session interactions
**Rationale**:
- Consistent with existing streaming architecture
- Agent sees messages in natural chat flow
- User can see system messages in session viewer

### Status Substates
**Decision**: Use single `status` field, not separate state machine
**Rationale**:
- Simpler database schema
- Existing status constants cover all states
- Frontend can derive substatus from git fields

## Migration Plan

1. **Backend First** (no frontend dependency):
   - Add git fields to SpecTask (auto-migration)
   - Create GitEventService (runs independently)
   - Create AgentInstructionService
   - Add new API endpoints

2. **Frontend Update**:
   - Generate TypeScript client
   - Add React Query hooks
   - Update DesignReviewViewer
   - Update Kanban cards

3. **Testing**:
   - Test with single task end-to-end
   - Enable for all users

## Success Criteria

✅ **Phase 1 Complete When:**
- Git polling service detects pushes/merges
- Agent receives automated messages at workflow transitions
- API endpoints work correctly

✅ **Phase 2 Complete When:**
- Approving design automatically starts implementation
- Card moves to review column when push detected
- "Review Implementation" button opens agent dialog
- "Approve Implementation" button triggers merge
- Card moves to completed when merge detected

✅ **Phase 3 Complete When:**
- No regressions in existing workflows
- Real-time updates feel responsive
- Error states handled gracefully
- Users can test features via exploratory sessions

## Timeline Estimate

**Total: 20-30 hours**

- Phase 1 (Backend): 8-10 hours
- Phase 2 (Frontend): 8-12 hours
- Phase 3 (Polish): 4-8 hours

**Suggested approach:**
- Do Phase 1 completely (backend foundation)
- Do Phase 2 incrementally (one workflow step at a time)
- Do Phase 3 as each workflow step is tested

## Future Enhancements (Out of Scope)

These could be added later without breaking changes:

1. **GitHub/GitLab PR Integration**: Instead of direct merge, create PR
2. **Code Review Tools**: Inline comments, diff viewer
3. **Automated Tests**: Run tests before allowing approval
4. **Deploy Preview**: Auto-deploy branch for testing
5. **Rollback**: Ability to revert merge if issues found
6. **Multi-Reviewer Approval**: Require N approvals
7. **CI/CD Integration**: Show build status on cards
8. **Slack/Email Notifications**: Notify when review needed

## Related Work

- **Design Review System**: Already implemented, this builds on it
- **Multi-Session Manager**: Handles agent sessions across workflow
- **Git Repository Service**: Manages git operations
- **Wolf Executor**: Runs external agents (Zed)

## Questions to Resolve Before Starting

1. **Branch Naming**: Auto-generate or let agent choose?
   - **Recommendation**: Auto-generate (predictable, prevents conflicts)

2. **Merge Strategy**: Direct merge or PR?
   - **Recommendation**: Direct merge for MVP, PR later with GitHub integration

3. **Conflict Resolution**: What if merge conflicts?
   - **Recommendation**: Tell agent to resolve, or fail and notify user

4. **Agent Restart**: If agent crashes during implementation?
   - **Recommendation**: User can manually restart, session persists

5. **Multiple Pushes**: What if agent pushes multiple times?
   - **Recommendation**: Only transition to review on first push, subsequent pushes update hash

## Conclusion

This automated workflow removes manual steps and creates a seamless experience from design approval through implementation to merge. The incremental approach allows testing each workflow transition independently, reducing risk.

**Recommendation:** Start implementation immediately. Begin with Phase 1 (backend) which has no frontend dependencies and provides foundation for all subsequent work.
