package server

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"encoding/base64"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// AlternativeModelOption represents a provider/model combination for fallback substitution
type AlternativeModelOption struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// applyModelSubstitutions attempts to substitute unavailable models with available alternatives
func (s *HelixAPIServer) applyModelSubstitutions(ctx context.Context, user *types.User, app *types.App, alternativeModels map[string][]AlternativeModelOption) error {
	// Get available providers for this user
	availableProviders, err := s.providerManager.ListProviders(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to list available providers: %w", err)
	}

	// Create a set of available providers for fast lookup
	providerSet := make(map[types.Provider]bool)
	for _, provider := range availableProviders {
		providerSet[provider] = true
	}

	// Check each assistant's model and substitute if necessary
	for i := range app.Config.Helix.Assistants {
		assistant := &app.Config.Helix.Assistants[i]

		// Check if current provider/model combination is available
		if assistant.Provider != "" && assistant.Model != "" {
			currentProvider := types.Provider(assistant.Provider)

			// If the current provider is not available, try to find a substitution
			if !providerSet[currentProvider] {
				log.Info().
					Str("assistant_name", assistant.Name).
					Str("original_provider", assistant.Provider).
					Str("original_model", assistant.Model).
					Msg("Original provider not available, attempting model substitution")

				// Try to find a substitution based on model class
				substituted := s.findModelSubstitution(assistant.Model, alternativeModels, providerSet)
				if substituted != nil {
					log.Info().
						Str("assistant_name", assistant.Name).
						Str("original_provider", assistant.Provider).
						Str("original_model", assistant.Model).
						Str("substituted_provider", substituted.Provider).
						Str("substituted_model", substituted.Model).
						Msg("Successfully substituted model")

					assistant.Provider = substituted.Provider
					assistant.Model = substituted.Model
				} else {
					log.Warn().
						Str("assistant_name", assistant.Name).
						Str("original_provider", assistant.Provider).
						Str("original_model", assistant.Model).
						Msg("No suitable model substitution found")
				}
			}
		}
	}

	return nil
}

// findModelSubstitution finds the first available model from the alternatives list
func (s *HelixAPIServer) findModelSubstitution(originalModel string, alternativeModels map[string][]AlternativeModelOption, availableProviders map[types.Provider]bool) *AlternativeModelOption {
	// Try to find alternatives for each model class
	for modelClass, alternatives := range alternativeModels {
		// Check if the original model matches this class (simple string contains check)
		// This could be enhanced with more sophisticated matching logic
		if strings.Contains(strings.ToLower(originalModel), strings.ToLower(modelClass)) ||
			strings.Contains(strings.ToLower(modelClass), strings.ToLower(originalModel)) {

			// Try each alternative in order of preference
			for _, alt := range alternatives {
				if availableProviders[types.Provider(alt.Provider)] {
					return &alt
				}
			}
		}
	}

	// If no class-specific match found, try to find any available alternative
	// by checking all alternatives and returning the first available one
	for _, alternatives := range alternativeModels {
		for _, alt := range alternatives {
			if availableProviders[types.Provider(alt.Provider)] {
				return &alt
			}
		}
	}

	return nil
}

// listApps godoc
// @Summary List apps
// @Description List apps for the user. Apps are pre-configured to spawn sessions with specific tools and config.
// @Tags    apps

