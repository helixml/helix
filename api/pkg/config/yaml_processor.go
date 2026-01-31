package config

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v2"
)

// ProcessYAMLConfig processes YAML content and returns an AppHelixConfig
// This handles both AppHelixConfig & AppHelixConfigCRD formats
func ProcessYAMLConfig(yamlContent []byte) (*types.AppHelixConfig, error) {
	// First, unmarshal as generic map to check structure
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(yamlContent, &rawMap); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return ProcessJSONConfig(rawMap)
}

// ProcessJSONConfig processes a parsed JSON/YAML map and returns an AppHelixConfig
// This handles both AppHelixConfig & AppHelixConfigCRD formats
func ProcessJSONConfig(rawMap map[string]interface{}) (*types.AppHelixConfig, error) {
	// Check if it has the CRD structure
	_, hasAPIVersion := rawMap["apiVersion"]
	_, hasKind := rawMap["kind"]
	_, hasSpec := rawMap["spec"]

	isCRD := hasAPIVersion && hasKind && hasSpec

	if isCRD {
		// If it looks like a CRD, we must treat it as one
		var crd types.AppHelixConfigCRD

		// Convert map to JSON then to struct for type safety
		rawBytes, err := yaml.Marshal(rawMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal raw map: %w", err)
		}

		if err := yaml.Unmarshal(rawBytes, &crd); err != nil {
			return nil, fmt.Errorf("file appears to be a CRD but failed to parse: %w", err)
		}

		spec := crd.Spec
		// If metadata.name is set, use it to overwrite spec.Name
		if crd.Metadata.Name != "" {
			spec.Name = crd.Metadata.Name
		}

		// Migrate agent types for backward compatibility
		spec.MigrateAgentTypes()

		return &spec, nil
	}

	// Not a CRD, try to unmarshal as AppHelixConfig
	var config types.AppHelixConfig

	// Convert map to JSON then to struct for type safety
	rawBytes, err := yaml.Marshal(rawMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw map: %w", err)
	}

	if err := yaml.Unmarshal(rawBytes, &config); err != nil {
		return nil, fmt.Errorf("error parsing as AppHelixConfig: %w", err)
	}

	// Migrate agent types for backward compatibility
	config.MigrateAgentTypes()

	return &config, nil
}
