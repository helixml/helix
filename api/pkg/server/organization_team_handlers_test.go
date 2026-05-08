package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateTeam_AutoAddCreatorAsMember(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_1"
	teamID := "team_456"

	gomock.InOrder(
		mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         userID,
		}).Return(&types.OrganizationMembership{
			OrganizationID: orgID,
			UserID:         userID,
			Role:           types.OrganizationRoleOwner,
		}, nil),
		mockStore.EXPECT().CreateTeam(gomock.Any(), gomock.Any()).Return(&types.Team{
			ID:             teamID,
			Name:           "Test Team",
			OrganizationID: orgID,
		}, nil),
		mockStore.EXPECT().CreateTeamMembership(gomock.Any(), &types.TeamMembership{
			TeamID:         teamID,
			UserID:         userID,
			OrganizationID: orgID,
		}).Return(&types.TeamMembership{
			TeamID:         teamID,
			UserID:         userID,
			OrganizationID: orgID,
		}, nil),
	)

	body, _ := json.Marshal(types.CreateTeamRequest{Name: "Test Team"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/teams", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.createTeam(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
}

func TestAddTeamMember_NonOwnerForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_1"
	teamID := "team_456"

	// User is a member, not an owner
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	body, _ := json.Marshal(types.AddOrganizationMemberRequest{UserReference: "other@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/teams/"+teamID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID, "team_id": teamID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.addTeamMember(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Contains(t, rr.Body.String(), "Could not authorize org owner")
}

func TestRemoveTeamMember_NonOwnerForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_1"
	teamID := "team_456"
	memberID := "user_2"

	// User is a member, not an owner
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID+"/teams/"+teamID+"/members/"+memberID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID, "team_id": teamID, "user_id": memberID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.removeTeamMember(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Contains(t, rr.Body.String(), "Could not authorize org owner")
}
