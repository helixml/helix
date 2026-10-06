package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

func dispatchedInteraction(requestID string) *types.Interaction {
	at := time.Now().Add(-time.Minute)
	return &types.Interaction{
		ID:                        "int-1",
		State:                     types.InteractionStateWaiting,
		ExternalAgentRequestID:    requestID,
		ExternalAgentDispatchedAt: &at,
	}
}

func undispatchedInteraction(requestID string) *types.Interaction {
	return &types.Interaction{
		ID:                     "int-1",
		State:                  types.InteractionStateWaiting,
		ExternalAgentRequestID: requestID,
	}
}

func TestParseAgentTurnReport(t *testing.T) {
	t.Run("absent field means the agent cannot report", func(t *testing.T) {
		report := parseAgentTurnReport(map[string]interface{}{"agent_name": "opencode"})
		require.False(t, report.Reported)
	})

	t.Run("empty array is an authoritative 'nothing running'", func(t *testing.T) {
		report := parseAgentTurnReport(map[string]interface{}{"active_turns": []interface{}{}})
		require.True(t, report.Reported)
		require.Empty(t, report.Active)
	})

	t.Run("parses request ids and states", func(t *testing.T) {
		report := parseAgentTurnReport(map[string]interface{}{
			"active_turns": []interface{}{
				map[string]interface{}{"request_id": "req-a", "state": "running", "acp_thread_id": "thr"},
				map[string]interface{}{"request_id": "req-b", "state": "queued"},
			},
		})
		require.True(t, report.Reported)
		require.Equal(t, map[string]string{"req-a": "running", "req-b": "queued"}, report.Active)
	})

	t.Run("skips malformed entries but stays authoritative", func(t *testing.T) {
		report := parseAgentTurnReport(map[string]interface{}{
			"active_turns": []interface{}{
				"not-an-object",
				map[string]interface{}{"request_id": ""},
				map[string]interface{}{"request_id": "req-a"},
			},
		})
		require.True(t, report.Reported)
		// A turn with no explicit state still counts as owned.
		require.Equal(t, map[string]string{"req-a": "running"}, report.Active)
	})
}

func TestDecideResume(t *testing.T) {
	reported := func(turns map[string]string) agentTurnReport {
		return agentTurnReport{Reported: true, Active: turns}
	}

	t.Run("agent owns the turn — attach, never re-send", func(t *testing.T) {
		action, _ := decideResume(reported(map[string]string{"req-a": "running"}), dispatchedInteraction("req-a"))
		require.Equal(t, resumeAttach, action)
	})

	t.Run("queued counts as owned", func(t *testing.T) {
		action, _ := decideResume(reported(map[string]string{"req-a": "queued"}), dispatchedInteraction("req-a"))
		require.Equal(t, resumeAttach, action)
	})

	t.Run("agent authoritatively does not have it — deliver even though dispatched", func(t *testing.T) {
		// This is the case only the handshake can decide: Helix handed the turn
		// over, but the agent restarted and lost it. Without a report this
		// would (correctly, conservatively) be attach_and_verify instead.
		action, _ := decideResume(reported(map[string]string{}), dispatchedInteraction("req-a"))
		require.Equal(t, resumeDeliver, action)
	})

	t.Run("agent reports a different turn — deliver", func(t *testing.T) {
		action, _ := decideResume(reported(map[string]string{"req-other": "running"}), dispatchedInteraction("req-a"))
		require.Equal(t, resumeDeliver, action)
	})

	t.Run("no report and never dispatched — deliver", func(t *testing.T) {
		action, _ := decideResume(agentTurnReport{}, undispatchedInteraction("req-a"))
		require.Equal(t, resumeDeliver, action)
	})

	t.Run("no report but already dispatched — attach and verify, not deliver", func(t *testing.T) {
		// The 2026-08-16 regression: this case used to re-send the prompt into a
		// live ACP session on every API restart.
		action, reason := decideResume(agentTurnReport{}, dispatchedInteraction("req-a"))
		require.Equal(t, resumeAttachAndVerify, action)
		require.Contains(t, reason, "already dispatched")
	})

	t.Run("nil interaction is inert", func(t *testing.T) {
		action, _ := decideResume(reported(nil), nil)
		require.Equal(t, resumeDeliver, action)
	})
}
