package server

import (
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listAppTriggers godoc
// @Summary List app triggers
// @Description List triggers for the app
// @Tags    apps
// @Success 200 {array} types.TriggerConfiguration
// @Param app_id path string true "App ID"
// @Router /api/v1/apps/{app_id}/triggers [get]
// @Security BearerAuth
func (s *HelixAPIServer) listAppTriggers(_ http.ResponseWriter, r *http.Request) ([]*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	id := getID(r)
	user := getRequestUser(r)

	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	err = s.authorizeUserToApp(r.Context(), user, app, types.ActionDelete)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	triggers, err := s.Store.ListTriggerConfigurations(ctx, &store.ListTriggerConfigurationsQuery{
		AppID:          id,
		OrganizationID: app.OrganizationID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Populate WebhookURL for applicable triggers
	for idx, trigger := range triggers {
		if trigger.Trigger.AzureDevOps != nil && trigger.Trigger.AzureDevOps.Enabled {
			triggers[idx].WebhookURL = fmt.Sprintf("%s/api/v1/webhooks/%s", s.Cfg.WebServer.URL, trigger.ID)
		}
	}

	return triggers, nil
}
