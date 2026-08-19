package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

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
