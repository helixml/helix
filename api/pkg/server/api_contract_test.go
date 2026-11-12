package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// boardSettingsRouter registers the board-settings routes and the task route
// they share a prefix with, in server.go's order.
func boardSettingsRouter(srv *HelixAPIServer) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/spec-tasks/board-settings", srv.getBoardSettings).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/spec-tasks/board-settings", srv.updateBoardSettings).Methods(http.MethodPut)
	r.HandleFunc("/api/v1/spec-tasks/{taskId}", srv.getTask).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/spec-tasks/{taskId}", srv.updateSpecTask).Methods(http.MethodPut)
	return r
}

func serveAs(h http.Handler, user types.User, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req = req.WithContext(setRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// GET /spec-tasks/board-settings is a real route keyed by project_id (it used
// to fall through to /spec-tasks/{taskId} and answer "task not found" for
// every input): no project is a 400, an unknown or someone else's project the
// same 404, and the caller's project its settings.
func TestBoardSettings_Contract(t *testing.T) {
	owner := types.User{ID: "usr_owner"}
	stranger := types.User{ID: "usr_stranger"}
	project := &types.Project{ID: "prj_1", UserID: owner.ID, Metadata: types.ProjectMetadata{
		BoardSettings: &types.BoardSettings{WIPLimits: types.WIPLimits{Planning: 4, Review: 1, Implementation: 6}},
	}}

	m := store.NewMockStore(gomock.NewController(t))
	m.EXPECT().GetProject(gomock.Any(), "prj_1").Return(project, nil).AnyTimes()
	m.EXPECT().GetProject(gomock.Any(), "prj_missing").Return(nil, store.ErrNotFound).AnyTimes()
	h := boardSettingsRouter(&HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}})

	rr := serveAs(h, owner, http.MethodGet, "/api/v1/spec-tasks/board-settings", "")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "project_id is required\n", rr.Body.String())

	rr = serveAs(h, owner, http.MethodGet, "/api/v1/spec-tasks/board-settings?project_id=prj_missing", "")
	require.Equal(t, http.StatusNotFound, rr.Code)
	missingBody := rr.Body.String()

	rr = serveAs(h, stranger, http.MethodGet, "/api/v1/spec-tasks/board-settings?project_id=prj_1", "")
	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, missingBody, rr.Body.String(), "a foreign project must look like a missing one")

	rr = serveAs(h, owner, http.MethodGet, "/api/v1/spec-tasks/board-settings?project_id=prj_1", "")
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.JSONEq(t, `{"wip_limits":{"planning":4,"review":1,"implementation":6}}`, rr.Body.String())

	// A stranger cannot change another tenant's settings, and nothing is
	// written (UpdateProject has no expectation).
	rr = serveAs(h, stranger, http.MethodPut, "/api/v1/spec-tasks/board-settings?project_id=prj_1", `{"wip_limits":{"planning":99}}`)
	require.Equal(t, http.StatusNotFound, rr.Code)

	rr = serveAs(h, owner, http.MethodPut, "/api/v1/spec-tasks/board-settings?project_id=prj_1", `{"wip_limits":{"planning":-1}}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

// PUT /spec-tasks/{id} with "archived" used to answer 200 and change nothing.
// Changing it is refused (archiving has its own endpoint) and nothing is
// written; echoing the stored value back, as clients that PUT the object they
// read do, is accepted.
func TestUpdateSpecTask_ArchivedField(t *testing.T) {
	owner := types.User{ID: "usr_owner"}
	m := store.NewMockStore(gomock.NewController(t))
	m.EXPECT().GetSpecTask(gomock.Any(), "spt_1").DoAndReturn(func(context.Context, string) (*types.SpecTask, error) {
		return &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", Archived: true, Status: types.TaskStatusBacklog}, nil
	}).Times(2)
	m.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", UserID: owner.ID}, nil).Times(2)
	h := boardSettingsRouter(&HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}})

	rr := serveAs(h, owner, http.MethodPut, "/api/v1/spec-tasks/spt_1", `{"archived":false}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), "/archive")

	m.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
		require.True(t, task.Archived)
		return nil
	})
	rr = serveAs(h, owner, http.MethodPut, "/api/v1/spec-tasks/spt_1", `{"archived":true,"name":"renamed"}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// An invalid completion decision is a 400 that names the completion request,
// not a pull request proposal, and changes nothing.
func TestDecideSpecTaskCompletion_InvalidDecision(t *testing.T) {
	owner := types.User{ID: "usr_owner"}
	m := store.NewMockStore(gomock.NewController(t))
	m.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{ID: "spt_1", ProjectID: "prj_1"}, nil)
	m.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", UserID: owner.ID}, nil)
	srv := &HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}, prProposals: services.NewPRProposalService(m, nil, nil, "")}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/spt_1/completion/decide", strings.NewReader(`{"decision":"maybe"}`))
	req = mux.SetURLVars(req, map[string]string{"spec_task_id": "spt_1"})
	req = req.WithContext(setRequestUser(req.Context(), owner))
	rr := httptest.NewRecorder()
	srv.decideSpecTaskCompletion(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), `invalid completion request: decision must be \"approve\" or \"reject\"`)
	require.NotContains(t, rr.Body.String(), "pull request proposal")
}

func webhookRequest(method, target string, vars map[string]string, user types.User) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req = mux.SetURLVars(req, vars)
	return req.WithContext(setRequestUser(req.Context(), user))
}

// Deleting a webhook endpoint marks it deleted (not merely disabled, which an
// update could undo). The store then no longer returns it, so its deliveries
// sub-resource answers 404.
func TestDeleteWebhookEndpoint_SoftDeletesAndHidesDeliveries(t *testing.T) {
	owner := types.User{ID: "usr_owner"}
	vars := map[string]string{"id": "org_1", "endpoint_id": "whe_1"}
	m := store.NewMockStore(gomock.NewController(t))
	m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_1"}, nil).Times(2)
	m.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationMembership{OrganizationID: "org_1", UserID: owner.ID, Role: types.OrganizationRoleOwner}, nil).Times(2)

	m.EXPECT().GetWebhookEndpoint(gomock.Any(), "org_1", "whe_1").
		Return(&types.WebhookEndpoint{ID: "whe_1", OrganizationID: "org_1", Status: types.WebhookEndpointStatusActive}, nil)
	m.EXPECT().UpdateWebhookEndpointConfig(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ep *types.WebhookEndpoint) error {
			require.Equal(t, types.WebhookEndpointStatusDeleted, ep.Status)
			return nil
		})
	srv := &HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}}

	rr := httptest.NewRecorder()
	srv.deleteWebhookEndpoint(rr, webhookRequest(http.MethodDelete, "/api/v1/organizations/org_1/webhook-endpoints/whe_1", vars, owner))
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())

	// The store filters deleted endpoints out (store_webhooks_test covers it).
	m.EXPECT().GetWebhookEndpoint(gomock.Any(), "org_1", "whe_1").Return(nil, store.ErrNotFound)
	rr = httptest.NewRecorder()
	srv.listWebhookDeliveries(rr, webhookRequest(http.MethodGet, "/api/v1/organizations/org_1/webhook-endpoints/whe_1/deliveries", vars, owner))
	require.Equal(t, http.StatusNotFound, rr.Code)
}
