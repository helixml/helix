package tests

import (
	"context"
	"strings"
	"testing"

	agentpod "github.com/helixml/helix/api/pkg/agent"
	helix_openai "github.com/helixml/helix/api/pkg/openai"

	openai "github.com/sashabaranov/go-openai"
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
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_query": map[string]interface{}{
							"type":        "string",
							"description": "Query from the user",
						},
					},
					"required": []string{"user_query"},
				},
			},
		},
	}
}

func (b *BestAppleFinder) Execute(ctx context.Context, meta agentpod.Meta, args map[string]interface{}) (string, error) {
	return "green apple", nil
}

// MockStorage implements the Storage interface for testing
type MockStorage struct {
	ConversationFn  func(agentpod.Meta, int, int) (*agentpod.MessageList, error)
	CreateMessageFn func(meta agentpod.Meta, userMessage string) error
	userMessages    map[string]string // Maps sessionID to userMessage
	wasCalled       map[string]bool   // Tracks if methods were called
}

// GetConversations returns the conversation history
func (m *MockStorage) GetConversations(meta agentpod.Meta, limit int, offset int) (*agentpod.MessageList, error) {
	return m.ConversationFn(meta, limit, offset)
}

// CreateConversation records the user message
func (m *MockStorage) CreateConversation(meta agentpod.Meta, userMessage string) error {
	if m.userMessages == nil {
		m.userMessages = make(map[string]string)
	}
	if m.wasCalled == nil {
		m.wasCalled = make(map[string]bool)
	}

	m.userMessages[meta.SessionID] = userMessage
	m.wasCalled["CreateConversation"] = true

	if m.CreateMessageFn != nil {
		return m.CreateMessageFn(meta, userMessage)
	}
	return nil
}

func (m *MockStorage) WasCreateConversationCalled() bool {
	if m.wasCalled == nil {
		return false
	}
	return m.wasCalled["CreateConversation"]
}

func (m *MockStorage) GetUserMessage(sessionID string) string {
	if m.userMessages == nil {
		return ""
	}
	return m.userMessages[sessionID]
}

func (m *MockStorage) FinishConversation(meta agentpod.Meta, assistantMessage string) error {
	return nil
}

// Default empty conversation history function that includes the user message if available
func getEmptyConversationHistory(s *MockStorage) func(meta agentpod.Meta, limit int, offset int) (*agentpod.MessageList, error) {
	return func(meta agentpod.Meta, limit int, offset int) (*agentpod.MessageList, error) {
		messages := agentpod.MessageList{}

		// If CreateConversation was called, include that message
		if userMsg := s.GetUserMessage(meta.SessionID); userMsg != "" {
			messages.Add(agentpod.UserMessage(userMsg))
		}

		return &messages, nil
	}
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

	// Create a mock storage with empty conversation history
	storage := &MockStorage{}
	storage.ConversationFn = getEmptyConversationHistory(storage)

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agentpod.NewSession(context.Background(), llm, mem, ai, storage, agentpod.Meta{
		CustomerID: orgID,
		SessionID:  sessionID,
		Extra:      map[string]string{"user_id": userID},
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

	// Verify CreateConversation was called with the correct message
	if !storage.WasCreateConversationCalled() {
		t.Fatal("Expected CreateConversation to be called")
	}
	if storage.GetUserMessage(sessionID) != "test confirmed" {
		t.Fatalf("Expected user message to be 'test confirmed', got: %s", storage.GetUserMessage(sessionID))
	}
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

	// Create a mock storage with empty conversation history
	storage := &MockStorage{}
	storage.ConversationFn = getEmptyConversationHistory(storage)

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()
	convSession := agentpod.NewSession(context.Background(), llm, mem, agent, storage, agentpod.Meta{
		CustomerID: orgID,
		SessionID:  sessionID,
		Extra:      map[string]string{"user_id": userID},
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

	// Verify CreateConversation was called with the correct message
	if !storage.WasCreateConversationCalled() {
		t.Fatal("Expected CreateConversation to be called")
	}
	if storage.GetUserMessage(sessionID) != "Which apple is the best?" {
		t.Fatalf("Expected user message to be 'Which apple is the best?', got: %s", storage.GetUserMessage(sessionID))
	}
}

// Function for non-empty conversation history
func getNonEmptyConversationHistory(s *MockStorage) func(meta agentpod.Meta, limit int, offset int) (*agentpod.MessageList, error) {
	return func(meta agentpod.Meta, limit int, offset int) (*agentpod.MessageList, error) {
		messages := agentpod.MessageList{}

		// Add pre-existing conversation history
		messages.Add(
			agentpod.UserMessage("Can you tell me which color is apple?"),
			agentpod.AssistantMessage("The apple is generally red"),
		)

		// If CreateConversation was called, include that new message as well
		if userMsg := s.GetUserMessage(meta.SessionID); userMsg != "" {
			messages.Add(agentpod.UserMessage(userMsg))
		}

		return &messages, nil
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

	// Create a mock storage with non-empty conversation history
	storage := &MockStorage{}
	storage.ConversationFn = getNonEmptyConversationHistory(storage)

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()
	convSession := agentpod.NewSession(context.Background(), llm, mem, ai, storage, agentpod.Meta{
		CustomerID: orgID,
		SessionID:  sessionID,
		Extra:      map[string]string{"user_id": userID},
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

	// Verify CreateConversation was called with the correct message
	if !storage.WasCreateConversationCalled() {
		t.Fatal("Expected CreateConversation to be called")
	}
	if storage.GetUserMessage(sessionID) != "is it a fruit or a vegetable? Answer in one word without extra punctuation." {
		t.Fatalf("Expected user message to match, got: %s", storage.GetUserMessage(sessionID))
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

	// Create a mock storage with empty conversation history
	storage := &MockStorage{}
	storage.ConversationFn = getEmptyConversationHistory(storage)

	orgID := GenerateNewTestID()
	sessionID := GenerateNewTestID()
	userID := GenerateNewTestID()

	convSession := agentpod.NewSession(context.Background(), llm, mem, ai, storage, agentpod.Meta{
		CustomerID: orgID,
		SessionID:  sessionID,
		Extra:      map[string]string{"user_id": userID},
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

	// Verify CreateConversation was called with the correct message
	if !storage.WasCreateConversationCalled() {
		t.Fatal("Expected CreateConversation to be called")
	}
	if storage.GetUserMessage(sessionID) != "Which country am I from?" {
		t.Fatalf("Expected user message to be 'Which country am I from?', got: %s", storage.GetUserMessage(sessionID))
	}
}
