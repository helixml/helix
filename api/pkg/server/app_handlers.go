package server

import (
	"encoding/json"
	"errors"
	"net/http"

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

	err = s.validateApp(&userContext, &app)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Checking if the tool already exists
	for _, a := range existingApps {
		if a.Name == app.Name {
			return nil, system.NewHTTPError400("tool (%s) with name %s already exists", a.ID, app.Name)
		}
	}

	// Creating the tool
	created, err := s.Store.CreateApp(r.Context(), &app)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return created, nil
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

	var app types.App
	err := json.NewDecoder(r.Body).Decode(&app)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body, error: %s", err)
	}

	id := getID(r)

	app.ID = id

	err = s.validateApp(&userContext, &app)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Getting existing tool
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

	app.Owner = existing.Owner
	app.OwnerType = existing.OwnerType

	// Updating the app
	updated, err := s.Store.UpdateApp(r.Context(), &app)
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

func (s *HelixAPIServer) validateApp(_ *types.RequestContext, _ *types.App) error {
	return nil
}
