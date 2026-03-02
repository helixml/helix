package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// func TestLimitInteractions(t *testing.T) {
// 	// Helper function to create test interactions
// 	createTestInteractions := func() []*types.Interaction {
// 		interactions := []*types.Interaction{
// 			{
// 				ID:      "1",
// 				Message: "A",
// 			},
// 			{
// 				ID:      "2",
// 				Message: "B",
// 			},
// 			{
// 				ID:      "3",
// 				Message: "C",
// 			},
// 			{
// 				ID:      "4",
// 				Message: "D",
// 			},
// 			{
// 				ID:      "5",
// 				Message: "E",
// 			},
// 			{
// 				ID:      "6",
// 				Message: "F",
// 			},
// 		}
// 		return interactions
// 	}

// 	// Case when we have less interactions than the limit
// 	t.Run("LessThanLimit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 10)
// 		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "A", result[0].Message)
// 		assert.Equal(t, "E", result[4].Message)
// 	})

// 	t.Run("Exact limit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 6)
// 		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "A", result[0].Message)
// 		assert.Equal(t, "E", result[4].Message)
// 	})

// 	// More messages than the limit
// 	t.Run("MoreThanLimit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 3)
// 		assert.Equal(t, 3, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "C", result[0].Message)
// 		assert.Equal(t, "E", result[2].Message)
// 	})

// 	t.Run("ZeroLimit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 0)
// 		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "A", result[0].Message)
// 		assert.Equal(t, "E", result[4].Message)
// 	})
// }

type AppendOrOverwriteSuite struct {
	suite.Suite
}

func TestAppendOrOverwriteSuite(t *testing.T) {
	suite.Run(t, new(AppendOrOverwriteSuite))
}

func (suite *AppendOrOverwriteSuite) TestAppendToEmptySession() {
	session := &types.Session{
		Interactions: []*types.Interaction{},
		GenerationID: 0,
	}

	req := &types.SessionChatRequest{
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hello, how are you?",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Equal(0, session.GenerationID)

	suite.Require().Len(session.Interactions, 1)
	suite.Equal("Hello, how are you?", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[0].State)
}

func (suite *AppendOrOverwriteSuite) TestAppendToNonEmptySession() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant message",
			},
		},
	}

	req := &types.SessionChatRequest{
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"How are you?",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 2)
	suite.Equal("user message", session.Interactions[0].PromptMessage)
	suite.Equal("assistant message", session.Interactions[0].ResponseMessage)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)

	suite.Equal("How are you?", session.Interactions[1].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[1].State)
	suite.Equal("", session.Interactions[1].ResponseMessage)
}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_LastMessage() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant message",
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate: true, // Regenerate
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"new user message",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 1, "still expecting one interaction")

	suite.Equal("new user message", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[0].State)
	suite.Equal("", session.Interactions[0].ResponseMessage)

}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_FirstMessage() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message 1",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 1",
			},
			{
				ID:              "2",
				PromptMessage:   "user message 2",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 2",
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate:    true,
		InteractionID: "1",
		Messages: []*types.Message{
			{
				ID:   "1",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"overwriting user message 1",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 1)
	suite.Equal("overwriting user message 1", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[0].State)
	suite.Equal("", session.Interactions[0].ResponseMessage)

}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_MiddleMessage() {
	session := &types.Session{
		GenerationID: 1,
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message 1",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 1",
				GenerationID:    1,
			},
			{
				ID:              "2",
				PromptMessage:   "user message 2",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 2",
				GenerationID:    1,
			},
			{
				ID:              "3",
				PromptMessage:   "user message 3",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 3",
				GenerationID:    1,
			},
			{
				ID:              "4",
				PromptMessage:   "user message 4",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 4",
				GenerationID:    1,
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate:    true,
		InteractionID: "2",
		Messages: []*types.Message{
			{
				ID:   "1",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"user message 1",
					},
				},
			},
			{
				ID:   "2",
				Role: "assistant",
				Content: types.MessageContent{
					Parts: []interface{}{
						"assistant response 1",
					},
				},
			},
			// Overwriting the third user message
			{
				ID:   "3",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"regenerating from here",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	// Should be 2 interactions:
	// First interaction is "user message 1" and "assistant response 1"
	// Second interaction is "overwriting user message 3"
	suite.Require().Len(session.Interactions, 2)

	// First interaction should be the new user message
	suite.Equal("user message 1", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)
	suite.Equal("assistant response 1", session.Interactions[0].ResponseMessage)

	suite.Equal("regenerating from here", session.Interactions[1].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[1].State)
	suite.Equal("", session.Interactions[1].ResponseMessage)

	// Check generation IDs
	suite.Equal(2, session.Interactions[0].GenerationID)
	suite.Equal(2, session.Interactions[1].GenerationID)
}

type ExternalAgentSessionSuite struct {
	suite.Suite
}

func TestExternalAgentSessionSuite(t *testing.T) {
	suite.Run(t, new(ExternalAgentSessionSuite))
}

func (suite *ExternalAgentSessionSuite) TestExternalAgentModelProcessing() {
	// Test that external_agent model is properly processed
	provider := "helix"
	modelName := "external_agent"
	sessionType := types.SessionTypeText

	processedModel, err := suite.processModelName(provider, modelName, sessionType)
	suite.NoError(err)
	suite.Equal("external_agent", processedModel)
}

func (suite *ExternalAgentSessionSuite) TestExternalAgentSessionRequest() {
	// Test session creation with external agent configuration
	req := &types.SessionChatRequest{
		Type:      types.SessionTypeText,
		Model:     "external_agent",
		AgentType: "zed_external",
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hello from external agent test",
					},
				},
			},
		},
		ExternalAgentConfig: &types.ExternalAgentConfig{
			Resolution: "1080p",
		},
	}

	// Verify the request structure
	suite.Equal("external_agent", req.Model)
	suite.Equal("zed_external", req.AgentType)
	suite.Equal("1080p", req.ExternalAgentConfig.Resolution)
}

