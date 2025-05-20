package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	agentpod "github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/agent/skill"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/gptscript"
	helix_openai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"go.uber.org/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Function to create memory with user preferences
func getEmptyPreferencesMemory(meta *agentpod.Meta) (*agentpod.MemoryBlock, error) {
	memoryBlock := agentpod.NewMemoryBlock()
	userDetailsBlock := agentpod.NewMemoryBlock()
	// userDetailsBlock.AddString("location", "Downtown")
	// userDetailsBlock.AddString("favorite_cuisines", "Italian")
	memoryBlock.AddBlock("UserDetails", userDetailsBlock)
	return memoryBlock, nil
}

const petStoreMainPrompt = `You are a pet store managing system expert tasked with helping users find the perfect pet based on their location and breed preferences. Provide concise and direct recommendations using the available data from authorized tools.

- Focus on the user's requests on what to do.
- When asked to add a new pet, use the available tools to add a new pet.
- Avoid making assumptions about pets that are not readily available through your tools.
- Clearly communicate the recommendation and justify the choice with relevant details that enhance the user's decision-making process.

(Note: Ensure all relevant data is provided and realistic for actual recommendations.)`

type Pet struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

func TestMultiAPICallingPetStoreManagement(t *testing.T) {
	prompt := "check if pet with name Lizzie is in the store system, if not add her to the system. She's a cat"
	testPetStoreManagement(t, prompt)
}

func testPetStoreManagement(t *testing.T, prompt string) {
	t.Helper()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var cfg config.ServerConfig
	err := envconfig.Process("", &cfg)
	require.NoError(t, err)

	store := store.NewMockStore(ctrl)
	executor := gptscript.NewMockExecutor(ctrl)

	config, err := LoadConfig()
	require.NoError(t, err)

	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL)

	planner, err := tools.NewChainStrategy(&cfg, store, executor, client)
	require.NoError(t, err)

	petsListCalled := false
	petsCreateCalled := false

	pets := []Pet{
		{ID: 1, Name: "Buddy", Tag: "dog"},
		{ID: 2, Name: "Fuffy", Tag: "dog"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request: %s %s", r.Method, r.URL.Path)

		if r.URL.Path != "/pets" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			return
		}

		if r.Method == "GET" {
			petsListCalled = true

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(pets)

			return
		}

		if r.Method == "POST" {
			petsCreateCalled = true

			// Read the request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			defer r.Body.Close()

			t.Logf("request body: %s", string(body))

			assert.Contains(t, string(body), `"name": "Lizzie"`)
			assert.Contains(t, string(body), `"tag": "cat"`)

			// Add the pet to the list
			pets = append(pets, Pet{ID: 3, Name: "Lizzie", Tag: "cat"})

			w.WriteHeader(http.StatusCreated)
			// No response body

			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()

	t.Logf("testing pet store management with prompt: %s", prompt)

	require.NotEmpty(t, config.OpenAIAPIKey, "OpenAI API Key is not set")

	llm := agentpod.NewLLM(
		client,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)

	// Create mock memory with user preferences
	mem := &MockMemory{
		RetrieveFn: getEmptyPreferencesMemory,
	}

	petStoreTool := &types.Tool{
		Name:         "petstore",
		Description:  "pet store API that is used to get details for the specified pet's ID",
		SystemPrompt: "You are an expert in the pet store, managing it through the API. You can use it to get information about pets or add new ones.",
		ToolType:     types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    ts.URL,
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

	petStoreSkill := skill.NewAPICallingSkill(planner, petStoreTool)

	// var skills []agentpod.Skill

	// for _, tool := range apiCallingTools {
	// 	skills = append(skills, agentpod.Skill{
	// 		Name:         tool.Name(),
	// 		Description:  tool.Description(),
	// 		SystemPrompt: fmt.Sprintf("You are an expert in the %s tool. You can use it to get information about pets.", tool.Name()),
	// 		Tools:        []agentpod.Tool{tool},
	// 	})
	// }
	restaurantAgent := agentpod.NewAgent(
		petStoreMainPrompt,
		[]agentpod.Skill{petStoreSkill},
	)

	// Create a mock storage with empty conversation history
	storage := &MockStorage{}
	storage.ConversationFn = getEmptyConversationHistory(storage)

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	// Create session with restaurant agent
	restaurantSession := agentpod.NewSession(context.Background(), llm, mem, restaurantAgent, storage, agentpod.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID, "domain": "test"},
	})

	restaurantSession.In(prompt)
	var response string
	for {
		out := restaurantSession.Out()

		if out.Type == agentpod.ResponseTypePartialText {
			response += out.Content
		}
		if out.Type == agentpod.ResponseTypeEnd {
			break
		}
	}

	t.Logf("agent response: %s", response)

	// Verify CreateConversation was called with the correct messages
	if !storage.WasCreateConversationCalled() {
		t.Fatal("Expected CreateConversation to be called")
	}
	if storage.GetUserMessage(sessionID) != prompt {
		t.Fatalf("Expected user message to match, got: %s", storage.GetUserMessage(sessionID))
	}

	require.True(t, petsListCalled, "expected to call listPets")
	require.True(t, petsCreateCalled, "expected to call createPets")
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
