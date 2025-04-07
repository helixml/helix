package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestTemplateAppEndpoints(t *testing.T) {
	// Create a new server
	server := &HelixAPIServer{}

	// Test list templates endpoint
	t.Run("ListTemplateApps", func(t *testing.T) {
		// Create a request to pass to our handler
		req, err := http.NewRequest("GET", "/api/v1/template_apps", nil)
		if err != nil {
			t.Fatal(err)
		}

		// Create a ResponseRecorder to record the response
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			templates, err := server.listTemplateApps(w, r)
			assert.NoError(t, err)

			// Write the response
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(templates)
			assert.NoError(t, err)
		})

		// Serve the request
		handler.ServeHTTP(rr, req)

		// Check the status code
		assert.Equal(t, http.StatusOK, rr.Code)

		// Parse the response
		var templates []*types.TemplateAppConfig
		err = json.Unmarshal(rr.Body.Bytes(), &templates)
		assert.NoError(t, err)

		// Verify we got some templates
		assert.NotEmpty(t, templates)
		assert.GreaterOrEqual(t, len(templates), 1)

		// Check if GitHub template is included
		var foundGitHub bool
		for _, template := range templates {
			if template.Type == types.TemplateAppTypeGitHub {
				foundGitHub = true
				break
			}
		}
		assert.True(t, foundGitHub, "GitHub template should be included")
	})

	// Test get template by type endpoint
	t.Run("GetTemplateApp", func(t *testing.T) {
		// Create a request to pass to our handler
		req, err := http.NewRequest("GET", "/api/v1/template_apps/github", nil)
		if err != nil {
			t.Fatal(err)
		}

		// Add the route parameters to the context
		router := mux.NewRouter()
		router.HandleFunc("/api/v1/template_apps/{type}", func(w http.ResponseWriter, r *http.Request) {
			// Pass the request to our handler
			template, err := server.getTemplateApp(w, r)
			assert.NoError(t, err)

			// Write the response
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(template)
			assert.NoError(t, err)
		})

		// Create a ResponseRecorder to record the response
		rr := httptest.NewRecorder()

		// Serve the request
		router.ServeHTTP(rr, req)

		// Check the status code
		assert.Equal(t, http.StatusOK, rr.Code)

		// Parse the response
		var template *types.TemplateAppConfig
		err = json.Unmarshal(rr.Body.Bytes(), &template)
		assert.NoError(t, err)

		// Verify the template
		assert.NotNil(t, template)
		assert.Equal(t, types.TemplateAppTypeGitHub, template.Type)
		assert.NotEmpty(t, template.Assistants)

		// Verify the GitHub API tool configuration
		assert.GreaterOrEqual(t, len(template.Assistants), 1)
		assert.GreaterOrEqual(t, len(template.Assistants[0].APIs), 1)
		assert.Equal(t, types.OAuthProviderTypeGitHub, template.Assistants[0].APIs[0].OAuthProvider)
	})

	// Test non-existent template
	t.Run("GetNonExistentTemplate", func(t *testing.T) {
		// Create a request to pass to our handler
		req, err := http.NewRequest("GET", "/api/v1/template_apps/nonexistent", nil)
		if err != nil {
			t.Fatal(err)
		}

		// Add the route parameters to the context
		router := mux.NewRouter()
		router.HandleFunc("/api/v1/template_apps/{type}", func(w http.ResponseWriter, r *http.Request) {
			// Pass the request to our handler
			template, err := server.getTemplateApp(w, r)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				err = json.NewEncoder(w).Encode(map[string]string{"error": "template not found"})
				assert.NoError(t, err)
				return
			}

			// Write the response
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(template)
			assert.NoError(t, err)
		})

		// Create a ResponseRecorder to record the response
		rr := httptest.NewRecorder()

		// Serve the request
		router.ServeHTTP(rr, req)

		// Check the status code
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	// Test creating app from template
	t.Run("CreateAppFromTemplate", func(t *testing.T) {
		// Get the GitHub template
		template := types.GetGitHubIssuesTemplate()
		assert.NotNil(t, template)

		// Create app config from template
		appConfig := types.CreateAppConfigFromTemplate(template)
		assert.NotNil(t, appConfig)

		// Verify the app configuration
		assert.Equal(t, template.Name, appConfig.Helix.Name)
		assert.Equal(t, template.Description, appConfig.Helix.Description)
		assert.Len(t, appConfig.Helix.Assistants, len(template.Assistants))

		// Verify the first assistant
		assert.Equal(t, template.Assistants[0].Name, appConfig.Helix.Assistants[0].Name)
		assert.Equal(t, template.Assistants[0].Description, appConfig.Helix.Assistants[0].Description)
		assert.Equal(t, template.Assistants[0].SystemPrompt, appConfig.Helix.Assistants[0].SystemPrompt)

		// Verify the tools
		assert.Len(t, appConfig.Helix.Assistants[0].Tools, 1)
		assert.Equal(t, template.Assistants[0].APIs[0].Name, appConfig.Helix.Assistants[0].Tools[0].Name)
		assert.Equal(t, template.Assistants[0].APIs[0].Description, appConfig.Helix.Assistants[0].Tools[0].Description)
		assert.Equal(t, template.Assistants[0].APIs[0].URL, appConfig.Helix.Assistants[0].Tools[0].Config.API.URL)
		assert.Equal(t, template.Assistants[0].APIs[0].OAuthProvider, appConfig.Helix.Assistants[0].Tools[0].Config.API.OAuthProvider)
	})

	// TestGitHubTemplateOAuth verifies that the GitHub template app has the correct OAuth provider configuration
	t.Run("TestGitHubTemplateOAuth", func(t *testing.T) {
		// Get the GitHub template
		template := types.GetGitHubIssuesTemplate()
		assert.NotNil(t, template)

		// Verify the template has OAuth provider configuration
		assert.Equal(t, types.TemplateAppTypeGitHub, template.Type)
		assert.Equal(t, "GitHub Repository Analyzer", template.Name)

		// Check that the template has assistants
		assert.NotEmpty(t, template.Assistants)

		// Check the first assistant has APIs
		assert.NotEmpty(t, template.Assistants[0].APIs)

		// Verify the OAuth configuration for GitHub API tool
		api := template.Assistants[0].APIs[0]
		assert.Equal(t, "GitHub API", api.Name)
		assert.Equal(t, types.OAuthProviderTypeGitHub, types.OAuthProviderType(api.OAuthProvider))
		assert.Contains(t, api.OAuthScopes, "repo")

		// Create app config from template
		appConfig := types.CreateAppConfigFromTemplate(template)
		assert.NotNil(t, appConfig)

		// Verify the OAuth configuration was correctly transferred to the app config
		assert.NotEmpty(t, appConfig.Helix.Assistants)
		assert.NotEmpty(t, appConfig.Helix.Assistants[0].Tools)
		apiTool := appConfig.Helix.Assistants[0].Tools[0]
		assert.Equal(t, types.ToolTypeAPI, apiTool.ToolType)
		assert.NotNil(t, apiTool.Config.API)
		assert.Equal(t, types.OAuthProviderTypeGitHub, types.OAuthProviderType(apiTool.Config.API.OAuthProvider))
		assert.Contains(t, apiTool.Config.API.OAuthScopes, "repo")
	})
}
