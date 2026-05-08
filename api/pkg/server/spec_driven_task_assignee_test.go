package server

import (
	"bytes"
	"context"
	"encoding/json"
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

type SpecTaskAssigneeSuite struct {
	suite.Suite

	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestSpecTaskAssigneeSuite(t *testing.T) {
	suite.Run(t, new(SpecTaskAssigneeSuite))
}

func (s *SpecTaskAssigneeSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.store,
	}
}

func (s *SpecTaskAssigneeSuite) TearDownTest() {
	s.ctrl.Finish()
}

// validateAssigneeIsOrgMember should short-circuit on empty assignee
// (unassigned is a valid state on both create and update paths).
func (s *SpecTaskAssigneeSuite) TestValidateAssignee_Empty_IsAlwaysValid() {
	err := s.server.validateAssigneeIsOrgMember(context.Background(), "org1", "")
	s.NoError(err)
}

func (s *SpecTaskAssigneeSuite) TestValidateAssignee_Member_IsValid() {
	s.store.EXPECT().
		GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: "org1",
			UserID:         "user_member",
		}).
		Return(&types.OrganizationMembership{OrganizationID: "org1", UserID: "user_member"}, nil)

	err := s.server.validateAssigneeIsOrgMember(context.Background(), "org1", "user_member")
	s.NoError(err)
}

func (s *SpecTaskAssigneeSuite) TestValidateAssignee_NonMember_ReturnsError() {
	s.store.EXPECT().
		GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: "org1",
			UserID:         "user_outsider",
		}).
		Return(nil, errors.New("not found"))

	err := s.server.validateAssigneeIsOrgMember(context.Background(), "org1", "user_outsider")
	s.Error(err)
}

// createTaskFromPrompt returns 400 when the requested assignee isn't a member
// of the project's organization. This is the new validation gate added so the
// kanban filter can't silently hide a freshly-created task with a bad assignee.
func (s *SpecTaskAssigneeSuite) TestCreateTaskFromPrompt_NonMemberAssignee_Returns400() {
	const (
		ownerID  = "user_owner"
		orgID    = "org1"
		projID   = "proj_with_assignee_test"
		nonMember = "user_outsider"
	)

	project := &types.Project{
		ID:             projID,
		OrganizationID: orgID,
		UserID:         ownerID,
	}

	// authorizeUserToProjectByID loads the project, then authorizeUserToProject
	// calls authorizeOrgMember which queries org membership for the *caller*.
	s.store.EXPECT().
		GetProject(gomock.Any(), projID).
		Return(project, nil)
	s.store.EXPECT().
		GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         ownerID,
		}).
		Return(&types.OrganizationMembership{OrganizationID: orgID, UserID: ownerID, Role: types.OrganizationRoleMember}, nil)

	// The validation block in createTaskFromPrompt loads the project a second
	// time to read OrganizationID, then queries membership for the *assignee*.
	s.store.EXPECT().
		GetProject(gomock.Any(), projID).
		Return(project, nil)
	s.store.EXPECT().
		GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         nonMember,
		}).
		Return(nil, errors.New("not found"))

	body, err := json.Marshal(types.CreateTaskRequest{
		ProjectID:  projID,
		Prompt:     "do the thing",
		AssigneeID: nonMember,
	})
	s.Require().NoError(err)

	req := httptest.NewRequest("POST", "/api/v1/spec-tasks/from-prompt", bytes.NewReader(body))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	s.server.createTaskFromPrompt(rr, req)

	s.Equal(http.StatusBadRequest, rr.Code)
	s.Contains(rr.Body.String(), "assignee must be an organization member")
}
