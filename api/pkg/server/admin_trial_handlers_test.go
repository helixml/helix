package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	helixstripe "github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	stripeapi "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/form"
	"go.uber.org/mock/gomock"
)

func activateTrialRequest(t *testing.T, body ActivateTrialRequest) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target/trial-activate", bytes.NewReader(b))
	req = mux.SetURLVars(req, map[string]string{"id": "target"})
	return req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "admin", Admin: true}))
}

func TestAdminActivateTrial_NoOrgStashes(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	user := &types.User{ID: "target", Waitlisted: true}

	db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(user, nil)
	db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return(nil, nil)
	db.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got *types.User) (*types.User, error) {
		require.False(t, got.Waitlisted)
		require.NotNil(t, got.TrialDaysOnFirstOrg)
		require.Equal(t, 30, *got.TrialDaysOnFirstOrg)
		return got, nil
	})

	s := &HelixAPIServer{Store: db, Cfg: cloudBillingCfg()}
	resp, err := s.adminActivateTrial(httptest.NewRecorder(), activateTrialRequest(t, ActivateTrialRequest{Days: 30}))
	require.NoError(t, err)
	require.Equal(t, "stashed", resp.Status)
}

func TestAdminActivateTrial_AppliesPaidPlanToSelectedOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	user := &types.User{ID: "target", Waitlisted: true}
	orgA := &types.Organization{ID: "org-a", Owner: "target"}
	orgB := &types.Organization{ID: "org-b", Owner: "target"}
	walletB := &types.Wallet{ID: "wallet-b", OrgID: "org-b"}

	db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(user, nil)
	db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return([]*types.Organization{orgA, orgB}, nil)
	db.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org-b"}).Return(orgB, nil)
	db.EXPECT().GetWalletByOrg(gomock.Any(), "org-b").Return(walletB, nil)
	db.EXPECT().UpdateWallet(gomock.Any(), walletB).DoAndReturn(func(_ context.Context, got *types.Wallet) (*types.Wallet, error) {
		require.Equal(t, types.PlanOverridePro, got.PlanOverride)
		return got, nil
	})
	db.EXPECT().UpdateUser(gomock.Any(), user).DoAndReturn(func(_ context.Context, got *types.User) (*types.User, error) {
		require.False(t, got.Waitlisted)
		return got, nil
	})
	s := &HelixAPIServer{Store: db, Cfg: cloudBillingCfg()}
	resp, err := s.adminActivateTrial(httptest.NewRecorder(), activateTrialRequest(t, ActivateTrialRequest{
		OrgID: "org-b",
		Plan:  types.PlanOverridePro,
	}))
	require.NoError(t, err)
	require.Equal(t, "org-b", resp.OrgID)
}

func TestAdminActivateTrial_RequiresOwnedOrgSelection(t *testing.T) {
	for _, orgID := range []string{"", "org-other"} {
		t.Run(orgID, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := store.NewMockStore(ctrl)
			user := &types.User{ID: "target"}
			db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(user, nil)
			db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return([]*types.Organization{{ID: "org-owned"}}, nil)

			s := &HelixAPIServer{Store: db, Cfg: cloudBillingCfg()}
			_, err := s.adminActivateTrial(httptest.NewRecorder(), activateTrialRequest(t, ActivateTrialRequest{OrgID: orgID}))
			require.Error(t, err)
		})
	}
}

func TestAdminActivateTrial_ApprovesBeforePlanSideEffects(t *testing.T) {
	for _, plan := range []string{"", types.PlanOverridePro} {
		t.Run(plan, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := store.NewMockStore(ctrl)
			user := &types.User{ID: "target", Waitlisted: true}

			db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(user, nil)
			db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return([]*types.Organization{{ID: "org-owned"}}, nil)
			db.EXPECT().UpdateUser(gomock.Any(), user).Return(nil, fmt.Errorf("write failed"))

			s := &HelixAPIServer{Store: db, Cfg: cloudBillingCfg()}
			_, err := s.adminActivateTrial(httptest.NewRecorder(), activateTrialRequest(t, ActivateTrialRequest{OrgID: "org-owned", Plan: plan}))
			require.ErrorContains(t, err, "failed to activate user")
		})
	}
}

func TestSendActivationEmail_UsesApprovalEventForWaitlistedUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	notifier := notification.NewMockNotifier(ctrl)
	notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got *types.Notification) error {
		require.Equal(t, types.EventWaitlistApproved, got.Event)
		require.Equal(t, 30, got.TrialDays)
		return nil
	})
	s := &HelixAPIServer{Controller: &controller.Controller{Options: controller.Options{Notifier: notifier}}}
	s.sendActivationEmail(context.Background(), &types.User{Email: "target@example.com"}, 30, false, true)
}

