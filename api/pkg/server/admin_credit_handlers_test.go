package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 50})
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
	body, _ := json.Marshal(GrantCreditsRequest{Credits: 25})
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
