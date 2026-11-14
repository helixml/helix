package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// A session reconnects: the new connection installs its readiness state
// before the old connection's handler returns. The old handler's teardown
// must not remove the successor's state or its queued commands.
func TestCleanupReadinessState_StaleTeardownKeepsSuccessorState(t *testing.T) {
	manager := NewExternalAgentWSManager()

	old := manager.initReadinessState("ses_A")
	successor := manager.initReadinessState("ses_A")
	defer manager.cleanupReadinessState("ses_A", successor)

	if !manager.queueOrSend("ses_A", types.ExternalAgentCommand{Type: "chat_message"}) {
		t.Fatal("command was not queued for a session that is not ready")
	}

	manager.cleanupReadinessState("ses_A", old)

	manager.readinessMu.RLock()
	state := manager.readinessState["ses_A"]
	manager.readinessMu.RUnlock()
	if state != successor {
		t.Fatal("stale teardown removed the successor connection's readiness state")
	}
	if len(state.PendingQueue) != 1 {
		t.Fatalf("pending queue = %d, want 1", len(state.PendingQueue))
	}
	if manager.isSessionReady("ses_A") {
		t.Fatal("session reported ready after a stale teardown")
	}
}

func TestCleanupReadinessState_OwnerRemovesItsState(t *testing.T) {
	manager := NewExternalAgentWSManager()

	state := manager.initReadinessState("ses_A")
	manager.cleanupReadinessState("ses_A", state)

	manager.readinessMu.RLock()
	_, exists := manager.readinessState["ses_A"]
	manager.readinessMu.RUnlock()
	if exists {
		t.Fatal("owner's teardown left its readiness state behind")
	}
}
