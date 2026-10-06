package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
)

// SessionMCPBackend provides session navigation MCP tools via HTTP
// This allows AI agents to navigate their own conversation history.
type SessionMCPBackend struct {
	store      store.Store
	notifier   notification.Notifier
	proposals  *services.PRProposalService
	mcpServer  *server.MCPServer
	httpServer *server.StreamableHTTPServer
}

// NewSessionMCPBackend creates a new session MCP backend
func NewSessionMCPBackend(s store.Store, notifier notification.Notifier) *SessionMCPBackend {
	backend := &SessionMCPBackend{
		store:    s,
		notifier: notifier,
	}

	// Create MCP server
	backend.mcpServer = server.NewMCPServer(
		"Helix Session",
		"1.0.0",
		server.WithResourceCapabilities(false, false),
		server.WithLogging(),
	)

	// Add current_session tool
	currentSessionTool := mcp.NewTool("current_session",
		mcp.WithDescription("Get quick overview of the current session including name, turn count, and recent activity."),
	)
	backend.mcpServer.AddTool(currentSessionTool, backend.handleCurrentSession)

	// Add session_toc tool
	sessionTocTool := mcp.NewTool("session_toc",
		mcp.WithDescription("Get the table of contents for a session - numbered list of all turns with one-line summaries. Great for finding specific past discussions."),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session from query param)"),
		),
	)
	backend.mcpServer.AddTool(sessionTocTool, backend.handleSessionTOC)

	// Add get_turn tool
	getTurnTool := mcp.NewTool("get_turn",
		mcp.WithDescription("Get the full content of a specific turn (interaction) by turn number."),
		mcp.WithNumber("turn",
			mcp.Required(),
			mcp.Description("Turn number (1-indexed)"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session)"),
		),
	)
	backend.mcpServer.AddTool(getTurnTool, backend.handleGetTurn)

	// Add session_title_history tool
	titleHistoryTool := mcp.NewTool("session_title_history",
		mcp.WithDescription("See how the session title evolved over time - shows topic changes and which turn triggered each title."),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session)"),
		),
	)
	backend.mcpServer.AddTool(titleHistoryTool, backend.handleTitleHistory)

	// Add search_session tool
	searchSessionTool := mcp.NewTool("search_session",
		mcp.WithDescription("Search within a session's interactions for specific terms or topics."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session)"),
		),
	)
	backend.mcpServer.AddTool(searchSessionTool, backend.handleSearchSession)

	// Add task_completed tool. It is session-scoped and only accepts recurring
	// job sessions with an active trigger execution.
	taskCompletedTool := mcp.NewTool("task_completed",
		mcp.WithDescription("Mark the current recurring task execution complete. Call this exactly once, only after all requested work is finished."),
		mcp.WithString("summary",
			mcp.Description("Optional concise summary of the completed work"),
		),
	)
	backend.mcpServer.AddTool(taskCompletedTool, backend.handleTaskCompleted)

	// Spec-task pull requests. Helix never opens a PR on its own: the agent
	// proposes one and a user approves the exact branch, base, title and body.
	proposePRTool := mcp.NewTool("propose_pull_request",
		mcp.WithDescription("Ask the user to approve opening a pull request for your spec task. "+
			"This is the ONLY way a pull request gets opened — never use gh, the GitHub API or other tools to create one. "+
			"A task can have several pull requests: to ship work in slices, propose each one with its own head_branch. "+
			"You may only push to your task branch and to branches the user approved through this tool; propose a new branch BEFORE pushing to it. "+
			"Returns immediately; the user's decision (and the PR link once opened) arrives later as a message in this session."),
		mcp.WithString("reason", mcp.Required(),
			mcp.Description("What this pull request contains and why it should be opened now. Shown to the user next to the approve button.")),
		mcp.WithString("title", mcp.Description("Pull request title. Defaults to the task name.")),
		mcp.WithString("body", mcp.Description("Pull request description (markdown).")),
		mcp.WithString("head_branch", mcp.Description("Branch to open the pull request from. Defaults to your task branch. "+
			"A new name gives you push rights to that branch once approved.")),
		mcp.WithString("base_branch", mcp.Description("Branch to merge into. Defaults to the task's base branch.")),
		mcp.WithString("repository", mcp.Description("Repository name or ID. Defaults to the project's primary repository.")),
	)
	backend.mcpServer.AddTool(proposePRTool, backend.handleProposePullRequest)

	listProposalsTool := mcp.NewTool("list_pull_request_proposals",
		mcp.WithDescription("List the pull request proposals of your spec task with their status (pending, approved, opened, rejected, failed), branches and PR links."),
	)
	backend.mcpServer.AddTool(listProposalsTool, backend.handleListPullRequestProposals)

	// Create Streamable HTTP server for direct POST support
	// Use stateless mode so each request is independent (no session tracking required)
	backend.httpServer = server.NewStreamableHTTPServer(backend.mcpServer,
		server.WithStateLess(true),
	)

	return backend
}

