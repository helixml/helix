package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// idleSession is a session the idle checker has already shut down: the DB row is
// terminal, but (as in production) the container metadata was never cleared.
func idleSession(id string) *types.Session {
	session := runningSession(id)
	session.Metadata.ExternalAgentStatus = "terminated_idle"
	return session
}

// The production bug. StopDesktop evicts the in-memory entry and the idle
// checker writes "terminated_idle", but a discovery sweep racing that stop
// re-adds the entry as "running" and the terminal DB write lands last. The two
// stores then permanently disagree, and each half used to hide the other: the
// candidate scan read only the DB, saw a terminal row, and skipped the session
// every sweep — so the phantom entry was never evicted — while StartDesktop
// short-circuited on that same entry and refused to create a container. Resume
// answered 200 with an empty DevContainerID and started nothing, forever.
//
// The phantom must be evicted even though the DB row needs no downgrade.
func TestMarkMissingSessionsStopped_EvictsPhantomWithTerminalDBRow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.sessions["ses_phantom"] = &ZedSession{
		SessionID:   "ses_phantom",
		Status:      "running",
		ContainerID: "cid-ses_phantom",
		SandboxID:   "local",
	}

	// The DB row is terminal, so the DB contributes no candidate at all — the
	// in-memory entry is the only reason this session gets looked at.
	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "local").
		Return([]*types.Session{idleSession("ses_phantom")}, nil)
	mockStore.EXPECT().
		GetSession(gomock.Any(), "ses_phantom").
		Return(idleSession("ses_phantom"), nil)

	// Crucially NO UpdateSession: "terminated_idle" is already correct, and
	// rewriting it to "stopped" would lose why the desktop went away.
	prober := &stubProber{containers: map[string]*hydra.DevContainerResponse{}}

	h.markMissingSessionsStopped(context.Background(), "local", map[string]bool{}, prober)

	h.mutex.RLock()
	_, stillTracked := h.sessions["ses_phantom"]
	h.mutex.RUnlock()
	assert.False(t, stillTracked, "phantom entry must be evicted so StartDesktop can recreate the container")
	assert.Equal(t, []string{"ses_phantom"}, prober.calls, "the phantom must be probed before eviction")
}

// The safety counterpart: an in-memory entry that hydra's snapshot missed but
// whose container the authoritative probe reports running must be left alone.
// Evicting it would let a second container be created for a live session.
func TestMarkMissingSessionsStopped_KeepsPhantomWhoseContainerIsAlive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.sessions["ses_alive"] = &ZedSession{
		SessionID:   "ses_alive",
		Status:      "running",
		ContainerID: "cid-ses_alive",
		SandboxID:   "local",
	}

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "local").
		Return([]*types.Session{idleSession("ses_alive")}, nil)
	mockStore.EXPECT().
		GetSession(gomock.Any(), "ses_alive").
		Return(idleSession("ses_alive"), nil)

	prober := &stubProber{containers: map[string]*hydra.DevContainerResponse{
		"ses_alive": {SessionID: "ses_alive", Status: hydra.DevContainerStatusRunning},
	}}

	h.markMissingSessionsStopped(context.Background(), "local", map[string]bool{}, prober)

	h.mutex.RLock()
	_, stillTracked := h.sessions["ses_alive"]
	h.mutex.RUnlock()
	assert.True(t, stillTracked, "a session whose container the probe reports running must survive")
}

