package services

import (
	"bytes"
	"context"
	"text/template"
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

// =============================================================================
// Template Data Structures
// =============================================================================

// ApprovalPromptData contains all data for the approval/implementation prompt
type ApprovalPromptData struct {
	Guidelines      string // Formatted guidelines section
	PrimaryRepoName string // Name of the primary repository (e.g., "my-app")
	TaskDirName     string // Design doc directory name
	BranchName      string // Feature branch name
	BaseBranch      string // Base branch (e.g., "main")
	TaskName        string // Human-readable task name
	OriginalPrompt  string // Original user request
}

// CommentPromptData contains data for design review comment prompts
type CommentPromptData struct {
	DocumentLabel string
	SectionPath   string
	LineNumber    int
	QuotedText    string
	CommentText   string
	TaskDirName   string
}

// RevisionPromptData contains data for revision instruction prompts
type RevisionPromptData struct {
	TaskDirName string
	Comments    string
}

// MergePromptData contains data for merge instruction prompts
type MergePromptData struct {
	BranchName string
	BaseBranch string
}

// ImplementationReviewPromptData contains data for implementation review prompts
type ImplementationReviewPromptData struct {
	BranchName  string
	TaskDirName string
}

// =============================================================================
// Compiled Templates
// =============================================================================

var approvalPromptTemplate = template.Must(template.New("approval").Parse(`# Design Approved - Begin Implementation

Speak English.
{{.Guidelines}}

Your design has been approved. Implement the code changes now.

## CRITICAL RULES

1. **PUSH after every task** - The UI tracks progress via git pushes to helix-specs
2. **Do the bare minimum** - Simple tasks = simple solutions. No over-engineering.
3. **Update tasks.md** - Mark [x] when you start each task, push immediately
4. **Update design docs as you go** - Modify requirements.md, design.md, tasks.md when you learn something new
5. **Include Code-Ref** - Always reference the code commit in helix-specs commits (see below)

## Two Repositories - Don't Confuse Them

1. **/home/retro/work/helix-specs/** = Design docs and progress tracking (push to helix-specs branch)
2. **/home/retro/work/{{.PrimaryRepoName}}/** = Code changes (push to feature branch) - THIS IS YOUR PRIMARY PROJECT

## Task Checklist

Your checklist: /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/tasks.md

- [ ] = not started
- [x] = done

When you START a task, change [ ] to [x] and push. Don't wait until "really done".
Small frequent pushes are better than one big push at the end.

**IMPORTANT:** Always include a Code-Ref in your commit messages to link specs to code versions:

` + "```bash" + `
# Get current code commit from your feature branch
cd /home/retro/work/{{.PrimaryRepoName}}
CODE_REF="$(git rev-parse --short HEAD)"

cd /home/retro/work/helix-specs
git add -A && git commit -m "Progress update

Code-Ref: {{.PrimaryRepoName}}/{{.BranchName}}@${CODE_REF}" && git push origin helix-specs
` + "```" + `

The **Code-Ref** line is machine-parsable and links spec versions to code versions.

## Steps

1. Read design docs: /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/
2. In the CODE repo, create feature branch: ` + "`git checkout -b {{.BranchName}}`" + `
3. For each task in tasks.md: mark [x], push helix-specs, then do the work
4. When all tasks done, push code: ` + "`git push origin {{.BranchName}}`" + `

## Don't Over-Engineer

- "Start a container" → docker-compose.yaml, NOT a Python wrapper
- "Create sample data" → write files directly, NOT a generator script
- "Run X at startup" → /home/retro/work/helix-specs/.helix/startup.sh (idempotent), NOT a service framework
- If it can be a one-liner, use a one-liner

## Update Design Docs As You Go (IMPORTANT)

**Your plan is a living document.** When you discover something new or make a decision:
- **Modify requirements.md** if requirements need clarification or discovered constraints
- **Modify design.md** with what you learned, decisions made, or approaches that didn't work
- **Modify tasks.md** to add new tasks, remove unnecessary ones, or adjust the plan
- Push to helix-specs so the record is saved

Example modifications:
` + "```markdown" + `
## Implementation Notes (add to design.md)

- Found existing utility X, reusing instead of building new
- Chose approach A over B because [reason]
- Blocker: Y didn't work, used Z instead
- Added new task: need to also update the config parser
` + "```" + `

Don't treat the original plan as fixed - update it based on what you learn during implementation.

---

**Task:** {{.TaskName}}
**Feature Branch:** {{.BranchName}} (base: {{.BaseBranch}})
**Design Docs:** /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/

**Original Request:**
{{.OriginalPrompt}}

**Primary Project Directory:** /home/retro/work/{{.PrimaryRepoName}}/
`))

var commentPromptTemplate = template.Must(template.New("comment").Parse(`# Review Comment

Speak English.

**Document:** {{.DocumentLabel}}
{{if .SectionPath}}**Section:** {{.SectionPath}}
{{end}}{{if gt .LineNumber 0}}**Line:** {{.LineNumber}}
{{end}}
{{if .QuotedText}}> {{.QuotedText}}

{{end}}**Comment:** {{.CommentText}}

---

If changes are needed, update /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/ and push:
` + "```bash" + `
cd /home/retro/work/helix-specs && git add -A && git commit -m "Address feedback" && git push origin helix-specs
` + "```" + `
`))

var implementationReviewPromptTemplate = template.Must(template.New("implementationReview").Parse(`# Implementation Ready for Review

Speak English.

Your code has been pushed. The user will now test your work.

If this is a web app, please start the dev server and provide the URL.

**Branch:** {{.BranchName}}
**Docs:** /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/
`))

var revisionPromptTemplate = template.Must(template.New("revision").Parse(`# Changes Requested

Speak English.

Update your design based on this feedback:

{{.Comments}}

---

**Your docs are in:** /home/retro/work/helix-specs/design/tasks/{{.TaskDirName}}/

After updating, push immediately:
` + "```bash" + `
cd /home/retro/work/helix-specs && git add -A && git commit -m "Address feedback" && git push origin helix-specs
` + "```" + `
`))

var mergePromptTemplate = template.Must(template.New("merge").Parse(`# Implementation Approved - Please Merge

Speak English.

Your implementation has been approved. Merge to {{.BaseBranch}}:

` + "```bash" + `
git checkout {{.BaseBranch}} && git pull origin {{.BaseBranch}} && git merge {{.BranchName}} && git push origin {{.BaseBranch}}
` + "```" + `
`))

// =============================================================================
// Prompt Builder Functions
// =============================================================================

// BuildApprovalInstructionPrompt builds the approval instruction prompt for an agent
// This is the single source of truth for this prompt - used by WebSocket and database approaches
// guidelines contains concatenated organization + project guidelines (can be empty)
// primaryRepoName is the name of the primary project repository (e.g., "my-app")
func BuildApprovalInstructionPrompt(task *types.SpecTask, branchName, baseBranch, guidelines, primaryRepoName string) string {
	taskDirName := getTaskDirName(task)

	// Build guidelines section if provided
	guidelinesSection := ""
	if guidelines != "" {
		guidelinesSection = `
## Guidelines

Follow these guidelines when implementing:

` + guidelines + `

---
`
	}

	data := ApprovalPromptData{
		Guidelines:      guidelinesSection,
		PrimaryRepoName: primaryRepoName,
		TaskDirName:     taskDirName,
		BranchName:      branchName,
		BaseBranch:      baseBranch,
		TaskName:        task.Name,
		OriginalPrompt:  task.OriginalPrompt,
	}

	var buf bytes.Buffer
	if err := approvalPromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating approval prompt: " + err.Error()
	}
	return buf.String()
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

	data := CommentPromptData{
		DocumentLabel: docLabel,
		SectionPath:   comment.SectionPath,
		LineNumber:    comment.LineNumber,
		QuotedText:    comment.QuotedText,
		CommentText:   comment.CommentText,
		TaskDirName:   taskDirName,
	}

	var buf bytes.Buffer
	if err := commentPromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating comment prompt: " + err.Error()
	}
	return buf.String()
}

