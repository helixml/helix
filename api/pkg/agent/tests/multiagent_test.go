package tests

import (
	"context"
	"strings"
	"testing"

	agentpod "github.com/helixml/helix/api/pkg/agent"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/stretchr/testify/require"
)

// RestaurantTool implements the Tool interface for restaurant recommendations
type RestaurantTool struct {
	toolName    string
	description string
	restaurants map[string]Restaurant
}

type Restaurant struct {
	Name     string
	Cuisine  string
	Location string
}

var _ agentpod.Tool = &RestaurantTool{}

func NewRestaurantTool() *RestaurantTool {
	return &RestaurantTool{
		toolName:    "RestaurantDatabase",
		description: "Provides information about restaurants in a specific location",
		restaurants: map[string]Restaurant{
			"Pasta Paradise": {
				Name:     "Pasta Paradise",
				Cuisine:  "Italian",
				Location: "Downtown",
			},
			"Sushi Master": {
				Name:     "Sushi Master",
				Cuisine:  "Japanese",
				Location: "Uptown",
			},
			"Taco Fiesta": {
				Name:     "Taco Fiesta",
				Cuisine:  "Mexican",
				Location: "Midtown",
			},
		},
	}
}

func (r *RestaurantTool) Name() string {
	return r.toolName
}

func (r *RestaurantTool) String() string {
	return r.toolName
}

func (r *RestaurantTool) Description() string {
	return r.description
}

func (r *RestaurantTool) StatusMessage() string {
	return "Finding the perfect restaurant for you"
}

func (r *RestaurantTool) OpenAI() []openai.ChatCompletionToolParam {
	return []openai.ChatCompletionToolParam{
		{
			Function: openai.FunctionDefinitionParam{
				Name:        r.toolName,
				Description: param.Opt[string]{Value: r.description},
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{
							"type":        "string",
							"description": "User's location",
						},
						"cuisine": map[string]interface{}{
							"type":        "string",
							"description": "Preferred cuisine",
						},
					},
					"required": []string{"location", "cuisine"},
				},
			},
		},
	}
}

func (r *RestaurantTool) Execute(ctx context.Context, meta agentpod.Meta, args map[string]interface{}) (string, error) {
	location := args["location"].(string)
	cuisine := args["cuisine"].(string)

	for _, restaurant := range r.restaurants {
		if restaurant.Location == location && restaurant.Cuisine == cuisine {
			return restaurant.Name, nil
		}
	}
	return "No matching restaurant found", nil
}

// CuisineTool implements the Tool interface for cuisine recommendations
type CuisineTool struct {
	toolName    string
	description string
	dishes      map[string][]string
}

var _ agentpod.Tool = &CuisineTool{}

func NewCuisineTool() *CuisineTool {
	return &CuisineTool{
		toolName:    "CuisineDatabase",
		description: "Database of all the available dishes in all the restaurants",
		dishes: map[string][]string{
			"Pasta Paradise": {"Carbonara", "Lasagna", "Risotto"},
			"Sushi Master":   {"Dragon Roll", "Sashimi Platter", "Tempura"},
			"Taco Fiesta":    {"Street Tacos", "Burrito Bowl", "Quesadilla"},
		},
	}
}

func (c *CuisineTool) Name() string {
	return c.toolName
}

func (c *CuisineTool) String() string {
	return c.toolName
}

func (c *CuisineTool) Description() string {
	return c.description
}

func (c *CuisineTool) StatusMessage() string {
	return "Finding the perfect dishes for you"
}

func (c *CuisineTool) OpenAI() []openai.ChatCompletionToolParam {
	return []openai.ChatCompletionToolParam{
		{
			Function: openai.FunctionDefinitionParam{
				Name:        c.toolName,
				Description: param.Opt[string]{Value: c.description},
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]interface{}{
						"restaurant": map[string]interface{}{
							"type":        "string",
							"description": "Restaurant name",
						},
					},
					"required": []string{"restaurant"},
				},
			},
		},
	}
}

func (c *CuisineTool) Execute(ctx context.Context, meta agentpod.Meta, args map[string]interface{}) (string, error) {
	restaurant := args["restaurant"].(string)
	if dishes, ok := c.dishes[restaurant]; ok {
		return strings.Join(dishes, ", "), nil
	}
	return "No dishes found for this restaurant", nil
}

