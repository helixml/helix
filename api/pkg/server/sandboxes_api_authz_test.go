package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// The org Sandboxes API reaches into spec-task desktops (session-backed rows
// whose hydra ops id is the session id). Org membership is not enough: a row
// is reached on the terms of its session or project, and a caller refused
// gets 404 before any hydra call or store write.

const (
	sbxOrgA       = "org_a"
	sbxOrgB       = "org_b"
	sbxProjectV   = "prj_v"
	sbxProjectM   = "prj_m"
	sbxTaskOwner  = "usr_task_owner"
	sbxOrgOwner   = "usr_org_owner"
	sbxMember     = "usr_member"
	sbxStranger   = "usr_stranger"
	sbxDeskID     = "sbx_desk_v"
	sbxDeskSessID = "ses_task_v"
)

func sbxDesktopRow() *types.Sandbox {
	return &types.Sandbox{ID: sbxDeskID, OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: sbxDeskSessID, Owner: sbxTaskOwner}
}

func sbxTaskSession() *types.Session {
	return &types.Session{ID: sbxDeskSessID, Owner: sbxTaskOwner, OrganizationID: sbxOrgA, ProjectID: sbxProjectV,
		Metadata: types.SessionMetadata{SpecTaskID: "spt_v", ProjectID: sbxProjectV}}
}

func sbxProjectVRow() *types.Project {
	return &types.Project{ID: sbxProjectV, OrganizationID: sbxOrgA, UserID: sbxTaskOwner}
}

// newSandboxAuthzServer is a server whose sandbox controller and authz run
// over a strict store mock: any store call the test does not expect (a
// write, a teardown) fails it.
func newSandboxAuthzServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	return newSandboxAuthzServerWithProject(t, sbxProjectVRow())
}

func newSandboxAuthzServerWithProject(t *testing.T, projectV *types.Project) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	st := store.NewMockStore(gomock.NewController(t))
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: sbxOrgA}).
		Return(&types.Organization{ID: sbxOrgA, Name: "acme"}, nil).AnyTimes()
	memberships := map[string]types.OrganizationRole{
		sbxTaskOwner: types.OrganizationRoleMember,
		sbxMember:    types.OrganizationRoleMember,
		sbxOrgOwner:  types.OrganizationRoleOwner,
	}
	st.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
			role, ok := memberships[q.UserID]
			if !ok || q.OrganizationID != sbxOrgA {
				return nil, store.ErrNotFound
			}
			return &types.OrganizationMembership{OrganizationID: q.OrganizationID, UserID: q.UserID, Role: role}, nil
		}).AnyTimes()
	st.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	st.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	st.EXPECT().GetProject(gomock.Any(), sbxProjectV).Return(projectV, nil).AnyTimes()
	srv := &HelixAPIServer{Store: st, sandboxController: sandbox.New(st, connman.New(), nil, "", "")}
	return srv, st
}

type sandboxRoute struct {
	name    string
	method  string
	suffix  string
	handler func(*HelixAPIServer) http.HandlerFunc
	vars    map[string]string
}

// sandboxRoutes is every /organizations/{org}/sandboxes/{id} route.
func sandboxRoutes() []sandboxRoute {
	h := func(f func(*HelixAPIServer, http.ResponseWriter, *http.Request)) func(*HelixAPIServer) http.HandlerFunc {
		return func(s *HelixAPIServer) http.HandlerFunc { return func(w http.ResponseWriter, r *http.Request) { f(s, w, r) } }
	}
	cmd := map[string]string{"cmd_id": "sbcmd_1"}
	return []sandboxRoute{
		{"get", http.MethodGet, "", h((*HelixAPIServer).getSandbox), nil},
		{"update", http.MethodPatch, "", h((*HelixAPIServer).updateSandbox), nil},
		{"delete", http.MethodDelete, "", h((*HelixAPIServer).deleteSandbox), nil},
		{"exec", http.MethodPost, "/commands", h((*HelixAPIServer).runSandboxCommand), nil},
		{"list commands", http.MethodGet, "/commands", h((*HelixAPIServer).listSandboxCommands), nil},
		{"get command", http.MethodGet, "/commands/sbcmd_1", h((*HelixAPIServer).getSandboxCommand), cmd},
		{"command logs", http.MethodGet, "/commands/sbcmd_1/logs", h((*HelixAPIServer).streamSandboxCommandLogs), cmd},
		{"kill command", http.MethodPost, "/commands/sbcmd_1/kill", h((*HelixAPIServer).killSandboxCommand), cmd},
		{"read file", http.MethodGet, "/files?path=/etc/passwd", h((*HelixAPIServer).sandboxFile), nil},
		{"write file", http.MethodPut, "/files?path=/tmp/x", h((*HelixAPIServer).sandboxFile), nil},
		{"delete file", http.MethodDelete, "/files?path=/tmp/x", h((*HelixAPIServer).sandboxFile), nil},
		{"list files", http.MethodGet, "/files/list?path=/", h((*HelixAPIServer).listSandboxFiles), nil},
		{"terminal", http.MethodGet, "/terminal", h((*HelixAPIServer).sandboxTerminal), nil},
		{"terminal sessions", http.MethodGet, "/terminal/sessions", h((*HelixAPIServer).sandboxTerminalSessions), nil},
		{"screenshot", http.MethodGet, "/screenshot", h((*HelixAPIServer).sandboxScreenshot), nil},
		{"billing", http.MethodGet, "/billing", h((*HelixAPIServer).sandboxBilling), nil},
	}
}

