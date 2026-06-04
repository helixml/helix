package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// cloudBillingCfg returns the minimal config that lets the credit-grant
// handler past its edition/billing gates.
func cloudBillingCfg() *config.ServerConfig {
	cfg := &config.ServerConfig{}
	cfg.Edition = "cloud"
	cfg.Stripe.BillingEnabled = true
	return cfg
}

func TestAdminGrantCredits_Gates(t *testing.T) {
	tests := []struct {
		name           string
		adminUser      *types.User
		cfg            *config.ServerConfig
		body           interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "non-admin is rejected",
			adminUser:      &types.User{ID: "u-1", Admin: false},
			cfg:            cloudBillingCfg(),
			body:           GrantCreditsRequest{Credits: 50},
			expectedStatus: http.StatusForbidden,
			expectedError:  "only admins",
		},
		{
			name: "non-cloud edition is rejected",
			adminUser: &types.User{ID: "admin-1", Admin: true},
			cfg: func() *config.ServerConfig {
				c := cloudBillingCfg()
				c.Edition = "server"
				return c
			}(),
			body:           GrantCreditsRequest{Credits: 50},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "cloud edition",
		},
		{
			name: "billing disabled is rejected",
			adminUser: &types.User{ID: "admin-1", Admin: true},
			cfg: func() *config.ServerConfig {
				c := cloudBillingCfg()
				c.Stripe.BillingEnabled = false
				return c
			}(),
			body:           GrantCreditsRequest{Credits: 50},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Stripe billing must be enabled",
		},
		{
			name:           "zero credits is rejected",
			adminUser:      &types.User{ID: "admin-1", Admin: true},
			cfg:            cloudBillingCfg(),
			body:           GrantCreditsRequest{Credits: 0},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "greater than 0",
		},
		{
			name:           "negative credits is rejected",
			adminUser:      &types.User{ID: "admin-1", Admin: true},
			cfg:            cloudBillingCfg(),
			body:           GrantCreditsRequest{Credits: -10},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStore := store.NewMockStore(ctrl)

			server := &HelixAPIServer{Store: mockStore, Cfg: tt.cfg}

			body, err := json.Marshal(tt.body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/u-2/credits", bytes.NewReader(body))
			req = mux.SetURLVars(req, map[string]string{"id": "u-2"})
			req = req.WithContext(setTestRequestUser(req.Context(), tt.adminUser))

			rr := httptest.NewRecorder()
			_, httpErr := server.adminGrantCredits(rr, req)

			require.NotNil(t, httpErr)
			assert.Contains(t, httpErr.Error(), tt.expectedError)
			he, ok := httpErr.(*system.HTTPError)
			require.True(t, ok, "expected *system.HTTPError, got %T", httpErr)
			assert.Equal(t, tt.expectedStatus, he.StatusCode)
		})
	}
}

func TestAdminGrantCredits_NoOrg_StashesOnUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	targetUser := &types.User{ID: "target-1", Email: "t@example.com"}

	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: "target-1"}).
		Return(targetUser, nil)
	// No owned orgs.
	mockStore.EXPECT().
		ListOrganizations(gomock.Any(), gomock.Any()).
		Return([]*types.Organization{}, nil)
	// Stash on user.
	mockStore.EXPECT().
		UpdateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, u *types.User) (*types.User, error) {
			require.NotNil(t, u.PendingAdminCreditsOnFirstOrg)
			assert.InDelta(t, 75.0, *u.PendingAdminCreditsOnFirstOrg, 0.0001)
			return u, nil
		})

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 75})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target-1/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	resp, httpErr := server.adminGrantCredits(rr, req)

	require.Nil(t, httpErr)
	require.NotNil(t, resp)
	assert.Equal(t, "stashed", resp.Status)
	assert.Empty(t, resp.OrgID)
}

