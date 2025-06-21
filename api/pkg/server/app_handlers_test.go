package server

import (
	"context"
	"fmt"
	"testing"

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
