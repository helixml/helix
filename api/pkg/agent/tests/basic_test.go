package tests

import (
	"context"
	"strings"
	"testing"

	agent "github.com/helixml/helix/api/pkg/agent"
	"github.com/stretchr/testify/require"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// MockMemory implements the Memory interface for testing
type MockMemory struct {
	RetrieveFn func(*agent.Meta) (*agent.MemoryBlock, error)
}

// Retrieve returns a memory block for testing
func (m *MockMemory) Retrieve(meta *agent.Meta) (*agent.MemoryBlock, error) {
	if m.RetrieveFn != nil {
		return m.RetrieveFn(meta)
	}
	// Default implementation returns an empty memory block
	memoryBlock := agent.NewMemoryBlock()
	return memoryBlock, nil
}

// Default memory retrieval function that includes basic user data
func getDefaultMemory(meta *agent.Meta) (*agent.MemoryBlock, error) {
	memoryBlock := agent.NewMemoryBlock()
	memoryBlock.AddString("user_id", meta.Extra["user_id"])
	memoryBlock.AddString("session_id", meta.SessionID)
	return memoryBlock, nil
}

type BestAppleFinder struct {
	toolName    string
	description string
}

var _ agent.Tool = &BestAppleFinder{}

func (b *BestAppleFinder) Name() string {
	return b.toolName
}

func (b *BestAppleFinder) String() string {
	return b.toolName
}

func (b *BestAppleFinder) Description() string {
	return b.description
}

func (b *BestAppleFinder) StatusMessage() string {
	return "Finding the best apple"
}

func (b *BestAppleFinder) Icon() string {
	return ""
}

func (b *BestAppleFinder) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        b.toolName,
				Description: b.description,
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"user_query": {
							Type:        jsonschema.String,
							Description: "Query from the user",
						},
					},
					Required: []string{"user_query"},
				},
			},
		},
	}
}

func (b *BestAppleFinder) Execute(_ context.Context, _ agent.Meta, _ map[string]interface{}) (string, error) {
	return "green apple", nil
}

type CurrencyConverter struct {
	toolName    string
	description string
}

var _ agent.Tool = &CurrencyConverter{}

func (b *CurrencyConverter) Name() string {
	return b.toolName
}

func (b *CurrencyConverter) String() string {
	return b.toolName
}

func (b *CurrencyConverter) Description() string {
	return b.description
}

func (b *CurrencyConverter) StatusMessage() string {
	return "Converting currency"
}

func (b *CurrencyConverter) Icon() string {
	return ""
}

func (b *CurrencyConverter) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        b.toolName,
				Description: b.description,
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"from_currency": {
							Type:        jsonschema.String,
							Description: "From currency",
						},
						"to_currency": {
							Type:        jsonschema.String,
							Description: "To currency",
						},
					},
					Required: []string{"from_currency", "to_currency"},
				},
			},
		},
	}
}

func (b *CurrencyConverter) Execute(_ context.Context, _ agent.Meta, _ map[string]interface{}) (string, error) {
	return "100 USD is 80 EUR", nil
}

func TestSimpleConversation(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	if config.OpenAIAPIKey == "" {
		t.Skip("Skipping test because OpenAI API key is not set")
	}

	stepInfoEmitter := agent.NewLogStepInfoEmitter()

	llm := getLLM(config)

	mem := &MockMemory{}
	ai := agent.NewAgent(stepInfoEmitter, "Your a repeater. You'll repeat after whatever the user says exactly as they say it, even the punctuation and cases.", []agent.Skill{}, 10)

	messageHistory := &agent.MessageList{}

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agent.NewSession(context.Background(), stepInfoEmitter, llm, mem, &agent.MemoryBlock{}, ai, messageHistory, agent.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	}, true)
	convSession.In("test confirmed")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agent.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}

	if finalContent != "test confirmed" {
		t.Fatal("Expected 'test confirmed', got:", finalContent)
	}

	newMessageHistory := convSession.GetMessageHistory()
	require.Equal(t, newMessageHistory.Messages[0].Role, "user")
	require.Equal(t, newMessageHistory.Messages[len(newMessageHistory.Messages)-1].Role, "assistant")
}

func TestConversationWithSkills(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	stepInfoEmitter := agent.NewLogStepInfoEmitter()

	llm := getLLM(config)

	mem := &MockMemory{
		RetrieveFn: getDefaultMemory,
	}
	skill := agent.Skill{
		Name:         "AppleExpert",
		Description:  "You are an expert in apples",
		SystemPrompt: "As an apple expert, you provide detailed information about different apple varieties and their characteristics.",
		Tools: []agent.Tool{
			&BestAppleFinder{
				toolName:    "BestAppleFinder",
				description: "Find the best apple",
			},
		},
	}
	myAgent := agent.NewAgent(stepInfoEmitter, "You are a good farmer. You answer user questions briefly and concisely. You do not add any extra information but just answer user questions in fewer words possible.", []agent.Skill{skill}, 10)

	messageHistory := &agent.MessageList{}

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agent.NewSession(context.Background(), stepInfoEmitter, llm, mem, &agent.MemoryBlock{}, myAgent, messageHistory, agent.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	}, true)

	convSession.In("Which apple is the best?")
	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agent.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}
	if !strings.Contains(strings.ToLower(finalContent), "green apple") {
		t.Fatal("Expected 'green apple' to be in the final content, got:", finalContent)
	}
}