func TestAdminGrantCredits_HasOrgWithWallet_TopsUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	targetUser := &types.User{ID: "target-1", Email: "t@example.com"}
	org := &types.Organization{ID: "org-1", Owner: "target-1"}
	wallet := &types.Wallet{ID: "wal-1", OrgID: "org-1", Balance: 0}

	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: "target-1"}).
		Return(targetUser, nil)
	mockStore.EXPECT().
		ListOrganizations(gomock.Any(), gomock.Any()).
		Return([]*types.Organization{org}, nil)
	mockStore.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org-1"}).
		Return(org, nil)
	mockStore.EXPECT().
		GetWalletByOrg(gomock.Any(), "org-1").
		Return(wallet, nil)
	mockStore.EXPECT().
		UpdateWalletBalance(gomock.Any(), "wal-1", 50.0, gomock.Any()).
		DoAndReturn(func(_ interface{}, _ string, amount float64, meta types.TransactionMetadata) (*types.Wallet, error) {
			assert.Equal(t, types.TransactionTypeAdminGrant, meta.TransactionType)
			return wallet, nil
		})

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 50, OrgID: "org-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target-1/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	resp, httpErr := server.adminGrantCredits(rr, req)

	require.Nil(t, httpErr)
	require.NotNil(t, resp)
	assert.Equal(t, "applied", resp.Status)
	assert.Equal(t, "org-1", resp.OrgID)
}

// User owns 3 orgs; admin picks the middle one. Verifies that the chosen
// wallet receives the credit and the others are untouched.
func TestAdminGrantCredits_MultipleOrgs_AppliesToSelected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	targetUser := &types.User{ID: "target-1", Email: "t@example.com"}
	orgA := &types.Organization{ID: "org-a", Owner: "target-1"}
	orgB := &types.Organization{ID: "org-b", Owner: "target-1"}
	orgC := &types.Organization{ID: "org-c", Owner: "target-1"}
	walletB := &types.Wallet{ID: "wal-b", OrgID: "org-b"}

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target-1"}).Return(targetUser, nil)
	mockStore.EXPECT().ListOrganizations(gomock.Any(), gomock.Any()).Return([]*types.Organization{orgA, orgB, orgC}, nil)
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org-b"}).Return(orgB, nil)
	mockStore.EXPECT().GetWalletByOrg(gomock.Any(), "org-b").Return(walletB, nil)
	mockStore.EXPECT().UpdateWalletBalance(gomock.Any(), "wal-b", 30.0, gomock.Any()).Return(walletB, nil)

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 30, OrgID: "org-b"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target-1/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	resp, httpErr := server.adminGrantCredits(rr, req)

	require.Nil(t, httpErr)
	assert.Equal(t, "applied", resp.Status)
	assert.Equal(t, "org-b", resp.OrgID)
}

// User owns ≥1 orgs but admin didn't pass org_id -- handler rejects rather
// than silently picking. This is the core determinism guarantee.
func TestAdminGrantCredits_HasOrgs_MissingOrgID_Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	targetUser := &types.User{ID: "target-1"}
	orgs := []*types.Organization{
		{ID: "org-a", Owner: "target-1"},
		{ID: "org-b", Owner: "target-1"},
	}

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target-1"}).Return(targetUser, nil)
	mockStore.EXPECT().ListOrganizations(gomock.Any(), gomock.Any()).Return(orgs, nil)
	// Critical: no UpdateWalletBalance, no GetOrganization, no UpdateUser.

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 50}) // OrgID omitted
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target-1/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	_, httpErr := server.adminGrantCredits(rr, req)

	require.NotNil(t, httpErr)
	he, ok := httpErr.(*system.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, he.StatusCode)
	assert.Contains(t, he.Error(), "org_id is required")
}

// Admin picks an org the user doesn't own (typo, wrong dropdown value, or
// an org owned by someone else entirely). Reject.
func TestAdminGrantCredits_OrgIDNotOwnedByUser_Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	targetUser := &types.User{ID: "target-1"}
	ownedOrg := &types.Organization{ID: "org-a", Owner: "target-1"}

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target-1"}).Return(targetUser, nil)
	mockStore.EXPECT().ListOrganizations(gomock.Any(), gomock.Any()).Return([]*types.Organization{ownedOrg}, nil)

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 50, OrgID: "org-someone-else"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target-1/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	_, httpErr := server.adminGrantCredits(rr, req)

	require.NotNil(t, httpErr)
	he, ok := httpErr.(*system.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, he.StatusCode)
	assert.Contains(t, he.Error(), "does not own")
}

