//go:build oauth_integration
// +build oauth_integration

package skills_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	goai "github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

// HelixTestFramework provides utilities for creating and managing Helix test resources
type HelixTestFramework struct {
	ctx            context.Context
	store          store.Store
	helixAPIServer *server.HelixAPIServer
	logger         zerolog.Logger
	testUser       *types.User
	serverURL      string
	testTimestamp  string
	testResultsDir string

	// Test resources for cleanup
	createdProviders   []*types.OAuthProvider
	createdApps        []*types.App
	createdConnections []*types.OAuthConnection
}

// NewHelixTestFramework creates a new test framework instance
func NewHelixTestFramework(ctx context.Context, store store.Store, helixAPIServer *server.HelixAPIServer, logger zerolog.Logger, testUser *types.User, serverURL string, testTimestamp string, testResultsDir string) *HelixTestFramework {
	return &HelixTestFramework{
		ctx:                ctx,
		store:              store,
		helixAPIServer:     helixAPIServer,
		logger:             logger,
		testUser:           testUser,
		serverURL:          serverURL,
		testTimestamp:      testTimestamp,
		testResultsDir:     testResultsDir,
		createdProviders:   make([]*types.OAuthProvider, 0),
		createdApps:        make([]*types.App, 0),
		createdConnections: make([]*types.OAuthConnection, 0),
	}
}

// CreateOAuthProvider creates an OAuth provider for testing
func (f *HelixTestFramework) CreateOAuthProvider(config OAuthProviderConfig) (*types.OAuthProvider, error) {
	f.logger.Info().Str("provider_type", config.ProviderType).Msg("Creating OAuth provider")

	callbackURL := f.serverURL + "/api/v1/oauth/callback/" + strings.ToLower(config.ProviderType)

	provider := &types.OAuthProvider{
		Name:         config.Name,
		Type:         types.OAuthProviderType(config.ProviderType),
		Enabled:      true,
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		AuthURL:      config.AuthURL,
		TokenURL:     config.TokenURL,
		UserInfoURL:  config.UserInfoURL,
		CallbackURL:  callbackURL,
		Scopes:       config.Scopes,
		CreatorID:    f.testUser.ID,
		CreatorType:  types.OwnerTypeUser,
	}

	f.logger.Info().
		Str("callback_url", callbackURL).
		Str("server_url", f.serverURL).
		Msg("Configuring OAuth provider with callback URL")

	createdProvider, err := f.store.CreateOAuthProvider(f.ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth provider: %w", err)
	}

	// Track for cleanup
	f.createdProviders = append(f.createdProviders, createdProvider)

	f.logger.Info().
		Str("provider_id", createdProvider.ID).
		Str("provider_name", createdProvider.Name).
		Str("provider_type", string(createdProvider.Type)).
		Msg("OAuth provider created successfully")

	return createdProvider, nil
}

// CreateTestApp creates a test app with skills configuration
func (f *HelixTestFramework) CreateTestApp(config AppConfig) (*types.App, error) {
	f.logger.Info().Str("app_name", config.Name).Msg("Creating test app with skills")

	appName := fmt.Sprintf("%s %d", config.Name, time.Now().Unix())

	app := &types.App{
		Owner:     f.testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        appName,
				Description: config.Description,
				Assistants:  config.Assistants,
			},
		},
	}

	createdApp, err := f.store.CreateApp(f.ctx, app)
	if err != nil {
		return nil, fmt.Errorf("failed to create test app: %w", err)
	}

	// Track for cleanup
	f.createdApps = append(f.createdApps, createdApp)

	f.logger.Info().
		Str("app_id", createdApp.ID).
		Str("app_name", createdApp.Config.Helix.Name).
		Int("assistants_count", len(createdApp.Config.Helix.Assistants)).
		Msg("Test app created successfully")

	return createdApp, nil
}