// @Success 200 {array} types.App
// @Param organization_id query string false "Organization ID"
// @Router /api/v1/apps [get]
// @Security BearerAuth
func (s *HelixAPIServer) listApps(_ http.ResponseWriter, r *http.Request) ([]*types.App, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	orgID := r.URL.Query().Get("organization_id") // If filtering for a specific organization

	if orgID != "" {
		orgApps, err := s.listOrganizationApps(ctx, user, orgID)
		if err != nil {
			return nil, err
		}

		orgApps = s.populateAppOwner(ctx, orgApps)

		return orgApps, nil
	}

	userApps, err := s.Store.ListApps(ctx, &store.ListAppsQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// remove global apps from the list in case this is the admin user who created the global app
	nonGlobalUserApps := []*types.App{}
	for _, app := range userApps {
		if !app.Global {
			nonGlobalUserApps = append(nonGlobalUserApps, app)
		}
	}

	globalApps, err := s.Store.ListApps(r.Context(), &store.ListAppsQuery{
		Global: true,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	allApps := append(nonGlobalUserApps, globalApps...)

	filteredApps := s.populateAppOwner(ctx, allApps)

	return filteredApps, nil
}

func (s *HelixAPIServer) populateAppOwner(ctx context.Context, apps []*types.App) []*types.App {
	userMap := make(map[string]*types.User)

	for _, app := range apps {
		// Populate the user map if the user is not already in the map
		if _, ok := userMap[app.Owner]; !ok {
			appOwner, err := s.Store.GetUser(ctx, &store.GetUserQuery{ID: app.Owner})
			if err != nil {
				continue
			}

			userMap[app.Owner] = appOwner
		}
	}

	// Assign the user to the app
	for _, app := range apps {
		user, ok := userMap[app.Owner]
		if !ok {
			app.User = types.User{}
			continue
		}

		app.User = *user
	}

	return apps
}

// listOrganizationApps lists apps for an organization based on the user's access grants
func (s *HelixAPIServer) listOrganizationApps(ctx context.Context, user *types.User, orgID string) ([]*types.App, *system.HTTPError) {
	orgMembership, err := s.authorizeOrgMember(ctx, user, orgID)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	apps, err := s.Store.ListApps(ctx, &store.ListAppsQuery{
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// If user is the org owner, skip authorization, they can view all apps
	if orgMembership.Role == types.OrganizationRoleOwner {
		return apps, nil
	}

	// User is not the org owner, so we need to authorize them to each app

	var authorizedApps []*types.App

	// Authorize the user to each app
	for _, app := range apps {
		err := s.authorizeUserToApp(ctx, user, app, types.ActionGet)
		if err != nil {
			log.Debug().
				Str("user_id", user.ID).
				Str("app_id", app.ID).
				Str("action", types.ActionGet.String()).
				Msg("user is not authorized to view app")
			continue
		}

		// Get user for the app
		appOwner, err := s.Store.GetUser(ctx, &store.GetUserQuery{ID: app.Owner})
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		app.User = *appOwner

		authorizedApps = append(authorizedApps, app)
	}

	return authorizedApps, nil
}

// createApp godoc
// @Summary Create new app
// @Description Create new app. Helix apps are configured with tools and knowledge. Supports both legacy format and new structured format with YAML config.
// @Tags    apps

// @Success 200 {object} types.App
// @Param request    body types.App true "Request body with app configuration. Can be legacy App format or structured format with organization_id, global, and yaml_config fields.")
// @Router /api/v1/apps [post]
// @Security BearerAuth
func (s *HelixAPIServer) createApp(_ http.ResponseWriter, r *http.Request) (*types.App, *system.HTTPError) {
	user := getRequestUser(r)
	ctx := r.Context()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to read request body, error: %s", err))
	}

	// Try to unmarshal as structured request first
	var structuredReq struct {
		OrganizationID    string                              `json:"organization_id"`
		Global            bool                                `json:"global"`
		YamlConfig        map[string]interface{}              `json:"yaml_config"`
		AlternativeModels map[string][]AlternativeModelOption `json:"alternative_models"`
	}

	var app *types.App
	var alternativeModels map[string][]AlternativeModelOption

	// Try structured format first
	if err := json.Unmarshal(body, &structuredReq); err == nil && structuredReq.YamlConfig != nil {
		// Use shared config processor
		helixConfig, err := config.ProcessJSONConfig(structuredReq.YamlConfig)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to process YAML config: %s", err))
		}

		// Build the app structure
		app = &types.App{
			OrganizationID: structuredReq.OrganizationID,
			Global:         structuredReq.Global,
			Config: types.AppConfig{
				Helix:          *helixConfig,
				Secrets:        make(map[string]string),
				AllowedDomains: []string{},
			},
		}

		// Store alternative models for later use
		alternativeModels = structuredReq.AlternativeModels

	} else {
		// Fall back to legacy format
		if err := json.Unmarshal(body, &app); err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body as JSON, error: %s", err))
		}
	}

	// If organization ID is set, authorize the user to the organization,
	// must be a member to create it
	if app.OrganizationID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, app.OrganizationID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}
	}

	// Getting existing tools for the user
	existingApps, err := s.Store.ListApps(ctx, &store.ListAppsQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	app.ID = system.GenerateAppID()
	app.Owner = user.ID
	app.OwnerType = user.Type
	app.Updated = time.Now()

	// Apply model substitutions BEFORE validation if provided
	if len(alternativeModels) > 0 {
		err = s.applyModelSubstitutions(ctx, user, app, alternativeModels)
		if err != nil {
			log.Warn().
				Err(err).
				Str("app_name", app.Config.Helix.Name).
				Msg("Failed to apply model substitutions, proceeding with original models")
		}
	}

	err = s.validateProviderAndModel(ctx, user, app)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Handle duplicate names by adding suffixes like (1), (2), etc.
	if app.Config.Helix.Name != "" {
		originalName := app.Config.Helix.Name
		finalName := originalName
		counter := 1

		// Keep checking until we find an available name
		for {
			nameExists := false
			for _, a := range existingApps {
				if a.Config.Helix.Name == finalName {
					nameExists = true
					break
				}
			}

			if !nameExists {
				break
			}

			// Try the next suffix
			finalName = fmt.Sprintf("%s (%d)", originalName, counter)
			counter++
		}

		// Update the app name to the final available name
		app.Config.Helix.Name = finalName

		// Also update the assistant name to match if it was the same as app name
		for i := range app.Config.Helix.Assistants {
			if app.Config.Helix.Assistants[i].Name == originalName {
				app.Config.Helix.Assistants[i].Name = finalName
			}
		}
	}

	var created *types.App

	err = s.validateTriggers(app.Config.Helix.Triggers)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	app, err = store.ParseAppTools(app)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Validate and default tools
	for idx := range app.Config.Helix.Assistants {
		assistant := &app.Config.Helix.Assistants[idx]

		// Ensure we don't have tools with duplicate names
		toolNames := make(map[string]bool)
		for _, tool := range assistant.Tools {
			if toolNames[tool.Name] {
				return nil, system.NewHTTPError400(fmt.Sprintf("tool '%s' has a duplicate name", tool.Name))
			}
			toolNames[tool.Name] = true
		}

		for idx := range assistant.Tools {
			tool := assistant.Tools[idx]
			err = tools.ValidateTool(assistant, tool, s.Controller.ToolsPlanner, true)
			if err != nil {
				return nil, system.NewHTTPError400(err.Error())
			}
		}

		for _, k := range assistant.Knowledge {
			err = s.validateKnowledge(k)
			if err != nil {
				return nil, system.NewHTTPError400(err.Error())
			}
		}
	}

	created, err = s.Store.CreateApp(ctx, app)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().Str("app_id", created.ID).Msg("Created Helix app")

	err = s.ensureKnowledge(ctx, created)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	_, err = s.Controller.CreateAPIKey(ctx, user, &types.ApiKey{
		Name:  "api key 1",
		Type:  types.APIkeytypeApp,
		AppID: &sql.NullString{String: created.ID, Valid: true},
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return created, nil
}

// validateProviderAndModel checks if the provider and model are valid. Provider
// can be empty, however model is required
func (s *HelixAPIServer) validateProviderAndModel(ctx context.Context, user *types.User, app *types.App) error {
	if len(app.Config.Helix.Assistants) == 0 {
		return fmt.Errorf("app must have at least one assistant")
	}

	providers, err := s.providerManager.ListProviders(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	// Validate providers
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.Model == "" && !assistant.AgentMode {
			return fmt.Errorf("assistant '%s' must have a model", assistant.Name)
		}

		// If provider set, check if we have it
		if assistant.Provider != "" && !assistant.AgentMode {
			if !slices.Contains(providers, types.Provider(assistant.Provider)) {
				return fmt.Errorf("provider '%s' is not available", assistant.Provider)
			}
			// OK
		}
	}

	return nil
}

func (s *HelixAPIServer) validateKnowledge(k *types.AssistantKnowledge) error {
	return knowledge.Validate(s.Cfg, k)
}

func (s *HelixAPIServer) validateTriggers(triggers []types.Trigger) error {
	// If it's cron, check that it runs not more than once every 90 seconds
	for _, trigger := range triggers {
		if trigger.Cron != nil && trigger.Cron.Schedule != "" && trigger.Cron.Enabled {
			cronSchedule, err := cron.ParseStandard(trigger.Cron.Schedule)
			if err != nil {
				return fmt.Errorf("invalid cron schedule: %w", err)
			}

			nextRun := cronSchedule.Next(time.Now())
			secondRun := cronSchedule.Next(nextRun)
			if secondRun.Sub(nextRun) < 90*time.Second {
				return fmt.Errorf("cron trigger must not run more than once per 90 seconds")
			}
		}
	}
	return nil
}

// ensureKnowledge creates or updates knowledge config in the database
func (s *HelixAPIServer) ensureKnowledge(ctx context.Context, app *types.App) error {
	var knowledge []*types.AssistantKnowledge

	// Get knowledge for all assistants
	for _, assistant := range app.Config.Helix.Assistants {
		knowledge = append(knowledge, assistant.Knowledge...)
	}

	// Used to track which knowledges are declared in the app config
	// so we can delete knowledges that are no longer specified
	foundKnowledge := make(map[string]bool)

	for _, k := range knowledge {
		var scopedPath string

		// Scope the filestore path to the app's directory if it exists
		if k.Source.Filestore != nil && (k.Source.Filestore.Path != "" || k.Source.Filestore.SeedZipURL != "") {
			// Translate simple paths like "pdfs" to "apps/:app_id/pdfs"
			ownerCtx := types.OwnerContext{
				Owner:     app.Owner,
				OwnerType: app.OwnerType,
			}

			// Use provided path or default to "files" if only zip URL is provided
			knowledgePath := k.Source.Filestore.Path
			if knowledgePath == "" && k.Source.Filestore.SeedZipURL != "" {
				knowledgePath = "files"
			}

			var err error
			scopedPath, err = s.Controller.GetFilestoreAppKnowledgePath(ownerCtx, app.ID, knowledgePath)
			if err != nil {
				return fmt.Errorf("failed to generate scoped path for knowledge '%s': %w", k.Name, err)
			}

			// Remove the global prefix from the path to store only the relative path in the database
			appPrefix := filestore.GetAppPrefix(s.Cfg.Controller.FilePrefixGlobal, app.ID)
			relativePath := strings.TrimPrefix(scopedPath, appPrefix)
			relativePath = strings.TrimPrefix(relativePath, "/")

			// Update the source path to use the scoped path
			k.Source.Filestore.Path = relativePath
		}

		existing, err := s.Store.LookupKnowledge(ctx, &store.LookupKnowledgeQuery{
			AppID: app.ID,
			Name:  k.Name,
		})

		var zipDownloadErr error
		if k.Source.Filestore != nil && k.Source.Filestore.SeedZipURL != "" {
			zipDownloadErr = s.downloadAndExtractZipToKnowledge(ctx, k.Source.Filestore.SeedZipURL, scopedPath)
		}

		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Determine initial state - if zip download fails, start as error state
				initialState := determineInitialState(k.Source)
				var errorMessage string

				if zipDownloadErr != nil {
					log.Error().
						Err(zipDownloadErr).
						Str("knowledge_name", k.Name).
						Str("zip_url", k.Source.Filestore.SeedZipURL).
						Msg("Failed to download and extract zip file, marking knowledge as failed")
					initialState = types.KnowledgeStateError
					errorMessage = fmt.Sprintf("Failed to download files from zip URL: %s", zipDownloadErr.Error())
				} else if k.Source.Filestore != nil && k.Source.Filestore.SeedZipURL != "" {
					// If we have a seed zip URL and it was successfully downloaded, start in pending state for indexing
					log.Info().
						Str("knowledge_name", k.Name).
						Str("zip_url", k.Source.Filestore.SeedZipURL).
						Msg("Successfully seeded knowledge from zip URL, moving to pending state for indexing")
					initialState = types.KnowledgeStatePending
				}

				// Create new knowledge
				created, err := s.Store.CreateKnowledge(ctx, &types.Knowledge{
					AppID:           app.ID,
					Name:            k.Name,
					Description:     k.Description,
					Owner:           app.Owner,
					OwnerType:       app.OwnerType,
					State:           initialState,
					Message:         errorMessage,
					RAGSettings:     k.RAGSettings,
					Source:          k.Source,
					RefreshEnabled:  k.RefreshEnabled,
					RefreshSchedule: k.RefreshSchedule,
				})
				if err != nil {
					return fmt.Errorf("failed to create knowledge '%s': %w", k.Name, err)
				}
				// OK, continue
				foundKnowledge[created.ID] = true
				continue
			}
			return fmt.Errorf("failed to create knowledge '%s': %w", k.Name, err)
		}

		// Update existing knowledge
		existing.Description = k.Description
		existing.RAGSettings = k.RAGSettings
		existing.Source = k.Source
		existing.RefreshEnabled = k.RefreshEnabled
		existing.RefreshSchedule = k.RefreshSchedule

		// If this is an existing knowledge with a seed zip URL, check if we need to re-download and move to pending
		if existing.Source.Filestore != nil && existing.Source.Filestore.SeedZipURL != "" {
			if zipDownloadErr != nil {
				log.Error().
					Err(zipDownloadErr).
					Str("knowledge_name", existing.Name).
					Str("zip_url", existing.Source.Filestore.SeedZipURL).
					Msg("Failed to download and extract zip file for existing knowledge, marking as error")
				existing.State = types.KnowledgeStateError
				existing.Message = fmt.Sprintf("Failed to download files from zip URL: %s", zipDownloadErr.Error())
			} else if existing.State == types.KnowledgeStatePreparing || existing.State == types.KnowledgeStateError {
				// If existing knowledge was in preparing or error state and zip download succeeded, move to pending
				log.Info().
					Str("knowledge_name", existing.Name).
					Str("zip_url", existing.Source.Filestore.SeedZipURL).
					Msg("Successfully seeded existing knowledge from zip URL, moving to pending state for indexing")
				existing.State = types.KnowledgeStatePending
				existing.Message = ""
			}
		}

		_, err = s.Store.UpdateKnowledge(ctx, existing)
		if err != nil {
			return fmt.Errorf("failed to update knowledge '%s': %w", existing.Name, err)
		}

		foundKnowledge[existing.ID] = true
	}

	existingKnowledge, err := s.Store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		AppID: app.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to list knowledge for app '%s': %w", app.ID, err)
	}

	for _, k := range existingKnowledge {
		if !foundKnowledge[k.ID] {
			// Delete knowledge that is no longer specified in the app config
			err = s.deleteKnowledgeAndVersions(k)
			if err != nil {
				return fmt.Errorf("failed to delete knowledge '%s': %w", k.Name, err)
			}
		}
	}

	return nil
}

