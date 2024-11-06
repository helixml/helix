package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoadingAppsWithDifferentToolConfigs(t *testing.T) {
	testCases := []struct {
		name      string
		id        string
		storedApp string // JSON string
		validate  func(*testing.T, *types.App)
	}{
		{
			name: "api tools defined in assistant.Tools",
			id:   "test-id-1",
			storedApp: `{
				"id": "test-id-1",
				"created": "2024-01-01T00:00:00Z",
				"updated": "2024-01-01T00:00:00Z",
				"owner": "test-owner",
				"owner_type": "user",
				"config": {
					"helix": {
						"name": "Test App",
						"assistants": [{
							"id": "test-assistant",
							"name": "Test Assistant",
							"model": "gpt-4",
							"tools": [{
								"name": "test-api",
								"description": "Test API",
								"tool_type": "api",
								"config": {
									"api": {
										"url": "http://example.com/api",
										"schema": "openapi: 3.0.0\ninfo:\n  title: Test API\n  version: 1.0.0",
										"headers": {
											"Authorization": "Bearer test"
										}
									}
								}
							}]
						}]
					}
				}
			}`,
			validate: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]
				require.Len(t, assistant.Tools, 1)
				assert.Empty(t, assistant.APIs)

				tool := assistant.Tools[0]
				assert.Equal(t, "test-api", tool.Name)
				assert.Equal(t, types.ToolTypeAPI, tool.ToolType)
				require.NotNil(t, tool.Config.API)
				assert.Equal(t, "http://example.com/api", tool.Config.API.URL)
			},
		},
		{
			name: "api tools defined in assistant.APIs",
			id:   "test-id-2",
			storedApp: `{
				"id": "test-id-2",
				"created": "2024-01-01T00:00:00Z",
				"updated": "2024-01-01T00:00:00Z",
				"owner": "test-owner",
				"owner_type": "user",
				"config": {
					"helix": {
						"name": "Test App",
						"assistants": [{
							"id": "test-assistant",
							"name": "Test Assistant",
							"model": "gpt-4",
							"apis": [{
								"name": "test-api",
								"description": "Test API",
								"url": "http://example.com/api",
								"schema": "openapi: 3.0.0\ninfo:\n  title: Test API\n  version: 1.0.0",
								"headers": {
									"Authorization": "Bearer test"
								}
							}]
						}]
					}
				}
			}`,
			validate: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]

				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api", api.URL)

				require.Len(t, assistant.Tools, 1)
				tool := assistant.Tools[0]
				assert.Equal(t, "test-api", tool.Name)
				assert.Equal(t, types.ToolTypeAPI, tool.ToolType)
				require.NotNil(t, tool.Config.API)
				assert.Equal(t, "http://example.com/api", tool.Config.API.URL)
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := store.NewMockStore(ctrl)

			// Set up mock to return the raw JSON bytes
			mockStore.EXPECT().
				GetApp(gomock.Any(), tc.id).
				DoAndReturn(func(_ context.Context, _ string) (*types.App, error) {
					var app types.App
					err := json.Unmarshal([]byte(tc.storedApp), &app)
					if err != nil {
						return nil, err
					}
					return &app, nil
				})

			// Load the app from the store
			loadedApp, err := mockStore.GetApp(context.Background(), tc.id)
			require.NoError(t, err)

			// Validate the loaded app
			tc.validate(t, loadedApp)
		})
	}
}

