package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Many handlers let a resource's owner through before any other check. A
// project-scoped key acts as its owner, so each of those shortcuts must also
// confine it to its project. The store mocks below expect no write: a refused
// request that still wrote would fail on the unexpected call.

// ownerProjectKey is the owner's spec-task key, scoped to project prj_a.
func ownerProjectKey() *types.User {
	return &types.User{
		ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI,
		OrganizationID: "org_a", ProjectID: "prj_a", SessionID: "ses_a", SpecTaskID: "spt_a",
	}
}

func requestAs(method, target, body string, user *types.User, vars map[string]string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	return req.WithContext(setRequestUser(req.Context(), *user))
}

func requireHTTPStatus(t *testing.T, err *system.HTTPError, status int) {
	t.Helper()
	require.NotNil(t, err, "expected HTTP %d", status)
	require.Equal(t, status, err.StatusCode, err.Message)
}

func TestDeleteSecret_ProjectKeyCannotDeletePersonalSecret(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetSecret(gomock.Any(), "sec_personal").Return(&types.Secret{ID: "sec_personal", Owner: "usr_owner", OwnerType: types.OwnerTypeUser}, nil)

	_, err := srv.deleteSecret(nil, requestAs(http.MethodDelete, "/api/v1/secrets/sec_personal", "", ownerProjectKey(), map[string]string{"id": "sec_personal"}))

	requireHTTPStatus(t, err, http.StatusForbidden)
}

func TestDeleteSecret_ProjectKeyCannotDeleteOwnSecretInAnotherProject(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetSecret(gomock.Any(), "sec_b").Return(&types.Secret{ID: "sec_b", Owner: "usr_owner", ProjectID: "prj_b"}, nil)
	st.EXPECT().GetProject(gomock.Any(), "prj_b").Return(&types.Project{ID: "prj_b", UserID: "usr_owner", OrganizationID: "org_a"}, nil)

	_, err := srv.deleteSecret(nil, requestAs(http.MethodDelete, "/api/v1/secrets/sec_b", "", ownerProjectKey(), map[string]string{"id": "sec_b"}))

	requireHTTPStatus(t, err, http.StatusForbidden)
}

func TestDeleteSecret_OwnerDeletesPersonalSecret(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetSecret(gomock.Any(), "sec_personal").Return(&types.Secret{ID: "sec_personal", Owner: "usr_owner"}, nil)
	st.EXPECT().DeleteSecret(gomock.Any(), "sec_personal").Return(nil)

	_, err := srv.deleteSecret(nil, requestAs(http.MethodDelete, "/api/v1/secrets/sec_personal", "", &types.User{ID: "usr_owner"}, map[string]string{"id": "sec_personal"}))

	require.Nil(t, err)
}

func TestUpdateSecret_ProjectKeyCannotRenamePersonalSecret(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetSecret(gomock.Any(), "sec_personal").Return(&types.Secret{ID: "sec_personal", Owner: "usr_owner"}, nil)

	_, err := srv.updateSecret(nil, requestAs(http.MethodPut, "/api/v1/secrets/sec_personal", `{"name":"renamed"}`, ownerProjectKey(), map[string]string{"id": "sec_personal"}))

	requireHTTPStatus(t, err, http.StatusNotFound)
}

func TestListSecrets_ProjectKeyCannotListPersonalSecrets(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)

	_, err := srv.listSecrets(nil, requestAs(http.MethodGet, "/api/v1/secrets", "", ownerProjectKey(), nil))

	requireHTTPStatus(t, err, http.StatusForbidden)
}

// projectAWithAgent is prj_a, whose only agent is app_in_project.
func expectProjectA(st *store.MockStore) {
	st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(&types.Project{ID: "prj_a", OrganizationID: "org_a", DefaultHelixAppID: "app_in_project"}, nil).AnyTimes()
}