func sandboxRouteRequest(route sandboxRoute, sandboxID string, user *types.User) *http.Request {
	req := httptest.NewRequest(route.method, "/api/v1/organizations/"+sbxOrgA+"/sandboxes/"+sandboxID+route.suffix, strings.NewReader(`{"cmd":"id","content":"x"}`))
	vars := map[string]string{"org_id": sbxOrgA, "id": sandboxID}
	for k, v := range route.vars {
		vars[k] = v
	}
	req = mux.SetURLVars(req, vars)
	return req.WithContext(setRequestUser(req.Context(), *user))
}

func TestOrgSandboxes_RefuseOtherTasksDesktopOnEveryRoute(t *testing.T) {
	callers := map[string]*types.User{
		"org member without project access": {ID: sbxMember},
		"other tenant":                      {ID: sbxStranger},
		"project key from project M": {ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectM},
		"session key of a task in project M": {ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectM, SessionID: "ses_task_m", SpecTaskID: "spt_m"},
		"task-confined key of another task in project V": {ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: "ses_task_v2", SpecTaskID: "spt_v2", TaskConfined: true},
		"task-confined key of the desktop's own task": {ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: sbxDeskSessID, SpecTaskID: "spt_v", TaskConfined: true},
	}
	for callerName, user := range callers {
		for _, route := range sandboxRoutes() {
			t.Run(callerName+"/"+route.name, func(t *testing.T) {
				srv, st := newSandboxAuthzServer(t)
				st.EXPECT().GetSandbox(gomock.Any(), sbxDeskID).Return(sbxDesktopRow(), nil).AnyTimes()
				st.EXPECT().GetSession(gomock.Any(), sbxDeskSessID).Return(sbxTaskSession(), nil).AnyTimes()

				rec := httptest.NewRecorder()
				route.handler(srv)(rec, sandboxRouteRequest(route, sbxDeskID, user))
				assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
				assert.Equal(t, "sandbox not found\n", rec.Body.String())
			})
		}
	}
}

// Callers that may reach a row get past authorization: for the container
// routes that is the hydra step, which answers 503 because the test row has
// no host (no hydra connection exists in the test).
func TestOrgSandboxes_AllowedCallersReachTheSandbox(t *testing.T) {
	plain := &types.Sandbox{ID: "sbx_plain", OrganizationID: sbxOrgA, Owner: sbxTaskOwner}
	inV := &types.Sandbox{ID: "sbx_in_v", OrganizationID: sbxOrgA, ProjectID: sbxProjectV, Owner: sbxTaskOwner}
	orphan := &types.Sandbox{ID: "sbx_orphan", OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: "ses_gone", Owner: sbxTaskOwner}
	cases := []struct {
		name string
		user *types.User
		row  *types.Sandbox
	}{
		{"task owner on their desktop", &types.User{ID: sbxTaskOwner}, sbxDesktopRow()},
		{"org owner on a task desktop", &types.User{ID: sbxOrgOwner}, sbxDesktopRow()},
		{"desktop's own session key", &types.User{ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: sbxDeskSessID, SpecTaskID: "spt_v"}, sbxDesktopRow()},
		{"org member on a project-less sandbox", &types.User{ID: sbxMember}, plain},
		{"project owner on a sandbox in the project", &types.User{ID: sbxTaskOwner}, inV},
		{"org owner on an orphaned desktop row", &types.User{ID: sbxOrgOwner}, orphan},
	}
	exec := sandboxRoutes()[3]
	require.Equal(t, "exec", exec.name)
	get := sandboxRoutes()[0]
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newSandboxAuthzServer(t)
			st.EXPECT().GetSandbox(gomock.Any(), tc.row.ID).Return(tc.row, nil).AnyTimes()
			st.EXPECT().GetSession(gomock.Any(), sbxDeskSessID).Return(sbxTaskSession(), nil).AnyTimes()
			st.EXPECT().GetSession(gomock.Any(), "ses_gone").Return(nil, store.ErrNotFound).AnyTimes()

			rec := httptest.NewRecorder()
			exec.handler(srv)(rec, sandboxRouteRequest(exec, tc.row.ID, tc.user))
			assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())

			rec = httptest.NewRecorder()
			get.handler(srv)(rec, sandboxRouteRequest(get, tc.row.ID, tc.user))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

