package external_agent

import (
	"context"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// newTestExecutor builds a HydraExecutor wired to a mock store, with the maps
// markMissingSessionsStopped touches initialised. connman is left nil so the
// method runs without a live sandbox; hydraClient is passed as nil by the tests
// so the authoritative GetDevContainer probe is skipped (the snapshot is treated
// as ground truth).
func newTestExecutor(mockStore store.Store) *HydraExecutor {
	return &HydraExecutor{
		store:         mockStore,
		sessions:      make(map[string]*ZedSession),
		creationLocks: make(map[string]*sync.Mutex),
	}
}

func runningSession(id string) *types.Session {
	return &types.Session{
		ID: id,
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			ContainerName:       "ubuntu-external-" + id,
			ContainerID:         "cid-" + id,
			ContainerIP:         "10.0.0.5",
		},
	}
}

// Hydra reports a subset of the sessions the DB thinks are running: the missing
// one is downgraded to stopped with cleared metadata, the live one is left alone,
// and a "starting" session is never touched.
func TestMarkMissingSessionsStopped_DowngradesMissingOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	// Stale in-memory entry for the gone session must be evicted.
	h.sessions["ses_gone"] = &ZedSession{SessionID: "ses_gone", Status: "running"}

	starting := runningSession("ses_starting")
	starting.Metadata.ExternalAgentStatus = "starting"

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "sbox_1").
		Return([]*types.Session{
			runningSession("ses_live"),
			runningSession("ses_gone"),
			starting,
		}, nil)

	// Only the gone session is re-read and updated.
	mockStore.EXPECT().
		GetSession(gomock.Any(), "ses_gone").
		Return(runningSession("ses_gone"), nil)
	mockStore.EXPECT().
		UpdateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s types.Session) (*types.Session, error) {
			assert.Equal(t, "ses_gone", s.ID)
			assert.Equal(t, "stopped", s.Metadata.ExternalAgentStatus)
			assert.Empty(t, s.Metadata.ContainerName)
			assert.Empty(t, s.Metadata.ContainerID)
			assert.Empty(t, s.Metadata.ContainerIP)
			return &s, nil
		})

	live := map[string]bool{"ses_live": true}
	h.markMissingSessionsStopped(context.Background(), "sbox_1", live, nil)

	// Stale in-memory entry evicted.
	h.mutex.RLock()
	_, stillTracked := h.sessions["ses_gone"]
	h.mutex.RUnlock()
	assert.False(t, stillTracked, "stale in-memory session should be evicted")
}

// Full-wipe case: hydra reports zero containers, so every DB-running session on
// the sandbox is downgraded.
func TestMarkMissingSessionsStopped_ZeroContainersDowngradesAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), "sbox_1").
		Return([]*types.Session{
			runningSession("ses_a"),
			runningSession("ses_b"),
		}, nil)

	for _, id := range []string{"ses_a", "ses_b"} {
		mockStore.EXPECT().GetSession(gomock.Any(), id).Return(runningSession(id), nil)
	}
	updated := map[string]bool{}
	mockStore.EXPECT().
		UpdateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s types.Session) (*types.Session, error) {
			assert.Equal(t, "stopped", s.Metadata.ExternalAgentStatus)
			updated[s.ID] = true
			return &s, nil
		}).
		Times(2)

	h.markMissingSessionsStopped(context.Background(), "sbox_1", map[string]bool{}, nil)

	assert.True(t, updated["ses_a"] && updated["ses_b"], "both sessions should be downgraded")
}

// A "local" sandbox is not container-counted and must be skipped entirely.
func TestMarkMissingSessionsStopped_SkipsLocalSandbox(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	// No store calls expected.
	h.markMissingSessionsStopped(context.Background(), "local", map[string]bool{}, nil)
	h.markMissingSessionsStopped(context.Background(), "", map[string]bool{}, nil)
}
