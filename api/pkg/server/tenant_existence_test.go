package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/orgstore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A spec task in another tenant must answer exactly like an ID that does not
// exist, so the status code cannot be used to confirm foreign IDs.
func TestGetTaskForeignTenantMatchesUnknownTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	caller := types.User{ID: "usr_bob", OrganizationID: "org_b"}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_unknown").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_foreign").Return(&types.SpecTask{ID: "spt_foreign", ProjectID: "prj_a"}, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_a").Return(&types.Project{ID: "prj_a", OrganizationID: "org_a", UserID: "usr_alice"}, nil)

	get := func(taskID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/spec-tasks/"+taskID, nil)
		req = mux.SetURLVars(req, map[string]string{"taskId": taskID})
		req = req.WithContext(setRequestUser(req.Context(), caller))
		rec := httptest.NewRecorder()
		server.getTask(rec, req)
		return rec
	}

	unknown, foreign := get("spt_unknown"), get("spt_foreign")
	require.Equal(t, http.StatusNotFound, unknown.Code)
	require.Equal(t, unknown.Code, foreign.Code)
	require.Equal(t, unknown.Body.String(), foreign.Body.String())
}

func TestWriteSpecTaskAuthzError(t *testing.T) {
	outside := []error{
		store.ErrNotFound,
		orgstore.ErrKeyOrgScope,
		fmt.Errorf("%w: key is scoped to project prj_b", errProjectScopedKey),
		errNotProjectOwner,
	}
	for _, err := range outside {
		rec := httptest.NewRecorder()
		writeSpecTaskAuthzError(rec, err)
		require.Equal(t, http.StatusNotFound, rec.Code, err.Error())
		require.Equal(t, "task not found\n", rec.Body.String())
	}

	// A member without a grant is inside the tenant: 403, without the
	// internal reason.
	rec := httptest.NewRecorder()
	writeSpecTaskAuthzError(rec, errors.New("user is not authorized to perform this action"))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "not authorized to access this task\n", rec.Body.String())
}

func TestListOrganizationsOrgScopedKeySeesOnlyItsOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	caller := types.User{ID: "usr_multi", OrganizationID: "org_a"}

	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{UserID: caller.ID}).
		Return([]*types.OrganizationMembership{{OrganizationID: "org_a"}, {OrganizationID: "org_b"}}, nil)
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_a"}).Return(&types.Organization{ID: "org_a"}, nil)
	mockStore.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	req = req.WithContext(setRequestUser(req.Context(), caller))
	rec := httptest.NewRecorder()
	server.listOrganizations(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var orgs []types.Organization
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &orgs))
	require.Len(t, orgs, 1)
	require.Equal(t, "org_a", orgs[0].ID)
}

func TestCreateOrgAPIKeyRejectsUnsupportedFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	expectResolveOrganizationByID(mockStore, "org_123")
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationMembership{OrganizationID: "org_123", UserID: "usr_1", Role: types.OrganizationRoleOwner}, nil)
	// No CreateAPIKey expectation: the key must not be created.

	body := []byte(`{"name":"ci","expires_at":"2026-01-01T00:00:00Z"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/org_123/api_keys", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "org_123"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_1"}))
	rec := httptest.NewRecorder()
	server.createOrgAPIKey(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "expires_at")
}
