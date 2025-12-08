package services

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// sanitizeForBranchName is defined in design_docs_helpers.go

// AgentInstructionService sends automated instructions to agent sessions
type AgentInstructionService struct {
	store store.Store
}

// NewAgentInstructionService creates a new agent instruction service
func NewAgentInstructionService(store store.Store) *AgentInstructionService {
	return &AgentInstructionService{
		store: store,
	}
}

// BuildApprovalInstructionPrompt builds the approval instruction prompt for an agent
// This is the single source of truth for this prompt - used by WebSocket and database approaches
func BuildApprovalInstructionPrompt(task *types.SpecTask, branchName, baseBranch string) string {
	// Generate task directory name (same format as planning phase)
	dateStr := task.CreatedAt.Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	return fmt.Sprintf(`# Design Approved - Begin Implementation

Your design has been approved. Implement the code changes now.

## CRITICAL RULES

1. **PUSH after every task** - The UI tracks progress via git pushes to helix-specs
2. **Do the bare minimum** - Simple tasks = simple solutions. No over-engineering.
3. **Update tasks.md** - Mark [~] when starting, [x] when done
4. **Update design docs as you go** - Record discoveries, decisions, and blockers in design.md

## Task Checklist

Your checklist: ~/work/helix-specs/design/tasks/%[4]s/tasks.md

- [ ] = pending
- [~] = in progress (you are working on it)
- [x] = completed

After each status change, push immediately:
%[1]sbash
cd ~/work/helix-specs && git add -A && git commit -m "Progress update" && git push origin helix-specs
%[1]s

## Steps

1. Create feature branch: %[1]sgit checkout -b %[2]s%[1]s
2. Read your design docs: ~/work/helix-specs/design/tasks/%[4]s/
3. Work through tasks.md one by one
4. For each task: mark [~], do work, mark [x], push helix-specs
5. When done: %[1]sgit push origin %[2]s%[1]s

## Don't Over-Engineer

- "Start a container" → docker-compose.yaml, NOT a Python wrapper
- "Create sample data" → write files directly, NOT a generator script
- "Run X at startup" → .helix/startup.sh (idempotent), NOT a service framework
- If it can be a one-liner, use a one-liner

## Update Design Docs As You Go

When you discover something new or make a decision:
- Update design.md with what you learned or decided
- Push to helix-specs so the record is saved

Example additions to design.md:
%[1]smarkdown
## Implementation Notes

- Found existing utility X, reusing instead of building new
- Chose approach A over B because [reason]
- Blocker: Y didn't work, used Z instead
%[1]s

---

**Task:** %[5]s
**Feature Branch:** %[2]s (base: %[3]s)
**Design Docs:** ~/work/helix-specs/design/tasks/%[4]s/
**SpecTask ID:** %[6]s

**Original Request:**
%[7]s
`, "```", branchName, baseBranch, taskDirName, task.Name, task.ID, task.OriginalPrompt)
}

// BuildCommentPrompt builds a prompt for sending a design review comment to an agent
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildCommentPrompt(specTask *types.SpecTask, comment *types.SpecTaskDesignReviewComment) string {
	// Generate task directory name (same format as planning phase)
	dateStr := specTask.CreatedAt.Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(specTask.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, specTask.ID)

	// Map document types to readable labels
	documentTypeLabels := map[string]string{
		"requirements":        "Requirements (requirements.md)",
		"technical_design":    "Technical Design (design.md)",
		"implementation_plan": "Implementation Plan (tasks.md)",
	}
	docLabel := documentTypeLabels[comment.DocumentType]
	if docLabel == "" {
		docLabel = comment.DocumentType
	}

	// Build the prompt - keep it concise for smaller models
	var promptBuilder string
	promptBuilder = "# Review Comment\n\n"
	promptBuilder += fmt.Sprintf("**Document:** %s\n", docLabel)

	if comment.SectionPath != "" {
		promptBuilder += fmt.Sprintf("**Section:** %s\n", comment.SectionPath)
	}
	if comment.LineNumber > 0 {
		promptBuilder += fmt.Sprintf("**Line:** %d\n", comment.LineNumber)
	}

	if comment.QuotedText != "" {
		promptBuilder += fmt.Sprintf("\n> %s\n", comment.QuotedText)
	}

	promptBuilder += fmt.Sprintf("\n**Comment:** %s\n\n", comment.CommentText)

	promptBuilder += "---\n\n"
	promptBuilder += fmt.Sprintf("If changes are needed, update ~/work/helix-specs/design/tasks/%s/ and push:\n", taskDirName)
	promptBuilder += fmt.Sprintf("```bash\ncd ~/work/helix-specs && git add -A && git commit -m \"Address feedback\" && git push origin helix-specs\n```\n", taskDirName)

	return promptBuilder
}