func TestOwnsKnowledge_ProjectKeyConfinedToProjectAgents(t *testing.T) {
	ctx := context.Background()
	srv, st := newScopedKeyTestServer(t)
	expectProjectA(st)
	st.EXPECT().GetApp(gomock.Any(), "app_other").Return(&types.App{ID: "app_other", OrganizationID: "org_a", Owner: "usr_owner"}, nil).AnyTimes()
	st.EXPECT().GetApp(gomock.Any(), "app_in_project").Return(&types.App{ID: "app_in_project", OrganizationID: "org_a", Owner: "usr_owner"}, nil).AnyTimes()

	other := &types.Knowledge{ID: "kno_1", Owner: "usr_owner", AppID: "app_other"}
	inProject := &types.Knowledge{ID: "kno_2", Owner: "usr_owner", AppID: "app_in_project"}

	require.False(t, srv.ownsKnowledge(ctx, ownerProjectKey(), other), "project key reached another agent's knowledge")
	require.True(t, srv.ownsKnowledge(ctx, ownerProjectKey(), inProject))
	require.True(t, srv.ownsKnowledge(ctx, &types.User{ID: "usr_owner"}, other), "owner keeps their knowledge")
	require.False(t, srv.ownsKnowledge(ctx, &types.User{ID: "usr_other"}, other))
}

func TestAuthorizeUserToTrigger_ProjectKeyConfinedToProjectAgents(t *testing.T) {
	ctx := context.Background()
	srv, st := newScopedKeyTestServer(t)
	expectProjectA(st)
	st.EXPECT().GetApp(gomock.Any(), "app_other").Return(&types.App{ID: "app_other", OrganizationID: "org_a", Owner: "usr_owner"}, nil).AnyTimes()
	trigger := &types.TriggerConfiguration{ID: "trg_1", Owner: "usr_owner", AppID: "app_other"}

	requireScopeError(t, srv.authorizeUserToTrigger(ctx, ownerProjectKey(), trigger))
	require.NoError(t, srv.authorizeUserToTrigger(ctx, &types.User{ID: "usr_owner"}, trigger))
}

// The creator can still clean up a trigger whose agent is gone.
func TestAuthorizeUserToTrigger_CreatorCanRemoveOrphan(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)

	require.NoError(t, srv.authorizeUserToTrigger(context.Background(), &types.User{ID: "usr_owner"},
		&types.TriggerConfiguration{ID: "trg_1", Owner: "usr_owner", AppID: "app_gone"}))
}

func TestAuthorizePersonalProviderEndpoint(t *testing.T) {
	endpoint := &types.ProviderEndpoint{ID: "pe_1", Owner: "usr_owner", OwnerType: types.OwnerTypeUser}

	require.NoError(t, authorizePersonalProviderEndpoint(&types.User{ID: "usr_owner"}, endpoint))
	require.Error(t, authorizePersonalProviderEndpoint(ownerProjectKey(), endpoint))
	require.Error(t, authorizePersonalProviderEndpoint(&types.User{ID: "usr_owner", SessionID: "ses_chat"}, endpoint))
	require.Error(t, authorizePersonalProviderEndpoint(&types.User{ID: "usr_other"}, endpoint))
	require.NoError(t, authorizePersonalProviderEndpoint(&types.User{ID: "usr_admin", Admin: true}, endpoint))
}

func TestCreateAPIKey_AppKeyRequiresAccessToTheApp(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetApp(gomock.Any(), "app_a").Return(&types.App{ID: "app_a", OrganizationID: "org_a", Owner: "usr_owner"}, nil)
	st.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)

	stranger := &types.User{ID: "usr_stranger", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI}
	_, err := srv.createAPIKey(nil, requestAs(http.MethodPost, "/api/v1/api_keys", `{"name":"k","type":"app","app_id":"app_a"}`, stranger, nil))

	require.Error(t, err)
	httpErr, ok := err.(*system.HTTPError)
	require.True(t, ok, "expected an HTTP error, got %v", err)
	require.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestCreateOrgAPIKey_ProjectKeyCannotMintOrgKey(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_a"}, nil)
	st.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(&types.OrganizationMembership{OrganizationID: "org_a", UserID: "usr_owner", Role: types.OrganizationRoleOwner}, nil)

	rec := httptest.NewRecorder()
	srv.createOrgAPIKey(rec, requestAs(http.MethodPost, "/api/v1/organizations/org_a/api_keys", `{"name":"k"}`, ownerProjectKey(), map[string]string{"id": "org_a"}))

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

func TestPinProject_RequiresAccessToTheProject(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(&types.Project{ID: "prj_a", OrganizationID: "org_a", UserID: "usr_owner"}, nil)
	st.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)

	_, err := srv.pinProject(nil, requestAs(http.MethodPost, "/api/v1/projects/prj_a/pin", "", &types.User{ID: "usr_stranger"}, map[string]string{"id": "prj_a"}))

	requireHTTPStatus(t, err, http.StatusNotFound)
}

