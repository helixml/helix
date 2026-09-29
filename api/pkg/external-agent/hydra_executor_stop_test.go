package external_agent

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

type failingConnman struct {
	err error
}

func (c failingConnman) Dial(context.Context, string) (net.Conn, error) {
	return nil, c.err
}

func TestHydraExecutorRejectsEmptySessionIDWithoutStoreAccess(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	h := &HydraExecutor{store: mockStore}

	assert.EqualError(t, h.StopDesktop(context.Background(), ""), "session ID is required to stop desktop")
	assert.EqualError(t, h.revokeSessionAPIKeys(context.Background(), ""), "session ID is required to revoke session keys")
}

func TestHydraExecutorStopFailurePreservesLiveSessionState(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.connman = failingConnman{err: errors.New("hydra disconnected")}
	tracked := &ZedSession{
		SessionID:   "ses_live",
		ContainerID: "container-live",
		SandboxID:   "runner-1",
		Status:      "running",
	}
	h.sessions["ses_live"] = tracked

	err := h.StopDesktop(context.Background(), "ses_live")

	assert.ErrorContains(t, err, "hydra disconnected")
	assert.Same(t, tracked, h.sessions["ses_live"], "failed stop must preserve in-memory tracking")
	assert.Equal(t, "container-live", h.sessions["ses_live"].ContainerID)
	// The strict mock has no store expectations: a failed delete must not revoke
	// API keys, close billing, decrement capacity, or clear session status.
}
