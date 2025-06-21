package skill

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitializeApiCallingSkill(t *testing.T) {
	petStoreTool := &types.Tool{
		Name:         "petstore",
		Description:  "pet store API that is used to get details for the specified pet's ID",
		SystemPrompt: "You are an expert in the pet store API. You can use it to get details for the specified pet's ID",
		ToolType:     types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    "https://petstore.swagger.io/v2",
				Schema: petStoreAPISpec,
				Actions: []*types.ToolAPIAction{
					{
						Name:        "listPets",
						Description: "List all pets",
						Method:      "GET",
						Path:        "/pets",
					},
					{
						Name:        "createPets",
						Description: "Create a pet record",
						Method:      "POST",
						Path:        "/pets",
					},
					{
						Name:        "showPetById",
						Description: "Info for a specific pet",
						Method:      "GET",
						Path:        "/pets/{petId}",
					},
				},
			},
		},
	}

	skill := NewAPICallingSkill(nil, petStoreTool)
	assert.NotNil(t, skill)

	assert.Equal(t, petStoreTool.Name, skill.Name)
	assert.Equal(t, petStoreTool.Description, skill.Description)
	assert.Equal(t, petStoreTool.SystemPrompt, skill.SystemPrompt)

	// We should have 3 tools
	t.Run("ToolCount", func(t *testing.T) {
		assert.Equal(t, 3, len(skill.Tools))
	})

	t.Run("ToolNamesAndDescriptions", func(t *testing.T) {
		assert.Equal(t, "listPets", skill.Tools[0].Name())
		assert.Equal(t, "createPets", skill.Tools[1].Name())
		assert.Equal(t, "showPetById", skill.Tools[2].Name())

		assert.Equal(t, "List all pets", skill.Tools[0].Description())
		assert.Equal(t, "Create a pet record", skill.Tools[1].Description())
		assert.Equal(t, "Info for a specific pet", skill.Tools[2].Description())
	})

	t.Run("ListPetsParameters", func(t *testing.T) {
		openAiSpec := skill.Tools[0].OpenAI()

		parameters := openAiSpec[0].Function.Parameters.(jsonschema.Definition)

		limitProperty, ok := parameters.Properties["limit"]
		require.True(t, ok)

		// Check description and type
		assert.Equal(t, "How many items to return at one time (max 100)", limitProperty.Description)
		assert.Equal(t, jsonschema.Integer, limitProperty.Type)
	})

	t.Run("CreatePetsParameters", func(t *testing.T) {
		openAiSpec := skill.Tools[1].OpenAI()

		parameters := openAiSpec[0].Function.Parameters.(jsonschema.Definition)

		petProperty, ok := parameters.Properties["body"]
		require.True(t, ok)

		assert.Equal(t, "Request body", petProperty.Description)

		assert.Equal(t, jsonschema.Integer, petProperty.Properties["id"].Type)
		assert.Equal(t, jsonschema.String, petProperty.Properties["name"].Type)
		assert.Equal(t, jsonschema.String, petProperty.Properties["tag"].Type)

		// Check required fields
		assert.Equal(t, []string{"id", "name"}, petProperty.Required)

		// assert.Equal(t, "The name of the pet", petProperty.Description)
		// assert.Equal(t, jsonschema.String, petProperty.Type)
	})
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

func Test_CurrencyExchangeRates(t *testing.T) {
	currencyExchangeRatesTool := &types.Tool{
		Name:         "currencyExchangeRates",
		Description:  "Get latest currency exchange rates",
		SystemPrompt: "You are an expert in the currency exchange rates API. You can use it to get the latest currency exchange rates",
		ToolType:     types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    "https://open.er-api.com/v6",
				Schema: currencyExchangeRatesAPISpec,
				Actions: []*types.ToolAPIAction{
					{
						Name:        "getExchangeRates",
						Description: "Get latest currency exchange rates",
						Method:      "GET",
						Path:        "/latest/{currency}",
					},
				},
			},
		},
	}

	skill := NewAPICallingSkill(nil, currencyExchangeRatesTool)
	assert.NotNil(t, skill)

	tool := skill.Tools[0].OpenAI()

	bts, err := json.MarshalIndent(tool, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(string(bts))

	t.Error("aa")
}

const currencyExchangeRatesAPISpec = `openapi: 3.0.0
info:
  title: Exchange Rates API
  description: Get latest currency exchange rates
  version: "1.0.0"
servers:
  - url: https://open.er-api.com/v6
paths:
  /latest/{currency}:
    get:
      operationId: getExchangeRates
      summary: Get latest exchange rates
      description: Get current exchange rates for a base currency
      parameters:
        - name: currency
          in: path
          required: true
          description: Base currency code (e.g., USD, EUR, GBP)
          schema:
            type: string
      responses:
        '200':
          description: Successful response with exchange rates
          content:
            application/json:
              schema:
                type: object
                properties:
                  result:
                    type: string
                    example: "success"
                  provider:
                    type: string
                    example: "Open Exchange Rates"
                  base_code:
                    type: string
                    example: "USD"
                  time_last_update_utc:
                    type: string
                    example: "2024-01-19 00:00:01"
                  rates:
                    type: object
                    properties:
                      EUR:
                        type: number
                        example: 0.91815
                      GBP:
                        type: number
                        example: 0.78543
                      JPY:
                        type: number
                        example: 148.192
                      AUD:
                        type: number
                        example: 1.51234
                      CAD:
                        type: number
                        example: 1.34521`