func TestAdminApproveUser_MarksStashedTrialPendingInEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)
	days := 30
	user := &types.User{ID: "target", Email: "target@example.com", Waitlisted: true, TrialDaysOnFirstOrg: &days}

	db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(user, nil)
	db.EXPECT().UpdateUser(gomock.Any(), user).Return(user, nil)
	notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got *types.Notification) error {
		require.Equal(t, types.EventWaitlistApproved, got.Event)
		require.Equal(t, days, got.TrialDays)
		require.True(t, got.TrialPending)
		return nil
	})

	s := &HelixAPIServer{Store: db, Controller: &controller.Controller{Options: controller.Options{Notifier: notifier}}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/target/approve", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "target"})
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "admin", Admin: true}))
	_, err := s.adminApproveUser(httptest.NewRecorder(), req)
	require.NoError(t, err)
}

func TestEnrichUserTrialDisplay_FindsTrialOnNonFirstOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	user := &types.User{ID: "target"}
	orgA := &types.Organization{ID: "org-a"}
	orgB := &types.Organization{ID: "org-b"}

	db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return([]*types.Organization{orgA, orgB}, nil)
	db.EXPECT().GetWalletByOrg(gomock.Any(), "org-a").Return(&types.Wallet{SubscriptionStatus: stripeapi.SubscriptionStatusActive}, nil)
	db.EXPECT().GetWalletByOrg(gomock.Any(), "org-b").Return(&types.Wallet{SubscriptionStatus: stripeapi.SubscriptionStatusTrialing, SubscriptionCurrentPeriodEnd: 1234}, nil)

	(&HelixAPIServer{Store: db}).enrichUserTrialDisplay(context.Background(), user)
	require.Equal(t, "active", user.TrialStatus)
	require.Equal(t, "org-b", user.TrialOrgID)
	require.Equal(t, int64(1234), *user.TrialEndsAt)
}

type adminTrialStripeBackend struct {
	t *testing.T
}

func (b *adminTrialStripeBackend) Call(method, path, _ string, _ stripeapi.ParamsContainer, out stripeapi.LastResponseSetter) error {
	switch method {
	case http.MethodPost:
		require.Equal(b.t, "/v1/subscriptions", path)
		*out.(*stripeapi.Subscription) = stripeapi.Subscription{
			ID:                 "sub-b",
			Status:             stripeapi.SubscriptionStatusTrialing,
			CurrentPeriodStart: 1000,
			CurrentPeriodEnd:   2000,
		}
	case http.MethodDelete:
		require.Equal(b.t, "/v1/subscriptions/sub-b", path)
	default:
		return fmt.Errorf("unexpected call: %s %s", method, path)
	}
	return nil
}

func (*adminTrialStripeBackend) CallStreaming(string, string, string, stripeapi.ParamsContainer, stripeapi.StreamingLastResponseSetter) error {
	return fmt.Errorf("unexpected streaming call")
}
func (b *adminTrialStripeBackend) CallRaw(method, path, _ string, _ *form.Values, _ *stripeapi.Params, out stripeapi.LastResponseSetter) error {
	require.Equal(b.t, http.MethodGet, method)
	require.Equal(b.t, "/v1/prices", path)
	*out.(*stripeapi.PriceList) = stripeapi.PriceList{Data: []*stripeapi.Price{{
		ID:         "price-trial",
		Currency:   stripeapi.CurrencyUSD,
		UnitAmount: 49900,
		Recurring: &stripeapi.PriceRecurring{
			Interval: stripeapi.PriceRecurringIntervalMonth,
		},
	}}}
	return nil
}
func (*adminTrialStripeBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripeapi.Params, stripeapi.LastResponseSetter) error {
	return fmt.Errorf("unexpected multipart call")
}
func (*adminTrialStripeBackend) SetMaxNetworkRetries(int64) {}

