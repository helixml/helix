package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestToolsSuite(t *testing.T) {
	suite.Run(t, new(ToolsTestSuite))
}

type ToolsTestSuite struct {
	suite.Suite

	store  *store.MockStore
	pubsub pubsub.PubSub

	authCtx context.Context
	userID  string

	server *HelixAPIServer
}

func (suite *ToolsTestSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.store = store.NewMockStore(ctrl)
	ps, err := pubsub.New()
	suite.NoError(err)

	suite.pubsub = ps

	suite.userID = "user_id"
	suite.authCtx = setRequestUser(context.Background(), types.UserData{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	janitor := janitor.NewJanitor(janitor.JanitorOptions{})

	suite.server = &HelixAPIServer{
		pubsub:  suite.pubsub,
		Store:   suite.store,
		Janitor: janitor,
		keyCloakMiddleware: &keyCloakMiddleware{
			store: suite.store,
		},
		Controller: &controller.Controller{
			Options: controller.ControllerOptions{
				Store:   suite.store,
				Janitor: janitor,
			},
		},
		adminAuth: &adminAuth{},
	}

	_, err = suite.server.registerRoutes(context.Background())
	suite.NoError(err)
}

func (suite *ToolsTestSuite) TestListTools() {
	tools := []*types.Tool{
		{
			ID:   "tool_1",
			Name: "tool_1_name",
		},
		{
			ID:   "tool_2",
			Name: "tool_2_name",
		},
	}

	suite.store.EXPECT().CheckAPIKey(gomock.Any(), "hl-API_KEY").Return(&types.ApiKey{
		Owner:     suite.userID,
		OwnerType: types.OwnerTypeUser,
	}, nil)

	suite.store.EXPECT().ListTools(gomock.Any(), &store.ListToolsQuery{
		Owner:     suite.userID,
		OwnerType: types.OwnerTypeUser,
	}).Return(tools, nil)

	req, err := http.NewRequest("GET", "/api/v1/tools", http.NoBody)
	suite.NoError(err)

	req.Header.Set("Authorization", "Bearer hl-API_KEY")

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.server.router.ServeHTTP(rec, req)

	suite.Require().Equal(http.StatusOK, rec.Code)

	var resp []*types.Tool
	suite.NoError(json.NewDecoder(rec.Body).Decode(&resp))
	suite.Equal(tools, resp)
}
