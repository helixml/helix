package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/types"
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
func (suite *ActionTestSuite) TestAction_getAPIRequestParameters_Path_SingleItem() {
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
				URL: ts.URL + "/pets/{petId}",
				Parameters: []*types.APIParameter{
					{
						Name:        "petId",
						Description: "The id of the pet to retrieve",
						AutoFill:    true,
					},
				},
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "Can you please give me the details for pet 55443?"

	resp, err := suite.strategy.getAPIRequestParameters(suite.ctx, getPetDetailsAPI, history, currentMessage)
	suite.NoError(err)

	spew.Dump(resp)

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
				URL: ts.URL + "/pets/{petId}",
				Parameters: []*types.APIParameter{
					{
						Name:        "petId",
						Description: "The id of the pet to retrieve",
						AutoFill:    true,
					},
				},
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "Can you please give me the details for pet 55443?"

	resp, err := suite.strategy.getAPIRequestParameters(suite.ctx, getPetDetailsAPI, history, currentMessage)
	suite.NoError(err)

	spew.Dump(resp)

}