// determineInitialState decides whether a new knowledge source should start in the 'preparing' or 'pending' state
// Only file-based knowledge sources should start in 'preparing' state, all others should start in 'pending'
func determineInitialState(source types.KnowledgeSource) types.KnowledgeState {
	// If the source is file-based, it requires user to upload files first
	if source.Filestore != nil {
		return types.KnowledgeStatePreparing
	}
	// For all other sources (web, S3, GCS, direct content), start in pending state
	return types.KnowledgeStatePending
}

// what the user can change about a github app fromm the frontend
type AppUpdatePayload struct {
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	ActiveTools    []string          `json:"active_tools"`
	Secrets        map[string]string `json:"secrets"`
	AllowedDomains []string          `json:"allowed_domains"`
	Global         bool              `json:"global"`
}

// getApp godoc
// @Summary Get app by ID
// @Description Get app by ID.
// @Tags    apps

// @Success 200 {object} types.App
// @Param id path string true "App ID"
// @Router /api/v1/apps/{id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getApp(_ http.ResponseWriter, r *http.Request) (*types.App, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	err = s.authorizeUserToApp(r.Context(), user, app, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	return app, nil
}

// updateApp godoc
// @Summary Update an existing app
// @Description Update existing app
// @Tags    apps

// @Success 200 {object} types.App
// @Param request    body types.App true "Request body with app configuration.")
// @Param id path string true "Tool ID"
// @Router /api/v1/apps/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateApp(_ http.ResponseWriter, r *http.Request) (*types.App, *system.HTTPError) {
	user := getRequestUser(r)

	var update types.App
	err := json.NewDecoder(r.Body).Decode(&update)
	if err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body 2, error: %s", err))
	}

	// Getting existing app
	existing, err := s.Store.GetApp(r.Context(), update.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing == nil {
		return nil, system.NewHTTPError404(store.ErrNotFound.Error())
	}

	// Some fields are not allowed to be changed
	update.OrganizationID = existing.OrganizationID
	update.Owner = existing.Owner
	update.OwnerType = existing.OwnerType
	update.Created = existing.Created

	err = s.authorizeUserToApp(r.Context(), user, existing, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	err = s.validateProviderAndModel(r.Context(), user, &update)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	err = s.validateTriggers(update.Config.Helix.Triggers)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	updatedWithTools, err := store.ParseAppTools(&update)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	updatedWithTools.Updated = time.Now()

	// Validate and default tools
	for idx := range updatedWithTools.Config.Helix.Assistants {
		assistant := &update.Config.Helix.Assistants[idx]

		// Ensure we don't have tools with duplicate names
		toolNames := make(map[string]bool)
		for _, tool := range assistant.Tools {
			if toolNames[tool.Name] {
				return nil, system.NewHTTPError400(fmt.Sprintf("tool '%s' has a duplicate name", tool.Name))
			}
			toolNames[tool.Name] = true
		}

		for idx := range assistant.Tools {
			tool := assistant.Tools[idx]
			err = tools.ValidateTool(assistant, tool, s.Controller.ToolsPlanner, true)
			if err != nil {
				return nil, system.NewHTTPError400(err.Error())
			}
		}

		for _, k := range assistant.Knowledge {
			err = s.validateKnowledge(k)
			if err != nil {
				return nil, system.NewHTTPError400(err.Error())
			}
		}
	}

	// Updating the app
	updated, err := s.Store.UpdateApp(r.Context(), updatedWithTools)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	err = s.ensureKnowledge(r.Context(), updated)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return updated, nil
}

// deleteApp godoc
// @Summary Delete app
// @Description Delete app.
// @Tags    apps

// @Success 200
// @Param id path string true "App ID"
// @Router /api/v1/apps/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteApp(_ http.ResponseWriter, r *http.Request) (*types.App, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	// Users need to specify if they want to keep the knowledge, by default - deleting
	keepKnowledge := r.URL.Query().Get("keep_knowledge") == "true"

	existing, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	err = s.authorizeUserToApp(r.Context(), user, existing, types.ActionDelete)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	if !keepKnowledge {
		knowledge, err := s.Store.ListKnowledge(r.Context(), &store.ListKnowledgeQuery{
			AppID: id,
		})
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		for _, k := range knowledge {
			err = s.Store.DeleteKnowledge(r.Context(), k.ID)
			if err != nil {
				return nil, system.NewHTTPError500(err.Error())
			}
		}
	}

	err = s.Store.DeleteApp(r.Context(), id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return existing, nil
}

// getAppOAuthTokenEnv retrieves OAuth tokens for the app
func (s *HelixAPIServer) getAppOAuthTokenEnv(ctx context.Context, user *types.User, appRecord *types.App) map[string]string {
	oauthTokens := make(map[string]string)

	// Skip if OAuth manager is not available
	if s.oauthManager == nil {
		log.Debug().Msg("OAuth manager is not available, skipping token retrieval")
		return oauthTokens
	}

	log.Info().
		Str("app_id", appRecord.ID).
		Str("user_id", user.ID).
		Msg("Getting OAuth tokens for app")

	// DEBUG: Print available OAuth providers for diagnostics
	providers, err := s.Store.ListOAuthProviders(ctx, &store.ListOAuthProvidersQuery{
		Enabled: true,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list available OAuth providers")
	} else {
		providerStrings := make([]string, 0, len(providers))
		for _, p := range providers {
			providerStrings = append(providerStrings, p.Name)
		}
		log.Info().
			Strs("available_providers", providerStrings).
			Str("user_id", user.ID).
			Msg("Available OAuth providers for user")
	}

	// First check each tool for OAuth provider requirements
	for idx, assistant := range appRecord.Config.Helix.Assistants {
		log.Debug().
			Int("assistant_index", idx).
			Str("assistant_name", assistant.Name).
			Int("tool_count", len(assistant.Tools)).
			Msg("Checking assistant for OAuth tools")

		// Check each tool for OAuth requirements
		for tIdx, tool := range assistant.Tools {
			log.Debug().
				Int("tool_index", tIdx).
				Str("tool_name", tool.Name).
				Str("tool_type", string(tool.ToolType)).
				Interface("tool_config", tool.Config).
				Msg("Checking tool for OAuth requirements")

			if tool.ToolType == types.ToolTypeAPI && tool.Config.API != nil {
				log.Debug().
					Str("tool_name", tool.Name).
					Str("oauth_provider", tool.Config.API.OAuthProvider).
					Strs("oauth_scopes", tool.Config.API.OAuthScopes).
					Msg("Tool API configuration")

				if tool.Config.API.OAuthProvider != "" {
					providerName := tool.Config.API.OAuthProvider
					requiredScopes := tool.Config.API.OAuthScopes

					log.Debug().
						Str("provider", providerName).
						Strs("scopes", requiredScopes).
						Msg("Checking OAuth token for tool")

					// DEBUG: Check if user has connections for this provider
					connections, connErr := s.Store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{
						UserID:     user.ID,
						ProviderID: "", // We'll do filtering manually
					})
					if connErr != nil {
						log.Error().
							Err(connErr).
							Str("provider", providerName).
							Str("user_id", user.ID).
							Msg("Failed to list user connections for provider")
					} else {
						// Filter connections for this provider
						var matchingConnections []*types.OAuthConnection
						var matchingProvider *types.OAuthProvider
						for _, p := range providers {
							if p.Name == providerName {
								matchingProvider = p
								break
							}
						}

						if matchingProvider != nil {
							for _, conn := range connections {
								if conn.ProviderID == matchingProvider.ID {
									matchingConnections = append(matchingConnections, conn)
								}
							}

							log.Info().
								Str("provider", providerName).
								Str("provider_id", matchingProvider.ID).
								Str("user_id", user.ID).
								Int("connection_count", len(matchingConnections)).
								Msg("Found OAuth connections for provider")
						}
					}

					token, err := s.oauthManager.GetTokenForTool(ctx, user.ID, providerName, requiredScopes)
					if err == nil && token != "" {
						// Add the token directly to the map
						oauthTokens[providerName] = token
						log.Info().
							Str("provider", providerName).
							Str("provider_key", providerName).
							Str("token_prefix", token[:10]+"...").
							Msg("Added OAuth token to app environment")
					} else {
						var scopeErr *oauth.ScopeError
						if errors.As(err, &scopeErr) {
							log.Warn().
								Str("app_id", appRecord.ID).
								Str("user_id", user.ID).
								Str("provider", providerName).
								Strs("missing_scopes", scopeErr.Missing).
								Strs("required_scopes", requiredScopes).
								Msg("Missing required OAuth scopes for tool")
						} else {
							log.Warn().
								Err(err).
								Str("provider", providerName).
								Str("error_type", fmt.Sprintf("%T", err)).
								Msg("Failed to get OAuth token for tool")
						}
					}
				}
			}
		}
	}

	// Log if no OAuth tokens were found
	if len(oauthTokens) == 0 {
		log.Warn().
			Str("app_id", appRecord.ID).
			Str("user_id", user.ID).
			Msg("No OAuth tokens found for app")
	} else {
		log.Info().
			Str("app_id", appRecord.ID).
			Str("user_id", user.ID).
			Int("token_count", len(oauthTokens)).
			Interface("provider_keys", maps.Keys(oauthTokens)).
			Msg("Retrieved OAuth tokens for app")
	}

	return oauthTokens
}

// appRunAPIAction godoc
// @Summary Run an API action
// @Description Runs an API action for an app
// @Accept json
// @Produce json
// @Param request body types.RunAPIActionRequest true "Request"
// @Router /api/v1/apps/{id}/api-actions [post]
// @Success 200 {object} types.RunAPIActionResponse
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
func (s *HelixAPIServer) appRunAPIAction(_ http.ResponseWriter, r *http.Request) (*types.RunAPIActionResponse, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	app, err := s.Store.GetAppWithTools(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("app not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if user.ID != app.Owner && !app.Global {
		return nil, system.NewHTTPError403("you do not have permission to run this action")
	}

	// load the body of the request
	var req types.RunAPIActionRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body 4, error: %s", err))
	}

	if req.Action == "" {
		return nil, system.NewHTTPError400("action is required")
	}

	if len(app.Config.Helix.Assistants) == 0 {
		return nil, system.NewHTTPError400("action is required")
	}

	// Validate whether action is valid
	tool, ok := tools.GetToolFromAction(app.Config.Helix.Assistants[0].Tools, req.Action)
	if !ok {
		return nil, system.NewHTTPError400(fmt.Sprintf("action %s not found in the assistant tools", req.Action))
	}

	req.Tool = tool

	log.Info().
		Str("app_id", id).
		Str("user_id", user.ID).
		Str("action", req.Action).
		Str("tool", tool.Name).
		Msg("Running API action")

	// Get OAuth tokens directly as a map
	req.OAuthTokens = s.getAppOAuthTokenEnv(r.Context(), user, app)

	log.Info().
		Str("app_id", id).
		Str("user_id", user.ID).
		Str("action", req.Action).
		Int("oauth_token_count", len(req.OAuthTokens)).
		Interface("oauth_providers", maps.Keys(req.OAuthTokens)).
		Msg("API action with OAuth tokens")

	response, err := s.Controller.ToolsPlanner.RunAPIActionWithParameters(r.Context(), &req)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", id).
			Str("action", req.Action).
			Msg("Failed to run API action")
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().
		Str("app_id", id).
		Str("action", req.Action).
		Msg("API action completed successfully")

	return response, nil
}

// getAppUsage godoc
// @Summary Get app usage
// @Description Get app daily usage
// @Accept json
// @Produce json
// @Tags    apps
// @Param   id path string true "App ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/apps/{id}/daily-usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getAppDailyUsage(_ http.ResponseWriter, r *http.Request) ([]*types.AggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()

	var err error

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	}

	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	err = s.authorizeUserToApp(r.Context(), user, app, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	metrics, err := s.Store.GetAppDailyUsageMetrics(r.Context(), id, from, to)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return metrics, nil
}