// GetOAuthConnection retrieves an OAuth connection for a user and provider
func (f *HelixTestFramework) GetOAuthConnection(providerID string) (*types.OAuthConnection, error) {
	connections, err := f.store.ListOAuthConnections(f.ctx, &store.ListOAuthConnectionsQuery{
		UserID: f.testUser.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list OAuth connections: %w", err)
	}

	for _, conn := range connections {
		if conn.ProviderID == providerID {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("no OAuth connection found for provider %s", providerID)
}

// ExecuteAgentSession executes an agent session with the given query
func (f *HelixTestFramework) ExecuteAgentSession(app *types.App, userMessage string, sessionName string) (*AgentSessionResult, error) {
	f.logger.Info().
		Str("user_message", userMessage).
		Str("session_name", sessionName).
		Str("app_id", app.ID).
		Msg("Executing agent session")

	// Create a new session
	session, err := f.store.CreateSession(f.ctx, types.Session{
		Name:      sessionName,
		Owner:     f.testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
		ModelName: "anthropic:claude-3-5-haiku-20241022",
		ParentApp: app.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	f.logger.Info().
		Str("session_id", session.ID).
		Str("app_id", app.ID).
		Msg("Created session for execution")

	// Add user interaction
	userInteraction := &types.Interaction{
		ID:      system.GenerateUUID(),
		Created: time.Now(),
		Updated: time.Now(),
		Creator: types.CreatorTypeUser,
		Mode:    types.SessionModeInference,
		Message: userMessage,
		Content: types.MessageContent{
			ContentType: types.MessageContentTypeText,
			Parts:       []any{userMessage},
		},
		State:    types.InteractionStateWaiting,
		Finished: false,
		Metadata: map[string]string{},
	}

	// Update session with user interaction
	session.Interactions = append(session.Interactions, userInteraction)

	// Prepare OpenAI chat completion request
	openaiReq := goai.ChatCompletionRequest{
		Model: "anthropic:claude-3-5-haiku-20241022",
		Messages: []goai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: userMessage,
			},
		},
		Stream: false,
	}

	// Set up controller options with app context for OAuth
	options := &controller.ChatCompletionOptions{
		AppID: app.ID,
	}

	// Set app ID and user ID in context for OAuth token retrieval
	ctx := oai.SetContextAppID(f.ctx, app.ID)
	ctx = oai.SetContextSessionID(ctx, session.ID)
	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       f.testUser.ID,
		SessionID:     session.ID,
		InteractionID: userInteraction.ID,
	})

	f.logger.Info().
		Str("session_id", session.ID).
		Str("app_id", app.ID).
		Msg("Executing chat completion with OAuth context")

	// Execute the chat completion
	response, _, err := f.helixAPIServer.Controller.ChatCompletion(ctx, f.testUser, openaiReq, options)
	if err != nil {
		return nil, fmt.Errorf("failed to execute chat completion: %w", err)
	}

	// Extract agent response
	agentResponse := ""
	if len(response.Choices) > 0 {
		agentResponse = response.Choices[0].Message.Content
	}

	if agentResponse == "" {
		return nil, fmt.Errorf("no response from agent")
	}

	// Update session with assistant response
	assistantInteraction := &types.Interaction{
		ID:      system.GenerateUUID(),
		Created: time.Now(),
		Updated: time.Now(),
		Creator: types.CreatorTypeAssistant,
		Mode:    types.SessionModeInference,
		Message: agentResponse,
		Content: types.MessageContent{
			ContentType: types.MessageContentTypeText,
			Parts:       []any{agentResponse},
		},
		State:     types.InteractionStateComplete,
		Finished:  true,
		Completed: time.Now(),
		Metadata:  map[string]string{},
	}

	session.Interactions = append(session.Interactions, assistantInteraction)

	// Write session back to store
	err = f.helixAPIServer.Controller.WriteSession(f.ctx, session)
	if err != nil {
		f.logger.Warn().Err(err).Msg("Failed to write session to store")
	}

	f.logger.Info().
		Str("session_id", session.ID).
		Str("agent_response", agentResponse[:min(len(agentResponse), 100)]+"...").
		Msg("Received agent response")

	result := &AgentSessionResult{
		SessionID:        session.ID,
		UserMessage:      userMessage,
		AgentResponse:    agentResponse,
		UserInteraction:  userInteraction,
		AgentInteraction: assistantInteraction,
	}

	// Log the conversation
	f.logAgentConversation(sessionName, userMessage, agentResponse, app)

	return result, nil
}

// logAgentConversation logs the agent conversation to a file
func (f *HelixTestFramework) logAgentConversation(sessionName, userMessage, agentResponse string, app *types.App) {
	conversationFilename := filepath.Join(f.testResultsDir, fmt.Sprintf("oauth_e2e_%s_conversation_%s.txt",
		f.testTimestamp, strings.ReplaceAll(strings.ToLower(sessionName), " ", "_")))

	conversationContent := fmt.Sprintf(`=== OAuth Skills E2E Test - %s ===
Timestamp: %s
Test User: %s
App ID: %s
App Name: %s

=== CONVERSATION ===

USER: %s

AGENT: %s

=== TEST METADATA ===
- OAuth connection verified: YES
- Expected to contain real data from OAuth API calls
- Agent should NOT return generic/mock responses

=== VERIFICATION NOTES ===
- Agent response should contain real data from OAuth API calls
- OAuth tokens should be used for actual API requests
`,
		sessionName,
		time.Now().Format("2006-01-02 15:04:05"),
		f.testUser.Username,
		app.ID,
		app.Config.Helix.Name,
		userMessage,
		agentResponse,
	)

	err := os.WriteFile(conversationFilename, []byte(conversationContent), 0644)
	if err != nil {
		f.logger.Error().Err(err).Str("filename", conversationFilename).Msg("Failed to write conversation log")
	} else {
		f.logger.Info().Str("filename", conversationFilename).Msg("Agent conversation logged to file")
	}
}

// CleanupTestResources performs cleanup of all created test resources
func (f *HelixTestFramework) CleanupTestResources() {
	f.logger.Info().Msg("=== Starting Helix test resources cleanup ===")

	// Delete OAuth connections
	for _, conn := range f.createdConnections {
		err := f.store.DeleteOAuthConnection(f.ctx, conn.ID)
		if err != nil {
			f.logger.Error().Err(err).Str("connection_id", conn.ID).Msg("Failed to delete OAuth connection")
		} else {
			f.logger.Info().Str("connection_id", conn.ID).Msg("OAuth connection deleted")
		}
	}

	// Delete apps
	for _, app := range f.createdApps {
		err := f.store.DeleteApp(f.ctx, app.ID)
		if err != nil {
			f.logger.Error().Err(err).Str("app_id", app.ID).Msg("Failed to delete test app")
		} else {
			f.logger.Info().Str("app_id", app.ID).Msg("Test app deleted")
		}
	}

	// Delete OAuth providers
	for _, provider := range f.createdProviders {
		err := f.store.DeleteOAuthProvider(f.ctx, provider.ID)
		if err != nil {
			f.logger.Error().Err(err).Str("provider_id", provider.ID).Msg("Failed to delete OAuth provider")
		} else {
			f.logger.Info().Str("provider_id", provider.ID).Msg("OAuth provider deleted")
		}
	}

	f.logger.Info().Msg("=== Helix test resources cleanup completed ===")
}

// CleanupExistingOAuthData removes existing OAuth data from previous test runs
func (f *HelixTestFramework) CleanupExistingOAuthData() error {
	f.logger.Info().Msg("Cleaning up existing OAuth data from previous test runs")

	// Delete all OAuth connections
	connections, err := f.store.ListOAuthConnections(f.ctx, &store.ListOAuthConnectionsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list OAuth connections: %w", err)
	}

	for _, conn := range connections {
		err = f.store.DeleteOAuthConnection(f.ctx, conn.ID)
		if err != nil {
			f.logger.Warn().Err(err).Str("connection_id", conn.ID).Msg("Failed to delete OAuth connection")
		} else {
			f.logger.Debug().Str("connection_id", conn.ID).Msg("Deleted OAuth connection from previous run")
		}
	}

	// Delete test OAuth providers
	providers, err := f.store.ListOAuthProviders(f.ctx, &store.ListOAuthProvidersQuery{})
	if err != nil {
		return fmt.Errorf("failed to list OAuth providers: %w", err)
	}

	for _, provider := range providers {
		// Only delete test providers
		if strings.Contains(provider.Name, "Skills Test") || strings.Contains(provider.Name, "Test") {
			err = f.store.DeleteOAuthProvider(f.ctx, provider.ID)
			if err != nil {
				f.logger.Warn().Err(err).Str("provider_id", provider.ID).Msg("Failed to delete OAuth provider")
			} else {
				f.logger.Debug().Str("provider_id", provider.ID).Msg("Deleted OAuth provider from previous run")
			}
		}
	}

	f.logger.Info().
		Int("connections_cleaned", len(connections)).
		Msg("Cleanup of existing OAuth data completed")

	return nil
}

// LoadSkillYAML loads a skill configuration from YAML file
func (f *HelixTestFramework) LoadSkillYAML(skillName string) (*SkillYAML, error) {
	yamlPath := filepath.Join("pkg", "skills", skillName+".yaml")
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s.yaml file at %s: %w", skillName, yamlPath, err)
	}

	var skillYAML SkillYAML
	err = yaml.Unmarshal(yamlData, &skillYAML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s.yaml: %w", skillName, err)
	}

	f.logger.Info().
		Str("name", skillYAML.Metadata.Name).
		Str("display_name", skillYAML.Metadata.DisplayName).
		Str("oauth_provider", skillYAML.Spec.OAuth.Provider).
		Interface("oauth_scopes", skillYAML.Spec.OAuth.Scopes).
		Str("base_url", skillYAML.Spec.API.BaseURL).
		Msg("Loaded skill configuration from YAML")

	return &skillYAML, nil
}

