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

// SendApprovalInstruction sends a message to the agent to start implementation
func (s *AgentInstructionService) SendApprovalInstruction(
	ctx context.Context,
	sessionID string,
	userID string,
	task *types.SpecTask,
	branchName string,
	baseBranch string,
) error {
	// Generate task directory name (same format as planning phase)
	dateStr := task.CreatedAt.Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	message := fmt.Sprintf(`# Design Approved! 🎉

Your design has been approved. Please begin implementation.

## 🚨🚨🚨 #1 RULE - PUSH PROGRESS AFTER EVERY TASK 🚨🚨🚨

The tasks.md file in helix-specs contains your task checklist. You MUST update it as you work:
- [ ] Task description (pending)
- [~] Task description (in progress - YOU mark this)
- [x] Task description (completed - YOU mark this)

**Before starting EACH task:**
%[1]sbash
cd ~/work/helix-specs/design/tasks/%[4]s
# Edit tasks.md: change "- [ ] Task" to "- [~] Task"
git add tasks.md && git commit -m "🤖 Started: Task name" && git push origin helix-specs
%[1]s

**After completing EACH task:**
%[1]sbash
cd ~/work/helix-specs/design/tasks/%[4]s
# Edit tasks.md: change "- [~] Task" to "- [x] Task"
git add tasks.md && git commit -m "🤖 Completed: Task name" && git push origin helix-specs
%[1]s

**WHY:** The UI shows your live progress by monitoring pushes to helix-specs. No push = user thinks you're stuck!

---

## 🚨 CRITICAL: DO THE BARE MINIMUM - BE CONCISE 🚨

- Only do what is STRICTLY NECESSARY to meet the requirements
- DO NOT write code unless absolutely required - prefer existing tools, commands, or scripts
- Simple tasks should have simple solutions (e.g., shell commands, not Python scripts)
- Avoid over-engineering - no abstractions, helpers, or utilities unless explicitly needed
- If a task can be done with a one-liner, use a one-liner
- DO NOT add extra features, error handling, or edge cases beyond what's specified
- Match solution complexity to task complexity - simple tasks get simple solutions

**Don't over-engineer simple tasks:**
- "Start a container" → docker-compose.yaml or docker run in .helix/startup.sh, NOT a Python framework
- "Create sample data" → write data directly to files (unless it's too large or complex to write by hand)
- "Run X at startup" → add to .helix/startup.sh (runs at sandbox startup), NOT a service wrapper

**.helix/startup.sh - Startup Script:**
- Located in the primary repo, runs automatically at sandbox startup
- Use for: starting containers, background services, environment setup
- MUST be idempotent (safe to run multiple times) - use "docker compose up -d" not "docker run"
- After modifying, run it manually to start services for this session (changes apply to future sessions automatically)

---

## Your Mission

1. Create feature branch: %[1]sgit checkout -b %[2]s%[1]s
2. Read design docs from ~/work/helix-specs/design/tasks/%[4]s/
3. Read tasks.md to see your task checklist
4. Work through tasks one by one (discrete, trackable)
5. Mark each task [~] when starting, [x] when done
6. **CRITICAL: Push progress updates to helix-specs after EACH task**
7. Implement code in the main repository
8. Create feature branch and push when all tasks complete: %[1]sgit push origin %[2]s%[1]s

## Guidelines

- ALWAYS mark your progress in tasks.md with [~] and [x]
- **CRITICAL: After ANY change to tasks.md, you MUST commit and push to helix-specs immediately**
- The backend tracks your progress by monitoring pushes to helix-specs
- Follow the technical design - don't add unnecessary complexity
- Implement what's in the acceptance criteria
- Write tests that verify core functionality
- Handle edge cases sensibly, but don't over-engineer

## 📓 Use Design Docs as a Lab Notebook

Update the design documents as you work - they're your living record of discoveries:

**Update design.md when you:**
- Discover something that changes the approach (e.g., "Found existing utility that handles this")
- Hit a blocker or limitation not anticipated in the design
- Make a design decision that differs from the original plan
- Learn something about the codebase relevant to future work

**Update requirements.md when:**
- User changes their mind about requirements
- You discover edge cases that need clarification
- Original requirements were ambiguous and you made a decision

**Format for updates:**
%[1]smarkdown
## Implementation Notes (YYYY-MM-DD)

### Discoveries
- Found that X already exists in Y, reusing instead of building new
- Database schema requires Z constraint not originally planned

### Decisions Made
- Chose approach A over B because [reason]
- Simplified scope: skipping X as it's not needed for MVP

### Blockers Resolved
- Issue: Could not access X
- Resolution: Used Y instead
%[1]s

**⚠️ PUSH REQUIREMENTS:**
- After completing each task: commit and push to helix-specs
- After modifying requirements.md: commit and push to helix-specs
- After modifying design.md: commit and push to helix-specs
- After modifying tasks.md: commit and push to helix-specs
- The orchestrator monitors these pushes to track your progress

Start by reading the spec documents from the worktree, then work through the task list systematically.

---

**Task:** %[5]s
**SpecTask ID:** %[6]s
**Feature Branch:** %[2]s
**Base Branch:** %[3]s
**Design Documents:** ~/work/helix-specs/design/tasks/%[4]s/

**Original User Request:**
%[7]s

🚨 **REMEMBER:** Push to helix-specs after EVERY task change! The UI tracks your progress via git pushes. 🚨
`, "```", branchName, baseBranch, taskDirName, task.Name, task.ID, task.OriginalPrompt)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Msg("Sending approval instruction to agent")

	return s.sendMessage(ctx, sessionID, userID, message)
}

// SendImplementationReviewRequest notifies agent that implementation is ready for review
func (s *AgentInstructionService) SendImplementationReviewRequest(
	ctx context.Context,
	sessionID string,
	userID string,
	branchName string,
) error {
	message := fmt.Sprintf(`# Implementation Review 🔍

Great work pushing your changes! The implementation is now ready for review.

The user will test your work. If this is a web application, please:

1. Start the development server
2. Provide the URL where the user can test the application
3. Answer any questions about your implementation

**Branch:** %[1]s%[2]s%[1]s

I'm here to help with any feedback or iterations needed.
`, "```", branchName)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Msg("Sending implementation review request to agent")

	return s.sendMessage(ctx, sessionID, userID, message)
}

// SendMergeInstruction tells agent to merge their branch to main
func (s *AgentInstructionService) SendMergeInstruction(
	ctx context.Context,
	sessionID string,
	userID string,
	branchName string,
	baseBranch string,
) error {
	message := fmt.Sprintf(`# Implementation Approved! ✅

Your implementation has been approved. Please merge to main:

**Steps:**
1. %[1]sgit checkout %[2]s%[1]s
2. %[1]sgit pull origin %[2]s%[1]s (ensure up to date)
3. %[1]sgit merge %[3]s%[1]s
4. %[1]sgit push origin %[2]s%[1]s

Let me know once the merge is complete!
`, "```", baseBranch, branchName)

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
