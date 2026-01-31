package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"
)

func Test_populateAppOwner_PopulateUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userStore := store.NewMockStore(ctrl)
	userStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID: "user1",
	}, nil)

	server := &HelixAPIServer{
		Store: userStore,
	}

	apps := []*types.App{
		{
			Owner: "user1",
		},
	}

	populatedApps := server.populateAppOwner(context.Background(), apps)

	require.Equal(t, "user1", populatedApps[0].User.ID)
}

func Test_populateAppOwner_OwnerNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userStore := store.NewMockStore(ctrl)
	userStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)

	server := &HelixAPIServer{
		Store: userStore,
	}

	apps := []*types.App{
		{
			Owner: "user1",
		},
	}

	populatedApps := server.populateAppOwner(context.Background(), apps)

	require.Equal(t, "", populatedApps[0].User.ID)
}

// Test the duplicate name handling logic
func Test_handleDuplicateAppNames(t *testing.T) {
	// Create existing apps with various names
	existingApps := []*types.App{
		{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Name: "Test Agent",
				},
			},
		},
		{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Name: "Test Agent (1)",
				},
			},
		},
		{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Name: "Another Agent",
				},
			},
		},
	}

	// Test case 1: App with unique name should keep its name
	app1 := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name: "Unique Agent",
				Assistants: []types.AssistantConfig{
					{Name: "Unique Agent"},
				},
			},
		},
	}

	handleDuplicateAppNames(app1, existingApps)
	require.Equal(t, "Unique Agent", app1.Config.Helix.Name)
	require.Equal(t, "Unique Agent", app1.Config.Helix.Assistants[0].Name)

	// Test case 2: App with duplicate name should get (2) suffix
	app2 := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name: "Test Agent",
				Assistants: []types.AssistantConfig{
					{Name: "Test Agent"},
				},
			},
		},
	}

	handleDuplicateAppNames(app2, existingApps)
	require.Equal(t, "Test Agent (2)", app2.Config.Helix.Name)
	require.Equal(t, "Test Agent (2)", app2.Config.Helix.Assistants[0].Name)

	// Test case 3: App with empty name should not be modified
	app3 := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name: "",
			},
		},
	}

	handleDuplicateAppNames(app3, existingApps)
	require.Equal(t, "", app3.Config.Helix.Name)
}

// Helper function to handle duplicate names (extracted from createApp handler)
func handleDuplicateAppNames(app *types.App, existingApps []*types.App) {
	// Handle duplicate names by adding suffixes like (1), (2), etc.
	if app.Config.Helix.Name != "" {
		originalName := app.Config.Helix.Name
		finalName := originalName
		counter := 1

		// Keep checking until we find an available name
		for {
			nameExists := false
			for _, a := range existingApps {
				if a.Config.Helix.Name == finalName {
					nameExists = true
					break
				}
			}

			if !nameExists {
				break
			}

			// Try the next suffix
			finalName = fmt.Sprintf("%s (%d)", originalName, counter)
			counter++
		}

		// Update the app name to the final available name
		app.Config.Helix.Name = finalName

		// Also update the assistant name to match if it was the same as app name
		for i := range app.Config.Helix.Assistants {
			if app.Config.Helix.Assistants[i].Name == originalName {
				app.Config.Helix.Assistants[i].Name = finalName
			}
		}
	}
}

