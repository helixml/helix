package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	helixstripe "github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/webservice"
	"github.com/stretchr/testify/require"
	stripeapi "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/form"
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

	userSandbox, err := st.CreateSandbox(ctx, &types.Sandbox{
		ID: "sbx_del_" + suffix, OrganizationID: sharedOrg, Owner: userID, Runtime: types.SandboxRuntimeHeadlessUbuntu,
	})
	require.NoError(t, err)
	// The shared project's web service, last deployed by the user: it is the
	// org's and must keep serving.
	webServiceSandbox, err := st.CreateSandbox(ctx, &types.Sandbox{
		ID: "sbx_web_" + suffix, OrganizationID: sharedOrg, Owner: userID, Runtime: types.SandboxRuntimeHeadlessUbuntu,
		ProjectID: sharedProject.ID, Purpose: types.SandboxPurposeWebService, Status: types.SandboxStatusRunning,
	})
	require.NoError(t, err)
	_, err = st.CreateWallet(ctx, &types.Wallet{UserID: userID})
	require.NoError(t, err)

	// A desktop the user ran in the shared org: its meter row is still
	// "running" and its sandbox host is gone.
	desktopSandbox, err := st.CreateSandbox(ctx, &types.Sandbox{
		ID: "sbx_desk_" + suffix, OrganizationID: sharedOrg, Owner: userID, Runtime: types.SandboxRuntimeUbuntuDesktop,
		SessionID: "ses_desk_" + suffix, Status: types.SandboxStatusRunning,
	})
	require.NoError(t, err)

	// Chat sessions: the personal one goes with the user, the one in the
	// shared org is the org's and stays.
	personalSession, err := st.CreateSession(ctx, types.Session{
		ID: "ses_p_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser, Created: time.Now(), Updated: time.Now(),
	})
	require.NoError(t, err)
	orgSession, err := st.CreateSession(ctx, types.Session{
		ID: "ses_o_" + suffix, Owner: userID, OwnerType: types.OwnerTypeUser, OrganizationID: sharedOrg,
		Created: time.Now(), Updated: time.Now(),
	})
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
	sandboxController := sandbox.New(st, nil, nil, "", "")
	sandboxController.SetDesktopStopper(func(context.Context, string) error {
		return fmt.Errorf("dial hydra: %w", connman.ErrNoConnection)
	})
	server := &HelixAPIServer{
		Store:                st,
		gitRepositoryService: services.NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@example.com"),
		webServiceController: webservice.New(st, sandbox.New(st, nil, nil, "", "")),
		sandboxController:    sandboxController,
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
	for _, id := range []string{userSandbox.ID, desktopSandbox.ID} {
		_, err = st.GetSandbox(ctx, id)
		require.ErrorIs(t, err, store.ErrNotFound, "sandbox %s must be deleted", id)
	}
	_, err = st.GetSession(ctx, personalSession.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "the personal session must be deleted")
	kept, err := st.GetSession(ctx, orgSession.ID)
	require.NoError(t, err, "the shared org's session must be kept")
	require.Equal(t, sharedOrg, kept.OrganizationID)
	_, err = os.Stat(filestoreDir + "/users/" + userID)
	require.True(t, os.IsNotExist(err), "user files must be removed")

	// The shared organization and its project stay; the user is no longer a member.
	_, err = st.GetOrganization(ctx, &store.GetOrganizationQuery{ID: sharedOrg})
	require.NoError(t, err)
	_, err = st.GetProject(ctx, sharedProject.ID)
	require.NoError(t, err)
	_, err = st.GetSandbox(ctx, webServiceSandbox.ID)
	require.NoError(t, err, "the shared project's web service must be kept")
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
		mockStore.EXPECT().GetWalletByUser(gomock.Any(), "usr_1").Return(&types.Wallet{ID: "wal_1"}, nil),
		mockStore.EXPECT().RevokeUserAPIKeys(gomock.Any(), "usr_1").Return(nil),
		mockStore.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return(nil, int64(0), context.DeadlineExceeded),
	)

	_, err := server.adminDeleteUser(httptest.NewRecorder(), adminDeleteUserRequest("usr_1"))
	httpErr, ok := err.(*system.HTTPError)
	require.True(t, ok, "%v", err)
	require.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
}

// cancelRecordingStripeBackend records subscription cancellations and fails
// the one named by failID.
type cancelRecordingStripeBackend struct {
	canceled []string
	failID   string
	goneID   string // answers as Stripe does for a subscription it no longer has
}