// getAppUsersDailyUsage godoc
// @Summary Get app users daily usage
// @Description Get app users daily usage
// @Accept json
// @Produce json
// @Tags    apps
// @Param   id path string true "App ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/apps/{id}/users-daily-usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getAppUsersDailyUsage(_ http.ResponseWriter, r *http.Request) ([]*types.UsersAggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)
	id := getID(r)

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()

	var err error

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	}

	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	err = s.authorizeUserToApp(r.Context(), user, app, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	metrics, err := s.Store.GetAppUsersAggregatedUsageMetrics(r.Context(), id, from, to)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return metrics, nil
}

// getAppUserAccess godoc
// @Summary Get current user's access level for an app
// @Description Returns the access rights the current user has for this app
// @Tags    apps
// @Success 200 {object} types.UserAppAccessResponse
// @Param id path string true "App ID"
// @Router /api/v1/apps/{id}/user-access [get]
// @Security BearerAuth
func (s *HelixAPIServer) getAppUserAccess(_ http.ResponseWriter, r *http.Request) (*types.UserAppAccessResponse, *system.HTTPError) {
	// Get current user and app ID
	user := getRequestUser(r)
	id := getID(r)

	// Get the app
	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// Initialize response with no permissions
	response := &types.UserAppAccessResponse{
		CanRead:  false,
		CanWrite: false,
		IsAdmin:  false,
	}

	// Check if user is the owner or an admin
	if app.Owner == user.ID || isAdmin(user) {
		// Owner or admin has full access
		response.CanRead = true
		response.CanWrite = true
		response.IsAdmin = true
		return response, nil
	}

	// If the app is global, anyone can read it
	if app.Global {
		response.CanRead = true
		return response, nil
	}

	if app.OrganizationID == "" {
		return response, nil
	}

	// For organization apps, check access grants

	// Check if user is in the organization
	orgMembership, err := s.Store.GetOrganizationMembership(r.Context(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         user.ID,
	})
	if err != nil {
		// Always return the response, even if the user has no access
		// This way the frontend can know the user's permission level
		return response, nil
	}

	// If user is organization owner, they have admin access
	if orgMembership.Role == types.OrganizationRoleOwner {
		response.CanRead = true
		response.CanWrite = true
		response.IsAdmin = true
		return response, nil
	}

	readErr := s.authorizeUserToResource(r.Context(), user, app.OrganizationID, app.ID, types.ResourceApplication, types.ActionGet)
	writeErr := s.authorizeUserToResource(r.Context(), user, app.OrganizationID, app.ID, types.ResourceApplication, types.ActionUpdate)

	if readErr == nil {
		response.CanRead = true
	}
	if writeErr == nil {
		response.CanWrite = true
	}

	// Always return the response, even if the user has no access
	// This way the frontend can know the user's permission level
	return response, nil
}

