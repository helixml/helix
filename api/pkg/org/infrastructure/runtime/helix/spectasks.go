package helix

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// SpecTaskStore is the slice of the Helix store the spec-task runtime
// impl needs. *helixstore.Store satisfies it structurally, so the server
// wires the real store with no adapter. Keeping it an interface keeps
// this package free of a direct helix-store import and makes the impl
// unit-testable with a fake.
type SpecTaskStore interface {
	CreateSpecTask(ctx context.Context, task *types.SpecTask) error
	GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error)
	ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error)
	UpdateSpecTask(ctx context.Context, task *types.SpecTask) error
	GetProject(ctx context.Context, id string) (*types.Project, error)
	IncrementGlobalTaskNumber(ctx context.Context) (int, error)
}

// SpecTaskWorkflow is the service-level slice the impl delegates the
// two genuinely-orchestrated verbs to. The server wires it from
// services.SpecDrivenTaskService (ApproveSpecs) and the HelixAPIServer's
// ensurePullRequestsForAllRepos method (EnsurePullRequests). Both reuse
// the exact code the REST UI drives.
type SpecTaskWorkflow interface {
	ApproveSpecs(ctx context.Context, task *types.SpecTask) error
	EnsurePullRequests(ctx context.Context, task *types.SpecTask, primaryRepoID, userID string) error
}

// SpecTasks is the helix-runtime implementation of runtime.SpecTasks. It
// resolves a workerID → Helix projectID via the WorkerRuntimeState
// sidecar (the same mechanism ProjectConfig uses), enforces that every
// referenced task belongs to that project, and delegates to the Helix
// store + workflow service. No existing helix code is modified.
type SpecTasks struct {
	orgStore *store.Store
	tasks    SpecTaskStore
	workflow SpecTaskWorkflow
}

// NewSpecTasks builds the impl. All three collaborators are required —
// there is no degraded mode that makes sense.
func NewSpecTasks(orgStore *store.Store, tasks SpecTaskStore, workflow SpecTaskWorkflow) (*SpecTasks, error) {
	if orgStore == nil {
		return nil, errors.New("helix.NewSpecTasks: org store is nil")
	}
	if tasks == nil {
		return nil, errors.New("helix.NewSpecTasks: spec task store is nil")
	}
	if workflow == nil {
		return nil, errors.New("helix.NewSpecTasks: workflow is nil")
	}
	return &SpecTasks{orgStore: orgStore, tasks: tasks, workflow: workflow}, nil
}

var _ runtime.SpecTasks = (*SpecTasks)(nil)

// project resolves the worker's project ID and hiring user from runtime
// state. Returns ErrSpecTasksUnsupported when the worker has no project
// (hired against a different runtime / not yet activated).
func (s *SpecTasks) project(ctx context.Context, orgID string, workerID orgchart.BotID) (projectID, hiringUserID string, err error) {
	state, err := LoadState(ctx, s.orgStore, orgID, workerID)
	if err != nil {
		return "", "", fmt.Errorf("load worker state: %w", err)
	}
	if state.ProjectID == "" {
		return "", "", fmt.Errorf("worker %s: %w", workerID, runtime.ErrSpecTasksUnsupported)
	}
	return state.ProjectID, state.HiringUserID, nil
}

// ownedTask fetches a task and verifies it belongs to the caller's
// project. Prevents a Worker from touching another project's tasks.
func (s *SpecTasks) ownedTask(ctx context.Context, projectID, taskID string) (*types.SpecTask, error) {
	task, err := s.tasks.GetSpecTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get spec task: %w", err)
	}
	if task.ProjectID != projectID {
		return nil, fmt.Errorf("task %s does not belong to this worker's project", taskID)
	}
	return task, nil
}

