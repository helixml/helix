package services

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// AgentInstructionService sends automated instructions to agent sessions
type AgentInstructionService struct {
	controller *controller.Controller
}

// NewAgentInstructionService creates a new agent instruction service
func NewAgentInstructionService(controller *controller.Controller) *AgentInstructionService {
	return &AgentInstructionService{
		controller: controller,
	}
}

// SendApprovalInstruction sends a message to the agent to start implementation
func (s *AgentInstructionService) SendApprovalInstruction(
	ctx context.Context,
	sessionID string,
	branchName string,
	baseBranch string,
) error {
	message := fmt.Sprintf(`# Design Approved! 🎉

Your design has been approved. Please begin implementation:

**Steps:**
1. Create and checkout feature branch: %[1]s git checkout -b %[2]s%[1]s
2. Implement the features according to the approved design
3. Write tests for all new functionality
4. Commit your work with clear, descriptive messages
5. When ready for review, push your branch: %[1]s git push origin %[2]s%[1]s

I'll be watching for your push and will notify you when it's time for review.

**Design Documents:**
The approved design documents are in your repository under the design/ directory.
`, "```", branchName)

	log.Info().
		Str("session_id", sessionID).
		Str("branch_name", branchName).
		Msg("Sending approval instruction to agent")

	return s.sendMessage(ctx, sessionID, message)
}

// SendImplementationReviewRequest notifies agent that implementation is ready for review
func (s *AgentInstructionService) SendImplementationReviewRequest(
	ctx context.Context,
	sessionID string,
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

	return s.sendMessage(ctx, sessionID, message)
}

// SendMergeInstruction tells agent to merge their branch to main
func (s *AgentInstructionService) SendMergeInstruction(
	ctx context.Context,
	sessionID string,
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

	return s.sendMessage(ctx, sessionID, message)
}

// sendMessage sends a system message to an agent session
func (s *AgentInstructionService) sendMessage(ctx context.Context, sessionID string, message string) error {
	// Get the session
	session, err := s.controller.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Create a system interaction
	now := time.Now()
	interaction := &types.Interaction{
		ID:        system.GenerateUUID(),
		Created:   now,
		Updated:   now,
		Scheduled: now,
		Completed: now,
		Creator:   types.CreatorTypeSystem,
		Mode:      types.SessionModeInference,
		Message:   message,
		State:     types.InteractionStateComplete,
		Finished:  true,
	}

	// Add the interaction to the session
	session.Interactions = append(session.Interactions, interaction)

	// Update the session
	err = s.controller.UpdateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to update session with instruction: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Msg("Successfully sent instruction to agent")

	return nil
}
