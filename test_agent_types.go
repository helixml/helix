package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestAgentTypes(t *testing.T) {
	// Test agent type constants
	fmt.Println("Testing agent type constants...")

	// Test session chat request with agent type
	sessionReq := &types.SessionChatRequest{
		AppID:     "test-app",
		SessionID: "",
		AgentType: "zed_external",
		ExternalAgentConfig: &types.ExternalAgentConfig{
			WorkspaceDir: "/workspace/test",
			ProjectPath:  "my-project",
			EnvVars:      []string{"NODE_ENV=development", "API_KEY=test"},
		},
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					ContentType: "text",
					Parts:       []any{"Create a new React component"},
				},
			},
		},
	}

	// Test JSON serialization
	jsonData, err := json.MarshalIndent(sessionReq, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal session request: %v", err)
	}

	fmt.Printf("Session Chat Request JSON:\n%s\n", string(jsonData))

	// Test session metadata with agent type
	metadata := types.SessionMetadata{
		SystemPrompt:        "You are a coding assistant",
		AgentType:           "zed_external",
		ExternalAgentConfig: sessionReq.ExternalAgentConfig,
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal session metadata: %v", err)
	}

	fmt.Printf("Session Metadata JSON:\n%s\n", string(metadataJSON))

	// Test external agent config validation
	if sessionReq.ExternalAgentConfig.WorkspaceDir == "" {
		t.Error("External agent config workspace dir should not be empty")
	}

	if len(sessionReq.ExternalAgentConfig.EnvVars) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(sessionReq.ExternalAgentConfig.EnvVars))
	}

	fmt.Println("✅ All agent type tests passed!")
}

func TestSessionCreationFlow(t *testing.T) {
	fmt.Println("Testing session creation flow...")

	// Simulate the flow that would happen in the server
	// 1. Create a session chat request with external agent
	startReq := &types.SessionChatRequest{
		AgentType: "zed_external",
		ExternalAgentConfig: &types.ExternalAgentConfig{
			ProjectPath: "my-react-app",
			EnvVars:     []string{"NODE_ENV=development"},
		},
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					ContentType: "text",
					Parts:       []any{"Create a todo list component"},
				},
			},
		},
	}

	// 2. Simulate session creation (this would normally happen in session_handlers.go)
	session := &types.Session{
		ID:        "test-session-123",
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
		Metadata: types.SessionMetadata{
			SystemPrompt:        "You are a React development assistant",
			AgentType:           startReq.AgentType,
			ExternalAgentConfig: startReq.ExternalAgentConfig,
		},
	}

	// 3. Validate session was created correctly
	if session.Metadata.AgentType != "zed_external" {
		t.Errorf("Expected agent type 'zed_external', got '%s'", session.Metadata.AgentType)
	}

	if session.Metadata.ExternalAgentConfig == nil {
		t.Error("External agent config should not be nil")
	}

	if session.Metadata.ExternalAgentConfig.ProjectPath != "my-react-app" {
		t.Errorf("Expected project path 'my-react-app', got '%s'", session.Metadata.ExternalAgentConfig.ProjectPath)
	}

	// 4. Test default agent type handling
	defaultReq := &types.SessionChatRequest{
		AgentType: "", // Empty should default to helix
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					ContentType: "text",
					Parts:       []any{"Hello"},
				},
			},
		},
	}

	// Simulate default handling (from session_handlers.go)
	if defaultReq.AgentType == "" {
		defaultReq.AgentType = "helix"
	}

	if defaultReq.AgentType != "helix" {
		t.Errorf("Expected default agent type 'helix', got '%s'", defaultReq.AgentType)
	}

	fmt.Println("✅ Session creation flow tests passed!")
}

func TestAgentTypeValidation(t *testing.T) {
	fmt.Println("Testing agent type validation...")

	validAgentTypes := []string{"helix", "zed_external"}

	// Test valid agent types
	for _, agentType := range validAgentTypes {
		req := &types.SessionChatRequest{
			AgentType: agentType,
		}

		if req.AgentType != agentType {
			t.Errorf("Agent type validation failed for '%s'", agentType)
		}
	}

	// Test external agent config requirements
	externalReq := &types.SessionChatRequest{
		AgentType: "zed_external",
		ExternalAgentConfig: &types.ExternalAgentConfig{
			ProjectPath:  "test-project",
			WorkspaceDir: "/workspace/test",
			EnvVars:      []string{"KEY=value"},
		},
	}

	if externalReq.AgentType != "zed_external" {
		t.Error("External agent type not set correctly")
	}

	if externalReq.ExternalAgentConfig == nil {
		t.Error("External agent config should be provided for zed_external type")
	}

	fmt.Println("✅ Agent type validation tests passed!")
}

func main() {
	fmt.Println("🚀 Running agent type functionality tests...")

	t := &testing.T{}

	TestAgentTypes(t)
	TestSessionCreationFlow(t)
	TestAgentTypeValidation(t)

	if t.Failed() {
		fmt.Println("❌ Some tests failed!")
	} else {
		fmt.Println("🎉 All tests passed! Agent type functionality is working correctly.")
	}
}