func TestFindModelSubstitution(t *testing.T) {
	server := &HelixAPIServer{}

	// Create test model classes similar to substitutions.yaml
	modelClasses := []ModelClass{
		{
			Name: "lightweight",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.1:8b-instruct-q8_0"},
				{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"},
				{Provider: "openai", Model: "gpt-4o-mini"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"},
			},
		},
		{
			Name: "large-reasoning",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.3:70b-instruct-q4_K_M"},
				{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022"},
				{Provider: "openai", Model: "gpt-4o"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo"},
			},
		},
		{
			Name: "vision-model",
			Alternatives: []AlternativeModelOption{
				{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022"},
				{Provider: "openai", Model: "gpt-4o"},
			},
		},
	}

	t.Run("finds exact match and returns first available alternative from same class", func(t *testing.T) {
		availableProviders := map[types.Provider]bool{
			"helix":     true,
			"anthropic": false,
			"openai":    false,
			"together":  false,
		}

		result := server.findModelSubstitution("together", "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", modelClasses, availableProviders)

		require.NotNil(t, result)
		require.Equal(t, "helix", result.Provider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", result.Model)
	})

	t.Run("skips unavailable providers in same class", func(t *testing.T) {
		availableProviders := map[types.Provider]bool{
			"helix":     false, // First option unavailable
			"anthropic": true,  // Second option available
			"openai":    false,
			"together":  false,
		}

		result := server.findModelSubstitution("together", "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", modelClasses, availableProviders)

		require.NotNil(t, result)
		require.Equal(t, "anthropic", result.Provider)
		require.Equal(t, "claude-3-5-haiku-20241022", result.Model)
	})

	t.Run("returns nil when no providers available in same class", func(t *testing.T) {
		availableProviders := map[types.Provider]bool{
			"helix":     false,
			"anthropic": false,
			"openai":    false,
			"together":  false,
		}

		result := server.findModelSubstitution("together", "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", modelClasses, availableProviders)

		require.Nil(t, result)
	})

	t.Run("returns nil when model not found in any class", func(t *testing.T) {
		availableProviders := map[types.Provider]bool{
			"helix":     false,
			"anthropic": true,
			"openai":    false,
			"together":  false,
		}

		// Use a model that doesn't exist in any class - should NOT fall back
		result := server.findModelSubstitution("unknown", "unknown-model", modelClasses, availableProviders)

		require.Nil(t, result)
	})

	t.Run("returns nil when no alternatives available at all", func(t *testing.T) {
		availableProviders := map[types.Provider]bool{}

		result := server.findModelSubstitution("together", "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", modelClasses, availableProviders)

		require.Nil(t, result)
	})

	t.Run("handles empty model classes", func(t *testing.T) {
		availableProviders := map[types.Provider]bool{
			"helix": true,
		}

		result := server.findModelSubstitution("together", "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", []ModelClass{}, availableProviders)

		require.Nil(t, result)
	})

	t.Run("matches different class correctly", func(t *testing.T) {
		availableProviders := map[types.Provider]bool{
			"helix":     true,
			"anthropic": false,
			"openai":    false,
			"together":  false,
		}

		// Test model that appears in multiple classes - gpt-4o appears in both large-reasoning and vision-model
		// Since lightweight class is checked first and helix is available in large-reasoning, it should return helix alternative
		result := server.findModelSubstitution("openai", "gpt-4o", modelClasses, availableProviders)

		require.NotNil(t, result) // Should find helix alternative from large-reasoning class (processed first)
		require.Equal(t, "helix", result.Provider)
		require.Equal(t, "llama3.3:70b-instruct-q4_K_M", result.Model)
	})
}

func TestApplyModelSubstitutions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProviderManager := manager.NewMockProviderManager(ctrl)
	server := &HelixAPIServer{
		providerManager: mockProviderManager,
	}

	modelClasses := []ModelClass{
		{
			Name: "lightweight",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.1:8b-instruct-q8_0"},
				{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"},
			},
		},
	}

	user := &types.User{ID: "user1"}
	ctx := context.Background()

	t.Run("substitutes model when provider unavailable", func(t *testing.T) {
		// Mock provider manager to return only "helix" as available
		mockProviderManager.EXPECT().
			ListProviders(ctx, user.ID).
			Return([]types.Provider{types.ProviderHelix}, nil)

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{
						{
							Name:     "test-assistant",
							Provider: "together",
							Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						},
					},
				},
			},
		}

		substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
		require.NoError(t, err)
		require.Len(t, substitutions, 1)
		require.Equal(t, "test-assistant", substitutions[0].AssistantName)
		require.Equal(t, "together", substitutions[0].OriginalProvider)
		require.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", substitutions[0].OriginalModel)
		require.Equal(t, "helix", substitutions[0].NewProvider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", substitutions[0].NewModel)

		// Verify the substitution occurred
		require.Equal(t, "helix", app.Config.Helix.Assistants[0].Provider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[0].Model)
	})

	t.Run("preserves all other fields during substitution", func(t *testing.T) {
		// Mock provider manager to return only "helix" as available
		mockProviderManager.EXPECT().
			ListProviders(ctx, user.ID).
			Return([]types.Provider{types.ProviderHelix}, nil)

		// Create an assistant with all possible fields populated
		originalAssistant := types.AssistantConfig{
			Name:        "test-assistant",
			Provider:    "together",
			Model:       "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
			Description: "A test assistant",
			ConversationStarters: []string{
				"Hello, how can I help?",
				"What would you like to know?",
			},
			APIs: []types.AssistantAPI{
				{
					Name:                    "Test API",
					Description:             "A test API",
					URL:                     "https://api.test.com",
					Schema:                  "openapi: 3.0.0\ninfo:\n  title: Test API",
					RequestPrepTemplate:     "Test request template",
					ResponseSuccessTemplate: "Test success template",
					ResponseErrorTemplate:   "Test error template",
				},
			},
			Knowledge: []*types.AssistantKnowledge{
				{
					Name:        "Test Knowledge",
					Description: "Test knowledge base",
					Source: types.KnowledgeSource{
						Web: &types.KnowledgeSourceWeb{
							URLs: []string{"https://example.com"},
						},
					},
				},
			},
			MaxTokens:    1000,
			Temperature:  0.7,
			SystemPrompt: "You are a helpful assistant",
		}

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{originalAssistant},
				},
			},
		}

		// Apply substitutions
		substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
		require.NoError(t, err)
		require.Len(t, substitutions, 1)

		// Get the modified assistant
		assistant := app.Config.Helix.Assistants[0]

		// Verify ONLY provider and model were changed
		require.Equal(t, "helix", assistant.Provider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", assistant.Model)

		// Verify ALL other fields were preserved
		require.Equal(t, originalAssistant.Name, assistant.Name)
		require.Equal(t, originalAssistant.Description, assistant.Description)
		require.Equal(t, originalAssistant.ConversationStarters, assistant.ConversationStarters)
		require.Equal(t, originalAssistant.APIs, assistant.APIs)
		require.Equal(t, originalAssistant.Knowledge, assistant.Knowledge)
		require.Equal(t, originalAssistant.MaxTokens, assistant.MaxTokens)
		require.Equal(t, originalAssistant.Temperature, assistant.Temperature)
		require.Equal(t, originalAssistant.SystemPrompt, assistant.SystemPrompt)
	})

	t.Run("preserves original values when no substitution needed", func(t *testing.T) {
		// Mock provider manager to return "together" as available
		mockProviderManager.EXPECT().
			ListProviders(ctx, user.ID).
			Return([]types.Provider{"together"}, nil)

		originalAssistant := types.AssistantConfig{
			Name:                 "test-assistant",
			Provider:             "together",
			Model:                "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
			Description:          "Original description",
			ConversationStarters: []string{"Original starter"},
		}

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{originalAssistant},
				},
			},
		}

		substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
		require.NoError(t, err)
		require.Len(t, substitutions, 0) // No substitutions should have occurred

		// Verify NO provider/model changes occurred
		// Note: AgentType will be migrated to "helix_basic" by MigrateAgentMode()
		assistant := app.Config.Helix.Assistants[0]
		require.Equal(t, originalAssistant.Name, assistant.Name)
		require.Equal(t, originalAssistant.Provider, assistant.Provider)
		require.Equal(t, originalAssistant.Model, assistant.Model)
		require.Equal(t, originalAssistant.Description, assistant.Description)
		require.Equal(t, originalAssistant.ConversationStarters, assistant.ConversationStarters)
		require.Equal(t, types.AgentTypeHelixBasic, assistant.AgentType) // AgentType migrated
	})

	t.Run("handles multiple assistants independently", func(t *testing.T) {
		// Mock provider manager to return only "helix" as available
		mockProviderManager.EXPECT().
			ListProviders(ctx, user.ID).
			Return([]types.Provider{types.ProviderHelix}, nil)

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{
						{
							Name:                 "assistant-1",
							Provider:             "together",
							Model:                "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
							Description:          "First assistant",
							ConversationStarters: []string{"Hello from assistant 1"},
						},
						{
							Name:                 "assistant-2",
							Provider:             "helix", // Already using available provider
							Model:                "llama3.1:8b-instruct-q8_0",
							Description:          "Second assistant",
							ConversationStarters: []string{"Hello from assistant 2"},
						},
					},
				},
			},
		}

		substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
		require.NoError(t, err)
		require.Len(t, substitutions, 1) // Only one substitution should have occurred

		// Verify first assistant was substituted
		require.Equal(t, "helix", app.Config.Helix.Assistants[0].Provider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[0].Model)
		require.Equal(t, "First assistant", app.Config.Helix.Assistants[0].Description)
		require.Equal(t, []string{"Hello from assistant 1"}, app.Config.Helix.Assistants[0].ConversationStarters)

		// Verify second assistant was unchanged
		require.Equal(t, "helix", app.Config.Helix.Assistants[1].Provider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[1].Model)
		require.Equal(t, "Second assistant", app.Config.Helix.Assistants[1].Description)
		require.Equal(t, []string{"Hello from assistant 2"}, app.Config.Helix.Assistants[1].ConversationStarters)
	})
}