func (b *cancelRecordingStripeBackend) Call(method, path, _ string, _ stripeapi.ParamsContainer, _ stripeapi.LastResponseSetter) error {
	id, ok := strings.CutPrefix(path, "/v1/subscriptions/")
	if method != http.MethodDelete || !ok {
		return fmt.Errorf("unexpected call: %s %s", method, path)
	}
	if id == b.failID {
		return fmt.Errorf("stripe unavailable")
	}
	if id == b.goneID {
		return &stripeapi.Error{HTTPStatusCode: http.StatusNotFound, Code: stripeapi.ErrorCodeResourceMissing}
	}
	b.canceled = append(b.canceled, id)
	return nil
}
func (*cancelRecordingStripeBackend) CallStreaming(string, string, string, stripeapi.ParamsContainer, stripeapi.StreamingLastResponseSetter) error {
	return fmt.Errorf("unexpected streaming call")
}
func (*cancelRecordingStripeBackend) CallRaw(string, string, string, *form.Values, *stripeapi.Params, stripeapi.LastResponseSetter) error {
	return fmt.Errorf("unexpected raw call")
}
func (*cancelRecordingStripeBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripeapi.Params, stripeapi.LastResponseSetter) error {
	return fmt.Errorf("unexpected multipart call")
}
func (*cancelRecordingStripeBackend) SetMaxNetworkRetries(int64) {}

// The user's live subscriptions, and those of organizations deleted with
// them, are cancelled before anything else changes; a failed cancellation
// aborts the delete with the account untouched.
func TestAdminDeleteUser_CancelsSubscriptionsFirst(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failID   string
		goneID   string
		canceled []string
	}{
		{"all cancelled, delete proceeds", "", "", []string{"sub_user", "sub_org"}},
		{"org cancellation fails, delete aborts", "sub_org", "", []string{"sub_user"}},
		{"org subscription already gone from Stripe, delete proceeds", "", "sub_org", []string{"sub_user"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &cancelRecordingStripeBackend{failID: tc.failID, goneID: tc.goneID}
			original := stripeapi.GetBackend(stripeapi.APIBackend)
			stripeapi.SetBackend(stripeapi.APIBackend, backend)
			t.Cleanup(func() { stripeapi.SetBackend(stripeapi.APIBackend, original) })

			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			stripeCfg := config.Stripe{SecretKey: "sk_test", WebhookSigningSecret: "whsec_test"}
			server := &HelixAPIServer{Store: mockStore, Stripe: helixstripe.NewStripe(stripeCfg, mockStore)}

			mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_1"}).Return(&types.User{ID: "usr_1"}, nil)
			mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{UserID: "usr_1"}).
				Return([]*types.OrganizationMembership{{OrganizationID: "org_solo", UserID: "usr_1", Role: types.OrganizationRoleOwner}}, nil)
			mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{OrganizationID: "org_solo"}).
				Return([]*types.OrganizationMembership{{OrganizationID: "org_solo", UserID: "usr_1", Role: types.OrganizationRoleOwner}}, nil)
			mockStore.EXPECT().GetWalletByUser(gomock.Any(), "usr_1").Return(&types.Wallet{
				ID: "wal_user", StripeSubscriptionID: "sub_user", SubscriptionStatus: stripeapi.SubscriptionStatusPastDue,
			}, nil)
			mockStore.EXPECT().GetWalletByOrg(gomock.Any(), "org_solo").Return(&types.Wallet{
				ID: "wal_org", StripeSubscriptionID: "sub_org", SubscriptionStatus: stripeapi.SubscriptionStatusTrialing,
			}, nil)
			if tc.failID == "" {
				// Stop right after the cancellations; the rest is covered elsewhere.
				mockStore.EXPECT().RevokeUserAPIKeys(gomock.Any(), "usr_1").Return(context.DeadlineExceeded)
			}

			_, err := server.adminDeleteUser(httptest.NewRecorder(), adminDeleteUserRequest("usr_1"))
			require.Error(t, err)
			require.Equal(t, tc.canceled, backend.canceled)
		})
	}
}

// A canceled or never-started subscription is not touched; one the webhook
// has not synced yet is.
func TestWalletHasLiveSubscription(t *testing.T) {
	require.False(t, walletHasLiveSubscription(&types.Wallet{}))
	require.True(t, walletHasLiveSubscription(&types.Wallet{StripeSubscriptionID: "sub"}), "not yet synced by the webhook: can still bill")
	require.False(t, walletHasLiveSubscription(&types.Wallet{StripeSubscriptionID: "sub", SubscriptionStatus: stripeapi.SubscriptionStatusCanceled}))
	require.False(t, walletHasLiveSubscription(&types.Wallet{StripeSubscriptionID: "sub", SubscriptionStatus: stripeapi.SubscriptionStatusIncompleteExpired}))
	require.True(t, walletHasLiveSubscription(&types.Wallet{StripeSubscriptionID: "sub", SubscriptionStatus: stripeapi.SubscriptionStatusActive}))
	require.True(t, walletHasLiveSubscription(&types.Wallet{StripeSubscriptionID: "sub", SubscriptionStatus: stripeapi.SubscriptionStatusUnpaid}))
}
