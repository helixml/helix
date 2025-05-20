package skill

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
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

	skill := NewApiCallingSkill(nil, petStoreTool)
	assert.NotNil(t, skill)

	assert.Equal(t, petStoreTool.Name, skill.Name)
	assert.Equal(t, petStoreTool.Description, skill.Description)
	assert.Equal(t, petStoreTool.SystemPrompt, skill.SystemPrompt)

	// We should have 3 tools
	assert.Equal(t, 3, len(skill.Tools))

	// Check the tool names
	assert.Equal(t, "listPets", skill.Tools[0].Name())
	assert.Equal(t, "createPets", skill.Tools[1].Name())
	assert.Equal(t, "showPetById", skill.Tools[2].Name())

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