func (s *SpecTasks) Create(ctx context.Context, orgID string, workerID orgchart.BotID, in runtime.CreateSpecTaskInput) (runtime.SpecTaskView, error) {
	projectID, hiringUserID, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return runtime.SpecTaskView{}, errors.New("name is required")
	}
	if strings.TrimSpace(in.Description) == "" {
		return runtime.SpecTaskView{}, errors.New("description is required")
	}
	project, err := s.tasks.GetProject(ctx, projectID)
	if err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("get project: %w", err)
	}

	taskType := in.Type
	if taskType == "" {
		taskType = "feature"
	}
	priority := types.SpecTaskPriority(in.Priority)
	if priority == "" {
		priority = types.SpecTaskPriorityMedium
	}
	var dependsOn []types.SpecTask
	for _, id := range in.DependsOn {
		id = strings.TrimSpace(id)
		if id != "" {
			dependsOn = append(dependsOn, types.SpecTask{ID: id})
		}
	}

	now := time.Now()
	task := &types.SpecTask{
		ID:             system.GenerateSpecTaskID(),
		ProjectID:      projectID,
		UserID:         hiringUserID,
		OrganizationID: project.OrganizationID,
		Name:           in.Name,
		Description:    in.Description,
		Type:           taskType,
		Priority:       priority,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: in.OriginalPrompt,
		DependsOn:      dependsOn,
		JustDoItMode:   in.SkipPlanning,
		CreatedBy:      hiringUserID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	taskNumber, err := s.tasks.IncrementGlobalTaskNumber(ctx)
	if err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("assign task number: %w", err)
	}
	task.TaskNumber = taskNumber
	task.DesignDocPath = generateDesignDocPath(task.Name, taskNumber)

	if err := s.tasks.CreateSpecTask(ctx, task); err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("create spec task: %w", err)
	}
	return toView(task), nil
}

func (s *SpecTasks) List(ctx context.Context, orgID string, workerID orgchart.BotID, filter runtime.ListSpecTasksFilter) ([]runtime.SpecTaskView, error) {
	projectID, _, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.tasks.ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID: projectID,
		Status:    types.SpecTaskStatus(filter.Status),
		Type:      filter.Type,
		Priority:  filter.Priority,
	})
	if err != nil {
		return nil, fmt.Errorf("list spec tasks: %w", err)
	}
	out := make([]runtime.SpecTaskView, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, toView(t))
	}
	return out, nil
}

func (s *SpecTasks) Get(ctx context.Context, orgID string, workerID orgchart.BotID, taskID string) (runtime.SpecTaskView, error) {
	projectID, _, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	task, err := s.ownedTask(ctx, projectID, taskID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return toView(task), nil
}

func (s *SpecTasks) StartPlanning(ctx context.Context, orgID string, workerID orgchart.BotID, taskID string) (runtime.SpecTaskView, error) {
	projectID, _, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	task, err := s.ownedTask(ctx, projectID, taskID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	// Mirror the Optimus skill's StartSpecTask: queue for the orchestrator,
	// which performs the actual spec generation / implementation kickoff.
	if task.JustDoItMode {
		task.Status = types.TaskStatusQueuedImplementation
	} else {
		task.Status = types.TaskStatusQueuedSpecGeneration
	}
	task.UpdatedAt = time.Now()
	if err := s.tasks.UpdateSpecTask(ctx, task); err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("start planning: %w", err)
	}
	return toView(task), nil
}

func (s *SpecTasks) ReviewSpec(ctx context.Context, orgID string, workerID orgchart.BotID, taskID string) (runtime.SpecReviewView, error) {
	projectID, _, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return runtime.SpecReviewView{}, err
	}
	task, err := s.ownedTask(ctx, projectID, taskID)
	if err != nil {
		return runtime.SpecReviewView{}, err
	}
	if task.Status == types.TaskStatusBacklog || task.Status == types.TaskStatusSpecGeneration {
		return runtime.SpecReviewView{}, fmt.Errorf("specifications not yet generated for task %s", taskID)
	}
	return runtime.SpecReviewView{
		TaskID:       task.ID,
		Status:       string(task.Status),
		Requirements: task.RequirementsSpec,
		Design:       task.TechnicalDesign,
		Tasks:        task.ImplementationPlan,
	}, nil
}

