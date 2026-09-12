package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	helixstripe "github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestCheckoutHandlersRejectUnsafeReturnURLBeforeWalletLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Stripe.BillingEnabled = true
	cfg.Stripe.SecretKey = "sk_test"
	cfg.Stripe.WebhookSigningSecret = "whsec_test"
	server := &HelixAPIServer{
		Cfg:    cfg,
		Store:  mockStore,
		Stripe: helixstripe.NewStripe(cfg.Stripe, mockStore),
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/top-ups/new",
		bytes.NewBufferString(`{"amount":10,"return_url":"//evil.example"}`),
	)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_1"}))

	_, err := server.createTopUp(httptest.NewRecorder(), req)
	if err == nil {
		t.Fatal("expected unsafe return URL to be rejected")
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/subscription/new?return_url=%2F%2Fevil.example",
		nil,
	)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_1"}))
	_, err = server.subscriptionCreate(httptest.NewRecorder(), req)
	if err == nil {
		t.Fatal("expected unsafe subscription return URL to be rejected")
	}
}

func TestCreateTopUpAcceptsArbitraryPositiveAmount(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		GetWalletByUser(gomock.Any(), "usr_1").
		Return(nil, errors.New("stop before Stripe"))
	cfg := &config.ServerConfig{}
	cfg.Stripe.BillingEnabled = true
	server := &HelixAPIServer{Cfg: cfg, Store: mockStore}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/top-ups/new",
		bytes.NewBufferString(`{"amount":7,"return_url":"/onboarding"}`),
	)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_1"}))

	_, err := server.createTopUp(httptest.NewRecorder(), req)
	if err == nil || !strings.Contains(err.Error(), "stop before Stripe") {
		t.Fatalf("expected amount 7 to pass validation and reach wallet lookup, got %v", err)
	}
}

func TestCreateTopUpRejectsStripeAmountBoundsBeforeStoreCalls(t *testing.T) {
	for _, amount := range []string{"0.49", "1000000"} {
		t.Run(amount, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			cfg := &config.ServerConfig{}
			cfg.Stripe.BillingEnabled = true
			server := &HelixAPIServer{Cfg: cfg, Store: mockStore}
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/top-ups/new",
				bytes.NewBufferString(`{"amount":`+amount+`,"org_id":"org_1"}`),
			)
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_1"}))

			_, err := server.createTopUp(httptest.NewRecorder(), req)
			if err == nil || !strings.Contains(err.Error(), "amount must be between") {
				t.Fatalf("expected amount %s to be rejected before store calls, got %v", amount, err)
			}
		})
	}
}

type LookupOrgSuite struct {
	suite.Suite

	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestLookupOrgSuite(t *testing.T) {
	suite.Run(t, new(LookupOrgSuite))
}

func (s *LookupOrgSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Store: s.store}
}

// When the org row genuinely doesn't exist, the returned error must wrap
// store.ErrNotFound so callers can errors.Is-check it and respond with HTTP
// 404 instead of a generic 500. This is the load-bearing behaviour for the
// stale-org-slug bug — without the sentinel, every caller maps to 500.
func (s *LookupOrgSuite) TestErrNotFoundIsPreservedAsSentinel() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "ghost"}).
		Return(nil, store.ErrNotFound)

	_, err := s.server.lookupOrg(context.Background(), "ghost")
	s.Require().Error(err)
	s.True(errors.Is(err, store.ErrNotFound),
		"lookupOrg must wrap ErrNotFound so callers can map to 404; got %v", err)
	s.Contains(err.Error(), "ghost",
		"error message should name the supplied org reference so the user can spot a stale URL")
}

// A real DB error (connection failure, schema mismatch, …) must NOT be
// reported as ErrNotFound. Otherwise we'd silently turn server failures into
// 404s and hide real issues from on-call.
func (s *LookupOrgSuite) TestRealErrorIsNotConfusedWithNotFound() {
	dbErr := errors.New("connection refused")
	s.store.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any()).
		Return(nil, dbErr)

	_, err := s.server.lookupOrg(context.Background(), "real-org")
	s.Require().Error(err)
	s.False(errors.Is(err, store.ErrNotFound),
		"a non-ErrNotFound store error must not be reported as not-found")
	s.Contains(err.Error(), "connection refused")
}

// org_… IDs must route through query.ID; non-prefixed strings must route
// through query.Name. Captures the existing routing behaviour to guard against
// accidental regression while we were touching this function.
func (s *LookupOrgSuite) TestRoutesByIDPrefix() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_abc123"}).
		Return(&types.Organization{ID: "org_abc123", Name: "acme"}, nil)

	org, err := s.server.lookupOrg(context.Background(), "org_abc123")
	s.Require().NoError(err)
	s.Equal("org_abc123", org.ID)
}

func (s *LookupOrgSuite) TestRoutesBySlug() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "acme"}).
		Return(&types.Organization{ID: "org_abc123", Name: "acme"}, nil)

	org, err := s.server.lookupOrg(context.Background(), "acme")
	s.Require().NoError(err)
	s.Equal("acme", org.Name)
}

