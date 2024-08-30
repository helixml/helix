package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
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
	ps, err := pubsub.New(suite.T().TempDir())
	suite.NoError(err)

	suite.pubsub = ps

	user := types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	}

	suite.userID = "user_id"
	suite.authCtx = setRequestUser(context.Background(), user)

	janitor := janitor.NewJanitor(config.Janitor{})

	suite.server = &HelixAPIServer{
		Cfg:     &config.ServerConfig{},
		pubsub:  suite.pubsub,
		Store:   suite.store,
		Janitor: janitor,
		authMiddleware: &authMiddleware{
			store:         suite.store,
			authenticator: auth.NewMockAuthenticator(&user),
		},
		Controller: &controller.Controller{
			ToolsPlanner: &tools.ChainStrategy{},
			Options: controller.ControllerOptions{
				Store:   suite.store,
				Janitor: janitor,
			},
		},
	}

	_, err = suite.server.registerRoutes(context.Background())
	suite.NoError(err)
}

func (suite *ToolsTestSuite) TestListTools() {
	userTools := []*types.Tool{
		{
			ID:   "tool_1",
			Name: "tool_1_name",
		},
		{
			ID:   "tool_2",
			Name: "tool_2_name",
		},
	}

	globalTools := []*types.Tool{}

	suite.store.EXPECT().GetAPIKey(gomock.Any(), "hl-API_KEY").Return(&types.APIKey{
		Owner:     suite.userID,
		OwnerType: types.OwnerTypeUser,
	}, nil)

	suite.store.EXPECT().ListTools(gomock.Any(), &store.ListToolsQuery{
		Owner:     suite.userID,
		OwnerType: types.OwnerTypeUser,
	}).Return(globalTools, nil)

	suite.store.EXPECT().ListTools(gomock.Any(), &store.ListToolsQuery{
		Global: true,
	}).Return(userTools, nil)

	req, err := http.NewRequest("GET", "/api/v1/tools", http.NoBody)
	suite.NoError(err)

	req.Header.Set("Authorization", "Bearer hl-API_KEY")

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.server.router.ServeHTTP(rec, req)

	suite.Require().Equal(http.StatusOK, rec.Code)

	var resp []*types.Tool
	suite.NoError(json.NewDecoder(rec.Body).Decode(&resp))
	suite.Equal(userTools, resp)
}

func (suite *ToolsTestSuite) TestCreateTool() {
	suite.store.EXPECT().GetAPIKey(gomock.Any(), "hl-API_KEY").Return(&types.APIKey{
		Owner:     suite.userID,
		OwnerType: types.OwnerTypeUser,
	}, nil)

	suite.store.EXPECT().ListTools(gomock.Any(), &store.ListToolsQuery{
		Owner:     suite.userID,
		OwnerType: types.OwnerTypeUser,
	}).Return([]*types.Tool{}, nil)

	suite.store.EXPECT().CreateTool(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, tool *types.Tool) (*types.Tool, error) {
			tool.ID = "tool_1"

			// Assert that the tool is valid
			suite.Equal("tool_1_name", tool.Name)
			suite.Equal("tool_1_description", tool.Description)
			suite.Equal(types.ToolTypeAPI, tool.ToolType)
			suite.Equal("http://example.com", tool.Config.API.URL)
			suite.Equal(petStoreApiSpec, string(tool.Config.API.Schema))

			return tool, nil
		})

	bts, err := json.Marshal(&types.Tool{
		Name:        "tool_1_name",
		Description: "tool_1_description",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolApiConfig{
				URL:    "http://example.com",
				Schema: base64.StdEncoding.EncodeToString([]byte(petStoreApiSpec)),
			},
		},
	})
	suite.NoError(err)

	req, err := http.NewRequest("POST", "/api/v1/tools", bytes.NewBuffer(bts))
	suite.NoError(err)

	req.Header.Set("Authorization", "Bearer hl-API_KEY")

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.server.router.ServeHTTP(rec, req)

	suite.Require().Equal(http.StatusOK, rec.Code)

	var resp *types.Tool
	suite.NoError(json.NewDecoder(rec.Body).Decode(&resp))

	suite.Equal("tool_1_name", resp.Name)
	suite.Equal("tool_1_description", resp.Description)

}

const petStoreApiSpec = `openapi: "3.0.0"
info:
  version: 1.0.0
  title: Swagger Petstore
  license:
    name: MIT
servers:
  - url: http://petstore.swagger.io/v1
paths:
  /pets:
    get:
      summary: List all pets
      operationId: listPets
      tags:
        - pets
      parameters:
        - name: limit
          in: query
          description: How many items to return at one time (max 100)
          required: false
          schema:
            type: integer
            maximum: 100
            format: int32
      responses:
        '200':
          description: A paged array of pets
          headers:
            x-next:
              description: A link to the next page of responses
              schema:
                type: string
          content:
            application/json:    
              schema:
                $ref: "#/components/schemas/Pets"
        default:
          description: unexpected error
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Error"
    post:
      summary: Create a pet
      operationId: createPets
      tags:
        - pets
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pet'
        required: true
      responses:
        '201':
          description: Null response
        default:
          description: unexpected error
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Error"
  /pets/{petId}:
    get:
      summary: Info for a specific pet
      operationId: showPetById
      tags:
        - pets
      parameters:
        - name: petId
          in: path
          required: true
          description: The id of the pet to retrieve
          schema:
            type: string
      responses:
        '200':
          description: Expected response to a valid request
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Pet"
        default:
          description: unexpected error
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Error"
components:
  schemas:
    Pet:
      type: object
      required:
        - id
        - name
      properties:
        id:
          type: integer
          format: int64
        name:
          type: string
        tag:
          type: string
    Pets:
      type: array
      maxItems: 100
      items:
        $ref: "#/components/schemas/Pet"
    Error:
      type: object
      required:
        - code
        - message
      properties:
        code:
          type: integer
          format: int32
        message:
          type: string
`