func (s *SpecTasks) ApproveSpec(ctx context.Context, orgID string, workerID orgchart.BotID, taskID string) (runtime.SpecTaskView, error) {
	projectID, hiringUserID, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	task, err := s.ownedTask(ctx, projectID, taskID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	// Stamp the approver (the worker's hiring user) and persist before
	// delegating: ApproveSpecs reads SpecApprovedBy/At to build the
	// approval record.
	now := time.Now()
	task.SpecApprovedBy = hiringUserID
	task.SpecApprovedAt = &now
	task.Status = types.TaskStatusSpecApproved
	task.UpdatedAt = now
	if err := s.tasks.UpdateSpecTask(ctx, task); err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("approve spec (persist): %w", err)
	}
	if err := s.workflow.ApproveSpecs(ctx, task); err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("approve spec: %w", err)
	}
	// Re-read so the view reflects whatever the workflow advanced it to.
	if latest, gErr := s.tasks.GetSpecTask(ctx, taskID); gErr == nil {
		task = latest
	}
	return toView(task), nil
}

func (s *SpecTasks) RequestChanges(ctx context.Context, orgID string, workerID orgchart.BotID, taskID, comment string) (runtime.SpecTaskView, error) {
	projectID, _, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	if strings.TrimSpace(comment) == "" {
		return runtime.SpecTaskView{}, errors.New("comment is required when requesting changes")
	}
	task, err := s.ownedTask(ctx, projectID, taskID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	// Send the spec back for revision. The full design-review-comment
	// thread is the REST/UI path; here we make the same status transition
	// the orchestrator reacts to and bump the revision count.
	task.Status = types.TaskStatusSpecRevision
	task.SpecRevisionCount++
	task.UpdatedAt = time.Now()
	if err := s.tasks.UpdateSpecTask(ctx, task); err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("request changes: %w", err)
	}
	return toView(task), nil
}

func (s *SpecTasks) CreatePullRequests(ctx context.Context, orgID string, workerID orgchart.BotID, taskID string) (runtime.SpecTaskView, error) {
	projectID, hiringUserID, err := s.project(ctx, orgID, workerID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	task, err := s.ownedTask(ctx, projectID, taskID)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	project, err := s.tasks.GetProject(ctx, projectID)
	if err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("get project: %w", err)
	}
	// EnsurePullRequests opens one PR per external repo attached to the
	// project (the primary repo is the project's default).
	if err := s.workflow.EnsurePullRequests(ctx, task, project.DefaultRepoID, hiringUserID); err != nil {
		return runtime.SpecTaskView{}, fmt.Errorf("create pull requests: %w", err)
	}
	if latest, gErr := s.tasks.GetSpecTask(ctx, taskID); gErr == nil {
		task = latest
	}
	return toView(task), nil
}

// toView projects a SpecTask onto the tool-facing view.
func toView(t *types.SpecTask) runtime.SpecTaskView {
	v := runtime.SpecTaskView{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Status:      string(t.Status),
		Priority:    string(t.Priority),
		Type:        t.Type,
		BranchName:  t.BranchName,
	}
	for _, pr := range t.RepoPullRequests {
		v.PullRequests = append(v.PullRequests, runtime.PullRequestView{
			RepositoryName: pr.RepositoryName,
			URL:            pr.PRURL,
			State:          pr.PRState,
		})
	}
	return v
}

// generateDesignDocPath creates a human-readable directory path for
// design docs: "NNNNNN_shortname" e.g. "000001_add-login". A local copy
// of the same logic the Optimus skill carries (which itself copies it
// from the services package) to avoid an import cycle with services.
func generateDesignDocPath(taskName string, taskNumber int) string {
	name := strings.ToLower(taskName)
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	name = regexp.MustCompile(`[^a-z0-9- ]`).ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, " ", "-")
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if len(name) > 25 {
		truncated := name[:25]
		lastHyphen := strings.LastIndex(truncated, "-")
		if lastHyphen > 10 {
			name = truncated[:lastHyphen]
		} else {
			name = truncated
		}
	}
	name = strings.TrimRight(name, "-")
	return fmt.Sprintf("%06d_%s", taskNumber, name)
}
