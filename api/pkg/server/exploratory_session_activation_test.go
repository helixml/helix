package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// ExploratorySessionActivationSuite pins the contract that worker
// activation reuses the project's existing Human Desktop session row
// instead of minting a parallel one. Regression coverage for the bug
// where StartExternalAgentSession unconditionally generated a fresh
// session ID even when an exploratory row for req.ProjectID already
// existed, leaving GetProjectExploratorySession's ORDER-BY-created-DESC
// LIMIT 1 lookup disagreeing with whichever id StartDesktop registered
// in the hydra session map. Operator-visible symptom: clicking "Resume
// Human Desktop" succeeds, the container starts, but the status pill
// stays stuck on "Resume Human Desktop / stopped" because the project's
// "latest" exploratory row is a different one.
//
// See design at helix-specs:002090_ffs-looks-like-the-human/design.md
// for the full call-graph and reuse semantics.
type ExploratorySessionActivationSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestExploratorySessionActivationSuite(t *testing.T) {
	suite.Run(t, new(ExploratorySessionActivationSuite))
}

func (s *ExploratorySessionActivationSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

	s.server = &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
		Cfg:                   &config.ServerConfig{},
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
}

func (s *ExploratorySessionActivationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Case 1: regression gate — when the project already has an exploratory
// session (created via startExploratorySession), the worker-activation
// path that calls StartExternalAgentSession with SessionRole="exploratory"
// and a matching ProjectID must REUSE that row, not mint a parallel one.
//
// The bug on main: StartExternalAgentSession generates a fresh session
// ID unconditionally → two exploratory rows for the same project →
// GetProjectExploratorySession LIMIT 1 returns the newer one, while the
// resumed desktop is keyed off the older row's id in hydra's session
// map. Fix: pre-resolve the session id via GetProjectExploratorySession
// when (SessionRole=="exploratory" && ProjectID!="").
func (s *ExploratorySessionActivationSuite) TestActivationReusesExistingExploratorySession() {
	ctx := context.Background()

	const (
		projectID    = "prj_test"
		orgID        = "org_test"
		userID       = "user_op"
		existingSID  = "ses_existing_exploratory"
		existingName = "Explore: testproj"
	)

	// Pre-existing exploratory session for the project — the row that
	// would have been created by an earlier startExploratorySession call
	// (or a prior worker activation).
	existing := &types.Session{
		ID:             existingSID,
		Name:           existingName,
		ProjectID:      projectID,
		OrganizationID: orgID,
		Owner:          userID,
		OwnerType:      types.OwnerTypeUser,
		Provider:       "anthropic",
		ModelName:      "external_agent",
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Metadata: types.SessionMetadata{
			Stream:      true,
			AgentType:   "zed_external",
			ProjectID:   projectID,
			SessionRole: "exploratory",
		},
	}

	// Expectations: the lookup runs first and returns the existing row.
	s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID:             userID,
		OrganizationID: orgID,
		Type:           types.OwnerTypeUser,
	}, nil)
	s.store.EXPECT().
		GetProjectExploratorySession(gomock.Any(), projectID).
		Return(existing, nil)

	// WriteSession does GetSession → UpdateSession upsert (existing row).
	// CreateInteractions appends the new prompt as an interaction.
	s.store.EXPECT().GetSession(gomock.Any(), existingSID).Return(existing, nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(existing, nil).AnyTimes()
	s.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// attachProjectContext lookups (no repos in this test).
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil)

	// StartDesktop must be called with the EXISTING session id.
	var captured *types.DesktopAgent
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			captured = agent
			return &types.DesktopAgentResponse{DevContainerID: "dev_reused"}, nil
		},
	).Times(1)

	// The request shape sent by helix-org's inProcHelixClient.StartSession.
	req := &types.SessionChatRequest{
		ProjectID:      projectID,
		OrganizationID: orgID,
		SessionRole:    "exploratory",
		AgentType:      "zed_external",
		Messages: []*types.Message{{
			Role:    "user",
			Content: types.MessageContent{Parts: []any{"activation prompt"}},
		}},
	}

	got, err := s.server.StartExternalAgentSession(ctx, req, userID)
	s.Require().NoError(err)
	s.Require().NotNil(got)

	// The load-bearing assertion: returned session id is the existing one,
	// NOT a fresh system.GenerateSessionID(). On main today, this fails
	// because StartExternalAgentSession unconditionally generates a fresh id.
	s.Equal(existingSID, got.ID,
		"activation must reuse the project's existing exploratory session id, not mint a parallel one")

	// StartDesktop must have been called with the reused id so the hydra
	// session map key matches what GetProjectExploratorySession returns.
	s.Require().NotNil(captured)
	s.Equal(existingSID, captured.SessionID,
		"StartDesktop must be invoked with the reused session id so h.sessions[ID] aligns with GetProjectExploratorySession")
}