func TestNewLocalApp(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "localapp-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		name     string
		yamlData string
		validate func(*testing.T, *types.AppHelixConfig)
	}{
		{
			name: "api tools defined in assistant.Tools",
			yamlData: `
assistants:
- id: test-assistant
  name: Test Assistant
  model: gpt-4
  tools:
  - name: test-api
    description: Test API
    toolType: api
    config:
      api:
        url: http://example.com/api
        schema: |
          openapi: 3.0.0
          info:
            title: Test API
            version: 1.0.0
        headers:
          Authorization: Bearer test
`,
			validate: func(t *testing.T, config *types.AppHelixConfig) {
				require.Len(t, config.Assistants, 1)
				assistant := config.Assistants[0]

				// Tools field should contain the tool
				require.Len(t, assistant.Tools, 1)
				tool := assistant.Tools[0]
				assert.Equal(t, "test-api", tool.Name)
				assert.Equal(t, types.ToolTypeAPI, tool.ToolType)
				require.NotNil(t, tool.Config.API)
				assert.Equal(t, "http://example.com/api", tool.Config.API.URL)

				// APIs field should be empty since tools are in Tools field
				assert.Empty(t, assistant.APIs)
			},
		},
		{
			name: "api tools defined in assistant.APIs",
			yamlData: `
assistants:
- id: test-assistant
  name: Test Assistant
  model: gpt-4
  apis:
  - name: test-api
    description: Test API
    url: http://example.com/api
    schema: |
      openapi: 3.0.0
      info:
        title: Test API
        version: 1.0.0
    headers:
      Authorization: Bearer test
`,
			validate: func(t *testing.T, config *types.AppHelixConfig) {
				require.Len(t, config.Assistants, 1)
				assistant := config.Assistants[0]

				// Both APIs and Tools fields should be populated
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api", api.URL)

				require.Len(t, assistant.Tools, 1)
				tool := assistant.Tools[0]
				assert.Equal(t, "test-api", tool.Name)
				assert.Equal(t, types.ToolTypeAPI, tool.ToolType)
				require.NotNil(t, tool.Config.API)
				assert.Equal(t, "http://example.com/api", tool.Config.API.URL)
			},
		},
		{
			name: "mixed tools and apis definition",
			yamlData: `
assistants:
- id: test-assistant
  name: Test Assistant
  model: gpt-4
  apis:
  - name: api-1
    description: API 1
    url: http://example.com/api1
    schema: |
      openapi: 3.0.0
      info:
        title: API 1
        version: 1.0.0
  tools:
  - name: api-2
    description: API 2
    toolType: api
    config:
      api:
        url: http://example.com/api2
        schema: |
          openapi: 3.0.0
          info:
            title: API 2
            version: 1.0.0
`,
			validate: func(t *testing.T, config *types.AppHelixConfig) {
				require.Len(t, config.Assistants, 1)
				assistant := config.Assistants[0]

				// Both APIs and Tools fields should be populated
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "api-1", api.Name)

				// Tools field should contain both APIs
				require.Len(t, assistant.Tools, 2)

				// Find tools by name
				var api1Tool, api2Tool *types.Tool
				for _, tool := range assistant.Tools {
					if tool.Name == "api-1" {
						api1Tool = tool
					} else if tool.Name == "api-2" {
						api2Tool = tool
					}
				}

				require.NotNil(t, api1Tool)
				require.NotNil(t, api2Tool)
				assert.Equal(t, types.ToolTypeAPI, api1Tool.ToolType)
				assert.Equal(t, types.ToolTypeAPI, api2Tool.ToolType)
				assert.Equal(t, "http://example.com/api1", api1Tool.Config.API.URL)
				assert.Equal(t, "http://example.com/api2", api2Tool.Config.API.URL)
			},
		},
	}

	// Create a mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := store.NewMockStore(ctrl)

			// Set up mock for API create and get
			var storedApp *types.App

			mockStore.EXPECT().
				CreateApp(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, app *types.App) (*types.App, error) {
					app.ID = "test-id"
					storedApp = app
					return app, nil
				})

			mockStore.EXPECT().
				GetApp(gomock.Any(), "test-id").
				Return(storedApp, nil).
				AnyTimes()

			// Create test server with actual handlers
			router := mux.NewRouter()
			srv := httptest.NewServer(router)
			defer srv.Close()

			// Register app handlers
			router.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
				var app types.App
				err := json.NewDecoder(r.Body).Decode(&app)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				createdApp, err := mockStore.CreateApp(r.Context(), &app)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				json.NewEncoder(w).Encode(createdApp)
			}).Methods("POST")

			router.HandleFunc("/api/v1/apps/{id}", func(w http.ResponseWriter, r *http.Request) {
				vars := mux.Vars(r)
				app, err := mockStore.GetApp(r.Context(), vars["id"])
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				json.NewEncoder(w).Encode(app)
			}).Methods("GET")

			// Write test YAML to temporary file
			yamlPath := filepath.Join(tmpDir, "test.yaml")
			err := os.WriteFile(yamlPath, []byte(tc.yamlData), 0644)
			require.NoError(t, err)

			// Create LocalApp
			localApp, err := NewLocalApp(yamlPath)
			require.NoError(t, err)

			// Validate the parsed config
			tc.validate(t, localApp.GetAppConfig())

			// Create app via HTTP API
			app := &types.App{
				Owner:     "test-owner",
				OwnerType: "user",
				Config: types.AppConfig{
					Helix: *localApp.GetAppConfig(),
				},
			}

			appJSON, err := json.Marshal(app)
			require.NoError(t, err)

			resp, err := http.Post(srv.URL+"/api/v1/apps", "application/json", bytes.NewBuffer(appJSON))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var createdApp types.App
			err = json.NewDecoder(resp.Body).Decode(&createdApp)
			require.NoError(t, err)
			resp.Body.Close()

			// Load via HTTP API
			resp, err = http.Get(srv.URL + "/api/v1/apps/" + createdApp.ID)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var loadedApp types.App
			err = json.NewDecoder(resp.Body).Decode(&loadedApp)
			require.NoError(t, err)
			resp.Body.Close()

			// Validate the loaded config matches original
			tc.validate(t, &loadedApp.Config.Helix)
		})
	}
}

