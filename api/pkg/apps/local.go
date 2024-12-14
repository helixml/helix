package apps

import (
	"fmt"
	"os"
	"path/filepath"

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
	// this will handle both AppHelixConfig & AppHelixConfigCRD
	app, err := processConfig(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("error processing config file %s: %w", filename, err)
	}

	// Process any file references relative to the config file's directory
	err = processLocalFiles(app, filepath.Dir(filename))
	if err != nil {
		return nil, fmt.Errorf("error processing local files: %w", err)
	}

	return &LocalApp{
		filename: filename,
		app:      app,
	}, nil
}

func (a *LocalApp) GetAppConfig() *types.AppHelixConfig {
	return a.app
}

// Add this method to implement FilePathResolver
func (a *LocalApp) ResolvePath(path string) string {
	return filepath.Join(filepath.Dir(a.filename), path)
}

func processConfig(yamlFile []byte) (*types.AppHelixConfig, error) {
	// First, unmarshal as generic map to check structure
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(yamlFile, &rawMap); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Check if it has the CRD structure
	_, hasApiVersion := rawMap["apiVersion"]
	_, hasKind := rawMap["kind"]
	_, hasSpec := rawMap["spec"]

	isCRD := hasApiVersion && hasKind && hasSpec

	if isCRD {
		// If it looks like a CRD, we must treat it as one
		var crd types.AppHelixConfigCRD
		if err := yaml.Unmarshal(yamlFile, &crd); err != nil {
			return nil, fmt.Errorf("file appears to be a CRD but failed to parse: %w", err)
		}
		spec := crd.Spec
		// If metadata.name is set, use it to overwrite spec.Name
		if crd.Metadata.Name != "" {
			spec.Name = crd.Metadata.Name
		}
		return &spec, nil
	}

	// Not a CRD, try to unmarshal as AppHelixConfig
	var config types.AppHelixConfig
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		return nil, fmt.Errorf("error parsing yaml file as AppHelixConfig: %w", err)
	}

	return &config, nil
}
