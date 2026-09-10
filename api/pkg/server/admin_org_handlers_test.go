package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestOrganizationSearchNamePrefersDisplayName(t *testing.T) {
	org := &types.Organization{Name: "internal-slug", DisplayName: "Visible Name"}
	assert.Equal(t, "visible name", organizationSearchName(org))

	org.DisplayName = ""
	assert.Equal(t, "internal-slug", organizationSearchName(org))
}

func TestAdminListOrganizationsPaginatesFilteredResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	organizations := []*types.Organization{
		{ID: "org_other", DisplayName: "Other"},
		{ID: "org_b", DisplayName: "Match B"},
		{ID: "org_a", DisplayName: "Match A"},
	}

	mockStore.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{}).Return(organizations, nil)
	mockStore.EXPECT().ListWallets(gomock.Any(), &store.ListWalletsQuery{
		OwnerIDs: []string{"org_b"}, OwnerType: types.OwnerTypeOrg,
	}).Return([]*types.Wallet{}, nil)
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{
		OrganizationID: "org_b",
	}).Return([]*types.OrganizationMembership{}, nil)
	mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: "org_b",
	}).Return([]*types.Project{}, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/admin/orgs?query=match&page=2&per_page=1", nil)
	server.adminListOrganizations(recorder, request)

	require.Equal(t, 200, recorder.Code)
	var response AdminOrganizationsResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	assert.Equal(t, 2, response.Page)
	assert.Equal(t, 1, response.PageSize)
	assert.Equal(t, 2, response.TotalCount)
	assert.Equal(t, 2, response.TotalPages)
	require.Len(t, response.Organizations, 1)
	assert.Equal(t, "org_b", response.Organizations[0].Organization.ID)
}

func TestAdminListOrganizationsHandlesOverflowingPage(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{}).Return(
		[]*types.Organization{{ID: "org_a", DisplayName: "Match A"}}, nil,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/admin/orgs?page=4611686018427387904&per_page=4", nil)
	server.adminListOrganizations(recorder, request)

	require.Equal(t, 200, recorder.Code)
	var response AdminOrganizationsResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	assert.Empty(t, response.Organizations)
	assert.Equal(t, 1, response.TotalCount)
}