// GetWalletStatusSuite covers the HTTP status codes getWalletHandler returns
// for an org_id the caller can't use. A non-member has no membership row, so
// the store answers ErrNotFound; that used to be wrapped in a 500, which made
// a stale org slug in the URL look like the API had fallen over. Every other
// org-scoped endpoint (e.g. /provider-endpoints) answers 403 — so must this.
type GetWalletStatusSuite struct {
	suite.Suite

	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestGetWalletStatusSuite(t *testing.T) {
	suite.Run(t, new(GetWalletStatusSuite))
}

func (s *GetWalletStatusSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{
		Store: s.store,
		Cfg:   &config.ServerConfig{},
	}
	s.server.Cfg.Stripe.BillingEnabled = true
}

func (s *GetWalletStatusSuite) request(orgRef string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wallet?org_id="+orgRef, nil)
	return req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_outsider"}))
}

func (s *GetWalletStatusSuite) TestNonMemberGets403() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "unmanned-org"}).
		Return(&types.Organization{ID: "org_unmanned", Name: "unmanned-org"}, nil)
	s.store.EXPECT().
		GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	_, httpErr := s.server.getWalletHandler(httptest.NewRecorder(), s.request("unmanned-org"))
	s.Require().NotNil(httpErr)
	s.Equal(http.StatusForbidden, httpErr.StatusCode,
		"a caller who is not a member must get 403, not 500")
}

func (s *GetWalletStatusSuite) TestMissingOrgGets404() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "ghost-org"}).
		Return(nil, store.ErrNotFound)

	_, httpErr := s.server.getWalletHandler(httptest.NewRecorder(), s.request("ghost-org"))
	s.Require().NotNil(httpErr)
	s.Equal(http.StatusNotFound, httpErr.StatusCode,
		"an org slug that doesn't exist at all must get 404")
}

// A genuine store failure must still surface as a 500 — we don't want to hide
// outages behind a permission-shaped answer.
func (s *GetWalletStatusSuite) TestStoreFailureStill500() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused"))

	_, httpErr := s.server.getWalletHandler(httptest.NewRecorder(), s.request("acme"))
	s.Require().NotNil(httpErr)
	s.Equal(http.StatusInternalServerError, httpErr.StatusCode)
}

// GetOrganizationStatusSuite covers the status codes GET /organizations/{id}
// returns. The frontend treats 403 as a permanent "this org is not yours" and
// evicts the user from it, so a transient membership-query failure must not
// masquerade as one — otherwise a DB blip kicks users out of healthy orgs.
type GetOrganizationStatusSuite struct {
	suite.Suite

	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestGetOrganizationStatusSuite(t *testing.T) {
	suite.Run(t, new(GetOrganizationStatusSuite))
}

func (s *GetOrganizationStatusSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Store: s.store, Cfg: &config.ServerConfig{}}
}

func (s *GetOrganizationStatusSuite) do(orgRef string) *httptest.ResponseRecorder {
	req := mux.SetURLVars(
		httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgRef, nil),
		map[string]string{"id": orgRef},
	)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_outsider"}))

	rw := httptest.NewRecorder()
	s.server.getOrganization(rw, req)
	return rw
}

func (s *GetOrganizationStatusSuite) TestNonMemberGets403() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "unmanned-org"}).
		Return(&types.Organization{ID: "org_unmanned", Name: "unmanned-org"}, nil)
	s.store.EXPECT().
		GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	s.Equal(http.StatusForbidden, s.do("unmanned-org").Code)
}

// The case that makes the frontend's "403 is permanent" classifier safe.
func (s *GetOrganizationStatusSuite) TestTransientMembershipErrorGets500() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any()).
		Return(&types.Organization{ID: "org_mine", Name: "mine"}, nil)
	s.store.EXPECT().
		GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused"))

	s.Equal(http.StatusInternalServerError, s.do("mine").Code,
		"a DB failure must not be reported as a permission denial")
}

func (s *GetOrganizationStatusSuite) TestMissingOrgGets404() {
	s.store.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	s.Equal(http.StatusNotFound, s.do("ghost-org").Code)
}

func TestOnboardingTrialPeriodDays(t *testing.T) {
	tests := []struct {
		name      string
		user      *types.User
		wallet    *types.Wallet
		returnURL string
		want      int64
	}{
		{
			name:      "new user entering from onboarding",
			user:      &types.User{},
			wallet:    &types.Wallet{},
			returnURL: "/onboarding?org_id=org_123",
			want:      3,
		},
		{
			name:      "completed onboarding",
			user:      &types.User{OnboardingCompleted: true},
			wallet:    &types.Wallet{},
			returnURL: "/onboarding",
			want:      0,
		},
		{
			name:      "existing subscription",
			user:      &types.User{},
			wallet:    &types.Wallet{StripeSubscriptionID: "sub_123"},
			returnURL: "/onboarding",
			want:      0,
		},
		{
			name:      "organization billing entry point",
			user:      &types.User{},
			wallet:    &types.Wallet{},
			returnURL: "",
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := onboardingTrialPeriodDays(tt.user, tt.wallet, tt.returnURL); got != tt.want {
				t.Fatalf("onboardingTrialPeriodDays() = %d, want %d", got, tt.want)
			}
		})
	}
}