func TestCreateAppWithModelSubstitutions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProviderManager := manager.NewMockProviderManager(ctrl)
	server := &HelixAPIServer{
		providerManager: mockProviderManager,
	}

	ctx := context.Background()
	user := &types.User{ID: "user1"}

	// Model classes with substitution options
	modelClasses := []ModelClass{
		{
			Name: "lightweight",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.1:8b-instruct-q8_0"},
				{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"},
			},
		},
	}

	// Mock provider manager to return only "helix" as available (forcing substitution)
	mockProviderManager.EXPECT().
		ListProviders(ctx, user.ID).
		Return([]types.Provider{types.ProviderHelix}, nil)

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:     "test-assistant",
						Provider: "together",
						Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
					},
				},
			},
		},
	}

	substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
	require.NoError(t, err)

	// Verify that a substitution was recorded
	require.Len(t, substitutions, 1)
	assert.Equal(t, "test-assistant", substitutions[0].AssistantName)
	assert.Equal(t, "together", substitutions[0].OriginalProvider)
	assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", substitutions[0].OriginalModel)
	assert.Equal(t, "helix", substitutions[0].NewProvider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", substitutions[0].NewModel)
	assert.Equal(t, "Original provider 'together' not available for provider/model", substitutions[0].Reason)

	// Verify that the app was actually modified
	assert.Equal(t, "helix", app.Config.Helix.Assistants[0].Provider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[0].Model)
}