// uploadAppAvatar godoc
// @Summary Upload app avatar
// @Description Upload a base64 encoded image as the app's avatar
// @Tags    apps
// @Accept  text/plain
// @Produce json
// @Param id path string true "App ID"
// @Param image body string true "Base64 encoded image data"
// @Success 200
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/apps/{id}/avatar [post]
// @Security BearerAuth
func (s *HelixAPIServer) uploadAppAvatar(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	id := getID(r)

	// Get the app to check permissions
	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "app not found", http.StatusNotFound)
			return
		}
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to update the app
	err = s.authorizeUserToApp(r.Context(), user, app, types.ActionUpdate)
	if err != nil {
		http.Error(rw, "unauthorized", http.StatusForbidden)
		return
	}

	// Setup limit reader for 3MB
	limitReader := io.LimitReader(r.Body, 3*1024*1024)

	// Read the request body
	body, err := io.ReadAll(limitReader)
	if err != nil {
		http.Error(rw, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Decode base64 image
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		http.Error(rw, "invalid base64 image data", http.StatusBadRequest)
		return
	}

	// Validate image format
	contentType := http.DetectContentType(decoded)

	// Check if it's an SVG file by looking at the content
	isSVG := strings.Contains(string(decoded), "<svg") && strings.Contains(string(decoded), "</svg>")

	if !strings.HasPrefix(contentType, "image/") && !isSVG {
		http.Error(rw, fmt.Sprintf("invalid image format: %s", contentType), http.StatusBadRequest)
		return
	}

	// For SVG files, ensure we set the correct content type
	if isSVG {
		contentType = "image/svg+xml"
	}

	// Write to avatars bucket using just the app ID as the key
	key := getAvatarKey(id)
	err = s.avatarsBucket.WriteAll(r.Context(), key, decoded, nil)
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to save avatar: %s", err), http.StatusInternalServerError)
		return
	}

	// Update app config with avatar URL and content type
	app.Config.Helix.Avatar = fmt.Sprintf("/api/v1/apps/%s/avatar", id)
	app.Config.Helix.AvatarContentType = contentType
	_, err = s.Store.UpdateApp(r.Context(), app)
	if err != nil {
		http.Error(rw, "failed to update app", http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusOK)
}