func TestAdminActivateTrial_AppliesStripeTrialToSelectedOrgAndApprovesUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	user := &types.User{ID: "target", Waitlisted: true}
	orgA := &types.Organization{ID: "org-a", Owner: "target"}
	orgB := &types.Organization{ID: "org-b", Owner: "target"}
	walletB := &types.Wallet{ID: "wallet-b", OrgID: "org-b", StripeCustomerID: "cus-b"}

	db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(user, nil)
	db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return([]*types.Organization{orgA, orgB}, nil)
	db.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org-b"}).Return(orgB, nil)
	db.EXPECT().GetWalletByOrg(gomock.Any(), "org-b").Return(walletB, nil)
	db.EXPECT().UpdateWallet(gomock.Any(), walletB).DoAndReturn(func(_ context.Context, got *types.Wallet) (*types.Wallet, error) {
		require.Equal(t, "sub-b", got.StripeSubscriptionID)
		require.Equal(t, stripeapi.SubscriptionStatusTrialing, got.SubscriptionStatus)
		return got, nil
	})
	db.EXPECT().UpdateUser(gomock.Any(), user).DoAndReturn(func(_ context.Context, got *types.User) (*types.User, error) {
		require.False(t, got.Waitlisted)
		return got, nil
	})

	originalBackend := stripeapi.GetBackend(stripeapi.APIBackend)
	stripeapi.SetBackend(stripeapi.APIBackend, &adminTrialStripeBackend{t: t})
	t.Cleanup(func() { stripeapi.SetBackend(stripeapi.APIBackend, originalBackend) })
	cfg := cloudBillingCfg()
	cfg.Stripe.SecretKey = "sk_test"
	cfg.Stripe.WebhookSigningSecret = "whsec_test"
	cfg.Stripe.OrgPriceLookupKey = "org-trial"
	s := &HelixAPIServer{Store: db, Cfg: cfg, Stripe: helixstripe.NewStripe(cfg.Stripe, db)}

	resp, err := s.adminActivateTrial(httptest.NewRecorder(), activateTrialRequest(t, ActivateTrialRequest{Days: 30, OrgID: "org-b"}))
	require.NoError(t, err)
	require.Equal(t, "org-b", resp.OrgID)
	require.False(t, resp.User.Waitlisted)
}

func TestAdminRevokeTrial_CancelsOldestTrialingOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	oldOrg := &types.Organization{ID: "org-b", CreatedAt: time.Unix(1000, 0)}
	newOrg := &types.Organization{ID: "org-c", CreatedAt: time.Unix(2000, 0)}

	db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(&types.User{ID: "target"}, nil)
	db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return([]*types.Organization{newOrg, oldOrg}, nil)
	db.EXPECT().GetWalletByOrg(gomock.Any(), "org-b").Return(&types.Wallet{StripeSubscriptionID: "sub-b", SubscriptionStatus: stripeapi.SubscriptionStatusTrialing}, nil)

	originalBackend := stripeapi.GetBackend(stripeapi.APIBackend)
	stripeapi.SetBackend(stripeapi.APIBackend, &adminTrialStripeBackend{t: t})
	t.Cleanup(func() { stripeapi.SetBackend(stripeapi.APIBackend, originalBackend) })
	cfg := cloudBillingCfg()
	cfg.Stripe.SecretKey = "sk_test"
	cfg.Stripe.WebhookSigningSecret = "whsec_test"
	s := &HelixAPIServer{
		Store:  db,
		Cfg:    cfg,
		Stripe: helixstripe.NewStripe(cfg.Stripe, db),
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/target/trial-activate", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "target"})
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "admin", Admin: true}))

	resp, err := s.adminRevokeTrial(httptest.NewRecorder(), req)
	require.NoError(t, err)
	require.Equal(t, "cancelled", resp.Status)
	require.Equal(t, "org-b", resp.OrgID)
}

func TestAdminRevokeTrial_ContinuesAfterWalletReadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	oldOrg := &types.Organization{ID: "org-a", CreatedAt: time.Unix(1000, 0)}
	laterOrg := &types.Organization{ID: "org-b", CreatedAt: time.Unix(2000, 0)}

	db.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "target"}).Return(&types.User{ID: "target"}, nil)
	db.EXPECT().ListOrganizations(gomock.Any(), &store.ListOrganizationsQuery{Owner: "target"}).Return([]*types.Organization{oldOrg, laterOrg}, nil)
	db.EXPECT().GetWalletByOrg(gomock.Any(), "org-a").Return(nil, fmt.Errorf("read failed"))
	db.EXPECT().GetWalletByOrg(gomock.Any(), "org-b").Return(&types.Wallet{StripeSubscriptionID: "sub-b", SubscriptionStatus: stripeapi.SubscriptionStatusTrialing}, nil)

	originalBackend := stripeapi.GetBackend(stripeapi.APIBackend)
	stripeapi.SetBackend(stripeapi.APIBackend, &adminTrialStripeBackend{t: t})
	t.Cleanup(func() { stripeapi.SetBackend(stripeapi.APIBackend, originalBackend) })
	cfg := cloudBillingCfg()
	cfg.Stripe.SecretKey = "sk_test"
	cfg.Stripe.WebhookSigningSecret = "whsec_test"
	s := &HelixAPIServer{Store: db, Cfg: cfg, Stripe: helixstripe.NewStripe(cfg.Stripe, db)}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/target/trial-activate", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "target"})
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "admin", Admin: true}))

	resp, err := s.adminRevokeTrial(httptest.NewRecorder(), req)
	require.NoError(t, err)
	require.Equal(t, "cancelled", resp.Status)
	require.Equal(t, "org-b", resp.OrgID)
}
