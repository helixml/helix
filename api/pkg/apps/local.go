package apps

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v2"
)

// LocalApp parses a local file and returns the configured
// app. It reads the yaml, then looks up the assistant API configuration
// to also get the tools configuration if needed
type LocalApp struct {
	filename string
	app      *types.AppHelixConfig
}

func NewLocalApp(filename string) (*LocalApp, error) {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file %s does not exist", filename)
		}

		return nil, fmt.Errorf("error checking if file %s exists: %w", filename, err)
	}

	// Read the file
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filename, err)
	}

	// Parse the yaml
	app, err := processConfig(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("error processing config file %s: %w", filename, err)
	}

	var apiTools []types.Tool

	for _, assistant := range app.Assistants {
		for _, api := range assistant.APIs {
			schema, err := processApiSchema(filename, api.Schema)
			if err != nil {
				return nil, fmt.Errorf("error processing assistant %s api schema: %w", assistant.ID, err)
			}
			apiTools = append(apiTools, types.Tool{
				Name:        api.Name,
				Description: api.Description,
				ToolType:    types.ToolTypeAPI,
				Config: types.ToolConfig{
					API: &types.ToolApiConfig{
						URL:                     api.URL,
						Schema:                  schema,
						Headers:                 api.Headers,
						Query:                   api.Query,
						RequestPrepTemplate:     api.RequestPrepTemplate,
						ResponseSuccessTemplate: api.ResponseSuccessTemplate,
						ResponseErrorTemplate:   api.ResponseErrorTemplate,
					},
				},
			})
		}
	}

	return &LocalApp{
		filename: filename,
		app:      app,
	}, nil
}

func (a *LocalApp) GetAppConfig() *types.AppHelixConfig {
	return a.app
}

func processConfig(yamlFile []byte) (*types.AppHelixConfig, error) {
	var app types.AppHelixConfig
	err := yaml.Unmarshal(yamlFile, &app)
	if err != nil {
		return nil, fmt.Errorf("error parsing yaml file: %w", err)
	}

	return &app, nil
}

func processApiSchema(configPath, schemaPath string) (string, error) {
	if strings.HasPrefix(strings.ToLower(schemaPath), "http://") || strings.HasPrefix(strings.ToLower(schemaPath), "https://") {
		client := system.NewRetryClient(3)
		resp, err := client.Get(schemaPath)
		if err != nil {
			return "", fmt.Errorf("failed to get schema from URL: %w", err)
		}
		defer resp.Body.Close()
		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}
		return string(bts), nil
	}

	// if the schema is only one line then assume it's a file path
	if !strings.Contains(schemaPath, "\n") && !strings.Contains(schemaPath, "\r") {
		// it must be a YAML file
		if !strings.HasSuffix(schemaPath, ".yaml") && !strings.HasSuffix(schemaPath, ".yml") {
			return "", fmt.Errorf("schema must be in yaml format")
		}

		// Find schemaFile relative to the configPath
		schemaPath = filepath.Join(filepath.Dir(configPath), schemaPath)

		content, err := os.ReadFile(schemaPath)
		if err != nil {
			return "", fmt.Errorf("failed to read schema file: %w", err)
		}
		return string(content), nil
	}

	return schemaPath, nil
}
