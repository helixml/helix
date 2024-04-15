package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/apps"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listApps godoc
// @Summary List apps
// @Description List apps for the user. Apps are pre-configured to spawn sessions with specific tools and config.
// @Tags    apps

// @Success 200 {object} types.App
// @Router /api/v1/apps [get]
// @Security BearerAuth
func (s *HelixAPIServer) listApps(_ http.ResponseWriter, r *http.Request) ([]*types.App, *system.HTTPError) {
	userContext := s.getRequestContext(r)

	// Extract the "type" query parameter
	queryType := r.URL.Query().Get("type")

	allApps, err := s.Store.ListApps(r.Context(), &store.ListAppsQuery{
		Owner:     userContext.Owner,
		OwnerType: userContext.OwnerType,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Filter apps based on the "type" query parameter
	filteredApps := make([]*types.App, 0)
	for _, app := range allApps {
		if queryType != "" && app.AppType != types.AppType(queryType) {
			continue
		}
		app.Config.Github.KeyPair.PrivateKey = ""
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

	userContext := s.getRequestContext(r)

	// Getting existing tools for the user
	existingApps, err := s.Store.ListApps(r.Context(), &store.ListAppsQuery{
		Owner:     userContext.Owner,
		OwnerType: userContext.OwnerType,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	app.Owner = userContext.Owner
	app.OwnerType = userContext.OwnerType
	app.Updated = time.Now()

	if app.Config.Helix == nil {
		app.Config.Helix = &types.AppHelixConfig{
			ActiveTools: []string{},
			Secrets:     map[string]string{},
		}
	}

	// Checking if the tool already exists
	for _, a := range existingApps {
		if a.Name == app.Name {
			return nil, system.NewHTTPError400("app (%s) with name %s already exists", a.ID, app.Name)
		}
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
			App:          &app,
		})
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		newApp, err := githubApp.Create()
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}

		app = *newApp
	}

	// Creating the tool
	created, err := s.Store.CreateApp(r.Context(), &app)
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
	userContext := s.getRequestContext(r)

	var appUpdate AppUpdatePayload
	err := json.NewDecoder(r.Body).Decode(&appUpdate)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body, error: %s", err)
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

	if existing.Owner != userContext.Owner {
		return nil, system.NewHTTPError403("you do not have permission to update this app")
	}

	existing.Name = appUpdate.Name
	existing.Description = appUpdate.Description
	existing.Config.Helix.ActiveTools = appUpdate.ActiveTools
	existing.Config.Helix.Secrets = appUpdate.Secrets
	existing.Config.Helix.AllowedDomains = appUpdate.AllowedDomains

	if existing.AppType == types.AppTypeGithub {
		client, err := s.getGithubClientFromRequest(r)
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
		githubApp, err := apps.NewGithubApp(apps.GithubAppOptions{
			GithubConfig: s.Cfg.GitHub,
			Client:       client,
			App:          existing,
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
	userContext := s.getRequestContext(r)

	id := getID(r)

	existing, err := s.Store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != userContext.Owner {
		return nil, system.NewHTTPError404(store.ErrNotFound.Error())
	}

	err = s.Store.DeleteApp(r.Context(), id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return existing, nil
}
