package azure

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// AzureDevOps is triggered by webhooks from Azure DevOps
// ref: https://learn.microsoft.com/en-us/azure/devops/service-hooks/services/webhooks?view=azure-devops
type AzureDevOps struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *AzureDevOps {

	return &AzureDevOps{
		cfg:        cfg,
		store:      store,
		controller: controller,
	}
}

func (a *AzureDevOps) ProcessWebhook(ctx context.Context, triggerConfig *types.TriggerConfiguration, payload []byte) error {
	log.Info().
		Str("trigger_config_id", triggerConfig.ID).
		Str("trigger_config_app_id", triggerConfig.AppID).
		Msgf("AzureDevOps: processing webhook")

	return nil
}
