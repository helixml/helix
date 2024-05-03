package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
)

// listTools godoc
// @Summary List tools
// @Description List tools for the user. Tools are use by the LLMs to interact with external systems.
// @Tags    tools

// @Success 200 {object} types.Tool
// @Router /api/v1/tools [get]
// @Security BearerAuth
func (s *HelixAPIServer) listTools(rw http.ResponseWriter, r *http.Request) ([]*types.Tool, *system.HTTPError) {
	userContext := getRequestContext(r)

	userTools, err := s.Store.ListTools(r.Context(), &store.ListToolsQuery{
		Owner:     userContext.User.ID,
		OwnerType: userContext.User.Type,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// remove global tools from the list in case this is the admin user who created the global tool

	nonGlobalUserTools := []*types.Tool{}
	for _, tool := range userTools {
		if !tool.Global {
			nonGlobalUserTools = append(nonGlobalUserTools, tool)
		}
	}

	globalTools, err := s.Store.ListTools(r.Context(), &store.ListToolsQuery{
		Global: true,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Concatenate globalTools to userTools list
	allTools := append(nonGlobalUserTools, globalTools...)
	sanitizedTools := []*types.Tool{}

	// remove api keys from global tools in the response
	for _, tool := range allTools {
		if !isAdmin(userContext.User) && tool.Global {
			tool.Config.API.Headers = map[string]string{}
			tool.Config.API.Query = map[string]string{}
		}
		sanitizedTools = append(sanitizedTools, tool)
	}

	return sanitizedTools, nil
}

// createTool godoc
// @Summary Create new tool
// @Description Create new tool. Tools are used by the LLMs to interact with external systems.
// @Tags    tools

// @Success 200 {object} types.Tool
// @Param request    body types.Tool true "Request body with tool configuration. For API schemas, it can be base64 encoded.")
// @Router /api/v1/tools [post]
// @Security BearerAuth
func (s *HelixAPIServer) createTool(rw http.ResponseWriter, r *http.Request) (*types.Tool, *system.HTTPError) {
	var tool types.Tool
	err := json.NewDecoder(r.Body).Decode(&tool)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body, error: %s", err)
	}

	userContext := getRequestContext(r)

	// only let admins create global tools
	if tool.Global && !isAdmin(userContext.User) {
		return nil, system.NewHTTPError403("only admin users can create global tools")
	}

	// Getting existing tools for the user
	existingTools, err := s.Store.ListTools(r.Context(), &store.ListToolsQuery{
		Owner:     userContext.User.ID,
		OwnerType: userContext.User.Type,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	tool.Owner = userContext.User.ID
	tool.OwnerType = userContext.User.Type

	err = s.validateTool(&userContext, &tool)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Checking if the tool already exists
	for _, t := range existingTools {
		if t.Name == tool.Name {
			return nil, system.NewHTTPError400("tool (%s) with name %s already exists", t.ID, tool.Name)
		}
	}

	// Creating the tool
	created, err := s.Store.CreateTool(r.Context(), &tool)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return created, nil
}

// updateTool godoc
// @Summary Update an existing tool
// @Description Update existing tool
// @Tags    tools

// @Success 200 {object} types.Tool
// @Param request    body types.Tool true "Request body with tool configuration. For API schemas, it can be base64 encoded.")
// @Param id path string true "Tool ID"
// @Router /api/v1/tools/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateTool(rw http.ResponseWriter, r *http.Request) (*types.Tool, *system.HTTPError) {
	userContext := getRequestContext(r)

	var tool types.Tool
	err := json.NewDecoder(r.Body).Decode(&tool)
	if err != nil {
		return nil, system.NewHTTPError400("failed to decode request body, error: %s", err)
	}

	id := getID(r)

	tool.ID = id

	err = s.validateTool(&userContext, &tool)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Getting existing tool
	existing, err := s.Store.GetTool(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// let any admin update a global tool
	// but otherwise you must own the tool to update it
	if tool.Global {
		if !isAdmin(userContext.User) {
			return nil, system.NewHTTPError403("only admin users can update global tools")
		}
	} else {
		if existing.Owner != userContext.User.ID {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
	}

	tool.Owner = existing.Owner
	tool.OwnerType = existing.OwnerType

	// Updating the tool
	updated, err := s.Store.UpdateTool(r.Context(), &tool)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return updated, nil
}

func (s *HelixAPIServer) validateTool(_ *types.RequestContext, tool *types.Tool) error {
	return tools.ValidateTool(tool, s.Controller.ToolsPlanner, true)
}

// deleteTool godoc
// @Summary Delete tool
// @Description Delete tool. This removes the entry from the database, your models will not be able to use this tool anymore.
// @Tags    tools

// @Success 200
// @Param id path string true "Tool ID"
// @Router /api/v1/tools/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteTool(rw http.ResponseWriter, r *http.Request) (*types.Tool, *system.HTTPError) {
	userContext := getRequestContext(r)

	id := getID(r)

	existing, err := s.Store.GetTool(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// let any admin delete a global tool
	// but otherwise you must own the tool to update it
	if existing.Global {
		if !isAdmin(userContext.User) {
			return nil, system.NewHTTPError403("only admin users can delete global tools")
		}
	} else {
		if existing.Owner != userContext.User.ID {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
	}

	err = s.Store.DeleteTool(r.Context(), id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return existing, nil
}