// BuildImplementationReviewPrompt builds the prompt for notifying agent that implementation is ready for review
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildImplementationReviewPrompt(task *types.SpecTask, branchName string) string {
	// Generate task directory name (same format as planning phase)
	dateStr := task.CreatedAt.Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	return fmt.Sprintf(`# Implementation Ready for Review

Your code has been pushed. The user will now test your work.

If this is a web app, please start the dev server and provide the URL.

**Branch:** %s
**Docs:** ~/work/helix-specs/design/tasks/%s/
`, branchName, taskDirName)
}

// BuildRevisionInstructionPrompt builds the prompt for sending revision feedback to the agent
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildRevisionInstructionPrompt(task *types.SpecTask, comments string) string {
	// Generate task directory name (same format as planning phase)
	dateStr := task.CreatedAt.Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	return fmt.Sprintf(`# Changes Requested

Update your design based on this feedback:

%[2]s

---

**Your docs are in:** ~/work/helix-specs/design/tasks/%[1]s/

After updating, push immediately:
%[3]sbash
cd ~/work/helix-specs && git add -A && git commit -m "Address feedback" && git push origin helix-specs
%[3]s
`, taskDirName, comments, "```")
}

// BuildMergeInstructionPrompt builds the prompt for telling agent to merge their branch
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildMergeInstructionPrompt(branchName, baseBranch string) string {
	return fmt.Sprintf(`# Implementation Approved - Please Merge

Your implementation has been approved. Merge to %s:

%[1]sbash
git checkout %[2]s && git pull origin %[2]s && git merge %[3]s && git push origin %[2]s
%[1]s
`, "```", baseBranch, branchName)
}

// SendApprovalInstruction sends a message to the agent to start implementation
// NOTE: This creates a database interaction - for WebSocket-connected agents, use BuildApprovalInstructionPrompt
// and send via sendChatMessageToExternalAgent instead
func (s *AgentInstructionService) SendApprovalInstruction(
	ctx context.Context,
	sessionID string,
	userID string,
	task *types.SpecTask,
	branchName string,
	baseBranch string,
) error {
	message := BuildApprovalInstructionPrompt(task, branchName, baseBranch)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Msg("Sending approval instruction to agent")

	return s.sendMessage(ctx, sessionID, userID, message)
}

// SendImplementationReviewRequest notifies agent that implementation is ready for review
// NOTE: This creates a database interaction - for WebSocket-connected agents, use BuildImplementationReviewPrompt
// and send via sendMessageToSpecTaskAgent instead
func (s *AgentInstructionService) SendImplementationReviewRequest(
	ctx context.Context,
	sessionID string,
	userID string,
	task *types.SpecTask,
	branchName string,
) error {
	message := BuildImplementationReviewPrompt(task, branchName)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Msg("Sending implementation review request to agent")

	return s.sendMessage(ctx, sessionID, userID, message)
}

// SendRevisionInstruction sends a message to the agent with revision feedback
// NOTE: This creates a database interaction - for WebSocket-connected agents, use BuildRevisionInstructionPrompt
// and send via sendMessageToSpecTaskAgent instead
func (s *AgentInstructionService) SendRevisionInstruction(
	ctx context.Context,
	sessionID string,
	userID string,
	task *types.SpecTask,
	comments string,
) error {
	message := BuildRevisionInstructionPrompt(task, comments)

	log.Info().
		Str("session_id", sessionID).
		Str("task_id", task.ID).
		Msg("Sending revision instruction to agent")

	return s.sendMessage(ctx, sessionID, userID, message)
}

// SendMergeInstruction tells agent to merge their branch to main
// NOTE: This creates a database interaction - for WebSocket-connected agents, use BuildMergeInstructionPrompt
// and send via sendMessageToSpecTaskAgent instead
func (s *AgentInstructionService) SendMergeInstruction(
	ctx context.Context,
	sessionID string,
	userID string,
	branchName string,
	baseBranch string,
) error {
	message := BuildMergeInstructionPrompt(branchName, baseBranch)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Str("base_branch", baseBranch).
		Msg("Sending merge instruction to agent")

	return s.sendMessage(ctx, sessionID, userID, message)
}

// sendMessage sends a user message to an agent session (triggers agent response)
// Uses the same pattern as normal session message handling
func (s *AgentInstructionService) sendMessage(ctx context.Context, sessionID string, userID string, message string) error {
	// Create a user interaction that will trigger the agent to respond
	// This matches how normal user messages are created in spec_driven_task_service.go
	now := time.Now()
	interaction := &types.Interaction{
		ID:            system.GenerateInteractionID(),
		GenerationID:  0,
		Created:       now,
		Updated:       now,
		Scheduled:     now,
		SessionID:     sessionID,
		UserID:        userID, // User who created/owns the task
		Mode:          types.SessionModeInference,
		PromptMessage: message,
		State:         types.InteractionStateWaiting, // Waiting state triggers agent response
	}

	// Store the interaction - this will queue it for the agent to process
	_, err := s.store.CreateInteraction(ctx, interaction)
	if err != nil {
		return fmt.Errorf("failed to create instruction interaction: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", userID).
		Str("interaction_id", interaction.ID).
		Str("state", string(interaction.State)).
		Msg("Successfully sent instruction to agent (waiting for response)")

	return nil
}
