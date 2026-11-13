package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Org web search usage is readable by any member of the organization, as the
// org Usage page is (authorizeOrgMember). Every refusal is a 404 that reads
// the same whether or not the organization exists, and no refused request
// reaches the usage queries: the mock store has no expectation for them.
type WebSearchUsageHandlersSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	store   *store.MockStore
	server  *HelixAPIServer
	handler http.Handler
}

func TestWebSearchUsageHandlersSuite(t *testing.T) {
	suite.Run(t, new(WebSearchUsageHandlersSuite))
}

const (
	wsuOrgA     = "org_wsu_a"
	wsuOrgB     = "org_wsu_b"
	wsuOwnerA   = "usr_wsu_owner_a"
	wsuMemberA  = "usr_wsu_member_a"
	wsuOwnerB   = "usr_wsu_owner_b"
	wsuOutsider = "usr_wsu_outsider"
	// wsuBothOrgs is a member of both organizations.
	wsuBothOrgs = "usr_wsu_both"
)

var (
	wsuUsagePath    = "/api/v1/organizations/" + wsuOrgA + "/websearch/usage"
	wsuRequestsPath = "/api/v1/organizations/" + wsuOrgA + "/websearch/requests"
)

func (s *WebSearchUsageHandlersSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Store: s.store}

	memberships := map[string]map[string]types.OrganizationRole{
		wsuOwnerA:   {wsuOrgA: types.OrganizationRoleOwner},
		wsuMemberA:  {wsuOrgA: types.OrganizationRoleMember},
		wsuOwnerB:   {wsuOrgB: types.OrganizationRoleOwner},
		wsuBothOrgs: {wsuOrgA: types.OrganizationRoleMember, wsuOrgB: types.OrganizationRoleMember},
	}
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
			role, ok := memberships[q.UserID][q.OrganizationID]
			if !ok {
				return nil, store.ErrNotFound
			}
			return &types.OrganizationMembership{UserID: q.UserID, OrganizationID: q.OrganizationID, Role: role}, nil
		})
	s.store.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, q *store.GetOrganizationQuery) (*types.Organization, error) {
			for _, org := range []*types.Organization{{ID: wsuOrgA, Name: "wsu-a"}, {ID: wsuOrgB, Name: "wsu-b"}} {
				if q.ID == org.ID || (q.ID == "" && q.Name == org.Name) {
					return org, nil
				}
			}
			return nil, store.ErrNotFound
		})

	router := mux.NewRouter()
	s.server.registerWebSearchUsageRoutes(router.PathPrefix(APIPrefix).Subrouter())
	s.handler = router
}

func (s *WebSearchUsageHandlersSuite) get(caller types.User, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(setRequestUser(context.Background(), caller))
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s *WebSearchUsageHandlersSuite) day(date string) time.Time {
	t, err := time.Parse(time.DateOnly, date)
	s.Require().NoError(err)
	return t
}

func (s *WebSearchUsageHandlersSuite) TestMemberReadsUsage() {
	want := &types.WebSearchUsage{
		Days: []types.WebSearchUsageDay{{Date: "2026-11-02", Endpoint: types.WebSearchEndpointSearch,
			WebSearchUsageCounts: types.WebSearchUsageCounts{Requests: 3, CostDollars: 0.021, Succeeded: 2, OutOfCredits: 1}}},
		Totals:     types.WebSearchUsageCounts{Requests: 3, CostDollars: 0.021, Succeeded: 2, OutOfCredits: 1},
		ByEndpoint: []types.WebSearchEndpointUsage{{Endpoint: types.WebSearchEndpointSearch, WebSearchUsageCounts: types.WebSearchUsageCounts{Requests: 3, CostDollars: 0.021, Succeeded: 2, OutOfCredits: 1}}},
	}
	s.store.EXPECT().GetWebSearchUsage(gomock.Any(), &store.WebSearchRequestsQuery{
		OrganizationID: wsuOrgA, From: s.day("2026-11-01"), To: s.day("2026-11-08"),
	}).Return(want, nil)

	rec := s.get(types.User{ID: wsuMemberA}, "/api/v1/organizations/wsu-a/websearch/usage?from=2026-11-01&to=2026-11-07")

	s.Require().Equal(http.StatusOK, rec.Code, rec.Body.String())
	var got types.WebSearchUsage
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &got))
	s.Equal(*want, got)
	s.Contains(rec.Body.String(), `"outOfCredits":1`)
	s.Contains(rec.Body.String(), `"byEndpoint":[{"endpoint":"search","requests":3`)
}