func TestOrgSandboxes_RefusesProjectRowsAndOrphansToMembers(t *testing.T) {
	rows := []*types.Sandbox{
		{ID: "sbx_in_v", OrganizationID: sbxOrgA, ProjectID: sbxProjectV, Owner: sbxTaskOwner},
		{ID: "sbx_orphan", OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: "ses_gone", Owner: sbxTaskOwner},
	}
	exec := sandboxRoutes()[3]
	for _, row := range rows {
		t.Run(row.ID, func(t *testing.T) {
			srv, st := newSandboxAuthzServer(t)
			st.EXPECT().GetSandbox(gomock.Any(), row.ID).Return(row, nil).AnyTimes()
			st.EXPECT().GetSession(gomock.Any(), "ses_gone").Return(nil, store.ErrNotFound).AnyTimes()
			rec := httptest.NewRecorder()
			exec.handler(srv)(rec, sandboxRouteRequest(exec, row.ID, &types.User{ID: sbxMember}))
			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		})
	}
}

// A member with read-only access to a project (OrgMembersAccess) may read a
// sandbox row in it but not act inside it. (A task desktop in such a project
// follows authorizeUserToSession, which lets those members drive the session.)
func TestOrgSandboxes_ReadOnlyProjectAccessCannotExec(t *testing.T) {
	open := sbxProjectVRow()
	open.Metadata.OrgMembersAccess = true
	srv, st := newSandboxAuthzServerWithProject(t, open)
	row := &types.Sandbox{ID: "sbx_in_v", OrganizationID: sbxOrgA, ProjectID: sbxProjectV, Owner: sbxTaskOwner}
	st.EXPECT().GetSandbox(gomock.Any(), row.ID).Return(row, nil).AnyTimes()

	user := &types.User{ID: sbxMember}
	routes := sandboxRoutes()
	rec := httptest.NewRecorder()
	routes[0].handler(srv)(rec, sandboxRouteRequest(routes[0], row.ID, user))
	assert.Equal(t, http.StatusOK, rec.Code, "read-only member may read the row")
	rec = httptest.NewRecorder()
	routes[3].handler(srv)(rec, sandboxRouteRequest(routes[3], row.ID, user))
	assert.Equal(t, http.StatusNotFound, rec.Code, "read-only member may not exec")
}

func TestOrgSandboxes_ListIsFilteredInTheStoreQuery(t *testing.T) {
	cases := []struct {
		name string
		user *types.User
		want *store.SandboxVisibility
	}{
		{"org owner sees all", &types.User{ID: sbxOrgOwner}, nil},
		{"member without project access", &types.User{ID: sbxMember},
			&store.SandboxVisibility{SessionOwner: sbxMember, IncludeUnscoped: true}},
		{"project owner", &types.User{ID: sbxTaskOwner},
			&store.SandboxVisibility{ProjectIDs: []string{sbxProjectV}, SessionOwner: sbxTaskOwner, IncludeUnscoped: true}},
		{"session key of a task in project V", &types.User{ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: sbxDeskSessID},
			&store.SandboxVisibility{ProjectIDs: []string{sbxProjectV}}},
		{"org owner's key scoped to project M", &types.User{ID: sbxOrgOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectM, SessionID: "ses_m"},
			&store.SandboxVisibility{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newSandboxAuthzServer(t)
			st.EXPECT().ListProjects(gomock.Any(), gomock.Any()).Return([]*types.Project{sbxProjectVRow()}, nil).AnyTimes()
			st.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ any, q *store.ListSandboxesQuery) ([]*types.Sandbox, error) {
					assert.Equal(t, sbxOrgA, q.OrganizationID)
					assert.Equal(t, tc.want, q.Visibility)
					return nil, nil
				})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+sbxOrgA+"/sandboxes", http.NoBody)
			req = mux.SetURLVars(req, map[string]string{"org_id": sbxOrgA})
			rec := httptest.NewRecorder()
			srv.listOrgSandboxes(rec, req.WithContext(setRequestUser(req.Context(), *tc.user)))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

// Creating a sandbox in a project needs update access to that project, in
// the URL's organization; nothing is written when refused.
func TestOrgSandboxes_CreateAuthorizesTheProject(t *testing.T) {
	cases := []struct {
		name    string
		user    *types.User
		project string
		want    int
	}{
		{"member without project access", &types.User{ID: sbxMember}, sbxProjectV, http.StatusForbidden},
		{"other tenant", &types.User{ID: sbxStranger}, sbxProjectV, http.StatusForbidden},
		{"project in another org", &types.User{ID: sbxOrgOwner}, "prj_other_org", http.StatusNotFound},
		{"key scoped to project M names V", &types.User{ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectM, SessionID: "ses_m"}, sbxProjectV, http.StatusNotFound},
		{"task-confined key", &types.User{ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectV, SessionID: sbxDeskSessID, SpecTaskID: "spt_v", TaskConfined: true}, sbxProjectV, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newSandboxAuthzServer(t)
			st.EXPECT().GetProject(gomock.Any(), "prj_other_org").Return(&types.Project{ID: "prj_other_org", OrganizationID: sbxOrgB}, nil).AnyTimes()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+sbxOrgA+"/sandboxes",
				strings.NewReader(`{"name":"x","project_id":"`+tc.project+`"}`))
			req = mux.SetURLVars(req, map[string]string{"org_id": sbxOrgA})
			rec := httptest.NewRecorder()
			srv.createOrgSandbox(rec, req.WithContext(setRequestUser(req.Context(), *tc.user)))
			assert.Equal(t, tc.want, rec.Code, rec.Body.String())
		})
	}
}

