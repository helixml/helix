package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger/cron"
	"github.com/helixml/helix/api/pkg/types"
)

// listTriggers godoc
// @Summary List all triggers configurations for either user or the org or user within an org
// @Description List all triggers configurations for either user or the org or user within an org
// @Tags    apps
// @Success 200 {array} types.TriggerConfiguration
// @Param org_id query string false "Organization ID"
// @Param trigger_type query string false "Trigger type, defaults to 'cron'"
// @Router /api/v1/triggers [get]
// @Security BearerAuth
func (s *HelixAPIServer) listTriggers(_ http.ResponseWriter, r *http.Request) ([]*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	orgID := r.URL.Query().Get("org_id")
	triggerTypeStr := r.URL.Query().Get("trigger_type")

	var triggerType types.TriggerType

	if triggerTypeStr != "" {
		triggerType = types.TriggerType(triggerTypeStr)
	} else {
		triggerType = types.TriggerTypeCron
	}

	triggers, err := s.Store.ListTriggerConfigurations(ctx, &store.ListTriggerConfigurationsQuery{
		OrganizationID: orgID,
		Owner:          user.ID,
		TriggerType:    triggerType,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// For cron triggers populate next run information
	for idx, trigger := range triggers {
		if trigger.Trigger.Cron != nil {
			triggers[idx].OK = true
			triggers[idx].Status = cron.NextRunFormatted(trigger.Trigger.Cron)
		}
	}

	var filtered []*types.TriggerConfiguration

	for _, trigger := range triggers {
		if orgID == "" {
			// If org ID is not specified, only show triggers that are owned by the user and
			// not attached to the orr
			if trigger.Owner == user.ID && trigger.OrganizationID == orgID {
				filtered = append(filtered, trigger)
			}
		} else {
			// If org ID is specified, only show triggers that are attached to the org
			if trigger.OrganizationID == orgID {
				filtered = append(filtered, trigger)
			}
		}

	}

	return filtered, nil
}

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
// @Param request body types.TriggerConfiguration true "Trigger configuration"
// @Router /api/v1/triggers [post]
// @Security BearerAuth
func (s *HelixAPIServer) createAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	// Parse the request body to get trigger configurations
	var triggerConfig *types.TriggerConfiguration
	if err := json.NewDecoder(r.Body).Decode(&triggerConfig); err != nil {
		return nil, system.NewHTTPError400("Invalid request body")
	}

	if triggerConfig.AppID == "" {
		return nil, system.NewHTTPError400("App ID is required")
	}

	// Get the app to verify it exists and for authorization
	app, err := s.Store.GetApp(ctx, triggerConfig.AppID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Authorization is based only on whether the user can "read" the app as triggers
	// are owned by the user
	err = s.authorizeUserToApp(ctx, user, app, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Set the app ID and organization ID
	triggerConfig.AppID = app.ID
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
// @Param trigger_id path string true "Trigger ID"
// @Router /api/v1/triggers/{trigger_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	triggerID := vars["trigger_id"]

	// Get the trigger configuration to verify it exists
	triggerConfig, err := s.Store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{
		ID:    triggerID,
		Owner: user.ID,
	})
	if err != nil {
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
// @Param trigger_id path string true "Trigger ID"
// @Param request body types.TriggerConfiguration true "Trigger configuration"
// @Router /api/v1/triggers/{trigger_id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	triggerID := vars["trigger_id"]

	// Parse the request body to get the updated trigger configuration
	var updatedTrigger types.TriggerConfiguration
	if err := json.NewDecoder(r.Body).Decode(&updatedTrigger); err != nil {
		return nil, system.NewHTTPError400("Invalid request body")
	}

	// Get the app to verify it exists and for authorization
	app, err := s.Store.GetApp(ctx, updatedTrigger.AppID)
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
		ID:    triggerID,
		Owner: user.ID,
	})
	if err != nil {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	// Update the trigger configuration fields
	existingTrigger.Name = updatedTrigger.Name
	existingTrigger.Trigger = updatedTrigger.Trigger
	existingTrigger.Archived = updatedTrigger.Archived
	existingTrigger.Enabled = updatedTrigger.Enabled
	existingTrigger.AppID = updatedTrigger.AppID

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

// executeAppTrigger godoc
// @Summary Execute app trigger
// @Description Update triggers for the app, for example to change the cron schedule or enable/disable the trigger
// @Tags    apps
// @Success 200 {object} types.TriggerExecuteResponse
// @Param trigger_id path string true "Trigger ID"
// @Router /api/v1/triggers/{trigger_id}/execute [post]
// @Security BearerAuth
func (s *HelixAPIServer) executeAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerExecuteResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	triggerID := vars["trigger_id"]

	// Get the trigger configuration to verify it exists
	triggerConfig, err := s.Store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{
		ID:    triggerID,
		Owner: user.ID,
	})
	if err != nil {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	app, err := s.Store.GetAppWithTools(ctx, triggerConfig.AppID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Authorize user to execute triggers for this app
	err = s.authorizeUserToApp(ctx, user, app, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Execute the trigger
	response, err := cron.ExecuteCronTask(ctx, s.Store, s.Controller, s.Controller.Options.Notifier, app, user.ID, triggerID, triggerConfig.Trigger.Cron, triggerConfig.Name)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return &types.TriggerExecuteResponse{
		SessionID: response,
	}, nil
}

// listTriggerExecutions godoc
// @Summary List trigger executions
// @Description List executions for the trigger
// @Tags    apps
// @Success 200 {array} types.TriggerExecution
// @Param trigger_id path string true "Trigger ID"
// @Param offset query int false "Offset"
// @Param limit query int false "Limit"
// @Router /api/v1/triggers/{trigger_id}/executions [get]
// @Security BearerAuth
func (s *HelixAPIServer) listTriggerExecutions(_ http.ResponseWriter, r *http.Request) ([]*types.TriggerExecution, *system.HTTPError) {
	ctx := r.Context()

	vars := mux.Vars(r)
	triggerID := vars["trigger_id"]

	user := getRequestUser(r)

	// Load trigger to verify it exists and for authorization
	_, err := s.Store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{
		ID:    triggerID,
		Owner: user.ID,
	})
	if err != nil {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	var (
		offset int
		limit  int
	)

	if offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)
		if err != nil {
			return nil, system.NewHTTPError400("Invalid offset")
		}
	}

	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			return nil, system.NewHTTPError400("Invalid limit")
		}
	}

	executions, err := s.Store.ListTriggerExecutions(ctx, &store.ListTriggerExecutionsQuery{
		TriggerID: triggerID,
		Offset:    offset,
		Limit:     limit,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return executions, nil
}
