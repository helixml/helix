package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/apps"
	"github.com/helixml/helix/api/pkg/controller/knowledge"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// listApps godoc
// @Summary List apps
// @Description List apps for the user. Apps are pre-configured to spawn sessions with specific tools and config.
// @Tags    apps

// @Success 200 {array} types.App
// @Router /api/v1/apps [get]
// @Security BearerAuth
func (s *HelixAPIServer) listApps(_ http.ResponseWriter, r *http.Request) ([]*types.App, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	userApps, err := s.Store.ListApps(ctx, &store.ListAppsQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// remove global apps from the list in case this is the admin user who created the global tool
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

	// Extract the "type" query parameter
	queryType := r.URL.Query().Get("type")

	// Filter apps based on the "type" query parameter
	var filteredApps []*types.App
	for _, app := range allApps {
		if queryType != "" && app.AppSource != types.AppSource(queryType) {
			continue
		}
		if !isAdmin(user) && app.Global {
			if app.Config.Github != nil {
				app.Config.Github.KeyPair.PrivateKey = ""
				app.Config.Github.WebhookSecret = ""
			}
		}

		filteredApps = append(filteredApps, app)
	}

	return filteredApps, nil
}

// createTool godoc
// @Summary Create new app
// @Description Create new app. Apps are pre-configured to spawn sessions with specific tools and config.
// @Tags    apps

// @Success 200 {object} types.App
// @Param request    body types.App true "Request body with app configuration.")
// @Router /api/v1/apps [post]
// @Security BearerAuth
func (s *HelixAPIServer) createApp(_ http.ResponseWriter, r *http.Request) (*types.App, *system.HTTPError) {
	var app types.App
	body, err := io.ReadAll(r.Body)
	// log.Info().Msgf("createApp body: %s", string(body))
	if err != nil {
		return nil, system.NewHTTPError400("failed to read request body, error: %s", err)
	}
	err = json.Unmarshal(body, &app)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body 1, error: %s, body: %s", err, string(body))
	}

	user := getRequestUser(r)
	ctx := r.Context()

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

	for _, a := range existingApps {
		if app.Config.Helix.Name != "" && a.Config.Helix.Name == app.Config.Helix.Name {
			return nil, system.NewHTTPError400("app (%s) with name %s already exists", a.ID, a.Config.Helix.Name)
		}
	}

	var created *types.App

	// if this is a github app - then initialise it
	switch app.AppSource {
	case types.AppSourceHelix:
		err = s.validateTriggers(app.Config.Helix.Triggers)
		if err != nil {
			return nil, system.NewHTTPError400(err.Error())
		}

		// Validate and default tools
		for idx := range app.Config.Helix.Assistants {
			assistant := &app.Config.Helix.Assistants[idx]
			for idx := range assistant.Tools {
				tool := assistant.Tools[idx]
				err = s.validateTool(tool)
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

		created, err = s.Store.CreateApp(ctx, &app)
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		log.Info().Msgf("Created Helix (local source) app %s", created.ID)
	case types.AppSourceGithub:
		if app.Config.Github.Repo == "" {
			return nil, system.NewHTTPError400("github repo is required")
		}
		created, err = s.Store.CreateApp(ctx, &app)
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		log.Info().Msgf("Created Helix (local source) app %s", created.ID)

		client, err := s.getGithubClientFromRequest(r)
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
		githubApp, err := apps.NewGithubApp(apps.GithubAppOptions{
			GithubConfig: s.Cfg.GitHub,
			Client:       client,
			App:          created,
			ToolsPlanner: s.Controller.ToolsPlanner,
			UpdateApp: func(app *types.App) (*types.App, error) {
				return s.Store.UpdateApp(r.Context(), app)
			},
		})
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		newApp, err := githubApp.Create()
		if err != nil {
			s.Store.DeleteApp(r.Context(), created.ID)
			return nil, system.NewHTTPError500(err.Error())
		}

		created, err = s.Store.UpdateApp(r.Context(), newApp)
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
	default:
		return nil, system.NewHTTPError400(
			"unknown app source, available sources: %s, %s",
			types.AppSourceHelix,
			types.AppSourceGithub)
	}

	err = s.ensureKnowledge(ctx, created)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	_, err = s.Controller.CreateAPIKey(ctx, user, &types.APIKey{
		Name:  "api key 1",
		Type:  types.APIKeyType_App,
		AppID: &sql.NullString{String: created.ID, Valid: true},
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return created, nil
}

func (s *HelixAPIServer) validateKnowledge(k *types.AssistantKnowledge) error {
	return knowledge.Validate(k)
}

func (s *HelixAPIServer) validateTriggers(triggers []types.Trigger) error {
	// If it's cron, check that it runs not more than once every 90 seconds
	for _, trigger := range triggers {
		if trigger.Cron != nil && trigger.Cron.Schedule != "" {
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
		for _, k := range assistant.Knowledge {
			knowledge = append(knowledge, k)
		}
	}

	// Used to track which knowledges are declared in the app config
	// so we can delete knowledges that are no longer specified
	foundKnowledge := make(map[string]bool)

	for _, k := range knowledge {
		existing, err := s.Store.LookupKnowledge(ctx, &store.LookupKnowledgeQuery{
			AppID: app.ID,
			Name:  k.Name,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Create new knowledge
				created, err := s.Store.CreateKnowledge(ctx, &types.Knowledge{
					AppID:           app.ID,
					Name:            k.Name,
					Description:     k.Description,
					Owner:           app.Owner,
					OwnerType:       app.OwnerType,
					State:           types.KnowledgeStatePending,
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
			} else {
				return fmt.Errorf("failed to create knowledge '%s': %w", k.Name, err)
			}
		}

		// Update existing knowledge
		existing.Description = k.Description
		existing.RAGSettings = k.RAGSettings
		existing.Source = k.Source
		existing.RefreshEnabled = k.RefreshEnabled
		existing.RefreshSchedule = k.RefreshSchedule

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

// what the user can change about a github app fromm the frontend
type AppUpdatePayload struct {
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	ActiveTools    []string          `json:"active_tools"`
	Secrets        map[string]string `json:"secrets"`
	AllowedDomains []string          `json:"allowed_domains"`
	Shared         bool              `json:"shared"`
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

	if (!app.Global && !app.Shared) && app.Owner != user.ID {
		return nil, system.NewHTTPError404(store.ErrNotFound.Error())
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
		return nil, system.NewHTTPError400("failed to decode request body 2, error: %s", err)
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

	if existing.Global {
		if !isAdmin(user) {
			return nil, system.NewHTTPError403("only admin users can update global apps")
		}
	} else {
		if existing.Owner != user.ID {
			return nil, system.NewHTTPError403("you do not have permission to update this app")
		}
	}

	err = s.validateTriggers(update.Config.Helix.Triggers)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	update.Updated = time.Now()

	// Validate and default tools
	for idx := range update.Config.Helix.Assistants {
		assistant := &update.Config.Helix.Assistants[idx]
		for idx := range assistant.Tools {
			tool := assistant.Tools[idx]
			err = s.validateTool(tool)
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
	updated, err := s.Store.UpdateApp(r.Context(), &update)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	err = s.ensureKnowledge(r.Context(), updated)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return updated, nil
}

// updateGithubApp godoc
// @Summary Update an existing app
// @Description Update existing app
// @Tags    apps

// @Success 200 {object} types.App
// @Param request    body types.App true "Request body with app configuration.")
// @Param id path string true "Tool ID"
// @Router /api/v1/apps/github/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateGithubApp(_ http.ResponseWriter, r *http.Request) (*types.App, *system.HTTPError) {
	user := getRequestUser(r)

	var appUpdate AppUpdatePayload
	err := json.NewDecoder(r.Body).Decode(&appUpdate)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body 3, error: %s", err)
	}

	if appUpdate.ActiveTools == nil {
		appUpdate.ActiveTools = []string{}
	}

	if appUpdate.AllowedDomains == nil {
		appUpdate.AllowedDomains = []string{}
	}

	if appUpdate.Secrets == nil {
		appUpdate.Secrets = map[string]string{}
	}

	id := getID(r)

	// Getting existing app
	existing, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing == nil {
		return nil, system.NewHTTPError404(store.ErrNotFound.Error())
	}

	if appUpdate.Global {
		if !isAdmin(user) {
			return nil, system.NewHTTPError403("only admin users can update global apps")
		}
	} else {
		if existing.Owner != user.ID {
			return nil, system.NewHTTPError403("you do not have permission to update this app")
		}
	}

	if existing.AppSource == types.AppSourceGithub {
		client, err := s.getGithubClientFromRequest(r)
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
		githubApp, err := apps.NewGithubApp(apps.GithubAppOptions{
			GithubConfig: s.Cfg.GitHub,
			Client:       client,
			App:          existing,
			ToolsPlanner: s.Controller.ToolsPlanner,
			UpdateApp: func(app *types.App) (*types.App, error) {
				return s.Store.UpdateApp(r.Context(), app)
			},
		})
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		existing, err = githubApp.Update()
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
	}

	existing.Updated = time.Now()
	existing.Config.Secrets = appUpdate.Secrets
	existing.Config.AllowedDomains = appUpdate.AllowedDomains
	existing.Shared = appUpdate.Shared
	existing.Global = appUpdate.Global

	// Updating the app
	updated, err := s.Store.UpdateApp(r.Context(), existing)
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

	deleteKnowledge := r.URL.Query().Get("knowledge") == "true"

	existing, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Global {
		if !isAdmin(user) {
			return nil, system.NewHTTPError403("only admin users can delete global apps")
		}
	} else {
		if existing.Owner != user.ID {
			return nil, system.NewHTTPError403("you do not have permission to delete this app")
		}
	}

	if deleteKnowledge {
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

// appRunScript godoc
// @Summary Run a GPT script inside a github app
// @Description Run a GPT script inside a github app.
// @Tags    apps

// @Success 200 {object} types.GptScriptResponse
// @Param request    body types.GptScriptRequest true "Request body with script configuration.")
// @Router /api/v1/apps/{id}/gptscript [post]
// @Security BearerAuth
func (s *HelixAPIServer) appRunScript(w http.ResponseWriter, r *http.Request) (*types.GptScriptResponse, *system.HTTPError) {
	// TODO: authenticate the referer based on app settings
	addCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return nil, nil
	}
	user := getRequestUser(r)

	if user.AppID == "" {
		return nil, system.NewHTTPError403("no api key for app found")
	}

	appRecord, err := s.Store.GetApp(r.Context(), user.AppID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("app not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// load the body of the request
	var req types.GptScriptRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body 4, error: %s", err)
	}

	envPairs := []string{}
	for key, value := range appRecord.Config.Secrets {
		envPairs = append(envPairs, key+"="+value)
	}

	logger := log.With().
		Str("app_id", user.AppID).
		Str("user_id", user.ID).Logger()

	logger.Trace().Msg("starting app execution")

	app := &types.GptScriptGithubApp{
		Script: types.GptScript{
			FilePath: req.FilePath,
			Input:    req.Input,
			Env:      envPairs,
		},
		Repo:       appRecord.Config.Github.Repo,
		CommitHash: appRecord.Config.Github.Hash,
		KeyPair:    appRecord.Config.Github.KeyPair,
	}

	start := time.Now()

	result, err := s.gptScriptExecutor.ExecuteApp(r.Context(), app)
	if err != nil {

		logger.Warn().Err(err).Str("duration", time.Since(start).String()).Msg("app execution failed")

		// Log error
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, runErr := s.Store.CreateScriptRun(ctx, &types.ScriptRun{
			Owner:       user.ID,
			OwnerType:   user.Type,
			AppID:       user.AppID,
			State:       types.ScriptRunStateError,
			Type:        types.GptScriptRunnerTaskTypeGithubApp,
			SystemError: err.Error(),
			DurationMs:  int(time.Since(start).Milliseconds()),
			Request: &types.GptScriptRunnerRequest{
				GithubApp: app,
			},
		})
		if runErr != nil {
			log.Err(runErr).Msg("failed to create script run")
		}

		return nil, system.NewHTTPError500(err.Error())
	}

	logger.Info().Str("duration", time.Since(start).String()).Msg("app executed")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = s.Store.CreateScriptRun(ctx, &types.ScriptRun{
		Owner:      user.ID,
		OwnerType:  user.Type,
		AppID:      user.AppID,
		State:      types.ScriptRunStateComplete,
		Type:       types.GptScriptRunnerTaskTypeGithubApp,
		Retries:    result.Retries,
		DurationMs: int(time.Since(start).Milliseconds()),
		Request: &types.GptScriptRunnerRequest{
			GithubApp: app,
		},
		Response: result,
	})
	if err != nil {
		log.Err(err).Msg("failed to create script run")
	}

	return result, nil
}
