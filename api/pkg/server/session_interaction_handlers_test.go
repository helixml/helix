package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestListInteractionsPendingQuestionFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	session := &types.Session{ID: "session-1", Owner: "user-1", GenerationID: 2}

	mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	mockStore.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			require.True(t, query.HasPendingQuestion)
			require.Equal(t, session.GenerationID, query.GenerationID)
			require.Equal(t, "created ASC, id ASC", query.Order)
			return []*types.Interaction{{ID: "interaction-question"}}, 1, nil
		})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/session-1/interactions?pending_question=true", http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: session.Owner}))

	response, httpErr := server.listInteractions(httptest.NewRecorder(), req)
	require.Nil(t, httpErr)
	require.Len(t, response.Interactions, 1)
}

func TestListInteractionsDescendingOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	session := &types.Session{ID: "session-1", Owner: "user-1", GenerationID: 2}

	mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	mockStore.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			require.Equal(t, "created DESC, id DESC", query.Order)
			return nil, 0, nil
		})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/session-1/interactions?order=desc", http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: session.Owner}))

	_, httpErr := server.listInteractions(httptest.NewRecorder(), req)
	require.Nil(t, httpErr)
}