// SetPRProposals enables the spec-task pull request tools.
func (b *SessionMCPBackend) SetPRProposals(svc *services.PRProposalService) {
	b.proposals = svc
}

// ServeHTTP implements MCPBackend interface
func (b *SessionMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	// Store user and session_id in context for tool handlers
	ctx := r.Context()
	ctx = context.WithValue(ctx, "user", user)
	ctx = context.WithValue(ctx, "session_id", r.URL.Query().Get("session_id"))
	r = r.WithContext(ctx)

	b.httpServer.ServeHTTP(w, r)
}

// Helper to get session ID from context or request
func (b *SessionMCPBackend) getSessionID(ctx context.Context, requestedID string) string {
	if requestedID != "" {
		return requestedID
	}
	if id, ok := ctx.Value("session_id").(string); ok && id != "" {
		return id
	}
	return ""
}

// specTaskForCall resolves the spec task of the calling session, verifying the
// caller owns that session.
func (b *SessionMCPBackend) specTaskForCall(ctx context.Context) (*types.Session, *types.SpecTask, error) {
	sessionID := b.getSessionID(ctx, "")
	if sessionID == "" {
		return nil, nil, errors.New("session_id is required")
	}
	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get session: %w", err)
	}
	user, ok := ctx.Value("user").(*types.User)
	if !ok || user == nil || session.Owner != user.ID {
		return nil, nil, errors.New("not authorized for this session")
	}
	if session.Metadata.SpecTaskID == "" {
		return nil, nil, errors.New("this session is not working on a spec task; pull requests are only proposed from spec tasks")
	}
	task, err := b.store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get spec task: %w", err)
	}
	return session, task, nil
}