func TestUpdatingAppsWithDifferentToolConfigs(t *testing.T) {
	testCases := []struct {
		name            string
		id              string
		initialApp      string // JSON string
		updateApp       string // JSON string
		validateInitial func(*testing.T, *types.App)
		validateUpdated func(*testing.T, *types.App)
	}{
		{
			name: "update app with tools to apis",
			id:   "test-id-1",
			initialApp: `{
				"id": "test-id-1",
				"created": "2024-01-01T00:00:00Z",
				"updated": "2024-01-01T00:00:00Z",
				"owner": "test-owner",
				"owner_type": "user",
				"config": {
					"helix": {
						"name": "Test App",
						"assistants": [{
							"id": "test-assistant",
							"name": "Test Assistant",
							"model": "gpt-4",
							"tools": [{
								"name": "test-api",
								"description": "Test API",
								"tool_type": "api",
								"config": {
									"api": {
										"url": "http://example.com/api/v1",
										"schema": "openapi: 3.0.0\ninfo:\n  title: Test API\n  version: 1.0.0",
										"headers": {
											"Authorization": "Bearer test"
										}
									}
								}
							}]
						}]
					}
				}
			}`,
			updateApp: `{
				"id": "test-id-1",
				"created": "2024-01-01T00:00:00Z",
				"updated": "2024-01-01T00:00:00Z",
				"owner": "test-owner",
				"owner_type": "user",
				"config": {
					"helix": {
						"name": "Test App",
						"assistants": [{
							"id": "test-assistant",
							"name": "Test Assistant",
							"model": "gpt-4",
							"apis": [{
								"name": "test-api",
								"description": "Test API",
								"url": "http://example.com/api/v2",
								"schema": "openapi: 3.0.0\ninfo:\n  title: Test API\n  version: 1.0.0",
								"headers": {
									"Authorization": "Bearer test"
								}
							}]
						}]
					}
				}
			}`,
			validateInitial: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]

				// Should only have tools, no APIs
				assert.Empty(t, assistant.APIs)
				require.Len(t, assistant.Tools, 1)

				tool := assistant.Tools[0]
				assert.Equal(t, "test-api", tool.Name)
				assert.Equal(t, types.ToolTypeAPI, tool.ToolType)
				require.NotNil(t, tool.Config.API)
				assert.Equal(t, "http://example.com/api/v1", tool.Config.API.URL)
			},
			validateUpdated: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]

				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api/v2", api.URL)

				require.Len(t, assistant.Tools, 1)
				tool := assistant.Tools[0]
				assert.Equal(t, "test-api", tool.Name)
				assert.Equal(t, types.ToolTypeAPI, tool.ToolType)
				require.NotNil(t, tool.Config.API)
				assert.Equal(t, "http://example.com/api/v2", tool.Config.API.URL)
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := store.NewMockStore(ctrl)

			// Set up mock to return initial app
			mockStore.EXPECT().
				GetApp(gomock.Any(), tc.id).
				DoAndReturn(func(_ context.Context, _ string) (*types.App, error) {
					var app types.App
					err := json.Unmarshal([]byte(tc.initialApp), &app)
					if err != nil {
						return nil, err
					}
					return &app, nil
				})

			// Set up mock for update
			mockStore.EXPECT().
				UpdateApp(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, app *types.App) (*types.App, error) {
					return app, nil
				})

			// Load initial app
			initialApp, err := mockStore.GetApp(context.Background(), tc.id)
			require.NoError(t, err)

			// Validate initial state
			tc.validateInitial(t, initialApp)

			// Update the app
			var updateApp types.App
			err = json.Unmarshal([]byte(tc.updateApp), &updateApp)
			require.NoError(t, err)

			updatedApp, err := mockStore.UpdateApp(context.Background(), &updateApp)
			require.NoError(t, err)

			// Validate the updated app
			tc.validateUpdated(t, updatedApp)
		})
	}
}

