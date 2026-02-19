package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getUsage godoc
// @Summary Get daily usage
// @Description Get daily usage
// @Accept json
// @Produce json
// @Tags    usage
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Param   org_id query string false "Organization ID"
// @Param   project_id query string false "Project ID"
// @Param   spec_task_id query string false "Spec Task ID"
// @Param   aggregation_level query string false "Aggregation level"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getUsage(_ http.ResponseWriter, r *http.Request) ([]*types.AggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()
	orgID := r.URL.Query().Get("org_id")
	projectID := r.URL.Query().Get("project_id")
	specTaskID := r.URL.Query().Get("spec_task_id")

	aggregationLevel := store.AggregationLevelDaily
	if r.URL.Query().Get("aggregation_level") == "hourly" {
		aggregationLevel = store.AggregationLevelHourly
	}

	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	if orgID != "" {
		// Lookup org
		org, err := s.lookupOrg(r.Context(), orgID)
		if err != nil {
			return nil, system.NewHTTPError404(err.Error())
		}

		orgID = org.ID

		_, err = s.authorizeOrgMember(r.Context(), user, orgID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}
	}

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

	metrics, err := s.Store.GetAggregatedUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
		AggregationLevel: aggregationLevel,
		UserID:           user.ID,
		OrganizationID:   orgID,
		ProjectID:        projectID,
		SpecTaskID:       specTaskID,
		From:             from,
		To:               to,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return metrics, nil
}

// getSpecTaskUsage godoc
// @Summary Get spec task usage
// @Description Get spec task usage
// @Accept json
// @Produce json
// @Tags    spec-tasks
// @Param   taskId path string true "Spec Task ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Param   aggregation_level query string false "Aggregation level"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{taskId}/usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSpecTaskUsage(_ http.ResponseWriter, r *http.Request) ([]*types.AggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)

	specTaskID := getID(r)

	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	specTask, err := s.Store.GetSpecTask(r.Context(), specTaskID)
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}

	err = s.authorizeUserToProjectByID(r.Context(), user, specTask.ProjectID, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	var from time.Time
	var to time.Time

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	} else {
		switch {
		case specTask.PlanningStartedAt != nil:
			from = *specTask.PlanningStartedAt
		case specTask.StartedAt != nil:
			from = *specTask.StartedAt
		default:
			from = specTask.CreatedAt
		}

		minFrom := time.Now().Add(-30 * time.Minute)
		if from.After(minFrom) {
			from = minFrom
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	} else {
		// Default to now
		to = time.Now()
	}

	// Auto-select aggregation level based on time range to keep data points between 20-50
	// This prevents huge payloads for long-running tasks (e.g., 7-day task at 5min = 2016 points)
	aggregationLevel := selectAggregationLevel(from, to)

	metrics, err := s.Store.GetAggregatedUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
		AggregationLevel: aggregationLevel,
		UserID:           user.ID,
		ProjectID:        specTask.ProjectID,
		SpecTaskID:       specTaskID,
		From:             from,
		To:               to,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return metrics, nil
}

// selectAggregationLevel picks the finest granularity that keeps data points between minPoints and maxPoints
func selectAggregationLevel(from, to time.Time) store.AggregationLevel {
	const minPoints = 20
	const maxPoints = 50

	duration := to.Sub(from)
	pointsAt5Min := int(duration.Minutes() / 5)
	pointsAtHourly := int(duration.Hours())

	// Start with finest granularity and coarsen if too many points
	if pointsAt5Min <= maxPoints {
		return store.AggregationLevel5Min
	}
	if pointsAtHourly >= minPoints && pointsAtHourly <= maxPoints {
		return store.AggregationLevelHourly
	}
	if pointsAtHourly > maxPoints {
		return store.AggregationLevelDaily
	}
	// pointsAtHourly < minPoints but pointsAt5Min > maxPoints
	// Use hourly anyway (slightly sparse is better than 1000+ points)
	return store.AggregationLevelHourly
}

// BatchTaskUsageResponse contains usage metrics for all tasks in a project
type BatchTaskUsageResponse struct {
	ProjectID string                                    `json:"project_id"`
	Tasks     map[string][]*types.AggregatedUsageMetric `json:"tasks"` // keyed by task_id
}

// getBatchTaskUsage godoc
// @Summary Get usage for all tasks in a project
// @Description Get usage metrics for all spec-driven tasks in a project in a single request. This is more efficient than calling the individual usage endpoint for each task.
// @Tags    spec-driven-tasks
// @Produce json
// @Param   id path string true "Project ID"
// @Success 200 {object} BatchTaskUsageResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/projects/{id}/tasks-usage [get]
func (s *HelixAPIServer) getBatchTaskUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	projectID := vars["id"]

	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to access the project
	if err := s.authorizeUserToProjectByID(ctx, user, projectID, types.ActionGet); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get all non-archived tasks for this project
	tasks, err := s.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:       projectID,
		IncludeArchived: false,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to list tasks for batch usage")
		http.Error(w, "failed to list tasks", http.StatusInternalServerError)
		return
	}

	// Build response with usage for each task
	response := BatchTaskUsageResponse{
		ProjectID: projectID,
		Tasks:     make(map[string][]*types.AggregatedUsageMetric, len(tasks)),
	}

	// Fetch usage in parallel with concurrency limit
	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 10) // Limit to 10 concurrent DB queries

	for _, task := range tasks {
		wg.Add(1)
		go func(t *types.SpecTask) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Calculate time range for this task
			var from time.Time
			switch {
			case t.PlanningStartedAt != nil:
				from = *t.PlanningStartedAt
			case t.StartedAt != nil:
				from = *t.StartedAt
			default:
				from = t.CreatedAt
			}

			// Ensure minimum 30 min range
			minFrom := time.Now().Add(-30 * time.Minute)
			if from.After(minFrom) {
				from = minFrom
			}

			to := time.Now()
			aggregationLevel := selectAggregationLevel(from, to)

			metrics, err := s.Store.GetAggregatedUsageMetrics(ctx, &store.GetAggregatedUsageMetricsQuery{
				AggregationLevel: aggregationLevel,
				UserID:           user.ID,
				ProjectID:        projectID,
				SpecTaskID:       t.ID,
				From:             from,
				To:               to,
			})
			if err != nil {
				log.Error().Err(err).Str("task_id", t.ID).Msg("Failed to get usage for task")
				return
			}

			mu.Lock()
			response.Tasks[t.ID] = metrics
			mu.Unlock()
		}(task)
	}

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