func TestConversationWithHistory(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	if config.OpenAIAPIKey == "" {
		t.Skip("Skipping test because OpenAI API key is not set")
	}

	stepInfoEmitter := agent.NewLogStepInfoEmitter()

	llm := getLLM(config)

	mem := &MockMemory{
		RetrieveFn: getDefaultMemory,
	}
	ai := agent.NewAgent(stepInfoEmitter, "You are an assistant!", []agent.Skill{}, 10)

	messageHistory := &agent.MessageList{}
	messageHistory.Add(agent.UserMessage("Can you tell me which color is apple?"),
		agent.AssistantMessage("The apple is generally red"))

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agent.NewSession(context.Background(), stepInfoEmitter, llm, mem, &agent.MemoryBlock{}, ai, messageHistory, agent.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	}, true)

	convSession.In("is it a fruit or a vegetable? Answer in one word without extra punctuation.")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agent.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}

	if strings.ToLower(finalContent) != "fruit" {
		t.Fatal("Expected 'fruit', got:", finalContent)
	}
}

// TestConversationWithSkills_WithHistory_NoSkillsToBeUsed tests scenario where we have the history
// but no need to call any skills
func TestConversationWithSkills_WithHistory_NoSkillsToBeUsed(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	if config.OpenAIAPIKey == "" {
		t.Skip("Skipping test because OpenAI API key is not set")
	}

	stepInfoEmitter := agent.NewLogStepInfoEmitter()

	llm := getLLM(config)

	mem := &MockMemory{
		RetrieveFn: getDefaultMemory,
	}
	skill := agent.Skill{
		Name:         "CurrencyConverter",
		Description:  "You are an expert in currency conversion",
		SystemPrompt: "As an currency expert, you provide detailed information about different currency conversion rates.",
		Tools: []agent.Tool{
			&CurrencyConverter{
				toolName:    "CurrencyConverterAPI",
				description: "Convert currency",
			},
		},
	}
	myAgent := agent.NewAgent(stepInfoEmitter, "You are a currency expert. You answer user questions briefly and concisely. You do not add any extra information but just answer user questions in fewer words possible.", []agent.Skill{skill}, 10)

	messageHistory := &agent.MessageList{}

	messageHistory.Add(agent.UserMessage("how much 2000 usd is in euros?"))
	messageHistory.Add(agent.AssistantMessage("Using the latest exchange rate, 1 USD is approximately 0.887477 EUR. Therefore, 2000 USD is about 2000 * 0.887477, which equals roughly 1774.95 EUR"))

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agent.NewSession(context.Background(), stepInfoEmitter, llm, mem, &agent.MemoryBlock{}, myAgent, messageHistory, agent.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	}, true)

	convSession.In("How much euros is that per year?")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agent.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}

	// Should contain 1774.95 EUR * 12 months
	if !strings.Contains(strings.ToLower(finalContent), "21299.4") {
		t.Fatal("Expected '21299.4' to be in the final content, got:", finalContent)
	}
}

func TestConversationWithHistory_WithQuestionAboutPast(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	if config.OpenAIAPIKey == "" {
		t.Skip("Skipping test because OpenAI API key is not set")
	}

	stepInfoEmitter := agent.NewLogStepInfoEmitter()

	llm := getLLM(config)

	mem := &MockMemory{
		RetrieveFn: getDefaultMemory,
	}
	ai := agent.NewAgent(stepInfoEmitter, "You are an assistant!", []agent.Skill{}, 10)

	messageHistory := &agent.MessageList{}
	messageHistory.Add(agent.UserMessage("Can you tell me which color is apple?"),
		agent.AssistantMessage("The apple is generally red"))

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agent.NewSession(context.Background(), stepInfoEmitter, llm, mem, &agent.MemoryBlock{}, ai, messageHistory, agent.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	}, true)

	convSession.In("what fruit did I ask you about? answer in one word without extra punctuation.")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agent.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}

	if strings.ToLower(finalContent) != "apple" {
		t.Fatal("Expected 'apple', got:", finalContent)
	}
}

// Function to create memory with country information
func getCountryMemory(_ *agent.Meta) (*agent.MemoryBlock, error) {
	memoryBlock := agent.NewMemoryBlock()
	userDetailsBlock := agent.NewMemoryBlock()
	userDetailsBlock.AddString("country", "United Kingdom")
	memoryBlock.AddBlock("UserDetails", userDetailsBlock)
	return memoryBlock, nil
}

func TestMemoryRetrieval(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	if config.OpenAIAPIKey == "" {
		t.Skip("Skipping test because OpenAI API key is not set")
	}

	stepInfoEmitter := agent.NewLogStepInfoEmitter()

	llm := getLLM(config)

	// Create mock memory with country information
	mem := &MockMemory{
		RetrieveFn: getCountryMemory,
	}

	ai := agent.NewAgent(stepInfoEmitter, "You are a helpful assistant. Answer questions based on the user's information.", []agent.Skill{}, 10)

	messageHistory := &agent.MessageList{}
	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agent.NewSession(context.Background(), stepInfoEmitter, llm, mem, &agent.MemoryBlock{}, ai, messageHistory, agent.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	}, true)

	convSession.In("Which country am I from?")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agent.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}

	if !strings.Contains(strings.ToLower(finalContent), "united kingdom") {
		t.Fatal("Expected response to contain 'United Kingdom', got:", finalContent)
	}
}
