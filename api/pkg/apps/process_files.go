package apps

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// processLocalFiles takes an AppHelixConfig and a base directory path,
// and processes any file references (API schemas) relative to that directory,
// loading the contents of the files into the config.
func processLocalFiles(config *types.AppHelixConfig, basePath string) error {
	if config.Assistants == nil {
		config.Assistants = []types.AssistantConfig{}
	}

	for i := range config.Assistants {
		assistant := &config.Assistants[i]

		// Initialize empty slices
		if assistant.APIs == nil {
			assistant.APIs = []types.AssistantAPI{}
		}

		// Process API schemas
		for j, api := range assistant.APIs {
			if api.Schema == "" {
				return fmt.Errorf("api %s has no schema", api.Name)
			}

			schema, err := processSchemaContent(api.Schema, basePath)
			if err != nil {
				return fmt.Errorf("error processing assistant %s api schema: %w", assistant.ID, err)
			}
			assistant.APIs[j].Schema = schema

			if api.Headers == nil {
				assistant.APIs[j].Headers = map[string]string{}
			}
			if api.Query == nil {
				assistant.APIs[j].Query = map[string]string{}
			}
		}
	}

	return nil
}

func processSchemaContent(schemaPath, basePath string) (string, error) {
	if strings.HasPrefix(strings.ToLower(schemaPath), "http://") || strings.HasPrefix(strings.ToLower(schemaPath), "https://") {
		client := system.NewRetryClient(3, false)
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

		fullPath := filepath.Join(basePath, schemaPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return "", fmt.Errorf("failed to read schema file: %w", err)
		}
		return string(content), nil
	}

	return schemaPath, nil
}
