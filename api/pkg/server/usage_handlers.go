package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
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

	// from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	// to := time.Now()
	specTaskID := getID(r)

	aggregationLevel := store.AggregationLevelHourly
	switch r.URL.Query().Get("aggregation_level") {
	case "daily":
		aggregationLevel = store.AggregationLevelDaily
	case "5min":
		aggregationLevel = store.AggregationLevel5Min
	}

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
		// Default to spec task creation time
		if specTask.StartedAt != nil {
			from = *specTask.StartedAt
		} else {
			from = specTask.CreatedAt
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
