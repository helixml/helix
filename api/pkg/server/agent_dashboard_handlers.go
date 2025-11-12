package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getAgentDashboard godoc
// @Summary Get enhanced dashboard data with agent management
// @Description Get comprehensive dashboard data including agent sessions, work queue, and help requests
// @Tags    dashboard
// @Success 200 {object} types.AgentDashboardSummary
// @Router /api/v1/dashboard/agent [get]
// @Security BearerAuth
func (s *HelixAPIServer) getAgentDashboard(_ http.ResponseWriter, r *http.Request) (*types.AgentDashboardSummary, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Get base dashboard data
	baseDashboard, err := s.Controller.GetDashboardData(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get base dashboard data")
		return nil, system.NewHTTPError500("failed to get dashboard data")
	}

	// Get active sessions
	sessionsQuery := &store.ListAgentSessionsQuery{
		Page:       0,
		PageSize:   100,
		ActiveOnly: true,
	}
	sessionsResponse, err := s.Store.ListAgentSessionStatus(ctx, sessionsQuery)
	if err != nil {
		log.Error().Err(err).Msg("failed to get active sessions")
		return nil, system.NewHTTPError500("failed to get active sessions")
	}

	// Get sessions needing help
	sessionsNeedingHelpQuery := &store.ListAgentSessionsQuery{
		Page:       0,
		PageSize:   50,
		Status:     "waiting_for_help",
		ActiveOnly: false,
	}
	sessionsNeedingHelpResponse, err := s.Store.ListAgentSessionStatus(ctx, sessionsNeedingHelpQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get sessions needing help")
		sessionsNeedingHelpResponse = &types.AgentSessionsResponse{Sessions: []*types.AgentSessionStatus{}}
	}

	// Get pending work
	pendingWorkQuery := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "pending",
		OrderBy:  "priority ASC, created_at ASC",
	}
	pendingWorkResponse, err := s.Store.ListAgentWorkItems(ctx, pendingWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending work")
		pendingWorkResponse = &types.AgentWorkItemsListResponse{WorkItems: []*types.AgentWorkItem{}}
	}

	// Get running work
	runningWorkQuery := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "in_progress",
		OrderBy:  "started_at DESC",
	}
	runningWorkResponse, err := s.Store.ListAgentWorkItems(ctx, runningWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get running work")
		runningWorkResponse = &types.AgentWorkItemsListResponse{WorkItems: []*types.AgentWorkItem{}}
	}

	// Get recent completions
	recentCompletions, err := s.Store.GetRecentCompletions(ctx, 20)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get recent completions")
		recentCompletions = []*types.JobCompletion{}
	}

	// Get pending reviews
	pendingReviews, err := s.Store.GetPendingReviews(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending reviews")
		pendingReviews = []*types.JobCompletion{}
	}

	// Get active help requests
	activeHelpRequests, err := s.Store.ListActiveHelpRequests(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get active help requests")
		activeHelpRequests = []*types.HelpRequest{}
	}

	// Get work queue stats
	workQueueStats, err := s.Store.GetAgentWorkQueueStats(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get work queue stats")
		workQueueStats = &types.AgentWorkQueueStats{
			ByAgentType: make(map[string]int),
			BySource:    make(map[string]int),
			ByPriority:  make(map[string]int),
		}
	}

	// Convert AgentSession to AgentSessionStatus for summary
	// Use AgentSessionStatus directly since we're now querying the correct table
	activeSessions := sessionsResponse.Sessions
	sessionsNeedingHelpStatus := sessionsNeedingHelpResponse.Sessions

	return &types.AgentDashboardSummary{
		DashboardData:       baseDashboard,
		ActiveSessions:      activeSessions,
		SessionsNeedingHelp: sessionsNeedingHelpStatus,
		PendingWork:         pendingWorkResponse.WorkItems,
		RunningWork:         runningWorkResponse.WorkItems,
		RecentCompletions:   recentCompletions,
		PendingReviews:      pendingReviews,
		ActiveHelpRequests:  activeHelpRequests,
		WorkQueueStats:      workQueueStats,
		LastUpdated:         time.Now(),
	}, nil
}

