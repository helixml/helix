package server

import (
	"context"
	"errors"
	"testing"

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
