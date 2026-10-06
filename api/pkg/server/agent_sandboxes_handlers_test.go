package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEnrichDevContainerUsesWebServiceSandboxRow(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	deletedAt := time.Now()
	row := &types.Sandbox{
		ID: "sbx_web", Name: "fallback", Purpose: types.SandboxPurposeWebService,
		Owner: "user_1", OrganizationID: "org_1", ProjectID: "prj_1", CreatedAt: time.Now().Add(-time.Hour), DeletedAt: &deletedAt,
	}
	dc := &DevContainerWithClients{}
	dc.SessionID = row.ID

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "user_1"}).Return(&types.User{ID: "user_1", FullName: "Customer"}, nil)
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_1"}).Return(&types.Organization{ID: "org_1", DisplayName: "FindAI"}, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", Name: "Website"}, nil)

	server.enrichDevContainer(context.Background(), dc, map[string]*types.Sandbox{row.ID: row})

	require.Equal(t, types.SandboxPurposeWebService, dc.Purpose)
	require.Equal(t, "Web service: Website", dc.SessionName)
	require.NotEmpty(t, dc.SessionAge)
	require.Equal(t, "Customer", dc.OwnerName)
	require.Equal(t, "FindAI", dc.OrganizationName)
	require.Equal(t, "Website", dc.ProjectName)
	require.Equal(t, "org_1", dc.OrganizationID)
	require.Equal(t, "prj_1", dc.ProjectID)
}

func TestEnrichDevContainerFallsBackToWebServiceSandboxName(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	row := &types.Sandbox{ID: "sbx_web", Name: "FindAI fallback", Purpose: types.SandboxPurposeWebService, ProjectID: "prj_1"}
	dc := &DevContainerWithClients{}
	dc.SessionID = row.ID
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(nil, errors.New("not found"))

	server.enrichDevContainer(context.Background(), dc, map[string]*types.Sandbox{row.ID: row})

	require.Equal(t, "Web service: FindAI fallback", dc.SessionName)
}

func TestAgentSandboxesDebugFailsClosedWhenSandboxRowsCannotBeListed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().ListSandboxInstances(gomock.Any()).Return(nil, nil)
	mockStore.EXPECT().ListSandboxes(gomock.Any(), &store.ListSandboxesQuery{IncludeDeleted: true}).Return(nil, errors.New("database unavailable"))

	rw := httptest.NewRecorder()
	server.getAgentSandboxesDebug(rw, httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-sandboxes/debug", nil))

	require.Equal(t, http.StatusInternalServerError, rw.Code)
	require.Contains(t, rw.Body.String(), "Failed to list sandbox rows")
}