// getAgentFleet godoc
// @Summary Get agent fleet data
// @Description Get agent fleet data including active sessions, work queue, and help requests without dashboard data
// @Tags    agents
// @Success 200 {object} types.AgentFleetSummary
// @Router /api/v1/agents/fleet [get]
// @Security BearerAuth
func (s *HelixAPIServer) getAgentFleet(_ http.ResponseWriter, r *http.Request) (*types.AgentFleetSummary, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Get active sessions
	sessionsQuery := &store.ListAgentSessionsQuery{
		Page:       0,
		PageSize:   100,
		ActiveOnly: true,
	}
	sessionsResponse, err := s.Store.ListAgentSessionStatus(ctx, sessionsQuery)
	if err != nil {
		log.Error().Err(err).Msg("failed to get active sessions")
		return nil, system.NewHTTPError500("failed to get active sessions")
	}

	// Get sessions needing help
	sessionsNeedingHelpQuery := &store.ListAgentSessionsQuery{
		Page:       0,
		PageSize:   50,
		Status:     "waiting_for_help",
		ActiveOnly: false,
	}
	sessionsNeedingHelpResponse, err := s.Store.ListAgentSessionStatus(ctx, sessionsNeedingHelpQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get sessions needing help")
		sessionsNeedingHelpResponse = &types.AgentSessionsResponse{Sessions: []*types.AgentSessionStatus{}}
	}

	// Get pending work
	pendingWorkQuery := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "pending",
		OrderBy:  "priority ASC, created_at ASC",
	}
	pendingWorkResponse, err := s.Store.ListAgentWorkItems(ctx, pendingWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending work")
		pendingWorkResponse = &types.AgentWorkItemsListResponse{WorkItems: []*types.AgentWorkItem{}}
	}

	// Get running work
	runningWorkQuery := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "in_progress",
		OrderBy:  "started_at DESC",
	}
	runningWorkResponse, err := s.Store.ListAgentWorkItems(ctx, runningWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get running work")
		runningWorkResponse = &types.AgentWorkItemsListResponse{WorkItems: []*types.AgentWorkItem{}}
	}

	// Get recent completions
	recentCompletions, err := s.Store.GetRecentCompletions(ctx, 20)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get recent completions")
		recentCompletions = []*types.JobCompletion{}
	}

	// Get pending reviews
	pendingReviews, err := s.Store.GetPendingReviews(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending reviews")
		pendingReviews = []*types.JobCompletion{}
	}

	// Get active help requests
	activeHelpRequests, err := s.Store.ListActiveHelpRequests(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get active help requests")
		activeHelpRequests = []*types.HelpRequest{}
	}

	// Get work queue stats
	workQueueStats, err := s.Store.GetAgentWorkQueueStats(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get work queue stats")
		workQueueStats = &types.AgentWorkQueueStats{
			ByAgentType: make(map[string]int),
			BySource:    make(map[string]int),
			ByPriority:  make(map[string]int),
		}
	}

	// Use AgentSessionStatus directly since we're now querying the correct table
	activeSessions := sessionsResponse.Sessions

	sessionsNeedingHelpStatus := sessionsNeedingHelpResponse.Sessions

	// Get external agent runner connections (both sync and runner connections)
	var externalAgentRunners []*types.ExternalAgentConnection

	// Get Zed instance connections (via /external-agents/sync)
	if s.externalAgentWSManager != nil {
		connections := s.externalAgentWSManager.listConnections()
		externalAgentRunners = make([]*types.ExternalAgentConnection, len(connections))
		for i := range connections {
			externalAgentRunners[i] = &connections[i]
		}
	}

	// Get external agent runner connections (via /ws/external-agent-runner)
	if s.externalAgentRunnerManager != nil {
		runnerConnections := s.externalAgentRunnerManager.listConnections()
		// Append runner connections to the list
		for i := range runnerConnections {
			externalAgentRunners = append(externalAgentRunners, &runnerConnections[i])
		}
	}

	// If neither manager exists, return empty slice
	if s.externalAgentWSManager == nil && s.externalAgentRunnerManager == nil {
		externalAgentRunners = []*types.ExternalAgentConnection{}
	}

	return &types.AgentFleetSummary{
		ActiveSessions:       activeSessions,
		SessionsNeedingHelp:  sessionsNeedingHelpStatus,
		ExternalAgentRunners: externalAgentRunners,
		PendingWork:          pendingWorkResponse.WorkItems,
		RunningWork:          runningWorkResponse.WorkItems,
		RecentCompletions:    recentCompletions,
		PendingReviews:       pendingReviews,
		ActiveHelpRequests:   activeHelpRequests,
		WorkQueueStats:       workQueueStats,
		LastUpdated:          time.Now(),
	}, nil
}

