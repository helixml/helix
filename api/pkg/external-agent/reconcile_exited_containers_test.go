package external_agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// stubProber is a devContainerProber that answers from a fixture map. A session
// absent from the map is reported as "not found", mirroring a container hydra
// has forgotten entirely.
type stubProber struct {
	containers map[string]*hydra.DevContainerResponse
	calls      []string
}

func (s *stubProber) GetDevContainer(_ context.Context, sessionID string) (*hydra.DevContainerResponse, error) {
	s.calls = append(s.calls, sessionID)
	container, ok := s.containers[sessionID]
	if !ok {
		return nil, fmt.Errorf("dev container not found for session: %s", sessionID)
	}
	return container, nil
}

func TestFilterRunningContainers(t *testing.T) {
	containers := []hydra.DevContainerResponse{
		{SessionID: "ses_running", Status: hydra.DevContainerStatusRunning},
		{SessionID: "ses_exited", Status: hydra.DevContainerStatusStopped},
		{SessionID: "ses_also_running", Status: hydra.DevContainerStatusRunning},
		// An unset status must not be optimistically treated as running.
		{SessionID: "ses_unknown"},
	}

	running := filterRunningContainers(containers)

	require.Len(t, running, 2)
	assert.Equal(t, "ses_running", running[0].SessionID)
	assert.Equal(t, "ses_also_running", running[1].SessionID)
}

func TestFilterRunningContainers_Empty(t *testing.T) {
	assert.Empty(t, filterRunningContainers(nil))
	assert.Empty(t, filterRunningContainers([]hydra.DevContainerResponse{
		{SessionID: "ses_exited", Status: hydra.DevContainerStatusStopped},
	}))
}

// The regression this whole change exists for: hydra still tracks the container
// (so GetDevContainer succeeds) but Docker reports it exited. Before the fix the
// probe only checked `err == nil`, treated the dead container as alive, and left
// the session pinned at external_agent_status="running" forever — which is what
// rendered a green "Sandbox running" dot and a ticking "Working for…" timer
// against a container whose entrypoint had died.
func TestMarkMissingSessionsStopped_DowngradesTrackedButExitedContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.sessions["ses_exited"] = &ZedSession{SessionID: "ses_exited", Status: "running"}

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "sbox_1").
		Return([]*types.Session{runningSession("ses_exited")}, nil)
	mockStore.EXPECT().
		GetSession(gomock.Any(), "ses_exited").
		Return(runningSession("ses_exited"), nil)

	var downgraded *types.Session
	mockStore.EXPECT().
		UpdateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s types.Session) (*types.Session, error) {
			downgraded = &s
			return &s, nil
		})

	prober := &stubProber{containers: map[string]*hydra.DevContainerResponse{
		"ses_exited": {SessionID: "ses_exited", Status: hydra.DevContainerStatusStopped},
	}}

	h.markMissingSessionsStopped(context.Background(), "sbox_1", map[string]bool{}, prober)

	require.NotNil(t, downgraded, "an exited-but-tracked container must be downgraded")
	assert.Equal(t, "stopped", downgraded.Metadata.ExternalAgentStatus)
	assert.Empty(t, downgraded.Metadata.ContainerName)
	assert.Equal(t, []string{"ses_exited"}, prober.calls)

	h.mutex.RLock()
	_, stillTracked := h.sessions["ses_exited"]
	h.mutex.RUnlock()
	assert.False(t, stillTracked, "stale in-memory session should be evicted")
}

// The probe is what protects a container started between hydra's list snapshot
// and this sweep. A genuinely running container must survive even though the
// snapshot did not include it.
func TestMarkMissingSessionsStopped_KeepsRunningContainerMissingFromSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "sbox_1").
		Return([]*types.Session{runningSession("ses_fresh")}, nil)
	mockStore.EXPECT().
		GetSession(gomock.Any(), "ses_fresh").
		Return(runningSession("ses_fresh"), nil)
	// No UpdateSession expectation: downgrading here would tear down a live session.

	prober := &stubProber{containers: map[string]*hydra.DevContainerResponse{
		"ses_fresh": {SessionID: "ses_fresh", Status: hydra.DevContainerStatusRunning},
	}}

	h.markMissingSessionsStopped(context.Background(), "sbox_1", map[string]bool{}, prober)
}

// A container hydra has forgotten entirely (probe errors) is also dead.
func TestMarkMissingSessionsStopped_DowngradesUnknownContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "sbox_1").
		Return([]*types.Session{runningSession("ses_removed")}, nil)
	mockStore.EXPECT().
		GetSession(gomock.Any(), "ses_removed").
		Return(runningSession("ses_removed"), nil)

	downgraded := false
	mockStore.EXPECT().
		UpdateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s types.Session) (*types.Session, error) {
			assert.Equal(t, "stopped", s.Metadata.ExternalAgentStatus)
			downgraded = true
			return &s, nil
		})

	prober := &stubProber{containers: map[string]*hydra.DevContainerResponse{}}

	h.markMissingSessionsStopped(context.Background(), "sbox_1", map[string]bool{}, prober)
	assert.True(t, downgraded)
}