func TestRectifyingAppsOnRead(t *testing.T) {
	testCases := []struct {
		name           string
		id             string
		storedApp      string                       // JSON string with tools
		expectedApp    string                       // JSON string without tools, converted to APIs
		validateSaved  func(*testing.T, *types.App) // validate the app was saved back to store
		validateLoaded func(*testing.T, *types.App) // validate the app returned to caller
	}{
		{
			name: "convert tools to apis on read",
			id:   "test-id-1",
			storedApp: `{
				"id": "test-id-1",
				"created": "2024-01-01T00:00:00Z",
				"updated": "2024-01-01T00:00:00Z",
				"owner": "test-owner",
				"owner_type": "user",
				"config": {
					"helix": {
						"name": "Test App",
						"assistants": [{
							"id": "test-assistant",
							"name": "Test Assistant",
							"model": "gpt-4",
							"tools": [{
								"name": "test-api",
								"description": "Test API",
								"tool_type": "api",
								"config": {
									"api": {
										"url": "http://example.com/api/v1",
										"schema": "openapi: 3.0.0\ninfo:\n  title: Test API\n  version: 1.0.0",
										"headers": {
											"Authorization": "Bearer test"
										}
									}
								}
							}]
						}]
					}
				}
			}`,
			expectedApp: `{
				"id": "test-id-1",
				"created": "2024-01-01T00:00:00Z",
				"updated": "2024-01-01T00:00:00Z",
				"owner": "test-owner",
				"owner_type": "user",
				"config": {
					"helix": {
						"name": "Test App",
						"assistants": [{
							"id": "test-assistant",
							"name": "Test Assistant",
							"model": "gpt-4",
							"apis": [{
								"name": "test-api",
								"description": "Test API",
								"url": "http://example.com/api/v1",
								"schema": "openapi: 3.0.0\ninfo:\n  title: Test API\n  version: 1.0.0",
								"headers": {
									"Authorization": "Bearer test"
								}
							}]
						}]
					}
				}
			}`,
			validateSaved: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]

				// Tools should be empty after rectification
				assert.Empty(t, assistant.Tools, "tools should be empty after rectification")

				// APIs should contain the converted tool
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api/v1", api.URL)
			},
			validateLoaded: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]

				// Tools should be empty in returned app
				assert.Empty(t, assistant.Tools, "tools should be empty in returned app")

				// APIs should contain the converted tool
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api/v1", api.URL)
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := store.NewMockStore(ctrl)

			// First GetApp call returns the stored version with tools
			mockStore.EXPECT().
				GetApp(gomock.Any(), tc.id).
				DoAndReturn(func(_ context.Context, _ string) (*types.App, error) {
					var app types.App
					err := json.Unmarshal([]byte(tc.storedApp), &app)
					if err != nil {
						return nil, err
					}
					return &app, nil
				})

			// UpdateApp call should receive rectified version
			mockStore.EXPECT().
				UpdateApp(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, app *types.App) (*types.App, error) {
					tc.validateSaved(t, app)
					return app, nil
				})

			// Load the app
			loadedApp, err := mockStore.GetApp(context.Background(), tc.id)
			require.NoError(t, err)

			// Validate the loaded app
			tc.validateLoaded(t, loadedApp)
		})
	}

	// Test ListApps rectification
	t.Run("rectify on list", func(t *testing.T) {
		mockStore := store.NewMockStore(ctrl)

		// ListApps should return apps with tools
		mockStore.EXPECT().
			ListApps(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *store.ListAppsQuery) ([]*types.App, error) {
				var apps []*types.App
				for _, tc := range testCases {
					var app types.App
					err := json.Unmarshal([]byte(tc.storedApp), &app)
					require.NoError(t, err)
					apps = append(apps, &app)
				}
				return apps, nil
			})

		// Each app should be saved back rectified
		for range testCases {
			mockStore.EXPECT().
				UpdateApp(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, app *types.App) (*types.App, error) {
					// Tools should be empty after rectification
					for _, assistant := range app.Config.Helix.Assistants {
						assert.Empty(t, assistant.Tools, "tools should be empty after rectification")
					}
					return app, nil
				})
		}

		// List the apps
		apps, err := mockStore.ListApps(context.Background(), &store.ListAppsQuery{})
		require.NoError(t, err)

		// Validate each app
		require.Len(t, apps, len(testCases))
		for _, app := range apps {
			for _, assistant := range app.Config.Helix.Assistants {
				assert.Empty(t, assistant.Tools, "tools should be empty in returned apps")
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api/v1", api.URL)
			}
		}
	})
}