// Through the real router: the task-confined guard refuses the whole org
// Sandboxes API before any handler or lookup.
func TestTaskConfinedKey_GuardRefusesOrgSandboxes(t *testing.T) {
	auth := newAuthMiddleware(nil, nil, store.NewMockStore(gomock.NewController(t)), authMiddlewareConfig{}, nil, nil)
	targets := []struct{ method, target string }{
		{http.MethodGet, "/api/v1/organizations/acme/sandboxes"},
		{http.MethodPost, "/api/v1/organizations/acme/sandboxes"},
		{http.MethodGet, "/api/v1/organizations/acme/sandboxes/sbx_1"},
		{http.MethodPost, "/api/v1/organizations/acme/sandboxes/sbx_1/commands"},
		{http.MethodPut, "/api/v1/organizations/acme/sandboxes/sbx_1/files"},
		{http.MethodGet, "/api/v1/organizations/acme/sandboxes/sbx_1/terminal"},
	}
	for _, p := range targets {
		status, err := auth.taskConfinedKeyRefusal(finderKeyUser(), httptest.NewRequest(p.method, p.target, http.NoBody))
		assert.Equal(t, http.StatusForbidden, status, p.target)
		assert.ErrorIs(t, err, errTaskConfinedKeySandboxes, p.target)
		status, err = auth.taskConfinedKeyRefusal(unconfinedFinderKeyUser(), httptest.NewRequest(p.method, p.target, http.NoBody))
		assert.Zero(t, status, p.target)
		assert.NoError(t, err, p.target)
	}
}

func TestTaskConfinedKey_RouterRefusesOrgSandboxes(t *testing.T) {
	st := store.NewMockStore(gomock.NewController(t))
	expectFinderKeyAuth(st, true)
	srv := newRouteTestServer(t, st)
	for _, target := range []string{"/api/v1/organizations/acme/sandboxes", "/api/v1/organizations/acme/sandboxes/sbx_1/commands"} {
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+finderKeySecret)
		rec := httptest.NewRecorder()
		srv.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

// A key bound to a project or a session does not reach the org's project-less
// sandboxes, which org membership alone would open to its owner.
func TestOrgSandboxes_ScopedKeysDoNotReachProjectlessSandboxes(t *testing.T) {
	plain := &types.Sandbox{ID: "sbx_plain", OrganizationID: sbxOrgA, Owner: sbxTaskOwner}
	keys := map[string]*types.User{
		"session key of a task in project M": {ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectM, SessionID: "ses_task_m"},
		"project key of project M": {ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, ProjectID: sbxProjectM},
		"another sandbox's key": {ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
			OrganizationID: sbxOrgA, SessionID: "sbx_other"},
	}
	exec := sandboxRoutes()[3]
	for name, user := range keys {
		t.Run(name, func(t *testing.T) {
			srv, st := newSandboxAuthzServer(t)
			st.EXPECT().GetSandbox(gomock.Any(), plain.ID).Return(plain, nil).AnyTimes()
			rec := httptest.NewRecorder()
			exec.handler(srv)(rec, sandboxRouteRequest(exec, plain.ID, user))
			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		})
	}
	t.Run("the sandbox's own key", func(t *testing.T) {
		srv, st := newSandboxAuthzServer(t)
		st.EXPECT().GetSandbox(gomock.Any(), plain.ID).Return(plain, nil).AnyTimes()
		own := &types.User{ID: sbxTaskOwner, TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: sbxOrgA, SessionID: plain.ID}
		rec := httptest.NewRecorder()
		exec.handler(srv)(rec, sandboxRouteRequest(exec, plain.ID, own))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	})
}
