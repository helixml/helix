package apps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

				// APIs should be present as defined in YAML
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api", api.URL)

				// Tools should be empty since none were defined
				assert.Empty(t, assistant.Tools)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Write test YAML to temporary file
			yamlPath := filepath.Join(tmpDir, "test.yaml")
			err := os.WriteFile(yamlPath, []byte(tc.yamlData), 0644)
			require.NoError(t, err)

			// Create LocalApp
			localApp, err := NewLocalApp(yamlPath)
			require.NoError(t, err)

			// Validate the config
			tc.validate(t, localApp.GetAppConfig())
		})
	}
}

func TestLocalAppWithSchemaFile(t *testing.T) {
	// Create a temporary directory for our test files
	tmpDir, err := os.MkdirTemp("", "helix-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create the schema file
	schemaContent := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      summary: Test endpoint
      responses:
        '200':
          description: OK`

	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
	err = os.WriteFile(schemaPath, []byte(schemaContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create the helix config file that references the schema
	configContent := `assistants:
- id: test-assistant
  name: Test Assistant
  model: gpt-4
  apis:
  - name: test-api
    description: A test API
    url: http://test-api
    schema: test-schema.yaml`

	configPath := filepath.Join(tmpDir, "helix.yaml")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Load and test the config
	app, err := NewLocalApp(configPath)
	assert.NoError(t, err)
	assert.NotNil(t, app)

	// Verify the schema was loaded correctly
	assert.Len(t, app.app.Assistants, 1)
	assert.Len(t, app.app.Assistants[0].APIs, 1)
	assert.Equal(t, schemaContent, app.app.Assistants[0].APIs[0].Schema)
}