// BuildImplementationReviewPrompt builds the prompt for notifying agent that implementation is ready for review
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildImplementationReviewPrompt(task *types.SpecTask, branchName string) string {
	taskDirName := getTaskDirName(task)

	data := ImplementationReviewPromptData{
		BranchName:  branchName,
		TaskDirName: taskDirName,
	}

	var buf bytes.Buffer
	if err := implementationReviewPromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating implementation review prompt: " + err.Error()
	}
	return buf.String()
}

// BuildRevisionInstructionPrompt builds the prompt for sending revision feedback to the agent
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildRevisionInstructionPrompt(task *types.SpecTask, comments string) string {
	taskDirName := getTaskDirName(task)

	data := RevisionPromptData{
		TaskDirName: taskDirName,
		Comments:    comments,
	}

	var buf bytes.Buffer
	if err := revisionPromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating revision prompt: " + err.Error()
	}
	return buf.String()
}

// BuildMergeInstructionPrompt builds the prompt for telling agent to merge their branch
// This is the single source of truth for this prompt - used by WebSocket approaches
func BuildMergeInstructionPrompt(branchName, baseBranch string) string {
	data := MergePromptData{
		BranchName: branchName,
		BaseBranch: baseBranch,
	}

	var buf bytes.Buffer
	if err := mergePromptTemplate.Execute(&buf, data); err != nil {
		return "Error generating merge prompt: " + err.Error()
	}
	return buf.String()
}

// =============================================================================
// Service Methods (Database Interaction)
// =============================================================================

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

// getGuidelinesForTask fetches concatenated organization/user + project guidelines
func (s *AgentInstructionService) getGuidelinesForTask(ctx context.Context, task *types.SpecTask) string {
	if task.ProjectID == "" {
		return ""
	}

	project, err := s.store.GetProject(ctx, task.ProjectID)
	if err != nil || project == nil {
		return ""
	}

	guidelines := ""

	// Get organization guidelines (if project belongs to an org)
	if project.OrganizationID != "" {
		org, err := s.store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: project.OrganizationID})
		if err == nil && org != nil && org.Guidelines != "" {
			guidelines = org.Guidelines
		}
	} else if project.UserID != "" {
		// No organization - check for user guidelines (personal workspace)
		userMeta, err := s.store.GetUserMeta(ctx, project.UserID)
		if err == nil && userMeta != nil && userMeta.Guidelines != "" {
			guidelines = userMeta.Guidelines
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
		return err
	}

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", userID).
		Str("interaction_id", interaction.ID).
		Str("state", string(interaction.State)).
		Msg("Successfully sent instruction to agent (waiting for response)")

	return nil
}