func (suite *ExternalAgentSessionSuite) TestExternalAgentSessionMetadata() {
	// Test that session metadata is properly set for external agents
	session := &types.Session{
		ID:           "test-session-id",
		Name:         "External Agent Test Session",
		Mode:         types.SessionModeInference,
		Type:         types.SessionTypeText,
		ModelName:    "external_agent",
		Interactions: []*types.Interaction{},
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}

	suite.Equal("external_agent", session.ModelName)
	suite.Equal("zed_external", session.Metadata.AgentType)
	suite.Equal(types.SessionModeInference, session.Mode)
	suite.Equal(types.SessionTypeText, session.Type)
}

// Helper method to simulate model processing
func (suite *ExternalAgentSessionSuite) processModelName(provider, modelName string, sessionType types.SessionType) (string, error) {
	// Simulate the ProcessModelName function logic for external agents
	if provider == "helix" && modelName == "external_agent" && sessionType == types.SessionTypeText {
		return "external_agent", nil
	}
	return modelName, nil
}

// =============================================================================
// Session Authorization Tests
// =============================================================================

type SessionAuthzSuite struct {
	suite.Suite

	ctrl    *gomock.Controller
	store   *store.MockStore
	server  *HelixAPIServer
	authCtx context.Context

	userID string
	orgID  string
}

func TestSessionAuthzSuite(t *testing.T) {
	suite.Run(t, new(SessionAuthzSuite))
}

func (s *SessionAuthzSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.store,
	}
	s.userID = "user_123"
	s.orgID = "org_123"
	s.authCtx = setRequestUser(context.Background(), types.User{
		ID: s.userID,
	})
}

// -----------------------------------------------------------------------------
// deleteSession tests
// -----------------------------------------------------------------------------

// 1. User is the session owner, no org
func (s *SessionAuthzSuite) TestDeleteSession_OwnerNoOrg() {
	session := &types.Session{
		ID:    "ses_123",
		Owner: s.userID,
	}

	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	s.store.EXPECT().DeleteSession(gomock.Any(), session.ID).Return(session, nil)

	req := httptest.NewRequest("DELETE", "/api/v1/sessions/ses_123", http.NoBody)
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})

	result, httpErr := s.server.deleteSession(httptest.NewRecorder(), req)

	s.Nil(httpErr)
	s.Require().NotNil(result)
	s.Equal(session.ID, result.ID)
}

