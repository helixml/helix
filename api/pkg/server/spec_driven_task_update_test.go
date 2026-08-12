package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type SpecTaskUpdateSuite struct {
	suite.Suite

	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestSpecTaskUpdateSuite(t *testing.T) {
	suite.Run(t, new(SpecTaskUpdateSuite))
}

func (s *SpecTaskUpdateSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.store,
	}
}

func (s *SpecTaskUpdateSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SpecTaskUpdateSuite) TestExplicitNameWinsOverDescriptionDerivedName() {
	const (
		userID    = "user_update_test"
		projectID = "project_update_test"
		taskID    = "task_update_test"
	)

	task := &types.SpecTask{
		ID:          taskID,
		ProjectID:   projectID,
		Name:        "Old name",
		Description: "Old description",
	}
	project := &types.Project{ID: projectID, UserID: userID}

	s.store.EXPECT().GetSpecTask(gomock.Any(), taskID).Return(task, nil)
	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, updated *types.SpecTask) error {
			s.Equal("Harbor registry verification", updated.Name)
			s.Equal("Verify Harbor updates in k3s", updated.Description)
			s.Equal("Harbor registry verification", updated.UserShortTitle)
			return nil
		},
	)

	requestBody, err := json.Marshal(types.SpecTaskUpdateRequest{
		Name:           "Harbor registry verification",
		Description:    "Verify Harbor updates in k3s",
		UserShortTitle: stringPointer("Harbor registry verification"),
	})
	s.Require().NoError(err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/"+taskID, bytes.NewReader(requestBody))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))
	req = mux.SetURLVars(req, map[string]string{"taskId": taskID})
	rr := httptest.NewRecorder()

	s.server.updateSpecTask(rr, req)

	s.Equal(http.StatusOK, rr.Code)
}

func (s *SpecTaskUpdateSuite) TestDescriptionUpdatePreservesTaskName() {
	const (
		userID    = "user_description_update_test"
		projectID = "project_description_update_test"
		taskID    = "task_description_update_test"
	)

	task := &types.SpecTask{
		ID:             taskID,
		ProjectID:      projectID,
		Name:           "Custom task name",
		UserShortTitle: "Custom task name",
		Description:    "Old description",
	}
	project := &types.Project{ID: projectID, UserID: userID}

	s.store.EXPECT().GetSpecTask(gomock.Any(), taskID).Return(task, nil)
	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, updated *types.SpecTask) error {
			s.Equal("Custom task name", updated.Name)
			s.Equal("Custom task name", updated.UserShortTitle)
			s.Equal("New description", updated.Description)
			return nil
		},
	)

	requestBody, err := json.Marshal(types.SpecTaskUpdateRequest{
		Description: "New description",
	})
	s.Require().NoError(err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/"+taskID, bytes.NewReader(requestBody))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))
	req = mux.SetURLVars(req, map[string]string{"taskId": taskID})
	rr := httptest.NewRecorder()

	s.server.updateSpecTask(rr, req)

	s.Equal(http.StatusOK, rr.Code)
}

func stringPointer(value string) *string {
	return &value
}
