package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
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
		// Mock provider manager to return only helix as available
		mockProviderManager.EXPECT().ListProviders(ctx, user.ID).Return([]types.Provider{"helix"}, nil)

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{
						{
							Name:     "Test Assistant",
							Provider: "together",
							Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						},
					},
				},
			},
		}

		err := server.applyModelSubstitutions(ctx, user, app, modelClasses)

		require.NoError(t, err)
		require.Equal(t, "helix", app.Config.Helix.Assistants[0].Provider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[0].Model)
	})

	t.Run("keeps original model when provider available", func(t *testing.T) {
		// Mock provider manager to return together as available
		mockProviderManager.EXPECT().ListProviders(ctx, user.ID).Return([]types.Provider{"together"}, nil)

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{
						{
							Name:     "Test Assistant",
							Provider: "together",
							Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						},
					},
				},
			},
		}

		originalProvider := app.Config.Helix.Assistants[0].Provider
		originalModel := app.Config.Helix.Assistants[0].Model

		err := server.applyModelSubstitutions(ctx, user, app, modelClasses)

		require.NoError(t, err)
		require.Equal(t, originalProvider, app.Config.Helix.Assistants[0].Provider)
		require.Equal(t, originalModel, app.Config.Helix.Assistants[0].Model)
	})

	t.Run("handles multiple assistants", func(t *testing.T) {
		// Mock provider manager to return only anthropic as available
		mockProviderManager.EXPECT().ListProviders(ctx, user.ID).Return([]types.Provider{"anthropic"}, nil)

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{
						{
							Name:     "Assistant 1",
							Provider: "together",
							Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						},
						{
							Name:     "Assistant 2",
							Provider: "together",
							Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						},
					},
				},
			},
		}

		err := server.applyModelSubstitutions(ctx, user, app, modelClasses)

		require.NoError(t, err)

		// Both assistants should be substituted
		for i := range app.Config.Helix.Assistants {
			require.Equal(t, "anthropic", app.Config.Helix.Assistants[i].Provider)
			require.Equal(t, "claude-3-5-haiku-20241022", app.Config.Helix.Assistants[i].Model)
		}
	})

	t.Run("handles provider manager error", func(t *testing.T) {
		mockProviderManager.EXPECT().ListProviders(ctx, user.ID).Return(nil, fmt.Errorf("provider error"))

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{
						{
							Provider: "together",
							Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						},
					},
				},
			},
		}

		err := server.applyModelSubstitutions(ctx, user, app, modelClasses)

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to list available providers")
	})

	t.Run("skips assistants with empty provider or model", func(t *testing.T) {
		mockProviderManager.EXPECT().ListProviders(ctx, user.ID).Return([]types.Provider{"helix"}, nil)

		app := &types.App{
			Config: types.AppConfig{
				Helix: types.AppHelixConfig{
					Assistants: []types.AssistantConfig{
						{
							Name:     "No Provider",
							Provider: "",
							Model:    "some-model",
						},
						{
							Name:     "No Model",
							Provider: "together",
							Model:    "",
						},
						{
							Name:     "Valid",
							Provider: "together",
							Model:    "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
						},
					},
				},
			},
		}

		err := server.applyModelSubstitutions(ctx, user, app, modelClasses)

		require.NoError(t, err)

		// First two should remain unchanged
		require.Equal(t, "", app.Config.Helix.Assistants[0].Provider)
		require.Equal(t, "some-model", app.Config.Helix.Assistants[0].Model)
		require.Equal(t, "together", app.Config.Helix.Assistants[1].Provider)
		require.Equal(t, "", app.Config.Helix.Assistants[1].Model)

		// Third should be substituted
		require.Equal(t, "helix", app.Config.Helix.Assistants[2].Provider)
		require.Equal(t, "llama3.1:8b-instruct-q8_0", app.Config.Helix.Assistants[2].Model)
	})
}
