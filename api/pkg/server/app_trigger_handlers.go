package server

import (
	"context"
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

// requireTriggerAgentKind reports whether triggers may be attached to this agent.
//
// Both Helix and coding agents qualify. A cron trigger's default action runs the
// input as a session against a Helix agent; Action="spec_task" instead creates a spec
// task, and spec tasks REQUIRE a coding agent (see requireAgentKind in
// spec_driven_task_handlers.go). Pinning this surface to helix_agent alone therefore
// banned exactly the triggers that fire spec tasks — which HelixOS's bot schedules all
// are. The effect was not that they stopped running: already-created triggers kept
// firing, and became impossible to create, edit or disable through the API (spec 002867).
func requireTriggerAgentKind(app *types.Agent) error {
	if app == nil {
		return fmt.Errorf("agent is required")
	}
	switch app.AgentKind {
	case types.AgentKindHelix, types.AgentKindCoding:
		return nil
	default:
		return fmt.Errorf("agent triggers require agent kind %q or %q, got %q",
			types.AgentKindHelix, types.AgentKindCoding, app.AgentKind)
	}
}

// authorizeUserToTrigger allows a trigger's creator, or anyone who may delete the app
// it is attached to, to modify or remove it.
//
// The app arm matters: a trigger can outlive its creator's access (a rotated service
// key, a person who left), and without it such a trigger can never be cleaned up by
// anyone while the cron scheduler goes on firing it.
func (s *HelixAPIServer) authorizeUserToTrigger(ctx context.Context, user *types.User, trigger *types.TriggerConfiguration) error {
	if trigger.Owner == user.ID {
		return nil
	}
	app, err := s.Store.GetApp(ctx, trigger.AppID)
	if err != nil {
		return fmt.Errorf("could not load the agent this trigger belongs to: %w", err)
	}
	return s.authorizeUserToApp(ctx, user, app, types.ActionDelete)
}

// listTriggers godoc
// @Summary List all triggers configurations for either user or the org or user within an org
// @Description List all triggers configurations for either user or the org or user within an org
// @Tags    agents
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
// @Summary List agent triggers
// @Description List triggers for the agent
// @Tags    agents
// @Success 200 {array} types.TriggerConfiguration
// @Param agent_id path string true "Agent ID"
// @Router /api/v1/agents/{agent_id}/triggers [get]
// @Security BearerAuth
func (s *HelixAPIServer) listAppTriggers(_ http.ResponseWriter, r *http.Request) ([]*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	id := mux.Vars(r)["agent_id"]
	user := getRequestUser(r)

	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// A caller who may delete the app manages it, and sees EVERY trigger attached to
	// it. Anyone else sees only the ones they created.
	//
	// The owner filter used to apply unconditionally, which made a trigger on your own
	// app invisible to you the moment someone else created it — and createAppTrigger
	// only requires ActionGet, so anyone who can read an app can attach one. Nothing
	// could then list it, so nothing could prune it, while the cron scheduler
	// (getCronAppsFromTriggers) kept firing it: it selects every enabled cron trigger
	// globally, with no owner filter at all. That gap is how 22 HelixOS bot schedules
	// went on creating spec tasks for months after their owner's API key was rotated
	// to a different user — the reconciler that was supposed to prune them had become
	// a different caller and could no longer see its own past work (spec 002867).
	manageErr := s.authorizeUserToApp(r.Context(), user, app, types.ActionDelete)
	canManage := manageErr == nil
	if !canManage {
		if err := s.authorizeUserToApp(r.Context(), user, app, types.ActionGet); err != nil {
			return nil, system.NewHTTPError403(manageErr.Error())
		}
	}

	query := &store.ListTriggerConfigurationsQuery{
		AppID:          id,
		OrganizationID: app.OrganizationID,
	}
	if !canManage {
		query.Owner = user.ID // Loading user created triggers
	}

	triggers, err := s.Store.ListTriggerConfigurations(ctx, query)
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
// @Summary Create agent triggers
// @Description Create triggers for the agent. Used to create standalone trigger configurations such as cron tasks for agents that could be owned by a different user than the owner of the agent
// @Tags    agents
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
	if err := requireTriggerAgentKind(app); err != nil {
		return nil, system.NewHTTPError400(err.Error())
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
// @Summary Delete agent triggers
// @Description Delete triggers for the agent
// @Tags    agents
// @Success 200 {object} types.TriggerConfiguration
// @Param trigger_id path string true "Trigger ID"
// @Router /api/v1/triggers/{trigger_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteAppTrigger(_ http.ResponseWriter, r *http.Request) (*types.TriggerConfiguration, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	triggerID := vars["trigger_id"]

	// Get the trigger configuration to verify it exists. Looked up WITHOUT an owner
	// filter so an app's managers can clean up triggers other people attached to it;
	// authorization happens next.
	triggerConfig, err := s.Store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{
		ID: triggerID,
	})
	if err != nil {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	if err := s.authorizeUserToTrigger(ctx, user, triggerConfig); err != nil {
		// 404 rather than 403: the previous owner-scoped lookup returned "not found"
		// for anyone else's trigger, and leaking existence to an unauthorized caller
		// would be a regression.
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
// @Summary Update agent triggers
// @Description Update triggers for the agent, for example to change the cron schedule or enable/disable the trigger
// @Tags    agents
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

	// The body names the app the trigger should hang off after the update, so the
	// caller must be allowed to attach a trigger there — the same ActionGet bar
	// createAppTrigger applies. Authorization for the trigger ITSELF is separate and
	// happens below, against the app it currently belongs to.
	if err := s.authorizeUserToApp(ctx, user, app, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}
	if err := requireTriggerAgentKind(app); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Get the existing trigger configuration. No owner filter here — a trigger's own
	// creator may not hold ActionUpdate on the app it hangs off (createAppTrigger only
	// asks for ActionGet), so filtering by owner AND demanding ActionUpdate left
	// triggers that literally nobody could edit or disable. Authorization is done
	// against the trigger below instead.
	existingTrigger, err := s.Store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{
		ID: triggerID,
	})
	if err != nil {
		return nil, system.NewHTTPError404("Trigger configuration not found")
	}

	if err := s.authorizeUserToTrigger(ctx, user, existingTrigger); err != nil {
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
// @Summary Execute agent trigger
// @Description Update triggers for the agent, for example to change the cron schedule or enable/disable the trigger
// @Tags    agents
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
	response, err := cron.ExecuteCronTask(ctx, s.Store, s.Controller, s.Controller.Options.Notifier, s.specDrivenTaskService, s, app, user.ID, triggerID, triggerConfig.Trigger.Cron, triggerConfig.Name)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return response, nil
}

// listTriggerExecutions godoc
// @Summary List trigger executions
// @Description List executions for the trigger
// @Tags    agents
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
