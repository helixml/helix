package apps

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
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

	// Parse the yaml using shared config processor
	// this will handle both AppHelixConfig & AppHelixConfigCRD
	app, err := config.ProcessYAMLConfig(yamlFile)
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
