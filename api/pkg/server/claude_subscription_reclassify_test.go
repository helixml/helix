package server

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"

	"go.uber.org/mock/gomock"
)

type ReclassifySubAuthSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestReclassifySubAuthSuite(t *testing.T) {
	suite.Run(t, new(ReclassifySubAuthSuite))
}

func (s *ReclassifySubAuthSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: s.store}
}

const genericAbort = "agent turn aborted: the ACP agent process exited mid-turn or hit max tokens (see Zed.log 'Error in run turn' for the cause)"

func subscriptionApp() *types.App {
	app := &types.App{ID: "app_1"}
	app.Config.Helix.Assistants = []types.AssistantConfig{{
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
	}}
	return app
}

// A non-generic error is passed through untouched — we never rewrite a specific error.
func (s *ReclassifySubAuthSuite) TestNonGenericErrorUnchanged() {
	specific := "compilation failed: syntax error"
	got := s.server.maybeReclassifySubscriptionAuthError(context.Background(), "ses_1", specific)
	s.Equal(specific, got)
}

// Generic error + subscription-mode session whose owner has NO subscription -> legible message naming the owner.
func (s *ReclassifySubAuthSuite) TestNoSubscriptionProducesLegibleError() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{
		ID: "ses_1", ParentApp: "app_1", Owner: "usr_chris", OrganizationID: "org_1",
	}, nil)
	s.store.EXPECT().GetApp(gomock.Any(), "app_1").Return(subscriptionApp(), nil)
	s.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_chris"}).
		Return(&types.User{ID: "usr_chris", Email: "chris@helix.ml"}, nil)
	s.store.EXPECT().GetEffectiveClaudeSubscription(gomock.Any(), "usr_chris", "org_1").
		Return(nil, store.ErrNotFound)

	got := s.server.maybeReclassifySubscriptionAuthError(context.Background(), "ses_1", genericAbort)
	s.Contains(got, "chris@helix.ml")
	s.Contains(got, "no Claude subscription connected")
	s.NotContains(got, "exited mid-turn")
}

// Generic error but the agent is in API-key mode (not subscription) -> passed through untouched.
func (s *ReclassifySubAuthSuite) TestApiKeyModeUnchanged() {
	app := &types.App{ID: "app_1"}
	app.Config.Helix.Assistants = []types.AssistantConfig{{
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
	}}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{
		ID: "ses_1", ParentApp: "app_1", Owner: "usr_chris",
	}, nil)
	s.store.EXPECT().GetApp(gomock.Any(), "app_1").Return(app, nil)

	got := s.server.maybeReclassifySubscriptionAuthError(context.Background(), "ses_1", genericAbort)
	s.Equal(genericAbort, got)
	s.True(strings.Contains(got, "exited mid-turn"))
}
