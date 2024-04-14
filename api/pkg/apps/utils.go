package apps

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

func ValidateApp(app *types.App) error {
	if app.AppType == types.AppTypeGithub {
		if app.Config.Github.Repo == "" {
			return fmt.Errorf("github repo is required")
		}
	}
	return nil
}

func ValidateHelixConfig(config *types.AppHelixConfig) error {
	return nil
}