// deleteAppAvatar godoc
// @Summary Delete app avatar
// @Description Delete the app's avatar image
// @Tags    apps
// @Produce json
// @Param id path string true "App ID"
// @Success 200
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/apps/{id}/avatar [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteAppAvatar(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	id := getID(r)

	// Get the app to check permissions
	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "app not found", http.StatusNotFound)
			return
		}
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to update the app
	err = s.authorizeUserToApp(r.Context(), user, app, types.ActionUpdate)
	if err != nil {
		http.Error(rw, "unauthorized", http.StatusForbidden)
		return
	}

	// Delete the avatar file
	key := getAvatarKey(id)
	err = s.avatarsBucket.Delete(r.Context(), key)
	if err != nil {
		// Don't return error if file doesn't exist
		if !strings.Contains(err.Error(), "not found") {
			http.Error(rw, "failed to delete avatar", http.StatusInternalServerError)
			return
		}
	}

	// Update app config to remove avatar URL and content type
	app.Config.Helix.Avatar = ""
	app.Config.Helix.AvatarContentType = ""
	_, err = s.Store.UpdateApp(r.Context(), app)
	if err != nil {
		http.Error(rw, "failed to update app", http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusOK)
}

func (s *HelixAPIServer) getAppAvatar(rw http.ResponseWriter, r *http.Request) {
	id := getID(r)

	// Get the app to check if it exists
	app, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "app not found", http.StatusNotFound)
			return
		}
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if app has an avatar
	if app.Config.Helix.Avatar == "" {
		http.Error(rw, "avatar not found", http.StatusNotFound)
		return
	}

	// Read the avatar file
	key := getAvatarKey(id)
	data, err := s.avatarsBucket.ReadAll(r.Context(), key)
	if err != nil {
		http.Error(rw, "avatar not found", http.StatusNotFound)
		return
	}

	// Set content type and serve the image
	contentType := app.Config.Helix.AvatarContentType
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	rw.Header().Set("Content-Type", contentType)
	rw.Header().Set("Cache-Control", "no-cache")
	_, _ = rw.Write(data)
}