// listAgentSessions godoc
// @Summary List agent sessions
// @Description List agent sessions with filtering and pagination
// @Tags    agents
// @Success 200 {object} types.AgentSessionsResponse
// @Param page query int false "Page number" default(0)
// @Param page_size query int false "Page size" default(20)
// @Param status query string false "Session status filter"
// @Param agent_type query string false "Agent type filter"
// @Param active_only query bool false "Show only active sessions" default(false)
// @Router /api/v1/agents/sessions [get]
// @Security BearerAuth
func (s *HelixAPIServer) listAgentSessions(_ http.ResponseWriter, r *http.Request) (*types.AgentSessionsResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}

	status := r.URL.Query().Get("status")
	agentType := r.URL.Query().Get("agent_type")
	activeOnly := r.URL.Query().Get("active_only") == "true"

	query := &store.ListAgentSessionsQuery{
		Page:       page,
		PageSize:   pageSize,
		Status:     status,
		AgentType:  agentType,
		ActiveOnly: activeOnly,
	}

	response, err := s.Store.ListAgentSessions(ctx, query)
	if err != nil {
		log.Error().Err(err).Msg("failed to list agent sessions")
		return nil, system.NewHTTPError500("failed to list agent sessions")
	}

	return &types.AgentSessionsResponse{
		Sessions: make([]*types.AgentSessionStatus, len(response.Sessions)),
		Total:    response.Total,
		Page:     response.Page,
		PageSize: response.PageSize,
	}, nil
}

// listAgentWorkItems godoc
// @Summary List agent work items
// @Description List work items in the agent queue with filtering and pagination
// @Tags    agents
// @Success 200 {object} types.AgentWorkItemsResponse
// @Param page query int false "Page number" default(0)
// @Param page_size query int false "Page size" default(20)
// @Param status query string false "Work item status filter"
// @Param agent_type query string false "Agent type filter"
// @Param source query string false "Work item source filter"
// @Router /api/v1/agents/work [get]
// @Security BearerAuth
func (s *HelixAPIServer) listAgentWorkItems(_ http.ResponseWriter, r *http.Request) (*types.AgentWorkItemsResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}

	status := r.URL.Query().Get("status")
	agentType := r.URL.Query().Get("agent_type")
	source := r.URL.Query().Get("source")

	query := &store.ListAgentWorkItemsQuery{
		Page:      page,
		PageSize:  pageSize,
		Status:    status,
		AgentType: agentType,
		Source:    source,
		UserID:    user.ID, // Only show work items for this user
	}

	response, err := s.Store.ListAgentWorkItems(ctx, query)
	if err != nil {
		log.Error().Err(err).Msg("failed to list agent work items")
		return nil, system.NewHTTPError500("failed to list agent work items")
	}

	return &types.AgentWorkItemsResponse{
		Items:    response.WorkItems,
		Total:    response.Total,
		Page:     response.Page,
		PageSize: response.PageSize,
	}, nil
}