func TestUnifiedSearch_RefusesScopedKeys(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)
	orgKey := &types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: "org_a"}

	for _, user := range []*types.User{ownerProjectKey(), orgKey} {
		_, err := srv.unifiedSearch(nil, requestAs(http.MethodGet, "/api/v1/search?q=x", "", user, nil))
		requireHTTPStatus(t, err, http.StatusForbidden)
	}
}

func TestCreateOrgAPIKey_OrgKeyCannotMintOrgKey(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_a"}, nil)
	st.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(&types.OrganizationMembership{OrganizationID: "org_a", UserID: "usr_owner", Role: types.OrganizationRoleOwner}, nil)
	orgKey := &types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: "org_a"}

	rec := httptest.NewRecorder()
	srv.createOrgAPIKey(rec, requestAs(http.MethodPost, "/api/v1/organizations/org_a/api_keys", `{"name":"k"}`, orgKey, map[string]string{"id": "org_a"}))

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

func TestCreateAPIKey_AgentScopedKeyCannotMintAppKey(t *testing.T) {
	// No store expectations: refused before the app is even loaded.
	srv, _ := newScopedKeyTestServer(t)
	for _, user := range []*types.User{ownerProjectKey(), {ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, SessionID: "ses_chat"}} {
		_, err := srv.createAPIKey(nil, requestAs(http.MethodPost, "/api/v1/api_keys", `{"name":"k","type":"app","app_id":"app_in_project"}`, user, nil))
		var httpErr *system.HTTPError
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusForbidden, httpErr.StatusCode)
	}
}

// A scoped key is capped at member in the org graph, even when its owner is
// an owner (or the key somehow carried the admin flag): CanManageOrganization
// and tool grants downstream must agree with the REST routes.
func TestHelixOrgAuthority_AgentScopedKeyIsMember(t *testing.T) {
	role, admin := helixOrgAuthority(&types.User{ID: "u", ProjectID: "prj_a", SessionID: "ses_a", Admin: true}, types.OrganizationRoleOwner)
	require.Equal(t, types.OrganizationRoleMember, role)
	require.False(t, admin)

	role, admin = helixOrgAuthority(&types.User{ID: "u", SessionID: "ses_worker"}, types.OrganizationRoleOwner)
	require.Equal(t, types.OrganizationRoleMember, role)
	require.False(t, admin)

	role, admin = helixOrgAuthority(&types.User{ID: "u"}, types.OrganizationRoleOwner)
	require.Equal(t, types.OrganizationRoleOwner, role)
	require.False(t, admin)

	role, admin = helixOrgAuthority(&types.User{ID: "u", Admin: true}, types.OrganizationRoleMember)
	require.Equal(t, types.OrganizationRoleMember, role)
	require.True(t, admin)
}

func TestAuthorizePersonalProviderEndpoint_ScopeBeforeAdmin(t *testing.T) {
	endpoint := &types.ProviderEndpoint{ID: "pe_1", Owner: "usr_owner", OwnerType: types.OwnerTypeUser}
	require.Error(t, authorizePersonalProviderEndpoint(&types.User{ID: "usr_admin", Admin: true, ProjectID: "prj_a"}, endpoint))
}