// Admin tries to pass org_id when user owns no orgs (the stash path).
// Reject so the admin's intent isn't ambiguous (stash now vs apply later).
func TestAdminGrantCredits_NoOrg_OrgIDProvided_Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	targetUser := &types.User{ID: "target-1"}

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target-1"}).Return(targetUser, nil)
	mockStore.EXPECT().ListOrganizations(gomock.Any(), gomock.Any()).Return([]*types.Organization{}, nil)

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 50, OrgID: "org-anything"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target-1/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	_, httpErr := server.adminGrantCredits(rr, req)

	require.NotNil(t, httpErr)
	he, ok := httpErr.(*system.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, he.StatusCode)
	assert.Contains(t, he.Error(), "user owns no organisations")
}

// Verifies the "subscription state is irrelevant" promise: a wallet with an
// active Stripe subscription still accepts the top-up. This is the case that
// adminActivateTrial used to refuse with "already has an active subscription".
func TestAdminGrantCredits_HasOrgWithActiveSubscription_StillTopsUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	targetUser := &types.User{ID: "target-1", Email: "t@example.com"}
	org := &types.Organization{ID: "org-1", Owner: "target-1"}
	wallet := &types.Wallet{
		ID:                   "wal-1",
		OrgID:                "org-1",
		Balance:              0,
		StripeSubscriptionID: "sub_123",
		SubscriptionStatus:   "active",
	}

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target-1"}).Return(targetUser, nil)
	mockStore.EXPECT().ListOrganizations(gomock.Any(), gomock.Any()).Return([]*types.Organization{org}, nil)
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org-1"}).Return(org, nil)
	mockStore.EXPECT().GetWalletByOrg(gomock.Any(), "org-1").Return(wallet, nil)
	mockStore.EXPECT().UpdateWalletBalance(gomock.Any(), "wal-1", 25.0, gomock.Any()).Return(wallet, nil)

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 25, OrgID: "org-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target-1/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	resp, httpErr := server.adminGrantCredits(rr, req)

	require.Nil(t, httpErr)
	assert.Equal(t, "applied", resp.Status)
}

func TestAdminGrantCredits_UserNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: "ghost"}).
		Return(nil, errors.New("not found"))

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 10})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/ghost/credits", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "ghost"})
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "admin-1", Admin: true}))

	rr := httptest.NewRecorder()
	_, httpErr := server.adminGrantCredits(rr, req)

	require.NotNil(t, httpErr)
	he, ok := httpErr.(*system.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, he.StatusCode)
}

func TestListUserOwnedOrgs_ReturnsSortedSummaries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	adminUser := &types.User{ID: "admin-1", Admin: true}
	t0 := time.Now().Add(-72 * time.Hour)
	t1 := t0.Add(time.Hour)
	t2 := t1.Add(time.Hour)
	// Returned in arbitrary order; handler must sort by CreatedAt ascending.
	orgs := []*types.Organization{
		{ID: "org-c", Name: "c", DisplayName: "Org C", CreatedAt: t2, Owner: "target-1"},
		{ID: "org-a", Name: "a", DisplayName: "Org A", CreatedAt: t0, Owner: "target-1"},
		{ID: "org-b", Name: "b", DisplayName: "Org B", CreatedAt: t1, Owner: "target-1"},
	}

	mockStore.EXPECT().
		ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target-1"}).
		Return(orgs, nil)

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/target-1/owned-orgs", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), adminUser))

	rr := httptest.NewRecorder()
	resp, httpErr := server.listUserOwnedOrgs(rr, req)

	require.Nil(t, httpErr)
	require.Len(t, resp, 3)
	assert.Equal(t, "org-a", resp[0].ID)
	assert.Equal(t, "org-b", resp[1].ID)
	assert.Equal(t, "org-c", resp[2].ID)
	assert.Equal(t, "Org A", resp[0].DisplayName)
}

func TestListUserOwnedOrgs_NonAdminRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)

	server := &HelixAPIServer{Store: mockStore, Cfg: cloudBillingCfg()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/target-1/owned-orgs", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "target-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "u-1", Admin: false}))

	_, httpErr := server.listUserOwnedOrgs(httptest.NewRecorder(), req)
	require.NotNil(t, httpErr)
	he, ok := httpErr.(*system.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, he.StatusCode)
}
