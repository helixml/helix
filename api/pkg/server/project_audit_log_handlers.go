package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listProjectAuditLogs godoc
// @Summary List project audit logs
// @Description Get paginated audit logs for a project
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param event_type query string false "Filter by event type"
// @Param user_id query string false "Filter by user ID"
// @Param spec_task_id query string false "Filter by spec task ID"
// @Param start_date query string false "Filter by start date (RFC3339)"
// @Param end_date query string false "Filter by end date (RFC3339)"
// @Param search query string false "Search prompt text"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param offset query int false "Pagination offset"
// @Success 200 {object} types.ProjectAuditLogResponse
// @Failure 400 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Security ApiKeyAuth
// @Router /api/v1/projects/{id}/audit-logs [get]
func (s *HelixAPIServer) listProjectAuditLogs(_ http.ResponseWriter, r *http.Request) (*types.ProjectAuditLogResponse, *system.HTTPError) {
	ctx := r.Context()
	vars := mux.Vars(r)
	projectID := vars["id"]

	// Get user from context
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Verify user has access to project
	project, err := s.Store.GetProject(ctx, projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	// Check authorization
	if err := s.authorizeUserToProject(ctx, user, project, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Parse query parameters
	filters := &types.ProjectAuditLogFilters{
		ProjectID: projectID,
	}

	if eventType := r.URL.Query().Get("event_type"); eventType != "" {
		filters.EventType = types.AuditEventType(eventType)
	}
	if userID := r.URL.Query().Get("user_id"); userID != "" {
		filters.UserID = userID
	}
	if specTaskID := r.URL.Query().Get("spec_task_id"); specTaskID != "" {
		filters.SpecTaskID = specTaskID
	}
	if startDateStr := r.URL.Query().Get("start_date"); startDateStr != "" {
		if startDate, err := time.Parse(time.RFC3339, startDateStr); err == nil {
			filters.StartDate = &startDate
		}
	}
	if endDateStr := r.URL.Query().Get("end_date"); endDateStr != "" {
		if endDate, err := time.Parse(time.RFC3339, endDateStr); err == nil {
			filters.EndDate = &endDate
		}
	}
	if search := r.URL.Query().Get("search"); search != "" {
		filters.Search = search
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filters.Limit = limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filters.Offset = offset
		}
	}

	// Get audit logs
	response, err := s.Store.ListProjectAuditLogs(ctx, filters)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return response, nil
}
