package apps

import (
	"fmt"
	"os"

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
