package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// addSpecTaskProposalTools registers the three proposal MCP tools (propose_pull_request,
// propose_spec_task, mark_task_complete) when the given session_id resolves to the
// agent session of an active spec task. Returns an error (which the caller logs at
// debug) when the session does not belong to a spec task — that's the expected
// case for non-spec-task sessions.
func (b *HelixMCPBackend) addSpecTaskProposalTools(ctx context.Context, mcpServer *server.MCPServer, sessionID string) error {
	tasks, err := b.store.ListSpecTasks(ctx, &types.SpecTaskFilters{AgentSessionID: sessionID})
	if err != nil {
		return fmt.Errorf("failed to list spec tasks for session %s: %w", sessionID, err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("session %s does not belong to a spec task", sessionID)
	}
	task := tasks[0]

	// propose_pull_request
	prTool := mcp.NewTool("propose_pull_request",
		mcp.WithDescription(
			"Propose opening a pull request from the given branch. Requires user approval "+
				"via the Helix UI before any push or PR creation happens. You may call this "+
				"more than once per task to ship work as a series of slices. Opening zero PRs "+
				"is also a valid outcome — finish those tasks with mark_task_complete.",
		),
		mcp.WithString("reason",
			mcp.Required(),
			mcp.Description("Why you're proposing this PR. Shown to the user in the approval UI."),
		),
		mcp.WithString("repository_id",
			mcp.Description("Project repo ID. Defaults to the project's primary repository."),
		),
		mcp.WithString("head_branch",
			mcp.Description("Branch to open the PR from. Defaults to the system-generated branch for this task. You may request a non-default branch; the user can override during approval."),
		),
		mcp.WithString("base_branch",
			mcp.Description("Target branch. Defaults to the repository's default branch."),
		),
		mcp.WithString("title",
			mcp.Description("PR title. Defaults to the contents of pull_request.md or pull_request_<repo>.md in the design docs."),
		),
		mcp.WithString("body",
			mcp.Description("PR body markdown. Defaults to the contents of pull_request.md."),
		),
	)
	mcpServer.AddTool(prTool, b.createProposePullRequestHandler(task, sessionID))

	// propose_spec_task
	taskTool := mcp.NewTool("propose_spec_task",
		mcp.WithDescription(
			"Propose creating a new spec task in this project. Requires user approval "+
				"in the Helix UI before the task appears on the board. Use for follow-ups "+
				"discovered during implementation that should be tracked separately. "+
				"Do NOT use CreateSpecTask — that tool is reserved for the project-manager chat agent.",
		),
		mcp.WithString("reason",
			mcp.Required(),
			mcp.Description("Why you're proposing this task. Shown to the user in the approval UI."),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Short, descriptive name for the new task."),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Detailed description of what needs to be done."),
		),
		mcp.WithString("type",
			mcp.Description("Task type: feature, bug, or refactor (defaults to feature)."),
		),
		mcp.WithString("priority",
			mcp.Description("Task priority: low, medium, high, or critical (defaults to medium)."),
		),
		mcp.WithString("original_prompt",
			mcp.Description("The original user request or context that led to this proposed task."),
		),
	)
	mcpServer.AddTool(taskTool, b.createProposeSpecTaskHandler(task, sessionID))

	// mark_task_complete
	completeTool := mcp.NewTool("mark_task_complete",
		mcp.WithDescription(
			"Declare that you believe this spec task is finished. The user must confirm "+
				"in the Helix UI to actually move the task to 'done'. This is the ONLY way "+
				"a task reaches 'done' — there is no automatic completion based on PR merging. "+
				"Call this when the work is judged finished, regardless of how many PRs you "+
				"opened or what state they're in (zero PRs is fine for research/knowledge tasks).",
		),
		mcp.WithString("reason",
			mcp.Required(),
			mcp.Description("Brief summary of what was accomplished. Shown to the user."),
		),
	)
	mcpServer.AddTool(completeTool, b.createMarkTaskCompleteHandler(task, sessionID))

	log.Info().
		Str("session_id", sessionID).
		Str("spec_task_id", task.ID).
		Msg("registered spec task proposal MCP tools")
	return nil
}

// stringArg pulls a string argument out of an MCP CallToolRequest, returning "" if absent.
func stringArg(req mcp.CallToolRequest, key string) string {
	if v, ok := req.GetArguments()[key].(string); ok {
		return v
	}
	return ""
}