func (b *SessionMCPBackend) handleProposePullRequest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if b.proposals == nil {
		return mcp.NewToolResultError("pull request proposals are not available on this server"), nil
	}
	session, task, err := b.specTaskForCall(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p, err := b.proposals.Propose(ctx, task, services.ProposePRInput{
		RepositoryID: request.GetString("repository", ""),
		HeadBranch:   request.GetString("head_branch", ""),
		BaseBranch:   request.GetString("base_branch", ""),
		Title:        request.GetString("title", ""),
		Body:         request.GetString("body", ""),
		Reason:       request.GetString("reason", ""),
		SessionID:    session.ID,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if p.AutoApproved {
		switch p.Status {
		case types.PRProposalStatusOpened:
			return mcp.NewToolResultText(fmt.Sprintf(
				"Auto-approved (this task approves pull requests without asking). Pull request #%d is open in %s from %s into %s: %s. "+
					"Pushing more commits to %s updates it.", p.PRNumber, p.RepositoryName, p.HeadBranch, p.BaseBranch, p.PRURL, p.HeadBranch)), nil
		case types.PRProposalStatusApproved:
			return mcp.NewToolResultText(fmt.Sprintf(
				"Auto-approved (this task approves pull requests without asking). You may now push to %s; "+
					"Helix opens the pull request into %s as soon as the branch has commits that are not on %s.", p.HeadBranch, p.BaseBranch, p.BaseBranch)), nil
		case types.PRProposalStatusFailed:
			return mcp.NewToolResultText(fmt.Sprintf(
				"Auto-approved, but opening the pull request failed: %s. You still have push rights to %s; the user can retry from Helix.", p.Error, p.HeadBranch)), nil
		}
	}
	pushNote := ""
	if p.HeadBranch != task.BranchName {
		pushNote = fmt.Sprintf("You cannot push to %s until it is approved. ", p.HeadBranch)
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		"Proposal %s is awaiting the user's approval: open a pull request in %s from %s into %s titled %q. %s"+
			"You will receive a message in this session when the user decides; keep working meanwhile.",
		p.ID, p.RepositoryName, p.HeadBranch, p.BaseBranch, p.Title, pushNote)), nil
}

func (b *SessionMCPBackend) handleListPullRequestProposals(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, task, err := b.specTaskForCall(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	proposals, err := b.store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{SpecTaskID: task.ID})
	if err != nil {
		return mcp.NewToolResultError("failed to list proposals: " + err.Error()), nil
	}
	if len(proposals) == 0 {
		return mcp.NewToolResultText("No pull request proposals yet. Use propose_pull_request to ask for one."), nil
	}
	var sb strings.Builder
	for _, p := range proposals {
		fmt.Fprintf(&sb, "- %s [%s] %s: %s → %s, %q", p.ID, p.Status, p.RepositoryName, p.HeadBranch, p.BaseBranch, p.Title)
		if p.PRURL != "" {
			fmt.Fprintf(&sb, " — PR #%d %s", p.PRNumber, p.PRURL)
		}
		if p.Error != "" {
			fmt.Fprintf(&sb, " — error: %s", p.Error)
		}
		if p.DecisionComment != "" {
			fmt.Fprintf(&sb, " — reviewer: %s", p.DecisionComment)
		}
		sb.WriteString("\n")
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func (b *SessionMCPBackend) handleTaskCompleted(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := b.getSessionID(ctx, "")
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}
	user, ok := ctx.Value("user").(*types.User)
	if !ok || user == nil || session.Owner != user.ID {
		return mcp.NewToolResultError("not authorized to complete this task"), nil
	}
	if session.Metadata.SessionRole != "job" {
		return mcp.NewToolResultError("current session is not a recurring task"), nil
	}

	summary, _ := request.RequireString("summary")
	if summary == "" {
		summary = "Task completed"
	}
	execution, err := b.store.FinishTriggerExecution(ctx, sessionID, types.TriggerExecutionStatusSuccess, summary)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return mcp.NewToolResultError("no running recurring task execution exists for this session"), nil
		}
		return mcp.NewToolResultError("failed to complete recurring task: " + err.Error()), nil
	}

	if err := b.notifyTaskCompleted(ctx, session, execution, summary); err != nil {
		log.Error().Err(err).
			Str("session_id", session.ID).
			Str("execution_id", execution.ID).
			Msg("recurring task completed but notification failed")
	}

	return mcp.NewToolResultText(fmt.Sprintf("Recurring task execution %s marked complete.", execution.ID)), nil
}

func (b *SessionMCPBackend) notifyTaskCompleted(ctx context.Context, session *types.Session, execution *types.TriggerExecution, summary string) error {
	if b.notifier == nil {
		return nil
	}
	triggerConfig, err := b.store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{ID: execution.TriggerConfigurationID})
	if err != nil {
		return fmt.Errorf("load trigger notification configuration: %w", err)
	}
	if triggerConfig.Trigger.Cron == nil {
		return fmt.Errorf("trigger %s has no cron notification configuration", triggerConfig.ID)
	}
	return b.notifier.Notify(ctx, &types.Notification{
		Event:          types.EventCronTriggerComplete,
		Session:        session,
		Message:        summary,
		RenderMarkdown: true,
		Emails:         triggerConfig.Trigger.Cron.Emails,
		CallbackURL:    triggerConfig.Trigger.Cron.CallbackURL,
	})
}

