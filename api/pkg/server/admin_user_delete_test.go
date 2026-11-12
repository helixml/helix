package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/webservice"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func adminDeleteUserRequest(targetID string) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+targetID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": targetID})
	return req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "admin-1", Admin: true}))
}

// The only owner of an organization other people still use cannot be deleted:
// the org would be left without an owner. Nothing may change, not even the
// user's API keys.
func TestAdminDeleteUser_RefusesSoleOwnerOfSharedOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_owner"}).Return(&types.User{ID: "usr_owner"}, nil)
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{UserID: "usr_owner"}).
		Return([]*types.OrganizationMembership{{OrganizationID: "org_shared", UserID: "usr_owner", Role: types.OrganizationRoleOwner}}, nil)
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{OrganizationID: "org_shared"}).
		Return([]*types.OrganizationMembership{
			{OrganizationID: "org_shared", UserID: "usr_owner", Role: types.OrganizationRoleOwner},
			{OrganizationID: "org_shared", UserID: "usr_member", Role: types.OrganizationRoleMember},
		}, nil)
	// No RevokeUserAPIKeys / DeleteUser expected: gomock fails on any write.

	_, err := server.adminDeleteUser(httptest.NewRecorder(), adminDeleteUserRequest("usr_owner"))
	httpErr, ok := err.(*system.HTTPError)
	require.True(t, ok, "%v", err)
	require.Equal(t, http.StatusConflict, httpErr.StatusCode)
	require.Contains(t, httpErr.Message, "org_shared")
}

// End to end against Postgres: the deleted user's API keys stop
// authenticating as soon as the request returns, their personal project and
// the organization they were the only member of are gone, and an organization
// they shared keeps its data but no longer lists them.
func TestAdminDeleteUser_RevokesAccessAndRemovesPersonalDataIntegration(t *testing.T) {
	if os.Getenv("POSTGRES_HOST") == "" {
		t.Skip("POSTGRES_HOST not set; skipping Postgres-backed test")
	}
	ctx := context.Background()
	st := store.GetTestDB()
	suffix := system.GenerateID()
	userID, otherID := "usr_del_"+suffix, "usr_keep_"+suffix
	soloOrg, sharedOrg := "org_solo_"+suffix, "org_shared_"+suffix

	for _, id := range []string{userID, otherID} {
		_, err := st.CreateUser(ctx, &types.User{ID: id, Email: id + "@example.com"})
		require.NoError(t, err)
	}
	for _, org := range []*types.Organization{
		{ID: soloOrg, Name: "solo-" + suffix, Owner: userID},
		{ID: sharedOrg, Name: "shared-" + suffix, Owner: otherID},
	} {
		_, err := st.CreateOrganization(ctx, org)
		require.NoError(t, err)
	}
	for _, m := range []*types.OrganizationMembership{
		{OrganizationID: soloOrg, UserID: userID, Role: types.OrganizationRoleOwner},
		{OrganizationID: sharedOrg, UserID: userID, Role: types.OrganizationRoleMember},
		{OrganizationID: sharedOrg, UserID: otherID, Role: types.OrganizationRoleOwner},
	} {
		_, err := st.CreateOrganizationMembership(ctx, m)
		require.NoError(t, err)
	}
	personalProject, err := st.CreateProject(ctx, &types.Project{ID: "prj_p_" + suffix, Name: "p-" + suffix, UserID: userID})
	require.NoError(t, err)
	sharedProject, err := st.CreateProject(ctx, &types.Project{ID: "prj_s_" + suffix, Name: "s-" + suffix, UserID: userID, OrganizationID: sharedOrg})
	require.NoError(t, err)

	personalKey := "hl-del-" + suffix
	orgKey := "hl-del-org-" + suffix
	for _, key := range []*types.ApiKey{
		{Key: personalKey, Name: "personal", Owner: userID, OwnerType: types.OwnerTypeUser},
		{Key: orgKey, Name: "org", Owner: userID, OwnerType: types.OwnerTypeUser, OrganizationID: sharedOrg},
	} {
		_, err := st.CreateAPIKey(ctx, key)
		require.NoError(t, err)
	}

	auth := newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil)
	for _, key := range []string{personalKey, orgKey} {
		user, err := auth.getUserFromToken(ctx, key)
		require.NoError(t, err)
		require.Equal(t, userID, user.ID)
	}

	filestoreDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filestoreDir+"/users/"+userID, 0o755))
	require.NoError(t, os.WriteFile(filestoreDir+"/users/"+userID+"/notes.txt", []byte("x"), 0o600))
	server := &HelixAPIServer{
		Store:                st,
		gitRepositoryService: services.NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@example.com"),
		webServiceController: webservice.New(st, sandbox.New(st, nil, nil, "", "")),
		Controller: &controller.Controller{Options: controller.Options{
			Config:    &config.ServerConfig{},
			Filestore: filestore.NewFileSystemStorage(filestoreDir, "http://localhost:8080/files", "secret"),
		}},
	}

	result, err := server.adminDeleteUser(httptest.NewRecorder(), adminDeleteUserRequest(userID))
	require.NoError(t, err)
	require.Equal(t, userID, result["user_id"])

	for _, key := range []string{personalKey, orgKey} {
		user, err := auth.getUserFromToken(ctx, key)
		require.Error(t, err, "a deleted user's key must not authenticate")
		require.Nil(t, user)
	}

	_, err = st.GetUser(ctx, &store.GetUserQuery{ID: userID})
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = st.GetProject(ctx, personalProject.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = st.GetOrganization(ctx, &store.GetOrganizationQuery{ID: soloOrg})
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = os.Stat(filestoreDir + "/users/" + userID)
	require.True(t, os.IsNotExist(err), "user files must be removed")

	// The shared organization and its project stay; the user is no longer a member.
	_, err = st.GetOrganization(ctx, &store.GetOrganizationQuery{ID: sharedOrg})
	require.NoError(t, err)
	_, err = st.GetProject(ctx, sharedProject.ID)
	require.NoError(t, err)
	members, err := st.ListOrganizationMemberships(ctx, &store.ListOrganizationMembershipsQuery{OrganizationID: sharedOrg})
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, otherID, members[0].UserID)
}

// Keys are revoked before any teardown, so a teardown failure part-way
// through still leaves the account unable to authenticate.
func TestAdminDeleteUser_RevokesKeysBeforeTeardown(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_1"}).Return(&types.User{ID: "usr_1"}, nil)
	gomock.InOrder(
		mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{UserID: "usr_1"}).Return(nil, nil),
		mockStore.EXPECT().RevokeUserAPIKeys(gomock.Any(), "usr_1").Return(nil),
		mockStore.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return(nil, int64(0), context.DeadlineExceeded),
	)

	_, err := server.adminDeleteUser(httptest.NewRecorder(), adminDeleteUserRequest("usr_1"))
	httpErr, ok := err.(*system.HTTPError)
	require.True(t, ok, "%v", err)
	require.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
}
