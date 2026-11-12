package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetSessionOperatorAccess(t *testing.T) {
	t.Run("owner", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		server := &HelixAPIServer{Store: mockStore}
		session := &types.Session{ID: "ses_1", Owner: "user_1", ProjectID: "prj_1"}
		mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
		mockStore.EXPECT().GetProject(gomock.Any(), session.ProjectID).Return(&types.Project{
			ID:     session.ProjectID,
			UserID: session.Owner,
		}, nil)

		response := requestSessionOperatorAccess(server, session.ID, types.User{ID: session.Owner})

		require.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("missing session", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		server := &HelixAPIServer{Store: mockStore}
		mockStore.EXPECT().GetSession(gomock.Any(), "ses_missing").Return(nil, store.ErrNotFound)

		response := requestSessionOperatorAccess(server, "ses_missing", types.User{ID: "user_1"})

		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("other user", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		server := &HelixAPIServer{Store: mockStore}
		session := &types.Session{ID: "ses_1", Owner: "user_1"}
		mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)

		response := requestSessionOperatorAccess(server, session.ID, types.User{ID: "user_2"})

		require.Equal(t, http.StatusForbidden, response.Code)
	})

	t.Run("read-only project member", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		server := &HelixAPIServer{Store: mockStore}
		user := types.User{ID: "user_2"}
		session := &types.Session{
			ID:             "ses_1",
			Owner:          "user_1",
			OrganizationID: "org_1",
			ProjectID:      "prj_1",
		}
		project := &types.Project{
			ID:             session.ProjectID,
			UserID:         session.Owner,
			OrganizationID: session.OrganizationID,
			Metadata:       types.ProjectMetadata{OrgMembersAccess: true},
		}
		mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
		mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: session.OrganizationID,
			UserID:         user.ID,
		}).Return(&types.OrganizationMembership{
			OrganizationID: session.OrganizationID,
			UserID:         user.ID,
			Role:           types.OrganizationRoleMember,
		}, nil).AnyTimes()
		mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil).Times(2)
		mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
			OrganizationID: session.OrganizationID,
			UserID:         user.ID,
		}).Return([]*types.Team{}, nil)
		mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
			OrganizationID: session.OrganizationID,
			UserID:         user.ID,
			ResourceID:     project.ID,
		}).Return([]*types.AccessGrant{}, nil)

		response := requestSessionOperatorAccess(server, session.ID, user)

		require.Equal(t, http.StatusForbidden, response.Code)
	})
}

func requestSessionOperatorAccess(server *HelixAPIServer, sessionID string, user types.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/operator-access", nil)
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()
	server.getSessionOperatorAccess(response, req)
	return response
}
