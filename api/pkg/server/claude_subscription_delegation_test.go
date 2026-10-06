package server

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdateClaudeSubscriptionDelegationRejectsAnotherOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	user := &types.User{ID: "usr_a"}
	sub := &types.ClaudeSubscription{ID: "sub_a", OwnerID: user.ID, OwnerType: types.OwnerTypeUser}

	mockStore.EXPECT().GetClaudeSubscription(gomock.Any(), "sub_a").Return(sub, nil)
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{UserID: user.ID}).Return(
		[]*types.OrganizationMembership{{UserID: user.ID, OrganizationID: "org_1"}}, nil,
	)
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(
		&types.OrganizationMembership{UserID: user.ID, OrganizationID: "org_1", Role: types.OrganizationRoleOwner}, nil,
	)
	mockStore.EXPECT().GetOrgCodeAgentHarness(gomock.Any(), "org_1", types.CodeAgentRuntimeClaudeCode).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().UpdateClaudeSubscriptionDelegation(gomock.Any(), "sub_a", []string{"org_1"}).Return(
		nil, &store.ClaudeSubscriptionDelegationConflictError{OrganizationID: "org_1", OwnerID: "usr_b"},
	)
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_1"}).Return(
		&types.Organization{ID: "org_1", Name: "customer", DisplayName: "Customer Org"}, nil,
	)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_b"}).Return(
		&types.User{ID: "usr_b", FullName: "Beatrice"}, nil,
	)

	req, err := http.NewRequest(http.MethodPut, "/api/v1/claude-subscriptions/sub_a/delegation", bytes.NewBufferString(`{"delegated_org_ids":["org_1"]}`))
	require.NoError(t, err)
	req = mux.SetURLVars(req, map[string]string{"id": "sub_a"})
	req = req.WithContext(setRequestUser(req.Context(), *user))

	_, httpErr := server.updateClaudeSubscriptionDelegation(nil, req)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusConflict, httpErr.StatusCode)
	require.Equal(t, "Customer Org already uses Beatrice's Claude subscription. Ask Beatrice to remove that delegation before adding yours.", httpErr.Message)
	require.NotContains(t, httpErr.Message, "org_1")
}
