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

// getTaskDirName returns the task directory name, preferring DesignDocPath with fallback to task.ID
func getTaskDirName(task *types.SpecTask) string {
	if task.DesignDocPath != "" {
		return task.DesignDocPath
	}
	return task.ID // Backwards compatibility for old tasks
}

// BuildApprovalInstructionPrompt builds the approval instruction prompt for an agent
// This is the single source of truth for this prompt - used by WebSocket and database approaches
// guidelines contains concatenated organization + project guidelines (can be empty)
// primaryRepoName is the name of the primary project repository (e.g., "my-app")
func BuildApprovalInstructionPrompt(task *types.SpecTask, branchName, baseBranch, guidelines, primaryRepoName string) string {
	taskDirName := getTaskDirName(task)

	// Build guidelines section if provided
	guidelinesSection := ""
	if guidelines != "" {
		guidelinesSection = fmt.Sprintf(`
## Guidelines

Follow these guidelines when implementing:

%s

---
`, guidelines)
	}

	return fmt.Sprintf(`# Design Approved - Begin Implementation

Speak English.
%[8]s

Your design has been approved. Implement the code changes now.

## CRITICAL RULES

1. **PUSH after every task** - The UI tracks progress via git pushes to helix-specs
2. **Do the bare minimum** - Simple tasks = simple solutions. No over-engineering.
3. **Update tasks.md** - Mark [x] when you start each task, push immediately
4. **Update design docs as you go** - Record discoveries, decisions, and blockers in design.md

## Two Repositories - Don't Confuse Them

1. **/home/retro/work/helix-specs/** = Design docs and progress tracking (push to helix-specs branch)
2. **/home/retro/work/%[9]s/** = Code changes (push to feature branch) - THIS IS YOUR PRIMARY PROJECT

## Task Checklist

Your checklist: /home/retro/work/helix-specs/design/tasks/%[4]s/tasks.md

- [ ] = not started
- [x] = done

When you START a task, change [ ] to [x] and push. Don't wait until "really done".
Small frequent pushes are better than one big push at the end.

After ANY checklist change:
%[1]sbash
cd /home/retro/work/helix-specs && git add -A && git commit -m "Progress update" && git push origin helix-specs
%[1]s

## Steps

1. Read design docs: /home/retro/work/helix-specs/design/tasks/%[4]s/
2. In the CODE repo, create feature branch: %[1]sgit checkout -b %[2]s%[1]s
3. For each task in tasks.md: mark [x], push helix-specs, then do the work
4. When all tasks done, push code: %[1]sgit push origin %[2]s%[1]s

## Don't Over-Engineer

- "Start a container" → docker-compose.yaml, NOT a Python wrapper
- "Create sample data" → write files directly, NOT a generator script
- "Run X at startup" → /home/retro/work/%[9]s/.helix/startup.sh (idempotent), NOT a service framework
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
**Design Docs:** /home/retro/work/helix-specs/design/tasks/%[4]s/
**SpecTask ID:** %[6]s

**Original Request:**
%[7]s

**Primary Project Directory:** /home/retro/work/%[9]s/
`, "```", branchName, baseBranch, taskDirName, task.Name, task.ID, task.OriginalPrompt, guidelinesSection, primaryRepoName)
}

// BuildCommentPrompt builds a prompt for sending a design review comment to an agent
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildCommentPrompt(specTask *types.SpecTask, comment *types.SpecTaskDesignReviewComment) string {
	taskDirName := getTaskDirName(specTask)

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
	promptBuilder = "# Review Comment\n\nSpeak English.\n\n"
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
	promptBuilder += fmt.Sprintf("If changes are needed, update /home/retro/work/helix-specs/design/tasks/%s/ and push:\n", taskDirName)
	promptBuilder += fmt.Sprintf("```bash\ncd /home/retro/work/helix-specs && git add -A && git commit -m \"Address feedback\" && git push origin helix-specs\n```\n", taskDirName)

	return promptBuilder
}

// BuildImplementationReviewPrompt builds the prompt for notifying agent that implementation is ready for review
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildImplementationReviewPrompt(task *types.SpecTask, branchName string) string {
	taskDirName := getTaskDirName(task)

	return fmt.Sprintf(`# Implementation Ready for Review

Speak English.

Your code has been pushed. The user will now test your work.

If this is a web app, please start the dev server and provide the URL.

**Branch:** %s
**Docs:** /home/retro/work/helix-specs/design/tasks/%s/
`, branchName, taskDirName)
}

// BuildRevisionInstructionPrompt builds the prompt for sending revision feedback to the agent
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildRevisionInstructionPrompt(task *types.SpecTask, comments string) string {
	taskDirName := getTaskDirName(task)

	return fmt.Sprintf(`# Changes Requested

Speak English.

Update your design based on this feedback:

%[2]s

---

**Your docs are in:** /home/retro/work/helix-specs/design/tasks/%[1]s/

After updating, push immediately:
%[3]sbash
cd /home/retro/work/helix-specs && git add -A && git commit -m "Address feedback" && git push origin helix-specs
%[3]s
`, taskDirName, comments, "```")
}

// BuildMergeInstructionPrompt builds the prompt for telling agent to merge their branch
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildMergeInstructionPrompt(branchName, baseBranch string) string {
	return fmt.Sprintf(`# Implementation Approved - Please Merge

Speak English.

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
	primaryRepoName string,
) error {
	// Fetch guidelines from project and organization
	guidelines := s.getGuidelinesForTask(ctx, task)
	message := BuildApprovalInstructionPrompt(task, branchName, baseBranch, guidelines, primaryRepoName)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Msg("Sending approval instruction to agent")

	return s.sendMessage(ctx, sessionID, userID, message)
}

// getGuidelinesForTask fetches concatenated organization + project guidelines
func (s *AgentInstructionService) getGuidelinesForTask(ctx context.Context, task *types.SpecTask) string {
	if task.ProjectID == "" {
		return ""
	}

	project, err := s.store.GetProject(ctx, task.ProjectID)
	if err != nil || project == nil {
		return ""
	}

	guidelines := ""

	// Get organization guidelines
	if project.OrganizationID != "" {
		org, err := s.store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: project.OrganizationID})
		if err == nil && org != nil && org.Guidelines != "" {
			guidelines = org.Guidelines
		}
	}

	// Append project guidelines
	if project.Guidelines != "" {
		if guidelines != "" {
			guidelines += "\n\n---\n\n"
		}
		guidelines += project.Guidelines
	}

	return guidelines
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