func getAvatarKey(id string) string {
	return fmt.Sprintf("/app-avatars/%s", id)
}

// downloadAndExtractZipToKnowledge downloads a zip file from a URL and extracts its contents
// to the specified knowledge filestore path. It handles air-gapped scenarios by marking
// knowledge as failed if the download fails.
func (s *HelixAPIServer) downloadAndExtractZipToKnowledge(ctx context.Context, zipURL, knowledgeStorePath string) error {
	log.Info().
		Str("zip_url", zipURL).
		Str("destination_path", knowledgeStorePath).
		Msg("Starting zip file download and extraction")

	// Download the zip file with a timeout
	client := &http.Client{
		Timeout: 10 * time.Minute, // Allow up to 10 minutes for large zip files
	}

	req, err := http.NewRequestWithContext(ctx, "GET", zipURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for zip URL: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download zip file from %s: %w - this may indicate the system is air-gapped or the URL is not accessible", zipURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download zip file from %s: HTTP %d %s", zipURL, resp.StatusCode, resp.Status)
	}

	// Check content length if provided by server
	const maxZipSize = 1024 * 1024 * 1024 // 1GB limit
	if resp.ContentLength > maxZipSize {
		return fmt.Errorf("zip file is too large: %d bytes (max allowed: %d bytes)", resp.ContentLength, maxZipSize)
	}

	// Create a temporary file to store the zip
	tempFile, err := os.CreateTemp("", "helix-knowledge-seed-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for zip download: %w", err)
	}
	defer os.Remove(tempFile.Name()) // Clean up temp file
	defer tempFile.Close()

	log.Debug().
		Str("temp_file", tempFile.Name()).
		Msg("Created temporary file for zip download")

	// Copy the response body to the temp file with size limit
	limitedReader := io.LimitReader(resp.Body, maxZipSize+1) // +1 to detect if size exceeded
	bytesWritten, err := io.Copy(tempFile, limitedReader)
	if err != nil {
		return fmt.Errorf("failed to download zip file to temporary storage: %w", err)
	}

	if bytesWritten > maxZipSize {
		return fmt.Errorf("zip file exceeded maximum size limit: %d bytes (max allowed: %d bytes)", bytesWritten, maxZipSize)
	}

	log.Info().
		Int64("bytes_downloaded", bytesWritten).
		Msg("Successfully downloaded zip file to temporary storage")

	// Close the temp file to ensure all data is written
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Open the zip file for reading
	zipReader, err := zip.OpenReader(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer zipReader.Close()

	log.Info().
		Int("file_count", len(zipReader.File)).
		Msg("Extracting files from zip archive")

	// Extract each file from the zip
	for _, file := range zipReader.File {
		err := s.extractZipFile(ctx, file, knowledgeStorePath)
		if err != nil {
			log.Error().
				Err(err).
				Str("filename", file.Name).
				Msg("Failed to extract file from zip")
			return fmt.Errorf("failed to extract file %s: %w", file.Name, err)
		}
	}

	log.Info().
		Int("extracted_files", len(zipReader.File)).
		Str("destination_path", knowledgeStorePath).
		Int64("total_size", bytesWritten).
		Msg("Successfully extracted all files from zip archive")

	return nil
}