func (b *HelixMCPBackend) createProposePullRequestHandler(task *types.SpecTask, sessionID string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		reason := stringArg(request, "reason")
		if reason == "" {
			return mcp.NewToolResultError("reason is required"), nil
		}

		proposal := &types.SpecTaskProposal{
			ID:                system.GenerateSpecTaskProposalID(),
			SpecTaskID:        task.ID,
			ProjectID:         task.ProjectID,
			Kind:              types.ProposalKindPullRequest,
			Status:            types.ProposalStatusPending,
			ProposedBySession: sessionID,
			AgentReason:       reason,
			PRRepositoryID:    stringArg(request, "repository_id"),
			PRHeadBranch:      stringArg(request, "head_branch"),
			PRBaseBranch:      stringArg(request, "base_branch"),
			PRTitle:           stringArg(request, "title"),
			PRBody:            stringArg(request, "body"),
		}
		if err := b.store.CreateSpecTaskProposal(ctx, proposal); err != nil {
			return mcp.NewToolResultError("failed to create proposal: " + err.Error()), nil
		}
		out, _ := json.Marshal(map[string]any{
			"proposal_id": proposal.ID,
			"status":      proposal.Status,
			"message":     "Pull request proposal created. Awaiting user approval in the Helix UI. You will receive a follow-up message in this session when the user decides.",
		})
		return mcp.NewToolResultText(string(out)), nil
	}
}

func (b *HelixMCPBackend) createProposeSpecTaskHandler(task *types.SpecTask, sessionID string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		reason := stringArg(request, "reason")
		name := stringArg(request, "name")
		description := stringArg(request, "description")
		if reason == "" {
			return mcp.NewToolResultError("reason is required"), nil
		}
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}
		if description == "" {
			return mcp.NewToolResultError("description is required"), nil
		}

		priority := types.SpecTaskPriority(stringArg(request, "priority"))
		proposal := &types.SpecTaskProposal{
			ID:                 system.GenerateSpecTaskProposalID(),
			SpecTaskID:         task.ID,
			ProjectID:          task.ProjectID,
			Kind:               types.ProposalKindSpecTask,
			Status:             types.ProposalStatusPending,
			ProposedBySession:  sessionID,
			AgentReason:        reason,
			TaskName:           name,
			TaskDescription:    description,
			TaskType:           stringArg(request, "type"),
			TaskPriority:       priority,
			TaskOriginalPrompt: stringArg(request, "original_prompt"),
		}
		if err := b.store.CreateSpecTaskProposal(ctx, proposal); err != nil {
			return mcp.NewToolResultError("failed to create proposal: " + err.Error()), nil
		}
		out, _ := json.Marshal(map[string]any{
			"proposal_id": proposal.ID,
			"status":      proposal.Status,
			"message":     "Spec task proposal created. Awaiting user approval in the Helix UI. You will receive a follow-up message in this session when the user decides.",
		})
		return mcp.NewToolResultText(string(out)), nil
	}
}

func (b *HelixMCPBackend) createMarkTaskCompleteHandler(task *types.SpecTask, sessionID string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		reason := stringArg(request, "reason")
		if reason == "" {
			return mcp.NewToolResultError("reason is required"), nil
		}

		proposal := &types.SpecTaskProposal{
			ID:                system.GenerateSpecTaskProposalID(),
			SpecTaskID:        task.ID,
			ProjectID:         task.ProjectID,
			Kind:              types.ProposalKindMarkComplete,
			Status:            types.ProposalStatusPending,
			ProposedBySession: sessionID,
			AgentReason:       reason,
			CompleteReason:    reason,
		}
		if err := b.store.CreateSpecTaskProposal(ctx, proposal); err != nil {
			return mcp.NewToolResultError("failed to create proposal: " + err.Error()), nil
		}
		out, _ := json.Marshal(map[string]any{
			"proposal_id": proposal.ID,
			"status":      proposal.Status,
			"message":     "Mark-complete proposal created. The user will click Mark Done (or Send Back with feedback) in the Helix UI. You will receive a follow-up message in this session when the user decides.",
		})
		return mcp.NewToolResultText(string(out)), nil
	}
}
