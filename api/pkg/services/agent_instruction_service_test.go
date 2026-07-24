package services

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendApprovalInstruction_EnqueuesAsInterrupt is the regression guard for
// bug (b): the implementation kickoff is a phase-transition control signal that
// MUST be delivered even while interrupt review comments are streaming. As a
// non-interrupt queue prompt it required an idle session, lost every race, and
// was eventually abandoned past the retry cap. It must now be enqueued as an
// interrupt (like the sibling "request changes" control signal) so it preempts
// and is never starved.
//
// A task with no ProjectID short-circuits the guideline/repo lookups, so the
// method makes no store calls — the only observable effect is the enqueue.
func TestSendApprovalInstruction_EnqueuesAsInterrupt(t *testing.T) {
	var (
		capturedInterrupt bool
		capturedMessage   string
		capturedUserID    string
		enqueueCalls      int
	)

	enqueuer := func(_ context.Context, _ *types.SpecTask, message string, interrupt bool, notifyUserID string) error {
		enqueueCalls++
		capturedInterrupt = interrupt
		capturedMessage = message
		capturedUserID = notifyUserID
		return nil
	}

	svc := NewAgentInstructionService(nil, enqueuer, nil)

	task := &types.SpecTask{
		ID:                "spt_kickoff",
		PlanningSessionID: "ses_kickoff",
		// No ProjectID: skips guideline/repo store lookups entirely.
	}

	err := svc.SendApprovalInstruction(
		context.Background(),
		"ses_kickoff",
		"user_approver",
		task,
		"feature/002325-x",
		"main",
		"helix",
	)
	require.NoError(t, err)

	assert.Equal(t, 1, enqueueCalls, "approval instruction must be enqueued exactly once")
	assert.True(t, capturedInterrupt, "implementation kickoff must be enqueued as an interrupt so it is never starved")
	assert.Contains(t, capturedMessage, "CURRENT PHASE: IMPLEMENTATION", "kickoff carries the implementation phase prompt")
	assert.Equal(t, "user_approver", capturedUserID, "approver is carried as notifyUserID for response streaming")
}
