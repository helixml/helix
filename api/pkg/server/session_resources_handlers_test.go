package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/sessionresources"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	resourcesSessionA = "ses_resourcesa"
	resourcesSandboxA = "sandbox-a"
)

func newSessionResourcesTestServer(t *testing.T) *HelixAPIServer {
	srv := newUserDetailsTestServer(t)
	mockStore := srv.Store.(*store.MockStore)
	mockStore.EXPECT().GetSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.Session, error) {
			switch id {
			case resourcesSessionA:
				return &types.Session{ID: id, OrganizationID: tenantAOrg, Owner: tenantAAdmin, SandboxID: resourcesSandboxA}, nil
			case "ses_taskgone":
				return &types.Session{ID: id, SandboxID: resourcesSandboxA, Metadata: types.SessionMetadata{SpecTaskID: "spt_gone"}}, nil
			case "ses_projectgone":
				return &types.Session{ID: id, SandboxID: resourcesSandboxA, ProjectID: "prj_gone"}, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_gone").Return(nil, store.ErrNotFound).AnyTimes()
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_gone").Return(nil, store.ErrNotFound).AnyTimes()
	srv.sessionResources = sessionresources.New(t.TempDir(), "test")
	return srv
}

func callGetSessionResources(srv *HelixAPIServer, caller *types.User, id string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+id+"/resources", nil)
	req = mux.SetURLVars(req, map[string]string{"id": id})
	req = req.WithContext(setTestRequestUser(req.Context(), caller))
	rr := httptest.NewRecorder()
	system.Wrapper(srv.getSessionResources)(rr, req)
	return rr.Code
}

func TestGetSessionResources_CrossTenant(t *testing.T) {
	srv := newSessionResourcesTestServer(t)
	tests := []struct {
		name   string
		caller *types.User
		id     string
		want   int
	}{
		{"other tenant's owner is refused", &types.User{ID: tenantBAdmin, TokenType: types.TokenTypeAPIKey}, resourcesSessionA, http.StatusForbidden},
		{"other tenant's member is refused", &types.User{ID: tenantBUser}, resourcesSessionA, http.StatusForbidden},
		{"key scoped to another org is refused", &types.User{ID: tenantBAdmin, OrganizationID: tenantBOrg, TokenType: types.TokenTypeAPIKey}, resourcesSessionA, http.StatusForbidden},
		{"key claiming a non-member org is refused", &types.User{ID: tenantBAdmin, OrganizationID: tenantAOrg, TokenType: types.TokenTypeAPIKey}, resourcesSessionA, http.StatusForbidden},
		{"same-org member without project access is refused", &types.User{ID: tenantAUser}, resourcesSessionA, http.StatusForbidden},
		{"unknown session is 404", &types.User{ID: tenantAAdmin}, "ses_missing", http.StatusNotFound},
		{"session owner can read", &types.User{ID: tenantAAdmin}, resourcesSessionA, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, callGetSessionResources(srv, tt.caller, tt.id))
		})
	}
}

func TestGetSessionResourcesRejectsBadWindow(t *testing.T) {
	srv := newSessionResourcesTestServer(t)
	for _, hours := range []string{"0", "721", "x"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+resourcesSessionA+"/resources?hours="+hours, nil)
		req = mux.SetURLVars(req, map[string]string{"id": resourcesSessionA})
		req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: tenantAAdmin}))
		rr := httptest.NewRecorder()
		system.Wrapper(srv.getSessionResources)(rr, req)
		require.Equal(t, http.StatusBadRequest, rr.Code, hours)
	}
}

func TestRecordSessionResourcesOnlyFromTheSessionsSandbox(t *testing.T) {
	srv := newSessionResourcesTestServer(t)
	now := time.Now()
	samples := []types.SessionResourceSample{
		{SessionID: resourcesSessionA, Timestamp: 1, CPUUsageUsec: 7},
		{SessionID: "ses_missing", CPUUsageUsec: 8},
	}

	srv.recordSessionResources(context.Background(), "sandbox-b", samples, now)
	got, err := srv.sessionResources.Read(resourcesSessionA, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Empty(t, got, "a runner must not write another sandbox's session history")

	srv.recordSessionResources(context.Background(), resourcesSandboxA, samples, now)
	got, err = srv.sessionResources.Read(resourcesSessionA, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, now.UnixMilli(), got[0].Timestamp, "the API stamps samples on receipt")
	require.Equal(t, uint64(7), got[0].CPUUsageUsec)
}

func TestRecordSessionResourcesDropsSessionsOfDeletedTasksAndProjects(t *testing.T) {
	srv := newSessionResourcesTestServer(t)
	now := time.Now()
	srv.recordSessionResources(context.Background(), resourcesSandboxA, []types.SessionResourceSample{
		{SessionID: "ses_taskgone"}, {SessionID: "ses_projectgone"},
	}, now)
	for _, id := range []string{"ses_taskgone", "ses_projectgone"} {
		got, err := srv.sessionResources.Read(id, now.Add(-time.Hour), now.Add(time.Hour))
		require.NoError(t, err)
		require.Empty(t, got, "%s: a desktop outliving its task or project must not recreate history", id)
	}
}
