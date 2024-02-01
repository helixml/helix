package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	golden "gotest.tools/v3/golden"
)

// TestAction_CallAPI tests query formation for a single API call to
// fetch a single record from the database
/* Spec:
openapi: "3.0.0"
info:
  version: 1.0.0
  title: Swagger Petstore
  license:
    name: MIT
servers:
  - url: http://petstore.swagger.io/v1
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
				description:
          type: string
*/
func (suite *ActionTestSuite) TestAction_getAPIRequestParameters_Path_SingleParam() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/pets/55443", r.URL.Path)
		suite.Equal("GET", r.Method)

		fmt.Fprintln(w, "{\"id\": 55443, \"name\": \"doggie\", \"tag\": \"dog\", \"description\": \"a brown dog\"}")
	}))
	defer ts.Close()

	getPetDetailsAPI := &types.Tool{
		Name:        "getPetDetail",
		Description: "pet store API that is used to get details for the specified pet's ID",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolApiConfig{
				URL:    ts.URL,
				Schema: miniPetStoreApiSpec,
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "Can you please give me the details for pet 55443?"

	resp, err := suite.strategy.getAPIRequestParameters(suite.ctx, getPetDetailsAPI, history, currentMessage, "showPetById")
	suite.NoError(err)

	spew.Dump(resp)

	suite.Require().Len(resp, 1, "expected to find a single parameter")
	suite.Equal(resp["petId"], "55443")
}

func (suite *ActionTestSuite) TestAction_getAPIRequestParameters_Query_SingleParam() {
	// TODO
}

func (suite *ActionTestSuite) TestAction_getAPIRequestParameters_Query_MultipleParams() {
	// TODO
}

func (suite *ActionTestSuite) TestAction_getAPIRequestParameters_Body_SingleItem() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/pets/55443", r.URL.Path)
		suite.Equal("GET", r.Method)

		fmt.Fprintln(w, "{\"id\": 55443, \"name\": \"doggie\", \"tag\": \"dog\", \"description\": \"a brown dog\"}")
	}))
	defer ts.Close()

	getPetDetailsAPI := &types.Tool{
		Name:        "getPetDetail",
		Description: "pet store API that is used to get details for the specified pet's ID",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolApiConfig{
				URL:    ts.URL,
				Schema: petStoreApiSpec,
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "Can you please give me the details for pet 55443?"

	resp, err := suite.strategy.getAPIRequestParameters(suite.ctx, getPetDetailsAPI, history, currentMessage, "showPetById")
	suite.NoError(err)

	spew.Dump(resp)
}

func (suite *ActionTestSuite) Test_prepareRequest_Path() {
	tool := &types.Tool{
		Name:        "getPetDetail",
		Description: "pet store API that is used to get details for the specified pet's ID",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolApiConfig{
				URL:    "https://example.com",
				Schema: petStoreApiSpec,
				Headers: map[string]string{
					"X-Api-Key": "1234567890",
				},
			},
		},
	}

	params := map[string]string{
		"petId": "99944",
	}

	req, err := suite.strategy.prepareRequest(suite.ctx, tool, "showPetById", params)
	suite.NoError(err)

	suite.Equal("https://example.com/pets/99944", req.URL.String())
	suite.Equal("GET", req.Method)
	suite.Equal("1234567890", req.Header.Get("X-Api-Key"))
}

func Test_getActionsFromSchema(t *testing.T) {
	actions, err := GetActionsFromSchema(petStoreApiSpec)
	require.NoError(t, err)
	require.Len(t, actions, 3)

	assert.Contains(t, actions, &types.ToolApiAction{
		Name:        "listPets",
		Description: "List all pets",
		Method:      "GET",
		Path:        "/pets",
	})

	assert.Contains(t, actions, &types.ToolApiAction{
		Name:        "createPets",
		Description: "Create a pet",
		Method:      "POST",
		Path:        "/pets",
	})

	assert.Contains(t, actions, &types.ToolApiAction{
		Name:        "showPetById",
		Description: "Info for a specific pet",
		Method:      "GET",
		Path:        "/pets/{petId}",
	})
}

func Test_filterOpenAPISchema_GetBody(t *testing.T) {
	filtered, err := filterOpenAPISchema(&types.Tool{
		Config: types.ToolConfig{
			API: &types.ToolApiConfig{
				Schema: petStoreApiSpec,
			},
		},
	}, "showPetById")
	require.NoError(t, err)

	golden.Assert(t, filtered, "filtered-one-pet.golden.json")
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

const miniPetStoreApiSpec = `openapi: "3.0.0"
info:
  version: 1.0.0
  title: Swagger Petstore
  license:
    name: MIT
servers:
  - url: http://petstore.swagger.io/v1
paths:  
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