// handleCurrentSession returns quick overview of current session
func (b *SessionMCPBackend) handleCurrentSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := b.getSessionID(ctx, "")
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}

	// Count interactions
	_, total, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   1,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to count interactions: " + err.Error()), nil
	}

	result := map[string]interface{}{
		"session_id":    session.ID,
		"name":          session.Name,
		"total_turns":   total,
		"created":       session.Created,
		"updated":       session.Updated,
		"title_changes": len(session.Metadata.TitleHistory),
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleSessionTOC returns the table of contents
func (b *SessionMCPBackend) handleSessionTOC(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}

	// Get all interactions
	interactions, _, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   200, // Reasonable limit
	})
	if err != nil {
		return mcp.NewToolResultError("failed to list interactions: " + err.Error()), nil
	}

	// Build TOC
	var toc []map[string]interface{}
	for i, interaction := range interactions {
		entry := map[string]interface{}{
			"turn":    i + 1,
			"id":      interaction.ID,
			"summary": interaction.Summary,
		}
		if interaction.Summary == "" && interaction.PromptMessage != "" {
			// Fallback to first 80 chars of prompt
			summary := interaction.PromptMessage
			if len(summary) > 80 {
				summary = summary[:80] + "..."
			}
			entry["summary"] = summary
		}
		toc = append(toc, entry)
	}

	result := map[string]interface{}{
		"session_id":   sessionID,
		"session_name": session.Name,
		"total_turns":  len(interactions),
		"entries":      toc,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleGetTurn returns a specific turn's content
func (b *SessionMCPBackend) handleGetTurn(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	turn, err := request.RequireFloat("turn")
	if err != nil || turn < 1 {
		return mcp.NewToolResultError("turn number is required (1 or greater)"), nil
	}

	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	// Get interactions and find the one at this turn
	interactions, _, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   int(turn) + 1,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to list interactions: " + err.Error()), nil
	}

	turnIndex := int(turn) - 1
	if turnIndex >= len(interactions) {
		return mcp.NewToolResultError("turn " + strconv.Itoa(int(turn)) + " not found (session has " + strconv.Itoa(len(interactions)) + " turns)"), nil
	}

	interaction := interactions[turnIndex]
	result := map[string]interface{}{
		"turn":     int(turn),
		"id":       interaction.ID,
		"prompt":   interaction.PromptMessage,
		"response": interaction.ResponseMessage,
		"summary":  interaction.Summary,
		"created":  interaction.Created,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleTitleHistory returns the session's title evolution
func (b *SessionMCPBackend) handleTitleHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}

	result := map[string]interface{}{
		"session_id":    sessionID,
		"current_title": session.Name,
		"history":       session.Metadata.TitleHistory,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleSearchSession searches within session interactions
func (b *SessionMCPBackend) handleSearchSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	// Get all interactions and search
	interactions, _, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   200,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to list interactions: " + err.Error()), nil
	}

	// Simple text search
	var matches []map[string]interface{}
	for i, interaction := range interactions {
		if containsIgnoreCase(interaction.PromptMessage, query) ||
			containsIgnoreCase(interaction.ResponseMessage, query) ||
			containsIgnoreCase(interaction.Summary, query) {
			matches = append(matches, map[string]interface{}{
				"turn":    i + 1,
				"id":      interaction.ID,
				"summary": interaction.Summary,
			})
		}
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"query":      query,
		"matches":    matches,
		"total":      len(matches),
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// containsIgnoreCase performs case-insensitive substring search
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
