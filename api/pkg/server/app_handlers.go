package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/apps"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listApps godoc
// @Summary List apps
// @Description List apps for the user. Apps are pre-configured to spawn sessions with specific tools and config.
// @Tags    apps

// @Success 200 {object} types.App
// @Router /api/v1/apps [get]
// @Security BearerAuth
func (s *HelixAPIServer) listApps(_ http.ResponseWriter, r *http.Request) ([]*types.App, *system.HTTPError) {
	userContext := getRequestContext(r)

	// Extract the "type" query parameter
	queryType := r.URL.Query().Get("type")

	allApps, err := s.Store.ListApps(r.Context(), &store.ListAppsQuery{
		Owner:     userContext.User.ID,
		OwnerType: userContext.User.Type,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Filter apps based on the "type" query parameter
	var filteredApps []*types.App
	for _, app := range allApps {
		if queryType != "" && app.AppType != types.AppType(queryType) {
			continue
		}
		app.Config.Github.KeyPair.PrivateKey = ""
		app.Config.Github.WebhookSecret = ""
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
	err := json.NewDecoder(r.Body).Decode(&app)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body, error: %s", err)
	}

	userContext := getRequestContext(r)

	// Getting existing tools for the user
	existingApps, err := s.Store.ListApps(r.Context(), &store.ListAppsQuery{
		Owner:     userContext.User.ID,
		OwnerType: userContext.User.Type,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	app.ID = system.GenerateAppID()
	app.Owner = userContext.User.ID
	app.OwnerType = userContext.User.Type
	app.Updated = time.Now()

	if app.Config.Helix == nil {
		app.Config.Helix = &types.AppHelixConfig{
			Assistants: []types.AssistantConfig{},
		}
	}

	for _, a := range existingApps {
		if a.Name == app.Name {
			return nil, system.NewHTTPError400("app (%s) with name %s already exists", a.ID, app.Name)
		}
	}

	created, err := s.Store.CreateApp(r.Context(), &app)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// if this is a github app - then initialise it
	if app.AppType == types.AppTypeGithub {
		if app.AppType == types.AppTypeGithub {
			if app.Config.Github.Repo == "" {
				return nil, system.NewHTTPError400("github repo is required")
			}
		}
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

		app = *newApp
	}

	created, err = s.Store.UpdateApp(r.Context(), &app)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	_, err = s.Controller.CreateAPIKey(userContext, &types.APIKey{
		Name:  "api key 1",
		Type:  types.APIKeyType_App,
		AppID: &sql.NullString{String: created.ID, Valid: true},
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return created, nil
}

// what the user can change about a github app fromm the frontend
type AppUpdatePayload struct {
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	ActiveTools    []string          `json:"active_tools"`
	Secrets        map[string]string `json:"secrets"`
	AllowedDomains []string          `json:"allowed_domains"`
}

// updateTool godoc
// @Summary Update an existing app
// @Description Update existing app
// @Tags    apps

// @Success 200 {object} types.App
// @Param request    body types.App true "Request body with app configuration.")
// @Param id path string true "Tool ID"
// @Router /api/v1/apps/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateApp(_ http.ResponseWriter, r *http.Request) (*types.App, *system.HTTPError) {
	userContext := getRequestContext(r)

	var appUpdate AppUpdatePayload
	err := json.NewDecoder(r.Body).Decode(&appUpdate)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body, error: %s", err)
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

	if existing.Owner != userContext.User.ID {
		return nil, system.NewHTTPError403("you do not have permission to update this app")
	}

	if existing.AppType == types.AppTypeGithub {
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

	existing.Name = appUpdate.Name
	existing.Description = appUpdate.Description
	existing.Updated = time.Now()
	existing.Config.Secrets = appUpdate.Secrets
	existing.Config.AllowedDomains = appUpdate.AllowedDomains

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
	userContext := getRequestContext(r)

	id := getID(r)

	existing, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != userContext.User.ID {
		return nil, system.NewHTTPError404(store.ErrNotFound.Error())
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

// @Success 200 {object} types.GptScriptResult
// @Param request    body types.GptScriptRequest true "Request body with script configuration.")
// @Router /api/v1/apps/{id}/gptscript [post]
// @Security BearerAuth
func (s *HelixAPIServer) appRunScript(w http.ResponseWriter, r *http.Request) (*types.GptScriptResponse, *system.HTTPError) {
	// TODO: authenticate the referer based on app settings
	addCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return nil, nil
	}

	userContext := getRequestContext(r)

	if userContext.User.AppID == "" {
		return nil, system.NewHTTPError403("no api key for app found")
	}

	appRecord, err := s.Store.GetApp(r.Context(), userContext.User.AppID)
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
		return nil, system.NewHTTPError400("failed to decode request body, error: %s", err)
	}

	envPairs := []string{}
	for key, value := range appRecord.Config.Secrets {
		envPairs = append(envPairs, key+"="+value)
	}

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
		// Log error
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, runErr := s.Store.CreateScriptRun(ctx, &types.ScriptRun{
			Owner:       userContext.User.ID,
			OwnerType:   userContext.User.Type,
			AppID:       userContext.User.AppID,
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = s.Store.CreateScriptRun(ctx, &types.ScriptRun{
		Owner:      userContext.User.ID,
		OwnerType:  userContext.User.Type,
		AppID:      userContext.User.AppID,
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