// CreateAppFromSkill creates an app configuration from a skill YAML
func (f *HelixTestFramework) CreateAppFromSkill(skillYAML *SkillYAML, oauthProvider *types.OAuthProvider) AppConfig {
	return AppConfig{
		Name:        skillYAML.Metadata.DisplayName + " Test App",
		Description: "Test app for " + skillYAML.Metadata.DisplayName + " OAuth skills integration",
		Assistants: []types.AssistantConfig{
			{
				Name:         skillYAML.Metadata.DisplayName + " Assistant",
				Description:  "Assistant configured with " + skillYAML.Metadata.DisplayName + " OAuth skills",
				AgentMode:    true,
				SystemPrompt: skillYAML.Spec.SystemPrompt,

				// Configure LLM models
				Provider: "anthropic",
				Model:    "claude-3-5-haiku-20241022",

				// Configure reasoning and generation models
				ReasoningModelProvider:  "anthropic",
				ReasoningModel:          "claude-3-5-haiku-20241022",
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-3-5-haiku-20241022",

				// Configure small models
				SmallReasoningModelProvider:  "anthropic",
				SmallReasoningModel:          "claude-3-5-haiku-20241022",
				SmallGenerationModelProvider: "anthropic",
				SmallGenerationModel:         "claude-3-5-haiku-20241022",

				// Configure with skill
				APIs: []types.AssistantAPI{
					{
						Name:          skillYAML.Metadata.Name,
						Description:   skillYAML.Spec.Description,
						URL:           skillYAML.Spec.API.BaseURL,
						Schema:        skillYAML.Spec.API.Schema,
						Headers:       skillYAML.Spec.API.Headers,
						SystemPrompt:  skillYAML.Spec.SystemPrompt,
						OAuthProvider: oauthProvider.Name,
					},
				},
			},
		},
	}
}