// createAgentWorkItem godoc
// @Summary Create a new agent work item
// @Description Create a new work item in the agent queue
// @Tags    agents
// @Success 201 {object} types.AgentWorkItem
// @Param request body types.AgentWorkItemCreateRequest true "Work item details"
// @Router /api/v1/agents/work [post]
// @Security BearerAuth
func (s *HelixAPIServer) createAgentWorkItem(_ http.ResponseWriter, r *http.Request) (*types.AgentWorkItem, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	var req types.AgentWorkItemCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Validate required fields
	if req.Name == "" {
		return nil, system.NewHTTPError400("name is required")
	}
	if req.AgentType == "" {
		return nil, system.NewHTTPError400("agent_type is required")
	}

	// Create work item
	workItem := &types.AgentWorkItem{
		ID:          system.GenerateUUID(),
		Name:        req.Name,
		Description: req.Description,
		Source:      req.Source,
		SourceID:    req.SourceID,
		SourceURL:   req.SourceURL,
		Priority:    req.Priority,
		Status:      "pending",
		AgentType:   req.AgentType,
		UserID:      user.ID,
		MaxRetries:  req.MaxRetries,
	}

	if req.MaxRetries == 0 {
		workItem.MaxRetries = 3 // Default
	}

	// Set work data, config, and labels (GORM serializer handles JSON conversion)
	workItem.WorkData = req.WorkData
	workItem.Config = req.Config
	workItem.Labels = req.Labels

	err := s.Store.CreateAgentWorkItem(ctx, workItem)
	if err != nil {
		log.Error().Err(err).Msg("failed to create agent work item")
		return nil, system.NewHTTPError500("failed to create work item")
	}

	log.Info().
		Str("work_item_id", workItem.ID).
		Str("user_id", user.ID).
		Str("name", workItem.Name).
		Msg("created agent work item")

	return workItem, nil
}

// getAgentWorkItem godoc
// @Summary Get an agent work item
// @Description Get details of a specific work item
// @Tags    agents
// @Success 200 {object} types.AgentWorkItem
// @Param work_item_id path string true "Work item ID"
// @Router /api/v1/agents/work/{work_item_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getAgentWorkItem(_ http.ResponseWriter, r *http.Request) (*types.AgentWorkItem, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	workItemID := vars["work_item_id"]

	workItem, err := s.Store.GetAgentWorkItem(ctx, workItemID)
	if err != nil {
		log.Error().Err(err).Str("work_item_id", workItemID).Msg("failed to get work item")
		return nil, system.NewHTTPError404("work item not found")
	}

	// Check authorization - only allow access to own work items or admin
	if workItem.UserID != user.ID {
		return nil, system.NewHTTPError403("forbidden")
	}

	return workItem, nil
}

// updateAgentWorkItem godoc
// @Summary Update an agent work item
// @Description Update details of a specific work item
// @Tags    agents
// @Success 200 {object} types.AgentWorkItem
// @Param work_item_id path string true "Work item ID"
// @Param request body types.AgentWorkItemUpdateRequest true "Update details"
// @Router /api/v1/agents/work/{work_item_id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateAgentWorkItem(_ http.ResponseWriter, r *http.Request) (*types.AgentWorkItem, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	workItemID := vars["work_item_id"]

	var req types.AgentWorkItemUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Get existing work item
	workItem, err := s.Store.GetAgentWorkItem(ctx, workItemID)
	if err != nil {
		return nil, system.NewHTTPError404("work item not found")
	}

	// Check authorization
	if workItem.UserID != user.ID {
		return nil, system.NewHTTPError403("forbidden")
	}

	// Apply updates
	if req.Name != nil {
		workItem.Name = *req.Name
	}
	if req.Description != nil {
		workItem.Description = *req.Description
	}
	if req.Priority != nil {
		workItem.Priority = *req.Priority
	}
	if req.Status != nil {
		workItem.Status = *req.Status
	}
	// GORM serializer handles JSON conversion
	if req.WorkData != nil {
		workItem.WorkData = req.WorkData
	}
	if req.Config != nil {
		workItem.Config = req.Config
	}
	if req.Labels != nil {
		workItem.Labels = req.Labels
	}

	err = s.Store.UpdateAgentWorkItem(ctx, workItem)
	if err != nil {
		log.Error().Err(err).Str("work_item_id", workItemID).Msg("failed to update work item")
		return nil, system.NewHTTPError500("failed to update work item")
	}

	return workItem, nil
}

