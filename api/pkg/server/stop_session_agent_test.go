package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
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

func TestStopExternalAgentSessionReapsWaitingTurnsAfterDesktopStops(t *testing.T) {
	ctrl := gomock.NewController(t)
	executor := external_agent.NewMockExecutor(ctrl)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore, externalAgentExecutor: executor}
	user := types.User{ID: "usr_owner"}
	session := &types.Session{ID: "ses_stopped", Owner: user.ID, Metadata: types.SessionMetadata{AgentType: "zed_external"}}

	mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	executor.EXPECT().StopDesktop(gomock.Any(), session.ID).Return(nil)
	mockStore.EXPECT().ReapWaitingInteractions(
		gomock.Any(), session.ID, types.InteractionStateInterrupted, "external agent session stopped",
	).Return(nil, nil)
	mockStore.EXPECT().FinishTriggerExecution(
		gomock.Any(), session.ID, types.TriggerExecutionStatusError, "External agent session was stopped before task completion",
	).Return(nil, store.ErrNotFound)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+session.ID+"/stop-external-agent", http.NoBody)
	req = req.WithContext(setRequestUser(req.Context(), user))
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})
	result, httpErr := server.stopExternalAgentSession(httptest.NewRecorder(), req)

	require.Nil(t, httpErr)
	require.Equal(t, session.ID, result["session_id"])
}
