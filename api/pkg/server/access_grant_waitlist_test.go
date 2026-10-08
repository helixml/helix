package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	grantOrgID   = "org_grant"
	grantOwnerID = "user_owner"
)

func grantRequest(path, id, userReference string) *http.Request {
	body, _ := json.Marshal(types.CreateAccessGrantRequest{UserReference: userReference, Roles: []string{"write"}})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id})
	return req.WithContext(setRequestUser(req.Context(), types.User{ID: grantOwnerID, Email: "owner@helix.ml"}))
}

// expectGrantCommon wires the store calls shared by the access-grant handlers:
// the acting user is an org owner, the role exists and the grant is created.
func expectGrantCommon(t *testing.T, mockStore *store.MockStore, target *types.User, memberships []*types.OrganizationMembership) {
	expectOrgOwner(mockStore, grantOrgID, grantOwnerID).AnyTimes()
	mockStore.EXPECT().ListRoles(gomock.Any(), grantOrgID).Return([]*types.Role{{ID: "role_write", Name: "write"}}, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: target.Email}).Return(target, nil)
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{
		OrganizationID: grantOrgID, UserID: target.ID,
	}).Return(memberships, nil)
	mockStore.EXPECT().CreateAccessGrant(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, g *types.AccessGrant, _ []*types.Role) (*types.AccessGrant, error) {
			require.Equal(t, target.ID, g.UserID)
			return g, nil
		})
}

func expectApproval(t *testing.T, mockStore *store.MockStore, notifier *notification.MockNotifier, target *types.User) {
	gomock.InOrder(
		mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
				require.Equal(t, grantOrgID, m.OrganizationID)
				require.Equal(t, target.ID, m.UserID)
				return m, nil
			}),
		mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
				require.Equal(t, target.ID, u.ID)
				require.False(t, u.Waitlisted)
				require.True(t, u.OnboardingCompleted)
				return u, nil
			}),
		notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, n *types.Notification) error {
				require.Equal(t, types.EventWaitlistApproved, n.Event)
				require.Equal(t, target.Email, n.Email)
				return nil
			}),
	)
}

// Granting project access to a waitlisted non-member makes the org owner add
// them to the org — the same vouch as "Add member", so they are approved.
func TestCreateProjectAccessGrant_WaitlistedNonMember_AddedAndApproved(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	server := newTestServerWithNotifier(mockStore, notifier)

	target := &types.User{ID: "fake-abi-gmail-com", Email: "abi@gmail.com", Waitlisted: true}
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", OrganizationID: grantOrgID, UserID: grantOwnerID}, nil)
	expectGrantCommon(t, mockStore, target, nil)
	expectApproval(t, mockStore, notifier, target)

	rr := httptest.NewRecorder()
	server.createProjectAccessGrant(rr, grantRequest("/api/v1/projects/prj_1/access-grants", "prj_1", target.Email))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp types.CreateAccessGrantResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.AddedToOrganization)
}

// A non-waitlisted user auto-added to the org is untouched (today's behaviour).
func TestCreateProjectAccessGrant_ApprovedNonMember_Unchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	server := newTestServerWithNotifier(mockStore, notifier)

	target := &types.User{ID: "u1", Email: "dev@helix.ml"}
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", OrganizationID: grantOrgID, UserID: grantOwnerID}, nil)
	expectGrantCommon(t, mockStore, target, nil)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
			return m, nil
		})
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	server.createProjectAccessGrant(rr, grantRequest("/api/v1/projects/prj_1/access-grants", "prj_1", target.Email))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// A project grant for a user who is already an org member is not an org add,
// so it does not approve them (project admins who aren't org owners can grant).
func TestCreateProjectAccessGrant_WaitlistedExistingMember_NotApproved(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	server := newTestServerWithNotifier(mockStore, notifier)

	target := &types.User{ID: "u1", Email: "abi@gmail.com", Waitlisted: true}
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", OrganizationID: grantOrgID, UserID: grantOwnerID}, nil)
	expectGrantCommon(t, mockStore, target, []*types.OrganizationMembership{{OrganizationID: grantOrgID, UserID: target.ID}})
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).Times(0)
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).Times(0)

	rr := httptest.NewRecorder()
	server.createProjectAccessGrant(rr, grantRequest("/api/v1/projects/prj_1/access-grants", "prj_1", target.Email))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

func TestCreateRepositoryAccessGrant_WaitlistedNonMember_AddedAndApproved(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	server := newTestServerWithNotifier(mockStore, notifier)

	target := &types.User{ID: "fake-abi-gmail-com", Email: "abi@gmail.com", Waitlisted: true}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_1").Return(&types.GitRepository{ID: "repo_1", OrganizationID: grantOrgID, OwnerID: grantOwnerID}, nil)
	expectGrantCommon(t, mockStore, target, nil)
	expectApproval(t, mockStore, notifier, target)

	rr := httptest.NewRecorder()
	server.createRepositoryAccessGrant(rr, grantRequest("/api/v1/git/repositories/repo_1/access-grants", "repo_1", target.Email))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}