func TestApplyModelSubstitutions_AgentMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProviderManager := manager.NewMockProviderManager(ctrl)
	server := &HelixAPIServer{
		providerManager: mockProviderManager,
	}

	ctx := context.Background()
	user := &types.User{ID: "user1"}

	// Model classes with substitution options
	modelClasses := []ModelClass{
		{
			Name: "lightweight",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.1:8b-instruct-q8_0"},
				{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"},
			},
		},
		{
			Name: "large-reasoning",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.3:70b-instruct-q4_K_M"},
				{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo"},
			},
		},
	}

	// Mock provider manager to return only "helix" as available
	mockProviderManager.EXPECT().
		ListProviders(ctx, user.ID).
		Return([]types.Provider{types.ProviderHelix}, nil)

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:      "test-agent",
						Provider:  "together",
						Model:     "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						AgentMode: true,

						// Agent mode models using unavailable providers
						ReasoningModelProvider:       "together",
						ReasoningModel:               "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo",
						GenerationModelProvider:      "anthropic",
						GenerationModel:              "claude-3-5-haiku-20241022",
						SmallReasoningModelProvider:  "together",
						SmallReasoningModel:          "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						SmallGenerationModelProvider: "anthropic",
						SmallGenerationModel:         "claude-3-5-sonnet-20241022",
					},
				},
			},
		},
	}

	substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
	require.NoError(t, err)

	// Should have 3 substitutions (main provider/model, reasoning_model, and generation_model)
	// Note: reasoning_model gets substituted even though anthropic is available because
	// the substitution logic always uses the first available alternative from the same class
	require.Len(t, substitutions, 5) // All 5 model fields should be substituted

	// Verify the main provider/model substitution
	assert.Equal(t, "helix", app.Config.Helix.Assistants[0].Provider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[0].Model)

	// Verify agent mode model substitutions
	assistant := app.Config.Helix.Assistants[0]
	assert.Equal(t, "helix", assistant.ReasoningModelProvider)
	assert.Equal(t, "llama3.3:70b-instruct-q4_K_M", assistant.ReasoningModel)

	assert.Equal(t, "helix", assistant.GenerationModelProvider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", assistant.GenerationModel)

	assert.Equal(t, "helix", assistant.SmallReasoningModelProvider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", assistant.SmallReasoningModel)

	assert.Equal(t, "helix", assistant.SmallGenerationModelProvider)
	assert.Equal(t, "llama3.3:70b-instruct-q4_K_M", assistant.SmallGenerationModel)

	// Verify substitution records
	substitutionFields := make(map[string]bool)
	for _, sub := range substitutions {
		substitutionFields[sub.Reason] = true
		assert.Equal(t, "test-agent", sub.AssistantName)
	}

	// Check that all expected field substitutions were recorded
	assert.True(t, substitutionFields["Original provider 'together' not available for provider/model"])
	assert.True(t, substitutionFields["Original provider 'together' not available for reasoning_model"])
	assert.True(t, substitutionFields["Original provider 'anthropic' not available for generation_model"])
	assert.True(t, substitutionFields["Original provider 'together' not available for small_reasoning_model"])
	assert.True(t, substitutionFields["Original provider 'anthropic' not available for small_generation_model"])
}

func TestApplyModelSubstitutions_AgentModePartialSubstitution(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProviderManager := manager.NewMockProviderManager(ctrl)
	server := &HelixAPIServer{
		providerManager: mockProviderManager,
	}

	ctx := context.Background()
	user := &types.User{ID: "user1"}

	// Model classes with substitution options
	modelClasses := []ModelClass{
		{
			Name: "lightweight",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.1:8b-instruct-q8_0"},
				{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"},
			},
		},
	}

	// Mock provider manager to return "helix" and "anthropic" as available
	mockProviderManager.EXPECT().
		ListProviders(ctx, user.ID).
		Return([]types.Provider{types.ProviderHelix, types.ProviderAnthropic}, nil)

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:      "test-agent",
						Provider:  "together", // Will be substituted
						Model:     "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						AgentMode: true,

						// Mix of available and unavailable providers
						ReasoningModelProvider:       "anthropic", // Available - no substitution
						ReasoningModel:               "claude-3-5-haiku-20241022",
						GenerationModelProvider:      "together", // Unavailable - will be substituted
						GenerationModel:              "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						SmallReasoningModelProvider:  "", // Empty - no substitution
						SmallReasoningModel:          "",
						SmallGenerationModelProvider: "helix", // Available - no substitution
						SmallGenerationModel:         "llama3.1:8b-instruct-q8_0",
					},
				},
			},
		},
	}

	substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
	require.NoError(t, err)

	// Should only have 2 substitutions (main provider/model and generation_model)
	require.Len(t, substitutions, 2)

	// Verify the main provider/model substitution
	assert.Equal(t, "helix", app.Config.Helix.Assistants[0].Provider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[0].Model)

	// Verify agent mode model states
	assistant := app.Config.Helix.Assistants[0]

	// Reasoning model should remain unchanged (anthropic available)
	assert.Equal(t, "anthropic", assistant.ReasoningModelProvider)
	assert.Equal(t, "claude-3-5-haiku-20241022", assistant.ReasoningModel)

	// Generation model should be substituted (together not available)
	assert.Equal(t, "helix", assistant.GenerationModelProvider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", assistant.GenerationModel)

	// Small reasoning model should remain empty
	assert.Equal(t, "", assistant.SmallReasoningModelProvider)
	assert.Equal(t, "", assistant.SmallReasoningModel)

	// Small generation model should remain unchanged (helix available)
	assert.Equal(t, "helix", assistant.SmallGenerationModelProvider)
	assert.Equal(t, "llama3.1:8b-instruct-q8_0", assistant.SmallGenerationModel)
}