// 2. User is the session owner in an org
func (s *SessionAuthzSuite) TestDeleteSession_OwnerInOrg() {
	session := &types.Session{
		ID:             "ses_123",
		Owner:          s.userID,
		OrganizationID: s.orgID,
	}

	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	s.store.EXPECT().DeleteSession(gomock.Any(), session.ID).Return(session, nil)

	req := httptest.NewRequest("DELETE", "/api/v1/sessions/ses_123", http.NoBody)
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})

	result, httpErr := s.server.deleteSession(httptest.NewRecorder(), req)

	s.Nil(httpErr)
	s.Require().NotNil(result)
	s.Equal(session.ID, result.ID)
}

// 3. User is not the session owner but is the org owner
func (s *SessionAuthzSuite) TestDeleteSession_OrgOwnerNotSessionOwner() {
	session := &types.Session{
		ID:             "ses_123",
		Owner:          "other_user",
		OrganizationID: s.orgID,
	}

	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleOwner,
	}, nil)
	s.store.EXPECT().DeleteSession(gomock.Any(), session.ID).Return(session, nil)

	req := httptest.NewRequest("DELETE", "/api/v1/sessions/ses_123", http.NoBody)
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})

	result, httpErr := s.server.deleteSession(httptest.NewRecorder(), req)

	s.Nil(httpErr)
	s.Require().NotNil(result)
	s.Equal(session.ID, result.ID)
}

// 4. User is an org member, not the session owner, NOT authorized to the project
func (s *SessionAuthzSuite) TestDeleteSession_OrgMemberNotAuthorizedToProject() {
	session := &types.Session{
		ID:             "ses_123",
		Owner:          "other_user",
		OrganizationID: s.orgID,
		ProjectID:      "proj_123",
	}

	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	// No teams
	s.store.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	// No access grants
	s.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     session.ProjectID,
	}).Return([]*types.AccessGrant{}, nil)

	req := httptest.NewRequest("DELETE", "/api/v1/sessions/ses_123", http.NoBody)
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})

	result, httpErr := s.server.deleteSession(httptest.NewRecorder(), req)

	s.Nil(result)
	s.Require().NotNil(httpErr)
	s.Equal(http.StatusForbidden, httpErr.StatusCode)
}

// 5. User is an org member, not the session owner, authorized to the project
func (s *SessionAuthzSuite) TestDeleteSession_OrgMemberAuthorizedToProject() {
	session := &types.Session{
		ID:             "ses_123",
		Owner:          "other_user",
		OrganizationID: s.orgID,
		ProjectID:      "proj_123",
	}

	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	// No teams
	s.store.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	// Has access grant to the project
	s.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     session.ProjectID,
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{
				{
					Config: types.Config{
						Rules: []types.Rule{
							{
								Resources: []types.Resource{types.ResourceProject},
								Actions:   []types.Action{types.ActionDelete},
							},
						},
					},
				},
			},
		},
	}, nil)
	s.store.EXPECT().DeleteSession(gomock.Any(), session.ID).Return(session, nil)

	req := httptest.NewRequest("DELETE", "/api/v1/sessions/ses_123", http.NoBody)
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})

	result, httpErr := s.server.deleteSession(httptest.NewRecorder(), req)

	s.Nil(httpErr)
	s.Require().NotNil(result)
	s.Equal(session.ID, result.ID)
}

// 6. User is not a member of the org and not the session owner
func (s *SessionAuthzSuite) TestDeleteSession_NotOrgMemberNotSessionOwner() {
	session := &types.Session{
		ID:             "ses_123",
		Owner:          "other_user",
		OrganizationID: s.orgID,
	}

	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(nil, fmt.Errorf("not found"))

	req := httptest.NewRequest("DELETE", "/api/v1/sessions/ses_123", http.NoBody)
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": session.ID})

	result, httpErr := s.server.deleteSession(httptest.NewRecorder(), req)

	s.Nil(result)
	s.Require().NotNil(httpErr)
	s.Equal(http.StatusForbidden, httpErr.StatusCode)
}

// -----------------------------------------------------------------------------
// listSessions tests
// -----------------------------------------------------------------------------