// A sweep of one sandbox must not evict entries pinned to another. Each sandbox
// reports only its own containers, so treating another sandbox's absence from
// this snapshot as death would tear down healthy sessions.
func TestMarkMissingSessionsStopped_IgnoresPhantomsOnOtherSandboxes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.sessions["ses_elsewhere"] = &ZedSession{
		SessionID:   "ses_elsewhere",
		Status:      "running",
		ContainerID: "cid-ses_elsewhere",
		SandboxID:   "sbox_2",
	}

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "sbox_1").
		Return([]*types.Session{}, nil)

	// No GetSession and no probe: the other sandbox's session is never a
	// candidate for this sweep.
	prober := &stubProber{containers: map[string]*hydra.DevContainerResponse{}}

	h.markMissingSessionsStopped(context.Background(), "sbox_1", map[string]bool{}, prober)

	h.mutex.RLock()
	_, stillTracked := h.sessions["ses_elsewhere"]
	h.mutex.RUnlock()
	assert.True(t, stillTracked, "sessions on other sandboxes must be untouched")
	assert.Empty(t, prober.calls)
}

// A phantom whose DB row still claims to be running gets both remediations: the
// map entry evicted AND the row downgraded. This is the pre-existing behaviour,
// pinned so the candidate-set rework can't silently drop the DB downgrade.
func TestMarkMissingSessionsStopped_PhantomWithRunningDBRowIsAlsoDowngraded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.sessions["ses_both"] = &ZedSession{
		SessionID:   "ses_both",
		Status:      "running",
		ContainerID: "cid-ses_both",
		SandboxID:   "local",
	}

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "local").
		Return([]*types.Session{runningSession("ses_both")}, nil)
	mockStore.EXPECT().
		GetSession(gomock.Any(), "ses_both").
		Return(runningSession("ses_both"), nil)

	var downgraded *types.Session
	mockStore.EXPECT().
		UpdateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s types.Session) (*types.Session, error) {
			downgraded = &s
			return &s, nil
		})

	prober := &stubProber{containers: map[string]*hydra.DevContainerResponse{}}

	h.markMissingSessionsStopped(context.Background(), "local", map[string]bool{}, prober)

	require.NotNil(t, downgraded, "a DB row still claiming to run must be downgraded")
	assert.Equal(t, "stopped", downgraded.Metadata.ExternalAgentStatus)
	assert.Empty(t, downgraded.Metadata.ContainerName)

	h.mutex.RLock()
	_, stillTracked := h.sessions["ses_both"]
	h.mutex.RUnlock()
	assert.False(t, stillTracked)

	// Probed exactly once — the session appears in both the DB scan and the
	// in-memory scan and must not be reconciled twice.
	assert.Equal(t, []string{"ses_both"}, prober.calls)
}

func TestStaleReconcileCandidates(t *testing.T) {
	h := newTestExecutor(nil)
	h.sessions["ses_phantom"] = &ZedSession{SessionID: "ses_phantom", SandboxID: "local"}
	h.sessions["ses_dupe"] = &ZedSession{SessionID: "ses_dupe", SandboxID: "local"}
	h.sessions["ses_live"] = &ZedSession{SessionID: "ses_live", SandboxID: "local"}
	h.sessions["ses_other_sandbox"] = &ZedSession{SessionID: "ses_other_sandbox", SandboxID: "sbox_9"}
	// An unset SandboxID means the single-node "local" sandbox.
	h.sessions["ses_unset_sandbox"] = &ZedSession{SessionID: "ses_unset_sandbox"}

	starting := runningSession("ses_starting")
	starting.Metadata.ExternalAgentStatus = "starting"
	noContainer := runningSession("ses_no_container")
	noContainer.Metadata.ContainerName = ""

	sessions := []*types.Session{
		runningSession("ses_dupe"),    // in both sources — must appear once
		runningSession("ses_db_only"), // DB-running, not tracked in memory
		runningSession("ses_live"),    // hydra says alive — excluded
		idleSession("ses_phantom"),    // terminal row; reachable only via the map
		starting,                      // mid-flight start — never a candidate
		noContainer,                   // running but no container name
		idleSession("ses_long_gone"),  // terminal and untracked — nothing to do
	}

	candidates := h.staleReconcileCandidates(sessions, "local", map[string]bool{"ses_live": true})

	assert.ElementsMatch(t, []string{
		"ses_dupe",
		"ses_db_only",
		"ses_phantom",
		"ses_unset_sandbox",
	}, candidates)
}