func TestO3MiniSubstitution(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProviderManager := manager.NewMockProviderManager(ctrl)
	server := &HelixAPIServer{
		providerManager: mockProviderManager,
	}

	ctx := context.Background()
	user := &types.User{ID: "user1"}

	// Model classes with o3-mini in dedicated reasoning class
	modelClasses := []ModelClass{
		{
			Name: "lightweight",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.1:8b-instruct-q8_0"},
				{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"},
				{Provider: "openai", Model: "gpt-4o-mini"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo"},
			},
		},
		{
			Name: "reasoning",
			Alternatives: []AlternativeModelOption{
				{Provider: "openai", Model: "o3-mini"},
				{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022"},
				{Provider: "openai", Model: "gpt-4o"},
				{Provider: "helix", Model: "llama3.3:70b-instruct-q4_K_M"},
			},
		},
		{
			Name: "large-reasoning",
			Alternatives: []AlternativeModelOption{
				{Provider: "helix", Model: "llama3.3:70b-instruct-q4_K_M"},
				{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022"},
				{Provider: "openai", Model: "gpt-4o"},
				{Provider: "openai", Model: "o3-mini"},
				{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo"},
			},
		},
	}

	// Mock provider manager to return only "helix" and "anthropic" as available (no openai)
	mockProviderManager.EXPECT().
		ListProviders(ctx, user.ID).
		Return([]types.Provider{types.ProviderHelix, types.ProviderAnthropic}, nil)

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:                         "test-agent",
						Provider:                     "helix",
						Model:                        "llama3.1:8b-instruct-q8_0",
						AgentMode:                    true,
						ReasoningModelProvider:       "openai",
						ReasoningModel:               "o3-mini",
						GenerationModelProvider:      "openai",
						GenerationModel:              "gpt-4o",
						SmallReasoningModelProvider:  "openai",
						SmallReasoningModel:          "o3-mini",
						SmallGenerationModelProvider: "openai",
						SmallGenerationModel:         "gpt-4o",
					},
				},
			},
		},
	}

	substitutions, err := server.applyModelSubstitutions(ctx, user, app, modelClasses)
	require.NoError(t, err)

	// Should have 4 substitutions (reasoning_model, generation_model, small_reasoning_model, small_generation_model)
	// The main provider/model (helix) is already available so no substitution needed
	require.Len(t, substitutions, 4)

	// Verify that o3-mini was found and substituted correctly
	assistant := app.Config.Helix.Assistants[0]

	// o3-mini should be substituted with anthropic model (first available alternative in reasoning class)
	assert.Equal(t, "anthropic", assistant.ReasoningModelProvider)
	assert.Equal(t, "claude-3-5-sonnet-20241022", assistant.ReasoningModel)

	// gpt-4o should also be substituted with anthropic model (both are in reasoning class)
	assert.Equal(t, "anthropic", assistant.GenerationModelProvider)
	assert.Equal(t, "claude-3-5-sonnet-20241022", assistant.GenerationModel)

	// small_reasoning_model (o3-mini) should also be substituted with anthropic
	assert.Equal(t, "anthropic", assistant.SmallReasoningModelProvider)
	assert.Equal(t, "claude-3-5-sonnet-20241022", assistant.SmallReasoningModel)

	// small_generation_model (gpt-4o) should also be substituted with anthropic
	assert.Equal(t, "anthropic", assistant.SmallGenerationModelProvider)
	assert.Equal(t, "claude-3-5-sonnet-20241022", assistant.SmallGenerationModel)

	// Verify the substitution records
	var o3MiniSubstitutions []ModelSubstitution
	var gpt4oSubstitutions []ModelSubstitution

	for _, sub := range substitutions {
		if sub.OriginalModel == "o3-mini" {
			o3MiniSubstitutions = append(o3MiniSubstitutions, sub)
		}
		if sub.OriginalModel == "gpt-4o" {
			gpt4oSubstitutions = append(gpt4oSubstitutions, sub)
		}
	}

	// Should have 2 o3-mini substitutions (reasoning_model and small_reasoning_model)
	require.Len(t, o3MiniSubstitutions, 2)

	// Should have 2 gpt-4o substitutions (generation_model and small_generation_model)
	require.Len(t, gpt4oSubstitutions, 2)

	// Verify o3-mini substitutions use anthropic (first available in reasoning class)
	for _, sub := range o3MiniSubstitutions {
		assert.Equal(t, "anthropic", sub.NewProvider)
		assert.Equal(t, "claude-3-5-sonnet-20241022", sub.NewModel)
	}

	// Verify gpt-4o substitutions also use anthropic (both models are in reasoning class)
	for _, sub := range gpt4oSubstitutions {
		assert.Equal(t, "anthropic", sub.NewProvider)
		assert.Equal(t, "claude-3-5-sonnet-20241022", sub.NewModel)
	}
}
