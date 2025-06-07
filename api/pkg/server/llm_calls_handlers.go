package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listLLMCalls godoc
// @Summary List LLM calls
// @Description List LLM calls with pagination and optional session filtering
// @Tags    llm_calls
// @Produce json
// @Param   page          query    int     false  "Page number"
// @Param   pageSize      query    int     false  "Page size"
// @Param   sessionFilter query    string  false  "Filter by session ID"
// @Success 200 {object} types.PaginatedLLMCalls
// @Router /api/v1/llm_calls [get]
// @Security BearerAuth
func (s *HelixAPIServer) listLLMCalls(_ http.ResponseWriter, r *http.Request) (*types.PaginatedLLMCalls, *system.HTTPError) {
	// Parse query parameters
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if err != nil || pageSize < 1 {
		pageSize = 10 // Default page size
	}

	sessionFilter := r.URL.Query().Get("sessionFilter")

	// Call the ListLLMCalls function from the store with the session filter
	calls, totalCount, err := s.Store.ListLLMCalls(r.Context(), &store.ListLLMCallsQuery{
		Page:          page,
		PerPage:       pageSize,
		SessionFilter: sessionFilter,
		AppID:         r.URL.Query().Get("appId"),
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Calculate total pages
	totalPages := (int(totalCount) + pageSize - 1) / pageSize

	// Prepare the response
	response := &types.PaginatedLLMCalls{
		Calls:      calls,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	return response, nil
}

// listAppLLMCalls godoc
// @Summary List LLM calls
// @Description List user's LLM calls with pagination and optional session filtering for a specific app
// @Tags    llm_calls
// @Produce json
// @Param   page          query    int     false  "Page number"
// @Param   pageSize      query    int     false  "Page size"
// @Param   sessionFilter query    string  false  "Filter by session ID"
// @Success 200 {object} types.PaginatedLLMCalls
// @Router /api/v1/apps/{id}/llm-calls [get]
// @Security BearerAuth
func (s *HelixAPIServer) listAppLLMCalls(_ http.ResponseWriter, r *http.Request) (*types.PaginatedLLMCalls, *system.HTTPError) {
	appID := getID(r)
	user := getRequestUser(r)

	app, err := s.Store.GetApp(r.Context(), appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if app.Owner != user.ID && !isAdmin(user) {
		return nil, system.NewHTTPError403("you do not have permission to view this app's LLM calls")
	}

	// Parse query parameters
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if err != nil || pageSize < 1 {
		pageSize = 10 // Default page size
	}

	sessionFilter := r.URL.Query().Get("sessionFilter")

	// Call the ListLLMCalls function from the store with the session filter
	calls, totalCount, err := s.Store.ListLLMCalls(r.Context(), &store.ListLLMCallsQuery{
		Page:          page,
		PerPage:       pageSize,
		SessionFilter: sessionFilter,
		AppID:         appID,
		UserID:        user.ID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Calculate total pages
	totalPages := (int(totalCount) + pageSize - 1) / pageSize

	// Prepare the response
	response := &types.PaginatedLLMCalls{
		Calls:      calls,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	return response, nil
}

// listAppStepInfo godoc
// @Summary List step info
// @Description List step info for a specific app and interaction ID, used to build the timeline of events
// @Tags    step_info
// @Produce json
// @Param   interactionId query    string  false  "Interaction ID"
// @Success 200 {array} types.StepInfo
// @Router /api/v1/apps/{id}/step-info [get]
// @Security BearerAuth
func (s *HelixAPIServer) listAppStepInfo(_ http.ResponseWriter, r *http.Request) ([]*types.StepInfo, *system.HTTPError) {
	appID := getID(r)
	user := getRequestUser(r)

	interactionID := r.URL.Query().Get("interactionId")

	app, err := s.Store.GetApp(r.Context(), appID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	if app.Owner != user.ID && !isAdmin(user) {
		return nil, system.NewHTTPError403("you do not have permission to view this app's LLM calls")
	}

	stepInfos, err := s.Store.ListStepInfos(r.Context(), &store.ListStepInfosQuery{
		AppID:         appID,
		InteractionID: interactionID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return stepInfos, nil
}
