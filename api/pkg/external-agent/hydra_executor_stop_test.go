package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestHydraExecutorRejectsEmptySessionIDWithoutStoreAccess(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	h := &HydraExecutor{store: mockStore}

	assert.EqualError(t, h.StopDesktop(context.Background(), ""), "session ID is required to stop desktop")
	assert.EqualError(t, h.revokeSessionAPIKeys(context.Background(), ""), "session ID is required to revoke session keys")
}