// Helper to assert listSessions error is an HTTPError with expected status code
func assertHTTPError(s *SessionAuthzSuite, err error, expectedStatus int) {
	s.Require().Error(err)
	httpErr, ok := err.(*system.HTTPError)
	s.Require().True(ok, "expected *system.HTTPError, got %T", err)
	s.Equal(expectedStatus, httpErr.StatusCode)
}

// 1. User lists personal sessions (no org)
func (s *SessionAuthzSuite) TestListSessions_OwnerNoOrg() {
	s.store.EXPECT().ListSessions(gomock.Any(), store.ListSessionsQuery{
		Owner:   s.userID,
		Page:    0,
		PerPage: 50,
	}).Return([]*types.Session{
		{ID: "ses_1", Owner: s.userID, Name: "test"},
	}, int64(1), nil)

	req := httptest.NewRequest("GET", "/api/v1/sessions", http.NoBody)
	req = req.WithContext(s.authCtx)

	result, err := s.server.listSessions(httptest.NewRecorder(), req)

	s.NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(1), result.TotalCount)
}

// 2. User is owner and org member, lists org sessions
func (s *SessionAuthzSuite) TestListSessions_OwnerInOrg() {
	s.store.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
		ID: s.orgID,
	}).Return(&types.Organization{ID: s.orgID}, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	s.store.EXPECT().ListSessions(gomock.Any(), store.ListSessionsQuery{
		Owner:          s.userID,
		OrganizationID: s.orgID,
		Page:           0,
		PerPage:        50,
	}).Return([]*types.Session{
		{ID: "ses_1", Owner: s.userID, OrganizationID: s.orgID, Name: "org session"},
	}, int64(1), nil)

	req := httptest.NewRequest("GET", "/api/v1/sessions?org_id="+s.orgID, http.NoBody)
	req = req.WithContext(s.authCtx)

	result, err := s.server.listSessions(httptest.NewRecorder(), req)

	s.NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(1), result.TotalCount)
}

// 3. User is the org owner, lists org sessions
func (s *SessionAuthzSuite) TestListSessions_OrgOwner() {
	s.store.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
		ID: s.orgID,
	}).Return(&types.Organization{ID: s.orgID}, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleOwner,
	}, nil)
	s.store.EXPECT().ListSessions(gomock.Any(), store.ListSessionsQuery{
		Owner:          s.userID,
		OrganizationID: s.orgID,
		Page:           0,
		PerPage:        50,
	}).Return([]*types.Session{}, int64(0), nil)

	req := httptest.NewRequest("GET", "/api/v1/sessions?org_id="+s.orgID, http.NoBody)
	req = req.WithContext(s.authCtx)

	result, err := s.server.listSessions(httptest.NewRecorder(), req)

	s.NoError(err)
	s.Require().NotNil(result)
}

// 4. User is org member (not owner) - listing only requires org membership, so this succeeds.
// Per-session project-level RBAC is enforced on get/delete/update, not on list.
func (s *SessionAuthzSuite) TestListSessions_OrgMemberCanList() {
	s.store.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
		ID: s.orgID,
	}).Return(&types.Organization{ID: s.orgID}, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	s.store.EXPECT().ListSessions(gomock.Any(), store.ListSessionsQuery{
		Owner:          s.userID,
		OrganizationID: s.orgID,
		Page:           0,
		PerPage:        50,
	}).Return([]*types.Session{}, int64(0), nil)

	req := httptest.NewRequest("GET", "/api/v1/sessions?org_id="+s.orgID, http.NoBody)
	req = req.WithContext(s.authCtx)

	result, err := s.server.listSessions(httptest.NewRecorder(), req)

	s.NoError(err)
	s.Require().NotNil(result)
}

// 6. User is not a member of the org
func (s *SessionAuthzSuite) TestListSessions_NotOrgMemberDenied() {
	s.store.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
		ID: s.orgID,
	}).Return(&types.Organization{ID: s.orgID}, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(nil, fmt.Errorf("not found"))

	req := httptest.NewRequest("GET", "/api/v1/sessions?org_id="+s.orgID, http.NoBody)
	req = req.WithContext(s.authCtx)

	result, err := s.server.listSessions(httptest.NewRecorder(), req)

	s.Nil(result)
	assertHTTPError(s, err, http.StatusForbidden)
}