func (s *WebSearchUsageHandlersSuite) TestDefaultRangeIsTheLast30Days() {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	s.store.EXPECT().GetWebSearchUsage(gomock.Any(), &store.WebSearchRequestsQuery{
		OrganizationID: wsuOrgA, From: today.AddDate(0, 0, -29), To: today.AddDate(0, 0, 1),
	}).Return(&types.WebSearchUsage{}, nil)

	s.Equal(http.StatusOK, s.get(types.User{ID: wsuOwnerA}, wsuUsagePath).Code)
}

func (s *WebSearchUsageHandlersSuite) TestMemberListsRequests() {
	s.store.EXPECT().ListWebSearchRequests(gomock.Any(), &store.WebSearchRequestsQuery{
		OrganizationID: wsuOrgA, From: s.day("2026-08-15"), To: s.day("2026-11-13"),
		Endpoint: types.WebSearchEndpointFindSimilar, Limit: 200, Offset: 400,
	}).Return(&types.WebSearchRequestList{
		Requests: []*types.WebSearchRequest{{ID: "req_1", OrganizationID: wsuOrgA, Endpoint: types.WebSearchEndpointFindSimilar, Status: 200}},
		Total:    401,
	}, nil)

	rec := s.get(types.User{ID: wsuMemberA}, wsuRequestsPath+"?endpoint=findSimilar&from=2026-08-15&to=2026-11-12&limit=200&offset=400")

	s.Require().Equal(http.StatusOK, rec.Code, rec.Body.String())
	var got types.WebSearchRequestList
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &got))
	s.Equal(int64(401), got.Total)
	s.Equal("req_1", got.Requests[0].ID)
}

func (s *WebSearchUsageHandlersSuite) TestRequestsDefaultPage() {
	s.store.EXPECT().ListWebSearchRequests(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q *store.WebSearchRequestsQuery) (*types.WebSearchRequestList, error) {
			s.Equal(50, q.Limit)
			s.Zero(q.Offset)
			s.Empty(q.Endpoint)
			return &types.WebSearchRequestList{Requests: []*types.WebSearchRequest{}}, nil
		})

	rec := s.get(types.User{ID: wsuOwnerA}, wsuRequestsPath)
	s.Equal(http.StatusOK, rec.Code)
	s.JSONEq(`{"requests":[],"total":0}`, rec.Body.String())
}

func (s *WebSearchUsageHandlersSuite) TestInvalidParametersAre400() {
	for _, query := range []string{
		"from=2026-11-1",
		"to=13/11/2026",
		"from=2026-11-08&to=2026-11-07",
		"from=2026-08-14&to=2026-11-12", // 91 days
		"limit=0",
		"limit=201",
		"limit=ten",
		"offset=-1",
		"endpoint=crawl",
	} {
		rec := s.get(types.User{ID: wsuOwnerA}, wsuRequestsPath+"?"+query)
		s.Equal(http.StatusBadRequest, rec.Code, query)
	}
	s.Equal(http.StatusBadRequest, s.get(types.User{ID: wsuOwnerA}, wsuUsagePath+"?from=2026-08-14&to=2026-11-12").Code)
}