// Configuration structures

// OAuthProviderConfig contains configuration for creating an OAuth provider
type OAuthProviderConfig struct {
	Name         string
	ProviderType string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
}

// AppConfig contains configuration for creating a test app
type AppConfig struct {
	Name        string
	Description string
	Assistants  []types.AssistantConfig
}

// AgentSessionResult contains the results of executing an agent session
type AgentSessionResult struct {
	SessionID        string
	UserMessage      string
	AgentResponse    string
	UserInteraction  *types.Interaction
	AgentInteraction *types.Interaction
}

// SkillYAML represents the structure of skill YAML files
type SkillYAML struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name        string `yaml:"name"`
		DisplayName string `yaml:"displayName"`
		Provider    string `yaml:"provider"`
		Category    string `yaml:"category"`
	} `yaml:"metadata"`
	Spec struct {
		Description  string `yaml:"description"`
		SystemPrompt string `yaml:"systemPrompt"`
		Icon         struct {
			Type string `yaml:"type"`
			Name string `yaml:"name"`
		} `yaml:"icon"`
		OAuth struct {
			Provider string   `yaml:"provider"`
			Scopes   []string `yaml:"scopes"`
		} `yaml:"oauth"`
		API struct {
			BaseURL string            `yaml:"baseUrl"`
			Headers map[string]string `yaml:"headers"`
			Schema  string            `yaml:"schema"`
		} `yaml:"api"`
		Configurable bool `yaml:"configurable"`
	} `yaml:"spec"`
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
