package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	oai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	golden "gotest.tools/v3/golden"
)

// TestAction_CallAPI tests query formation for a single API call to
// fetch a single record from the database
/* Spec:
# Taken from https://github.com/OAI/OpenAPI-Specification/blob/main/examples/v3.0/petstore.yaml

openapi: "3.0.0"
info:
  version: 1.0.0
  title: Swagger Petstore
  license:
    name: MIT
servers:
  - url: https://petstore.swagger.io/v1
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
			API: &types.ToolAPIConfig{
				URL:    ts.URL,
				Schema: miniPetStoreAPISpec,
			},
		},
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    oai.ChatMessageRoleUser,
			Content: "Can you please give me the details for pet 55443?",
		},
	}

	// suite.store.EXPECT().CreateLLMCall(gomock.Any(), gomock.Any()).DoAndReturn(
	// 	func(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	// 		suite.Equal("session-123", call.SessionID)
	// 		suite.Equal(types.LLMCallStepPrepareAPIRequest, call.Step)

	// 		return call, nil
	// 	})

	resp, err := suite.strategy.getAPIRequestParameters(suite.ctx, suite.strategy.apiClient, "session-123", "i-123", getPetDetailsAPI, history, "showPetById")
	suite.NoError(err)

	suite.strategy.wg.Wait()

	suite.Require().Len(resp, 1, "expected to find a single parameter")
	suite.Equal(resp["petId"], "55443")
}

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
			API: &types.ToolAPIConfig{
				URL:    ts.URL,
				Schema: petStoreAPISpec,
			},
		},
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    oai.ChatMessageRoleUser,
			Content: "Can you please give me the details for pet 55443?",
		},
	}

	resp, err := suite.strategy.getAPIRequestParameters(suite.ctx, suite.strategy.apiClient, "session-123", "i-123", getPetDetailsAPI, history, "showPetById")
	suite.NoError(err)

	suite.strategy.wg.Wait()

	spew.Dump(resp)
}

func (suite *ActionTestSuite) Test_prepareRequest_Path() {
	tool := &types.Tool{
		Name:        "getPetDetail",
		Description: "pet store API that is used to get details for the specified pet's ID",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    "https://example.com",
				Schema: petStoreAPISpec,
				Headers: map[string]string{
					"X-Api-Key": "1234567890",
				},
			},
		},
	}

	params := map[string]interface{}{
		"petId": "99944",
	}

	req, err := suite.strategy.prepareRequest(suite.ctx, tool, "showPetById", params)
	suite.NoError(err)

	suite.Equal("https://example.com/pets/99944", req.URL.String())
	suite.Equal("GET", req.Method)
	suite.Equal("1234567890", req.Header.Get("X-Api-Key"))
}

type Pet struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Tag         string `json:"tag"`
}

func (suite *ActionTestSuite) Test_prepareRequest_Body() {
	tool := &types.Tool{
		Name:        "managePetsApi",
		Description: "pet store API that is used to manage pets",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    "https://example.com",
				Schema: petStoreAPISpec,
				Headers: map[string]string{
					"X-Api-Key": "1234567890",
				},
			},
		},
	}

	params := map[string]interface{}{
		"name":        "doggie",
		"description": "a brown dog",
		"tag":         "dog",
	}

	req, err := suite.strategy.prepareRequest(suite.ctx, tool, "createPets", params)
	suite.NoError(err)

	suite.Equal("https://example.com/pets", req.URL.String())
	suite.Equal("POST", req.Method)
	suite.Equal("1234567890", req.Header.Get("X-Api-Key"))

	body, err := io.ReadAll(req.Body)
	suite.NoError(err)

	var pet Pet
	err = json.Unmarshal(body, &pet)
	suite.NoError(err)

	suite.Equal("doggie", pet.Name)
	suite.Equal("dog", pet.Tag)

	suite.Equal("", pet.Description, "while we do set this, we don't have it in the API schema hence it should not be visible")
}

func (suite *ActionTestSuite) Test_prepareRequest_Body_Nested() {
	tool := &types.Tool{
		Name:        "managePetsApi",
		Description: "pet store API that is used to manage pets",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    "https://example.com",
				Schema: petStoreAPISpec,
				Headers: map[string]string{
					"X-Api-Key": "1234567890",
				},
			},
		},
	}

	params := map[string]interface{}{
		"name":        "doggie",
		"description": "a brown dog",
		"tag":         "dog",
	}

	req, err := suite.strategy.prepareRequest(suite.ctx, tool, "createPets", params)
	suite.NoError(err)

	suite.Equal("https://example.com/pets", req.URL.String())
	suite.Equal("POST", req.Method)
	suite.Equal("1234567890", req.Header.Get("X-Api-Key"))

	body, err := io.ReadAll(req.Body)
	suite.NoError(err)

	var pet Pet
	err = json.Unmarshal(body, &pet)
	suite.NoError(err)

	suite.Equal("doggie", pet.Name)
	suite.Equal("dog", pet.Tag)

	suite.Equal("", pet.Description, "while we do set this, we don't have it in the API schema hence it should not be visible")
}

func (suite *ActionTestSuite) Test_prepareRequest_Path_ProvidedQuery() {
	tool := &types.Tool{
		Name:        "getPetDetail",
		Description: "pet store API that is used to get details for the specified pet's ID",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    "https://example.com",
				Schema: petStoreAPISpec,
				Headers: map[string]string{
					"X-Api-Key": "1234567890",
				},
				Query: map[string]string{
					"appid": "app123",
				},
			},
		},
	}

	params := map[string]interface{}{
		"petId": "99944",
	}

	req, err := suite.strategy.prepareRequest(suite.ctx, tool, "showPetById", params)
	suite.NoError(err)

	suite.Equal("https://example.com/pets/99944?appid=app123", req.URL.String())
	suite.Equal("GET", req.Method)
	suite.Equal("1234567890", req.Header.Get("X-Api-Key"))
}

func (suite *ActionTestSuite) Test_prepareRequest_Query() {
	weatherSpec, err := os.ReadFile("./testdata/weather.yaml")
	suite.NoError(err)

	tool := &types.Tool{
		Name:        "getWeather",
		Description: "What's the weather in London?",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    "https://api.openweathermap.org/data/2.5",
				Schema: string(weatherSpec),
				Headers: map[string]string{
					"X-Api-Key": "1234567890",
				},
			},
		},
	}

	params := map[string]interface{}{
		"q": "London",
	}

	req, err := suite.strategy.prepareRequest(suite.ctx, tool, "CurrentWeatherData", params)
	suite.NoError(err)

	suite.Equal("https://api.openweathermap.org/data/2.5/weather?q=London", req.URL.String())
	suite.Equal("GET", req.Method)
	suite.Equal("1234567890", req.Header.Get("X-Api-Key"))
}

func (suite *ActionTestSuite) TestAction_getAPIRequestParameters_Query_MultipleParams() {
	// TODO
}

func (suite *ActionTestSuite) TestAction_CustomRequestPrompt() {
	defer suite.ctrl.Finish()

	apiClient := openai.NewMockClient(suite.ctrl)
	suite.strategy.apiClient = apiClient

	tool := &types.Tool{
		Name:     "productsAPI",
		ToolType: types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:                 "https://example.com",
				Schema:              petStoreAPISpec,
				RequestPrepTemplate: `CUSTOM_TEMPLATE_HERE`,
				Actions: []*types.ToolAPIAction{
					{
						Name:        "getProductDetails",
						Description: "database API that can be used to query product information in the database",
					},
				},
			},
		},
	}

	chatReq, err := suite.strategy.getAPISystemPrompt(tool, "getProductDetails")
	suite.Require().NoError(err)

	suite.Contains(chatReq.Content, "CUSTOM_TEMPLATE_HERE")

	suite.strategy.wg.Wait()

}

func Test_getActionsFromSchema(t *testing.T) {
	actions, err := GetActionsFromSchema(petStoreAPISpec)
	require.NoError(t, err)
	require.Len(t, actions, 3)

	assert.Contains(t, actions, &types.ToolAPIAction{
		Name:        "listPets",
		Description: "List all pets",
		Method:      "GET",
		Path:        "/pets",
	})

	assert.Contains(t, actions, &types.ToolAPIAction{
		Name:        "createPets",
		Description: "Create a pet",
		Method:      "POST",
		Path:        "/pets",
	})

	assert.Contains(t, actions, &types.ToolAPIAction{
		Name:        "showPetById",
		Description: "Info for a specific pet",
		Method:      "GET",
		Path:        "/pets/{petId}",
	})
}

func Test_filterOpenAPISchema_GetBody(t *testing.T) {
	filtered, err := filterOpenAPISchema(&types.Tool{
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				Schema: petStoreAPISpec,
			},
		},
	}, "showPetById", make(map[string]interface{}))
	require.NoError(t, err)

	golden.Assert(t, filtered, "filtered-one-pet.golden.json")
}

const petStoreAPISpec = `openapi: "3.0.0"
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

const miniPetStoreAPISpec = `openapi: "3.0.0"
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

func Test_unmarshalParams(t *testing.T) {
	type args struct {
		data string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]interface{}
		wantErr bool
	}{
		{
			name: "int",
			args: args{
				data: `{"id": 1000}`,
			},
			want: map[string]interface{}{
				"id": float64(1000),
			},
		},
		{
			name: "string",
			args: args{
				data: `{"id": "1000"}`,
			},
			want: map[string]interface{}{
				"id": "1000",
			},
		},
		{
			name: "float",
			args: args{
				data: `{"id": 1005.0}`,
			},
			want: map[string]interface{}{
				"id": float64(1005),
			},
		},
		{
			name: "float_2",
			args: args{
				data: `{"id": 1005.5}`,
			},
			want: map[string]interface{}{
				"id": 1005.5,
			},
		},
		{
			name: "bool",
			args: args{
				data: `{"yes": true}`,
			},
			want: map[string]interface{}{
				"yes": true,
			},
		},
		{
			name: "``` in json",
			args: args{
				data: "```json{\"id\": 1000}```",
			},
			want: map[string]interface{}{
				"id": float64(1000),
			},
		},
		{
			name: "``` in json",
			args: args{
				data: "```json{\"id\": 1000}```blah blah blah I am very smart LLM",
			},
			want: map[string]interface{}{
				"id": float64(1000),
			},
		},
		{
			name: "``` in json variant",
			args: args{
				data: "```\n{\"id\": 1000}```blah blah blah I am very stupid LLM that cannot follow instructions about backticks",
			},
			want: map[string]interface{}{
				"id": float64(1000),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unmarshalParams(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("unmarshalParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unmarshalParams() = %v, want %v", got, tt.want)
				spew.Dump(got)
				spew.Dump(tt.want)
			}
		})
	}
}

func Test_GetParametersFromSchema(t *testing.T) {
	t.Run("showPetById", func(t *testing.T) {
		params, err := GetParametersFromSchema(petStoreAPISpec, "showPetById")
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.Equal(t, "petId", params[0].Name)
		require.Equal(t, "The id of the pet to retrieve", params[0].Description)
		require.Equal(t, ParameterTypeString, params[0].Type)
	})

	t.Run("listPets", func(t *testing.T) {
		params, err := GetParametersFromSchema(petStoreAPISpec, "listPets")
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.Equal(t, "limit", params[0].Name)
		require.Equal(t, "How many items to return at one time (max 100)", params[0].Description)
		require.Equal(t, ParameterTypeInteger, params[0].Type)
	})

	t.Run("createPets", func(t *testing.T) {
		params, err := GetParametersFromSchema(petStoreAPISpec, "createPets")
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.Equal(t, "body", params[0].Name)
		require.Equal(t, ParameterTypeObject, params[0].Type)
		require.True(t, params[0].Required)
		require.NotNil(t, params[0].Schema)

		// Verify schema properties
		schema := params[0].Schema.Value
		require.NotNil(t, schema)
		require.Contains(t, schema.Required, "id")
		require.Contains(t, schema.Required, "name")

		// Verify schema properties
		require.Contains(t, schema.Properties, "id")
		require.Contains(t, schema.Properties, "name")
		require.Contains(t, schema.Properties, "tag")

		// Verify property types
		require.Equal(t, "integer", schema.Properties["id"].Value.Type.Slice()[0])
		require.Equal(t, "string", schema.Properties["name"].Value.Type.Slice()[0])
		require.Equal(t, "string", schema.Properties["tag"].Value.Type.Slice()[0])
	})
}

// Test_filterOpenAPISchema_CloudId tests that cloudId parameter is properly filtered
func Test_filterOpenAPISchema_CloudId(t *testing.T) {
	// Simple OpenAPI schema with cloudId parameter
	schema := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0"},
		"paths": {
			"/test/{cloudId}/endpoint": {
				"get": {
					"operationId": "testOperation",
					"parameters": [
						{
							"name": "cloudId",
							"in": "path",
							"required": true,
							"schema": {"type": "string"}
						},
						{
							"name": "otherParam",
							"in": "query",
							"schema": {"type": "string"}
						}
					]
				}
			}
		}
	}`

	// Create tool with the schema
	tool := &types.Tool{
		Name: "test-tool",
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				Schema: schema,
			},
		},
	}

	// Test with cloudId pre-configured
	preConfiguredParams := map[string]interface{}{
		"cloudId": "test-cloud-id-123",
	}

	filtered, err := filterOpenAPISchema(tool, "testOperation", preConfiguredParams)
	require.NoError(t, err)

	t.Logf("Filtered schema: %s", filtered)

	// Check that cloudId parameter is NOT present in filtered schema
	require.False(t, strings.Contains(filtered, `"name": "cloudId"`), "cloudId parameter should be filtered out")

	// Check that otherParam is still present
	require.True(t, strings.Contains(filtered, `"name": "otherParam"`), "otherParam should still be present")

	// Check that path has been substituted
	require.True(t, strings.Contains(filtered, "/test/test-cloud-id-123/endpoint"), "path should have cloudId substituted")
}
