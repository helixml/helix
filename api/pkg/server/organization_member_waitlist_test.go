package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestServerWithNotifier(mockStore *store.MockStore, notifier notification.Notifier) *HelixAPIServer {
	return &HelixAPIServer{
		Store:      mockStore,
		Cfg:        &config.ServerConfig{Notifications: config.Notifications{AppURL: "http://test"}},
		Controller: &controller.Controller{Options: controller.Options{Notifier: notifier}},
	}
}

func addMemberRequest(orgID, requesterID, userReference string) *http.Request {
	body, _ := json.Marshal(types.AddOrganizationMemberRequest{UserReference: userReference})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	return req.WithContext(setRequestUser(req.Context(), types.User{ID: requesterID, Email: requesterID + "@helix.ml"}))
}

// An org owner adding an existing waitlisted user (e.g. a @gmail.com sign-in
// waitlisted by OIDC_WAITLIST_OUTSIDE_ALLOWED_DOMAINS) vouches for them: the
// membership is created, then they are un-waitlisted, onboarding is skipped and
// the waitlist-approved email is sent — no admin approve call needed.
func TestAddOrganizationMember_ExistingWaitlistedUser_JoinsAndIsApproved(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	server := newTestServerWithNotifier(mockStore, notifier)

	orgID, ownerID := "org_vouch", "user_owner"
	waitlisted := &types.User{ID: "fake-pal-gmail-com", Email: "pal@gmail.com", FullName: "Pal Person", Waitlisted: true}

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "pal@gmail.com"}).Return(waitlisted, nil)
	gomock.InOrder(
		mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
				require.Equal(t, orgID, m.OrganizationID)
				require.Equal(t, waitlisted.ID, m.UserID)
				return m, nil
			}),
		mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
				require.Equal(t, waitlisted.ID, u.ID)
				require.False(t, u.Waitlisted, "invited user must be taken off the waitlist")
				require.True(t, u.OnboardingCompleted, "invited user joins an org, so skip the onboarding wizard")
				require.False(t, u.OnboardingCompletedAt.IsZero())
				return u, nil
			}),
		notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, n *types.Notification) error {
				require.Equal(t, types.EventWaitlistApproved, n.Event)
				require.Equal(t, "pal@gmail.com", n.Email)
				require.Equal(t, "Pal", n.FirstName)
				return nil
			}),
	)

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, addMemberRequest(orgID, ownerID, "pal@gmail.com"))

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var resp types.AddOrganizationMemberResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotNil(t, resp.Membership)
	require.False(t, resp.Invited)
}

// A waitlisted user who had already finished onboarding keeps their original
// completion timestamp.
func TestAddOrganizationMember_ExistingWaitlistedUser_KeepsOnboardingTimestamp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID, ownerID := "org_vouch", "user_owner"
	onboardedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	waitlisted := &types.User{ID: "u1", Email: "pal@gmail.com", Waitlisted: true, OnboardingCompleted: true, OnboardingCompletedAt: onboardedAt}

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(waitlisted, nil)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
			return m, nil
		})
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
			require.False(t, u.Waitlisted)
			require.Equal(t, onboardedAt, u.OnboardingCompletedAt)
			return u, nil
		})

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, addMemberRequest(orgID, ownerID, "pal@gmail.com"))
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

// Adding an existing, non-waitlisted user behaves exactly as before: the
// membership is created and the user record is not touched, no email is sent.
func TestAddOrganizationMember_ExistingApprovedUser_Unchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl) // strict: any Notify fails the test
	server := newTestServerWithNotifier(mockStore, notifier)

	orgID, ownerID := "org_vouch", "user_owner"
	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
		Return(&types.User{ID: "u1", Email: "dev@helix.ml", Waitlisted: false}, nil)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
			return m, nil
		})
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, addMemberRequest(orgID, ownerID, "dev@helix.ml"))
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

// Only an org owner can vouch: a plain member trying to add a waitlisted user
// is rejected before the user is even looked up, so nothing is approved.
func TestAddOrganizationMember_NonOwnerCannotApproveWaitlistedUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	server := newTestServerWithNotifier(mockStore, notifier)

	orgID, memberID := "org_vouch", "user_member"
	expectResolveOrganizationByID(mockStore, orgID)
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID, UserID: memberID,
	}).Return(&types.OrganizationMembership{OrganizationID: orgID, UserID: memberID, Role: types.OrganizationRoleMember}, nil)
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).Times(0)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, addMemberRequest(orgID, memberID, "pal@gmail.com"))
	require.Equal(t, http.StatusForbidden, rr.Code)
}

// If the membership can't be created, the user must not be approved.
func TestAddOrganizationMember_MembershipFails_DoesNotApprove(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	server := newTestServerWithNotifier(mockStore, notifier)

	orgID, ownerID := "org_vouch", "user_owner"
	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
		Return(&types.User{ID: "u1", Email: "pal@gmail.com", Waitlisted: true}, nil)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, addMemberRequest(orgID, ownerID, "pal@gmail.com"))
	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// A waitlisted requester never reaches a handler (so cannot add themselves to
// an org they own and self-approve): every authenticated route runs
// rejectWaitlisted first.
func TestRejectWaitlisted(t *testing.T) {
	rr := httptest.NewRecorder()
	require.True(t, rejectWaitlisted(rr, &types.User{ID: "u1", Waitlisted: true}))
	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Contains(t, rr.Body.String(), "waiting for approval")

	rr = httptest.NewRecorder()
	require.False(t, rejectWaitlisted(rr, &types.User{ID: "u1"}))
	require.False(t, rejectWaitlisted(rr, nil))
	require.Equal(t, http.StatusOK, rr.Code)
}