// Case 2: status pill flips to "running" after Resume. Pins the
// getProjectExploratorySession → external_agent_status contract that
// the UI reads. Not directly testing the new guard, but stops a future
// refactor from breaking the read side that the operator stares at.
func (s *ExploratorySessionActivationSuite) TestExploratoryStatusReportsRunningWhenExecutorHasSession() {
	const (
		projectID = "prj_status"
		userID    = "user_op"
		sessionID = "ses_explore_status"
	)

	// project.OrganizationID="" so authorizeUserToProject takes the
	// owner-only fast path and does no org-membership lookup.
	project := &types.Project{
		ID:     projectID,
		UserID: userID,
	}
	existing := &types.Session{
		ID:        sessionID,
		ProjectID: projectID,
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Metadata: types.SessionMetadata{
			ProjectID:   projectID,
			SessionRole: "exploratory",
		},
	}

	// Round 1: executor has no session — must report "stopped".
	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil).Times(1)
	s.store.EXPECT().GetProjectExploratorySession(gomock.Any(), projectID).Return(existing, nil).Times(1)
	s.executor.EXPECT().GetSession(sessionID).Return(nil, errors.New("session not found")).Times(1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/exploratory-session", nil)
	req = mux.SetURLVars(req, map[string]string{"id": projectID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))
	got, herr := s.server.getProjectExploratorySession(nil, req)
	s.Require().Nil(herr)
	s.Require().NotNil(got)
	s.Equal("stopped", got.Metadata.ExternalAgentStatus,
		"empty hydra session map → status must report stopped")

	// Round 2: executor has the session — must report "running".
	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil).Times(1)
	s.store.EXPECT().GetProjectExploratorySession(gomock.Any(), projectID).Return(existing, nil).Times(1)
	s.executor.EXPECT().GetSession(sessionID).Return(&external_agent.ZedSession{Status: "running"}, nil).Times(1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/exploratory-session", nil)
	req2 = mux.SetURLVars(req2, map[string]string{"id": projectID})
	req2 = req2.WithContext(setRequestUser(req2.Context(), types.User{ID: userID}))
	got2, herr2 := s.server.getProjectExploratorySession(nil, req2)
	s.Require().Nil(herr2)
	s.Require().NotNil(got2)
	s.Equal("running", got2.Metadata.ExternalAgentStatus,
		"hydra session map populated → status must report running")
}

// Case 3: no project → no reuse. The guard is gated on ProjectID != "",
// so a SessionRole="exploratory" request without a project must still
// mint a fresh id (no GetProjectExploratorySession call). Confirms we
// didn't accidentally make every exploratory session globally singleton.
func (s *ExploratorySessionActivationSuite) TestNoProjectStillMintsFreshSession() {
	ctx := context.Background()

	const userID = "user_op"

	s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID:   userID,
		Type: types.OwnerTypeUser,
	}, nil)

	// NO GetProjectExploratorySession call expected (gomock fails if called).
	// WriteSession + WriteInteractions for the freshly-minted session.
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil).AnyTimes()
	s.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	// attachProjectContext is a no-op when ProjectID is empty.

	var captured *types.DesktopAgent
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			captured = agent
			return &types.DesktopAgentResponse{DevContainerID: "dev_fresh"}, nil
		},
	).Times(1)

	req := &types.SessionChatRequest{
		SessionRole: "exploratory",
		AgentType:   "zed_external",
		Messages: []*types.Message{{
			Role:    "user",
			Content: types.MessageContent{Parts: []any{"orphan exploratory"}},
		}},
	}

	got, err := s.server.StartExternalAgentSession(ctx, req, userID)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.NotEmpty(got.ID, "must mint a fresh session id when no project to key against")
	s.True(strings.HasPrefix(got.ID, "ses_"), "minted id must be a real ses_… id, got %q", got.ID)

	s.Require().NotNil(captured)
	s.Equal(got.ID, captured.SessionID)
}

// Case 4: different SessionRole → no reuse. The guard is gated on
// SessionRole=="exploratory". A SessionRole="planning" request (or any
// non-exploratory role) must mint a fresh id — GetProjectExploratorySession's
// WHERE clause hard-codes session_role='exploratory' so we can't
// accidentally collide planning sessions onto exploratory ones.
func (s *ExploratorySessionActivationSuite) TestNonExploratoryRoleStillMintsFreshSession() {
	ctx := context.Background()

	const (
		projectID = "prj_planning"
		userID    = "user_op"
	)

	s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID:   userID,
		Type: types.OwnerTypeUser,
	}, nil)

	// NO GetProjectExploratorySession call expected.
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil).AnyTimes()
	s.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil)

	var captured *types.DesktopAgent
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			captured = agent
			return &types.DesktopAgentResponse{DevContainerID: "dev_planning"}, nil
		},
	).Times(1)

	req := &types.SessionChatRequest{
		ProjectID:   projectID,
		SessionRole: "planning",
		AgentType:   "zed_external",
		Messages: []*types.Message{{
			Role:    "user",
			Content: types.MessageContent{Parts: []any{"plan something"}},
		}},
	}

	got, err := s.server.StartExternalAgentSession(ctx, req, userID)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.NotEmpty(got.ID)
	s.True(strings.HasPrefix(got.ID, "ses_"), "minted id must be a real ses_… id, got %q", got.ID)

	s.Require().NotNil(captured)
	s.Equal(got.ID, captured.SessionID)
}
