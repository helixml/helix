package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStartDesktopRejectsMissingEligibleSandboxRunner(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)
	session := &types.Session{ID: "ses_1"}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(session, nil).Times(2)
	mockStore.EXPECT().FindAvailableSandboxInstance(gomock.Any(), "ubuntu", false).Return(nil, nil)

	_, err := executor.StartDesktop(context.Background(), &types.DesktopAgent{
		SessionID:   "ses_1",
		DesktopType: "headless",
	})
	require.EqualError(t, err, `no eligible sandbox runner available for container type "headless" (required image "ubuntu"); check runner status, heartbeat, and advertised desktop image versions`)
}
