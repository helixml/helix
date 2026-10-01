package server

import (
	"context"
	"errors"
	"testing"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStopSessionAgentDoesNotReapWhenDesktopStopFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	executor := external_agent.NewMockExecutor(ctrl)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore, externalAgentExecutor: executor}

	executor.EXPECT().StopDesktop(gomock.Any(), "ses_live").Return(errors.New("hydra disconnected"))

	err := server.stopSessionAgent(context.Background(), "ses_live", "agent stopped by user")

	require.ErrorContains(t, err, "hydra disconnected")
	// The strict store mock has no expectations: the live turn is untouched.
}
