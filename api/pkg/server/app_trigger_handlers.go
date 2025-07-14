package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
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
		Owner:          user.ID, // Loading user created triggers
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

// createAppTriggers godoc
// @Summary Create app triggers
// @Description Create triggers for the app. Used to create standalone trigger configurations such as cron tasks for agents that could be owned by a different user than the owner of the app
// @Tags    apps
// @Success 200 {object} types.TriggerConfiguration
// @Param app_id path string true "App ID"
// @Param request body types.TriggerConfiguration true "Trigger configuration"
// @Router /api/v1/apps/{app_id}/triggers [post]
// @Security BearerAuth
func (s *HelixAPIServer) createAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	appID := getID(r)
	user := getRequestUser(r)

	// Get the app to verify it exists and for authorization
	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Authorization is based only on whether the user can "read" the app as triggers
	// are owned by the user
	err = s.authorizeUserToApp(ctx, user, app, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Parse the request body to get trigger configurations
	var triggerConfig *types.TriggerConfiguration
	if err := json.NewDecoder(r.Body).Decode(&triggerConfig); err != nil {
		return nil, system.NewHTTPError400("Invalid request body")
	}

	// Set the app ID and organization ID
	triggerConfig.AppID = appID
	triggerConfig.OrganizationID = app.OrganizationID
	triggerConfig.Owner = user.ID
	triggerConfig.OwnerType = types.OwnerTypeUser

	// Create the trigger configuration
	created, err := s.Store.CreateTriggerConfiguration(ctx, triggerConfig)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Populate WebhookURL for applicable triggers
	if created.Trigger.AzureDevOps != nil && created.Trigger.AzureDevOps.Enabled {
		created.WebhookURL = fmt.Sprintf("%s/api/v1/webhooks/%s", s.Cfg.WebServer.URL, created.ID)
	}

	return created, nil
}

// deleteAppTriggers godoc
// @Summary Delete app triggers
// @Description Delete triggers for the app
// @Tags    apps
// @Success 200 {object} types.TriggerConfiguration
// @Param app_id path string true "App ID"
// @Param trigger_id path string true "Trigger ID"
// @Router /api/v1/apps/{app_id}/triggers/{trigger_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	appID := getID(r)
	user := getRequestUser(r)
	vars := mux.Vars(r)
	triggerID := vars["trigger_id"]

	// Get the app to verify it exists and for authorization
	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Authorization is based only on whether the user can "read" the app as triggers
	// are owned by the user
	err = s.authorizeUserToApp(ctx, user, app, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Get the trigger configuration to verify it exists
	triggerConfig, err := s.Store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{
		ID:             triggerID,
		OrganizationID: app.OrganizationID,
	})
	if err != nil {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	// Verify the trigger configuration belongs to the app
	if triggerConfig.AppID != appID {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	// Delete the trigger configuration
	err = s.Store.DeleteTriggerConfiguration(ctx, triggerID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Return the deleted trigger configuration
	return triggerConfig, nil
}

// updateAppTriggers godoc
// @Summary Update app triggers
// @Description Update triggers for the app, for example to change the cron schedule or enable/disable the trigger
// @Tags    apps
// @Success 200 {object} types.TriggerConfiguration
// @Param app_id path string true "App ID"
// @Param trigger_id path string true "Trigger ID"
// @Param request body types.TriggerConfiguration true "Trigger configuration"
// @Router /api/v1/apps/{app_id}/triggers/{trigger_id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	appID := getID(r)
	user := getRequestUser(r)
	vars := mux.Vars(r)
	triggerID := vars["trigger_id"]

	// Get the app to verify it exists and for authorization
	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Authorize user to update triggers for this app
	err = s.authorizeUserToApp(ctx, user, app, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Get the existing trigger configuration
	existingTrigger, err := s.Store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{
		ID:             triggerID,
		OrganizationID: app.OrganizationID,
	})
	if err != nil {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	// Verify the trigger configuration belongs to the app
	if existingTrigger.AppID != appID {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	// Parse the request body to get the updated trigger configuration
	var updatedTrigger types.TriggerConfiguration
	if err := json.NewDecoder(r.Body).Decode(&updatedTrigger); err != nil {
		return nil, system.NewHTTPError400("Invalid request body")
	}

	// Update the trigger configuration fields
	existingTrigger.Name = updatedTrigger.Name
	existingTrigger.Trigger = updatedTrigger.Trigger

	// Update the trigger configuration
	updated, err := s.Store.UpdateTriggerConfiguration(ctx, existingTrigger)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Populate WebhookURL for applicable triggers
	if updated.Trigger.AzureDevOps != nil && updated.Trigger.AzureDevOps.Enabled {
		updated.WebhookURL = fmt.Sprintf("%s/api/v1/webhooks/%s", s.Cfg.WebServer.URL, updated.ID)
	}

	return updated, nil
}