// Tenant B, an outsider, and keys scoped elsewhere get the same 404 as an
// organization that does not exist, for both routes, and nothing is read.
func (s *WebSearchUsageHandlersSuite) TestCrossTenantIsRefused() {
	var reads []string
	s.store.EXPECT().GetWebSearchUsage(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, q *store.WebSearchRequestsQuery) (*types.WebSearchUsage, error) {
			reads = append(reads, "usage "+q.OrganizationID)
			return &types.WebSearchUsage{Totals: types.WebSearchUsageCounts{Requests: 7}}, nil
		})
	s.store.EXPECT().ListWebSearchRequests(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, q *store.WebSearchRequestsQuery) (*types.WebSearchRequestList, error) {
			reads = append(reads, "requests "+q.OrganizationID)
			return &types.WebSearchRequestList{Requests: []*types.WebSearchRequest{{ID: "req_secret"}}, Total: 1}, nil
		})

	callers := map[string]types.User{
		"owner of another org":                     {ID: wsuOwnerB},
		"user in no org":                           {ID: wsuOutsider},
		"member's key scoped to another org":       {ID: wsuBothOrgs, OrganizationID: wsuOrgB, TokenType: types.TokenTypeAPIKey},
		"other org owner's key claiming this org":  {ID: wsuOwnerB, OrganizationID: wsuOrgA, TokenType: types.TokenTypeAPIKey},
		"platform admin's key scoped to other org": {ID: "usr_wsu_admin", Admin: true, OrganizationID: wsuOrgB, TokenType: types.TokenTypeAPIKey},
	}
	for name, caller := range callers {
		for _, path := range []string{wsuUsagePath, wsuRequestsPath} {
			reads = nil
			rec := s.get(caller, path)
			s.Equal(http.StatusNotFound, rec.Code, "%s: %s", name, path)
			s.NotContains(rec.Body.String(), "req_secret", "%s: %s", name, path)
			s.Empty(reads, "%s: %s must not reach the usage queries", name, path)
		}
	}
	for _, path := range []string{"/api/v1/organizations/org_missing/websearch/usage", "/api/v1/organizations/missing/websearch/requests"} {
		rec := s.get(types.User{ID: wsuOwnerB}, path)
		s.Equal(http.StatusNotFound, rec.Code)
		s.Equal(strings.TrimSpace(s.get(types.User{ID: wsuOwnerB}, wsuUsagePath).Body.String()), strings.TrimSpace(rec.Body.String()),
			"no existence oracle")
	}
}

func (s *WebSearchUsageHandlersSuite) TestKeyScopedToTheOrgIsServed() {
	s.store.EXPECT().ListWebSearchRequests(gomock.Any(), gomock.Any()).Return(&types.WebSearchRequestList{}, nil)

	rec := s.get(types.User{ID: wsuBothOrgs, OrganizationID: wsuOrgA, TokenType: types.TokenTypeAPIKey}, wsuRequestsPath)
	s.Equal(http.StatusOK, rec.Code)
}

// A waitlisted or deactivated member's own API key is refused by the auth
// middleware before the handler runs.
func (s *WebSearchUsageHandlersSuite) TestInactiveAccountIsRefused() {
	for state, set := range accountStates {
		for _, path := range []string{wsuUsagePath, wsuRequestsPath} {
			s.SetupTest()
			user := &types.User{ID: wsuMemberA}
			set(user)
			s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).
				Return(&types.ApiKey{Key: "hl-wsu", Owner: wsuMemberA, OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI}, nil)
			s.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: wsuMemberA}).Return(user, nil).AnyTimes()
			s.store.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: wsuMemberA}, nil).AnyTimes()
			auth := newAuthMiddleware(nil, nil, s.store, authMiddlewareConfig{}, nil, nil)
			auth.lastSeenCache.Store(wsuMemberA, time.Now())

			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer hl-wsu")
			rec := httptest.NewRecorder()
			auth.extractMiddleware(s.handler).ServeHTTP(rec, req)

			s.Equal(http.StatusForbidden, rec.Code, "%s: %s", state, path)
		}
	}
}
