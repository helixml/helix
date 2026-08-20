package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"

	"go.uber.org/mock/gomock"
)

type AppClaudeSubscriptionStatusSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestAppClaudeSubscriptionStatusSuite(t *testing.T) {
	suite.Run(t, new(AppClaudeSubscriptionStatusSuite))
}

func (s *AppClaudeSubscriptionStatusSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: s.store}
}

func (s *AppClaudeSubscriptionStatusSuite) statusRequest(viewer types.User, appID string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "/api/v1/agents/"+appID+"/claude-subscription-status", nil)
	s.Require().NoError(err)
	req = mux.SetURLVars(req, map[string]string{"id": appID})
	return req.WithContext(setRequestUser(req.Context(), viewer))
}

// A just-validated subscription so the handler does not re-probe (and does not
// need to decrypt credentials).
func freshSub() (sub *types.ClaudeSubscription, now time.Time) {
	now = time.Now()
	sub = &types.ClaudeSubscription{ID: "sub_1", Status: "active", LastValidatedAt: &now}
	return sub, now
}

// A user-owned subscription must surface the owner's email and the plan, and
// flag it as the viewer's own when they are the owner.
func (s *AppClaudeSubscriptionStatusSuite) TestUserOwnedSubExposesOwnerAndPlan() {
	viewer := types.User{ID: "usr_me"}
	app := &types.App{ID: "app_1", Owner: "usr_me"}
	sub, _ := freshSub()
	sub.OwnerID = "usr_me"
	sub.OwnerType = types.OwnerTypeUser
	sub.SubscriptionType = "max"

	s.store.EXPECT().GetApp(gomock.Any(), "app_1").Return(app, nil)
	s.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_me"}).
		Return(&types.User{ID: "usr_me", Email: "me@helix.ml"}, nil).AnyTimes()
	s.store.EXPECT().GetEffectiveClaudeSubscription(gomock.Any(), "usr_me", "").Return(sub, nil)

	status, httpErr := s.server.getAppClaudeSubscriptionStatus(nil, s.statusRequest(viewer, "app_1"))
	s.Nil(httpErr)
	s.Require().NotNil(status)

	s.True(status.Connected)
	s.True(status.Valid)
	s.True(status.IsCurrentUser)
	s.Equal("user", status.SubscriptionOwnerType)
	s.Equal("max", status.SubscriptionType)
	s.Equal("usr_me", status.SubscriptionOwnerID)
	s.Equal("me@helix.ml", status.SubscriptionOwnerName)
	s.True(status.SubscriptionOwnerIsCurrentUser)
}

// An org-owned subscription has no owner email — the org name is shown instead,
// and it can never be the viewer's own.
func (s *AppClaudeSubscriptionStatusSuite) TestOrgOwnedSubExposesOrgName() {
	viewer := types.User{ID: "usr_me"}
	app := &types.App{ID: "app_1", Owner: "usr_me"}
	sub, _ := freshSub()
	sub.OwnerID = "org_1"
	sub.OwnerType = types.OwnerTypeOrg
	sub.SubscriptionType = "pro"

	s.store.EXPECT().GetApp(gomock.Any(), "app_1").Return(app, nil)
	s.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_me"}).
		Return(&types.User{ID: "usr_me", Email: "me@helix.ml"}, nil).AnyTimes()
	s.store.EXPECT().GetEffectiveClaudeSubscription(gomock.Any(), "usr_me", "").Return(sub, nil)
	s.store.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_1"}).
		Return(&types.Organization{ID: "org_1", Name: "Acme"}, nil)

	status, httpErr := s.server.getAppClaudeSubscriptionStatus(nil, s.statusRequest(viewer, "app_1"))
	s.Nil(httpErr)
	s.Require().NotNil(status)

	s.True(status.Connected)
	s.False(status.SubscriptionOwnerIsCurrentUser)
	s.Equal("org", status.SubscriptionOwnerType)
	s.Equal("pro", status.SubscriptionType)
	s.Equal("Acme", status.SubscriptionOwnerName)
}

// The Claude account the token authenticates as (enriched from Anthropic's
// /api/oauth/profile) can differ from the Helix user who connected it — the
// account is the billing identity, the owner is who saved the subscription.
// The row-level fields surface verbatim when a validation has enriched them.
func (s *AppClaudeSubscriptionStatusSuite) TestClaudeAccountIdentitySurfaces() {
	viewer := types.User{ID: "usr_connector"}
	app := &types.App{ID: "app_1", Owner: "usr_connector"}
	sub, _ := freshSub()
	sub.OwnerID = "usr_connector"
	sub.OwnerType = types.OwnerTypeUser
	sub.SubscriptionType = "max"
	sub.RateLimitTier = "20x"
	sub.AccountEmail = "phil@winder.ai"
	sub.AccountDisplayName = "Phil Winder"

	s.store.EXPECT().GetApp(gomock.Any(), "app_1").Return(app, nil)
	s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).
		Return(&types.User{ID: "usr_connector", Email: "connector@helix.local"}, nil).AnyTimes()
	s.store.EXPECT().GetEffectiveClaudeSubscription(gomock.Any(), "usr_connector", "").Return(sub, nil)

	status, httpErr := s.server.getAppClaudeSubscriptionStatus(nil, s.statusRequest(viewer, "app_1"))
	s.Nil(httpErr)
	s.Require().NotNil(status)

	s.Equal("phil@winder.ai", status.ClaudeAccountEmail)
	s.Equal("Phil Winder", status.ClaudeAccountName)
	s.Equal("20x", status.SubscriptionRateLimitTier)
	s.Equal("max", status.SubscriptionType)
	s.Equal("connector@helix.local", status.SubscriptionOwnerName)
}

// Without a connected subscription the identity fields stay empty.
func (s *AppClaudeSubscriptionStatusSuite) TestNotConnectedLeavesIdentityEmpty() {
	viewer := types.User{ID: "usr_me"}
	app := &types.App{ID: "app_1", Owner: "usr_me"}

	s.store.EXPECT().GetApp(gomock.Any(), "app_1").Return(app, nil)
	s.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_me"}).
		Return(&types.User{ID: "usr_me", Email: "me@helix.ml"}, nil).AnyTimes()
	s.store.EXPECT().GetEffectiveClaudeSubscription(gomock.Any(), "usr_me", "").
		Return(nil, store.ErrNotFound)

	status, httpErr := s.server.getAppClaudeSubscriptionStatus(nil, s.statusRequest(viewer, "app_1"))
	s.Nil(httpErr)
	s.Require().NotNil(status)

	s.False(status.Connected)
	s.Empty(status.SubscriptionType)
	s.Empty(status.SubscriptionOwnerID)
	s.Empty(status.SubscriptionOwnerName)
	s.False(status.SubscriptionOwnerIsCurrentUser)
}