// extractZipFile extracts a single file from a zip archive to the filestore
func (s *HelixAPIServer) extractZipFile(ctx context.Context, file *zip.File, basePath string) error {
	// Skip directories
	if file.FileInfo().IsDir() {
		return nil
	}

	// Open the file in the zip
	fileReader, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %s in zip: %w", file.Name, err)
	}
	defer fileReader.Close()

	// Create the destination path
	destPath := filepath.Join(basePath, file.Name)

	// Ensure the destination directory exists
	destDir := filepath.Dir(destPath)
	if destDir != "." && destDir != "/" {
		// Create the directory structure
		_, err = s.Controller.Options.Filestore.CreateFolder(ctx, destDir)
		if err != nil {
			log.Debug().
				Err(err).
				Str("directory", destDir).
				Msg("Directory might already exist, continuing")
		}
	}

	// Write the file to the filestore
	_, err = s.Controller.Options.Filestore.WriteFile(ctx, destPath, fileReader)
	if err != nil {
		return fmt.Errorf("failed to write file %s to filestore: %w", destPath, err)
	}

	log.Debug().
		Str("source_file", file.Name).
		Str("destination_path", destPath).
		Int64("file_size", file.FileInfo().Size()).
		Msg("Successfully extracted file from zip")

	return nil
}
