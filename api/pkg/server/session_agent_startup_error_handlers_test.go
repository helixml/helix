package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestReportAgentStartupErrorFailsLatestWaitingInteraction(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	session := &types.Session{ID: "ses_1", Owner: "user_1", GenerationID: 2}
	interaction := &types.Interaction{
		ID:           "int_1",
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		State:        types.InteractionStateWaiting,
	}

	mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	mockStore.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			assert.Equal(t, session.ID, query.SessionID)
			assert.Equal(t, session.GenerationID, query.GenerationID)
			assert.Equal(t, 1, query.PerPage)
			assert.Equal(t, "created DESC", query.Order)
			return []*types.Interaction{interaction}, 1, nil
		},
	)
	mockStore.EXPECT().MarkInteractionErrorIfWaiting(
		gomock.Any(), interaction.ID, interaction.GenerationID,
		"Agent startup failed: failed to fetch config: status 422: provider not enabled",
	).Return(true, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/ses_1/agent-startup-error", bytes.NewBufferString(
		`{"error":"failed to fetch config: status 422: provider not enabled"}`,
	))
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: session.Owner}))

	response, httpErr := server.reportAgentStartupError(httptest.NewRecorder(), req)
	require.Nil(t, httpErr)
	require.NotNil(t, response)
	assert.True(t, response.Transitioned)
	assert.Equal(t, interaction.ID, response.InteractionID)
	assert.Equal(t, types.InteractionStateError, interaction.State)
	assert.Contains(t, interaction.Error, "provider not enabled")
}

func TestReportAgentStartupErrorIsIdempotentAfterInteractionCompletes(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	session := &types.Session{ID: "ses_1", Owner: "user_1", GenerationID: 2}

	mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	mockStore.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int_1", State: types.InteractionStateComplete}}, int64(1), nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/ses_1/agent-startup-error", bytes.NewBufferString(
		`{"error":"late failure"}`,
	))
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: session.Owner}))

	response, httpErr := server.reportAgentStartupError(httptest.NewRecorder(), req)
	require.Nil(t, httpErr)
	require.NotNil(t, response)
	assert.False(t, response.Transitioned)
}