// listHelpRequests godoc
// @Summary List help requests
// @Description List help requests from agents needing human assistance
// @Tags    agents
// @Success 200 {object} types.HelpRequestsListResponse
// @Param page query int false "Page number" default(0)
// @Param page_size query int false "Page size" default(20)
// @Param status query string false "Help request status filter"
// @Param urgency query string false "Urgency level filter"
// @Router /api/v1/agents/help-requests [get]
// @Security BearerAuth
func (s *HelixAPIServer) listHelpRequests(_ http.ResponseWriter, r *http.Request) (*types.HelpRequestsListResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}

	status := r.URL.Query().Get("status")
	urgency := r.URL.Query().Get("urgency")

	query := &store.ListHelpRequestsQuery{
		Page:     page,
		PageSize: pageSize,
		Status:   status,
		Urgency:  urgency,
		UserID:   user.ID, // Only show help requests for this user
	}

	response, err := s.Store.ListHelpRequests(ctx, query)
	if err != nil {
		log.Error().Err(err).Msg("failed to list help requests")
		return nil, system.NewHTTPError500("failed to list help requests")
	}

	return response, nil
}

// resolveHelpRequest godoc
// @Summary Resolve a help request
// @Description Provide resolution for a help request from an agent
// @Tags    agents
// @Success 200 {object} types.HelpRequest
// @Param request_id path string true "Help request ID"
// @Param request body map[string]string true "Resolution details"
// @Router /api/v1/agents/help-requests/{request_id}/resolve [post]
// @Security BearerAuth
func (s *HelixAPIServer) resolveHelpRequest(_ http.ResponseWriter, r *http.Request) (*types.HelpRequest, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	requestID := vars["request_id"]

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	resolution := req["resolution"]
	if resolution == "" {
		return nil, system.NewHTTPError400("resolution is required")
	}

	// Get the help request
	helpRequest, err := s.Store.GetHelpRequestByID(ctx, requestID)
	if err != nil {
		return nil, system.NewHTTPError404("help request not found")
	}

	// Update the help request
	helpRequest.Status = "resolved"
	helpRequest.ResolvedBy = user.ID
	helpRequest.Resolution = resolution
	resolvedAt := time.Now()
	helpRequest.ResolvedAt = &resolvedAt

	err = s.Store.UpdateHelpRequest(ctx, helpRequest)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestID).Msg("failed to resolve help request")
		return nil, system.NewHTTPError500("failed to resolve help request")
	}

	// Mark the associated session as active
	if helpRequest.SessionID != "" {
		err = s.Store.MarkSessionAsActive(ctx, helpRequest.SessionID, "Resumed after human assistance")
		if err != nil {
			log.Warn().Err(err).Str("session_id", helpRequest.SessionID).Msg("failed to mark session as active")
		}
	}

	log.Info().
		Str("request_id", requestID).
		Str("resolved_by", user.ID).
		Str("session_id", helpRequest.SessionID).
		Msg("resolved help request")

	return helpRequest, nil
}

// getWorkQueueStats godoc
// @Summary Get work queue statistics
// @Description Get statistics about the agent work queue
// @Tags    agents
// @Success 200 {object} types.AgentWorkQueueStats
// @Router /api/v1/agents/stats [get]
// @Security BearerAuth
func (s *HelixAPIServer) getWorkQueueStats(_ http.ResponseWriter, r *http.Request) (*types.AgentWorkQueueStats, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	stats, err := s.Store.GetAgentWorkQueueStats(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get work queue stats")
		return nil, system.NewHTTPError500("failed to get work queue stats")
	}

	return stats, nil
}