// Function to create memory with user preferences
func getUserPreferencesMemory(meta *agentpod.Meta) (*agentpod.MemoryBlock, error) {
	memoryBlock := agentpod.NewMemoryBlock()
	userDetailsBlock := agentpod.NewMemoryBlock()
	userDetailsBlock.AddString("location", "Downtown")
	userDetailsBlock.AddString("favorite_cuisines", "Italian")
	memoryBlock.AddBlock("UserDetails", userDetailsBlock)
	return memoryBlock, nil
}

const mainPrompt = `You are a restaurant recommendation expert tasked with helping users find the perfect restaurant based on their location and cuisine preferences. Provide concise and direct recommendations using the available data from authorized tools.

- Focus on the user's location and specified cuisine preferences.
- Avoid making assumptions about restaurants that are not readily available through your tools.
- Ensure recommendations are based solely on the data you can access.
- Clearly communicate the recommendation and justify the choice with relevant details that enhance the user's decision-making process.

(Note: Ensure all relevant data is provided and realistic for actual recommendations.)`

func testRestaurantRecommendation(t *testing.T, prompt string) {
	config, err := LoadConfig()
	require.NoError(t, err)

	require.NotEmpty(t, config.OpenAIAPIKey, "OpenAI API Key is not set")

	llm := agentpod.NewLLM(
		config.OpenAIAPIKey,
		config.BaseURL,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)

	// Create mock memory with user preferences
	mem := &MockMemory{
		RetrieveFn: getUserPreferencesMemory,
	}

	// Create restaurant agent with restaurant recommendation tool
	restaurantTool := NewRestaurantTool()
	cuisineTool := NewCuisineTool()
	restaurantAgent := agentpod.NewAgent(
		mainPrompt,
		[]agentpod.Skill{
			{
				Name:         "RestaurantExpert",
				Description:  "Expert in restaurant recommendations. You cannot make cusine recommendations. We have a cuisine expert for that.",
				SystemPrompt: "As a restaurant expert, you provide personalized restaurant recommendations. Do not make any recommendations on dishes. We have cusines expert for that.",
				Tools:        []agentpod.Tool{restaurantTool},
			},
			{
				Name:         "CuisineExpert",
				Description:  "Expert in cuisine and dishes, you provide dish recommendations for restaurants found by RestaurantExpert. Should not be called before restaurant expert made the restaurant recommendation.",
				SystemPrompt: "As a cuisine expert, you provide dish recommendations for restaurants found by RestaurantExpert. You should only do recommendations on cusines for the restaurants you have access to. You should not assume the existance of any restaurants that you don't have access to",
				Tools:        []agentpod.Tool{cuisineTool},
			},
		},
	)

	// Create a mock storage with empty conversation history
	storage := &MockStorage{}
	storage.ConversationFn = getEmptyConversationHistory(storage)

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	// Create session with restaurant agent
	restaurantSession := agentpod.NewSession(context.Background(), llm, mem, restaurantAgent, storage, agentpod.Meta{
		CustomerID: orgID,
		SessionID:  sessionID,
		Extra:      map[string]string{"user_id": userID, "domain": "test"},
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

	// Verify restaurant recommendation
	if !strings.Contains(strings.ToLower(response), "pasta paradise") {
		t.Fatal("Expected 'Pasta Paradise' to be in the restaurant recommendation, got:", response)
	}

	// Verify cuisine recommendation
	expectedDishes := []string{"carbonara", "lasagna", "risotto"}
	foundDish := false
	for _, dish := range expectedDishes {
		if strings.Contains(strings.ToLower(response), dish) {
			foundDish = true
			break
		}
	}
	if !foundDish {
		t.Fatal("Expected at least one of the dishes to be in the cuisine recommendation, got:", response)
	}

	// Verify CreateConversation was called with the correct messages
	if !storage.WasCreateConversationCalled() {
		t.Fatal("Expected CreateConversation to be called")
	}
	if storage.GetUserMessage(sessionID) != prompt {
		t.Fatalf("Expected user message to match, got: %s", storage.GetUserMessage(sessionID))
	}
}

func TestMultiAgentRestaurantRecommendationWithSummarizer(t *testing.T) {
	prompt := "What's a good restaurant for me? and what dishes do they have. Make sure to call summarizer to properly summarize the response."
	testRestaurantRecommendation(t, prompt)
}

func TestMultiAgentRestaurantRecommendationWithoutSummarizer(t *testing.T) {
	prompt := "What's a good restaurant for me? and what dishes do they have? Do not call summarizer. Return the final response as it is."
	testRestaurantRecommendation(t, prompt)
}
