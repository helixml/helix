package tests

import (
	"context"
	"strings"
	"testing"

	agentpod "github.com/helixml/helix/api/pkg/agent"
	helix_openai "github.com/helixml/helix/api/pkg/openai"
	"github.com/stretchr/testify/require"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// MockMemory implements the Memory interface for testing
type MockMemory struct {
	RetrieveFn func(*agentpod.Meta) (*agentpod.MemoryBlock, error)
}

// Retrieve returns a memory block for testing
func (m *MockMemory) Retrieve(meta *agentpod.Meta) (*agentpod.MemoryBlock, error) {
	if m.RetrieveFn != nil {
		return m.RetrieveFn(meta)
	}
	// Default implementation returns an empty memory block
	memoryBlock := agentpod.NewMemoryBlock()
	return memoryBlock, nil
}

// Default memory retrieval function that includes basic user data
func getDefaultMemory(meta *agentpod.Meta) (*agentpod.MemoryBlock, error) {
	memoryBlock := agentpod.NewMemoryBlock()
	memoryBlock.AddString("user_id", meta.Extra["user_id"])
	memoryBlock.AddString("session_id", meta.SessionID)
	return memoryBlock, nil
}

type BestAppleFinder struct {
	toolName    string
	description string
}

var _ agentpod.Tool = &BestAppleFinder{}

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

func (b *BestAppleFinder) Execute(ctx context.Context, meta agentpod.Meta, args map[string]interface{}) (string, error) {
	return "green apple", nil
}

func TestSimpleConversation(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL)

	llm := agentpod.NewLLM(
		client,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)
	mem := &MockMemory{}
	ai := agentpod.NewAgent("Your a repeater. You'll repeat after whatever the user says exactly as they say it, even the punctuation and cases.", []agentpod.Skill{})

	messageHistory := &agentpod.MessageList{}

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	stepInfoEmitter := agentpod.NewLogStepInfoEmitter()

	convSession := agentpod.NewSession(context.Background(), stepInfoEmitter, llm, mem, ai, messageHistory, agentpod.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	})
	convSession.In("test confirmed")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agentpod.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agentpod.ResponseTypeEnd {
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

	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL)

	llm := agentpod.NewLLM(
		client,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)
	mem := &MockMemory{
		RetrieveFn: getDefaultMemory,
	}
	skill := agentpod.Skill{
		Name:         "AppleExpert",
		Description:  "You are an expert in apples",
		SystemPrompt: "As an apple expert, you provide detailed information about different apple varieties and their characteristics.",
		Tools: []agentpod.Tool{
			&BestAppleFinder{
				toolName:    "BestAppleFinder",
				description: "Find the best apple",
			},
		},
	}
	agent := agentpod.NewAgent("You are a good farmer. You answer user questions briefly and concisely. You do not add any extra information but just answer user questions in fewer words possible.", []agentpod.Skill{skill})

	messageHistory := &agentpod.MessageList{}

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	stepInfoEmitter := agentpod.NewLogStepInfoEmitter()

	convSession := agentpod.NewSession(context.Background(), stepInfoEmitter, llm, mem, agent, messageHistory, agentpod.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	})

	convSession.In("Which apple is the best?")
	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agentpod.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agentpod.ResponseTypeEnd {
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

	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL)

	llm := agentpod.NewLLM(
		client,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)
	mem := &MockMemory{
		RetrieveFn: getDefaultMemory,
	}
	ai := agentpod.NewAgent("You are an assistant!", []agentpod.Skill{})

	messageHistory := &agentpod.MessageList{}
	messageHistory.Add(agentpod.UserMessage("Can you tell me which color is apple?"),
		agentpod.AssistantMessage("The apple is generally red"))

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	stepInfoEmitter := agentpod.NewLogStepInfoEmitter()

	convSession := agentpod.NewSession(context.Background(), stepInfoEmitter, llm, mem, ai, messageHistory, agentpod.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	})

	convSession.In("is it a fruit or a vegetable? Answer in one word without extra punctuation.")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agentpod.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agentpod.ResponseTypeEnd {
			break
		}
	}

	if strings.ToLower(finalContent) != "fruit" {
		t.Fatal("Expected 'fruit', got:", finalContent)
	}

}

func TestConversationWithHistory_WithQuestionAboutPast(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL)

	llm := agentpod.NewLLM(
		client,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)
	mem := &MockMemory{
		RetrieveFn: getDefaultMemory,
	}
	ai := agentpod.NewAgent("You are an assistant!", []agentpod.Skill{})

	messageHistory := &agentpod.MessageList{}
	messageHistory.Add(agentpod.UserMessage("Can you tell me which color is apple?"),
		agentpod.AssistantMessage("The apple is generally red"))

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	stepInfoEmitter := agentpod.NewLogStepInfoEmitter()

	convSession := agentpod.NewSession(context.Background(), stepInfoEmitter, llm, mem, ai, messageHistory, agentpod.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	})

	convSession.In("what fruit did I ask you about? answer in one word without extra punctuation.")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agentpod.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agentpod.ResponseTypeEnd {
			break
		}
	}

	if strings.ToLower(finalContent) != "apple" {
		t.Fatal("Expected 'apple', got:", finalContent)
	}
}

// Function to create memory with country information
func getCountryMemory(meta *agentpod.Meta) (*agentpod.MemoryBlock, error) {
	memoryBlock := agentpod.NewMemoryBlock()
	userDetailsBlock := agentpod.NewMemoryBlock()
	userDetailsBlock.AddString("country", "United Kingdom")
	memoryBlock.AddBlock("UserDetails", userDetailsBlock)
	return memoryBlock, nil
}

func TestMemoryRetrieval(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}

	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL)

	llm := agentpod.NewLLM(
		client,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)

	// Create mock memory with country information
	mem := &MockMemory{
		RetrieveFn: getCountryMemory,
	}

	ai := agentpod.NewAgent("You are a helpful assistant. Answer questions based on the user's information.", []agentpod.Skill{})

	messageHistory := &agentpod.MessageList{}
	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	stepInfoEmitter := agentpod.NewLogStepInfoEmitter()

	convSession := agentpod.NewSession(context.Background(), stepInfoEmitter, llm, mem, ai, messageHistory, agentpod.Meta{
		UserID:    orgID,
		SessionID: sessionID,
		Extra:     map[string]string{"user_id": userID},
	})

	convSession.In("Which country am I from?")

	var finalContent string
	for {
		out := convSession.Out()
		if out.Type == agentpod.ResponseTypePartialText {
			finalContent += out.Content
		}
		if out.Type == agentpod.ResponseTypeEnd {
			break
		}
	}

	if !strings.Contains(strings.ToLower(finalContent), "united kingdom") {
		t.Fatal("Expected response to contain 'United Kingdom', got:", finalContent)
	}
}
