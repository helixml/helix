package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// This file holds the MCP spec-task tools — the surface a helix-org
// Worker uses to manage the spec tasks in its own Helix project. Each
// tool is a thin adapter: parse args, call the spectasks application
// service (deps.SpecTasks) with the calling Worker, marshal the result.
// The service + runtime port own the scoping (a Worker only ever touches
// its own project) and the reuse of the canonical Helix spec-task code.
//
// They live together in one file because they are eight small, tightly
// related adapters over one service; splitting them buys nothing.

// --- create_spectask ------------------------------------------------------

const CreateSpecTaskName tool.Name = "create_spectask"

type CreateSpecTask struct{ deps Deps }

func NewCreateSpecTask(deps Deps) *CreateSpecTask { return &CreateSpecTask{deps: deps} }

type createSpecTaskArgs struct {
	ProjectID      string   `json:"project_id,omitempty"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Type           string   `json:"type,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	OriginalPrompt string   `json:"original_prompt,omitempty"`
	SkipPlanning   bool     `json:"skip_planning,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"`
}

var createSpecTaskSchema = mustSchema[createSpecTaskArgs]()

func (t *CreateSpecTask) Name() tool.Name                 { return CreateSpecTaskName }
func (t *CreateSpecTask) InputSchema() *jsonschema.Schema { return createSpecTaskSchema }
func (t *CreateSpecTask) Description() string {
	return "Create a new spec task. Provide a short name and a high-level " +
		"description of the desired outcome. Optional: project_id (a project you manage in " +
		"your org — omit to use your own project), type (feature|bug|refactor), priority " +
		"(low|medium|high|critical), skip_planning (go straight to implementation), depends_on " +
		"(task IDs). The task starts in backlog — call start_spectask_planning to begin work."
}
func (t *CreateSpecTask) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createSpecTaskArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	view, err := t.deps.SpecTasks.Create(ctx, inv.Caller, args.ProjectID, runtime.CreateSpecTaskInput{
		Name:           args.Name,
		Description:    args.Description,
		Type:           args.Type,
		Priority:       args.Priority,
		OriginalPrompt: args.OriginalPrompt,
		SkipPlanning:   args.SkipPlanning,
		DependsOn:      args.DependsOn,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

// --- list_spectasks -------------------------------------------------------

const ListSpecTasksName tool.Name = "list_spectasks"

type ListSpecTasks struct{ deps Deps }

func NewListSpecTasks(deps Deps) *ListSpecTasks { return &ListSpecTasks{deps: deps} }

type listSpecTasksArgs struct {
	ProjectID string `json:"project_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Type      string `json:"type,omitempty"`
}

var listSpecTasksSchema = mustSchema[listSpecTasksArgs]()

func (t *ListSpecTasks) Name() tool.Name                 { return ListSpecTasksName }
func (t *ListSpecTasks) InputSchema() *jsonschema.Schema { return listSpecTasksSchema }
func (t *ListSpecTasks) Description() string {
	return "List spec tasks, optionally filtered by status, priority, or type. Pass project_id " +
		"to list tasks in a project you manage in your org; omit it to list your own project's tasks."
}
func (t *ListSpecTasks) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args listSpecTasksArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	views, err := t.deps.SpecTasks.List(ctx, inv.Caller, args.ProjectID, runtime.ListSpecTasksFilter{
		Status:   args.Status,
		Priority: args.Priority,
		Type:     args.Type,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(views)
}

// --- get_spectask ---------------------------------------------------------

const GetSpecTaskName tool.Name = "get_spectask"

type GetSpecTask struct{ deps Deps }

func NewGetSpecTask(deps Deps) *GetSpecTask { return &GetSpecTask{deps: deps} }

type taskIDArgs struct {
	ProjectID string `json:"project_id,omitempty"`
	TaskID    string `json:"task_id"`
}

var getSpecTaskSchema = mustSchema[taskIDArgs]()

func (t *GetSpecTask) Name() tool.Name                 { return GetSpecTaskName }
func (t *GetSpecTask) InputSchema() *jsonschema.Schema { return getSpecTaskSchema }
func (t *GetSpecTask) Description() string {
	return "Get the full details of one spec task by its task_id. Pass project_id when the task " +
		"is in a project you manage in your org; omit it for your own project."
}
func (t *GetSpecTask) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	args, err := parseTaskID(inv.Args)
	if err != nil {
		return nil, err
	}
	view, err := t.deps.SpecTasks.Get(ctx, inv.Caller, args.ProjectID, args.TaskID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

// --- start_spectask_planning ---------------------------------------------

const StartSpecTaskPlanningName tool.Name = "start_spectask_planning"

type StartSpecTaskPlanning struct{ deps Deps }

func NewStartSpecTaskPlanning(deps Deps) *StartSpecTaskPlanning {
	return &StartSpecTaskPlanning{deps: deps}
}

var startSpecTaskPlanningSchema = mustSchema[taskIDArgs]()

func (t *StartSpecTaskPlanning) Name() tool.Name                 { return StartSpecTaskPlanningName }
func (t *StartSpecTaskPlanning) InputSchema() *jsonschema.Schema { return startSpecTaskPlanningSchema }
func (t *StartSpecTaskPlanning) Description() string {
	return "Start a spec task: begin spec generation (or go straight to implementation if the " +
		"task was created with skip_planning). Use after create_spectask. Pass project_id for a " +
		"task in a project you manage in your org; omit it for your own project."
}
func (t *StartSpecTaskPlanning) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	args, err := parseTaskID(inv.Args)
	if err != nil {
		return nil, err
	}
	view, err := t.deps.SpecTasks.StartPlanning(ctx, inv.Caller, args.ProjectID, args.TaskID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

// --- review_spectask_spec -------------------------------------------------

const ReviewSpecTaskSpecName tool.Name = "review_spectask_spec"

type ReviewSpecTaskSpec struct{ deps Deps }

func NewReviewSpecTaskSpec(deps Deps) *ReviewSpecTaskSpec { return &ReviewSpecTaskSpec{deps: deps} }

var reviewSpecTaskSpecSchema = mustSchema[taskIDArgs]()

func (t *ReviewSpecTaskSpec) Name() tool.Name                 { return ReviewSpecTaskSpecName }
func (t *ReviewSpecTaskSpec) InputSchema() *jsonschema.Schema { return reviewSpecTaskSpecSchema }
func (t *ReviewSpecTaskSpec) Description() string {
	return "Read the generated specification (requirements, design, implementation plan) for a " +
		"spec task so you can review it before approving or requesting changes. Pass project_id " +
		"for a task in a project you manage in your org; omit it for your own project."
}
func (t *ReviewSpecTaskSpec) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	args, err := parseTaskID(inv.Args)
	if err != nil {
		return nil, err
	}
	review, err := t.deps.SpecTasks.ReviewSpec(ctx, inv.Caller, args.ProjectID, args.TaskID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(review)
}

// --- approve_spectask_spec ------------------------------------------------

const ApproveSpecTaskSpecName tool.Name = "approve_spectask_spec"

type ApproveSpecTaskSpec struct{ deps Deps }

func NewApproveSpecTaskSpec(deps Deps) *ApproveSpecTaskSpec { return &ApproveSpecTaskSpec{deps: deps} }

var approveSpecTaskSpecSchema = mustSchema[taskIDArgs]()

func (t *ApproveSpecTaskSpec) Name() tool.Name                 { return ApproveSpecTaskSpecName }
func (t *ApproveSpecTaskSpec) InputSchema() *jsonschema.Schema { return approveSpecTaskSpecSchema }
func (t *ApproveSpecTaskSpec) Description() string {
	return "Approve a spec task's generated specification. This advances the task toward " +
		"implementation. Review the spec first with review_spectask_spec. Pass project_id for a " +
		"task in a project you manage in your org; omit it for your own project."
}
func (t *ApproveSpecTaskSpec) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	args, err := parseTaskID(inv.Args)
	if err != nil {
		return nil, err
	}
	view, err := t.deps.SpecTasks.ApproveSpec(ctx, inv.Caller, args.ProjectID, args.TaskID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

// --- request_spectask_changes --------------------------------------------

const RequestSpecTaskChangesName tool.Name = "request_spectask_changes"

type RequestSpecTaskChanges struct{ deps Deps }

func NewRequestSpecTaskChanges(deps Deps) *RequestSpecTaskChanges {
	return &RequestSpecTaskChanges{deps: deps}
}

type requestChangesArgs struct {
	ProjectID string `json:"project_id,omitempty"`
	TaskID    string `json:"task_id"`
	Comment   string `json:"comment"`
}

var requestSpecTaskChangesSchema = mustSchema[requestChangesArgs]()

func (t *RequestSpecTaskChanges) Name() tool.Name { return RequestSpecTaskChangesName }
func (t *RequestSpecTaskChanges) InputSchema() *jsonschema.Schema {
	return requestSpecTaskChangesSchema
}
func (t *RequestSpecTaskChanges) Description() string {
	return "Send a spec task's specification back for revision with a comment explaining what " +
		"needs to change. The task returns to revision and the agent regenerates the spec. Pass " +
		"project_id for a task in a project you manage in your org; omit it for your own project."
}
func (t *RequestSpecTaskChanges) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args requestChangesArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TaskID == "" {
		return nil, errors.New("task_id is required")
	}
	if args.Comment == "" {
		return nil, errors.New("comment is required")
	}
	view, err := t.deps.SpecTasks.RequestChanges(ctx, inv.Caller, args.ProjectID, args.TaskID, args.Comment)
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

// --- create_spectask_prs --------------------------------------------------

const CreateSpecTaskPRsName tool.Name = "create_spectask_prs"

type CreateSpecTaskPRs struct{ deps Deps }

func NewCreateSpecTaskPRs(deps Deps) *CreateSpecTaskPRs { return &CreateSpecTaskPRs{deps: deps} }

var createSpecTaskPRsSchema = mustSchema[taskIDArgs]()

func (t *CreateSpecTaskPRs) Name() tool.Name                 { return CreateSpecTaskPRsName }
func (t *CreateSpecTaskPRs) InputSchema() *jsonschema.Schema { return createSpecTaskPRsSchema }
func (t *CreateSpecTaskPRs) Description() string {
	return "When you're happy with the implemented code, tell the system to open the pull " +
		"request(s) for a spec task — one per repository attached to the project. This does NOT " +
		"merge or approve on GitHub; the merge approval still happens on GitHub itself. Pass " +
		"project_id for a task in a project you manage in your org; omit it for your own project."
}
func (t *CreateSpecTaskPRs) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	args, err := parseTaskID(inv.Args)
	if err != nil {
		return nil, err
	}
	view, err := t.deps.SpecTasks.CreatePullRequests(ctx, inv.Caller, args.ProjectID, args.TaskID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

// parseTaskID is the shared arg-parse for the single-task-id tools.
func parseTaskID(raw json.RawMessage) (taskIDArgs, error) {
	var args taskIDArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return taskIDArgs{}, fmt.Errorf("parse args: %w", err)
	}
	if args.TaskID == "" {
		return taskIDArgs{}, errors.New("task_id is required")
	}
	return args, nil
}
