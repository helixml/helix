package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// getDailyUsage godoc
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
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getDailyUsage(_ http.ResponseWriter, r *http.Request) ([]*types.AggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()
	orgID := r.URL.Query().Get("org_id")
	projectID := r.URL.Query().Get("project_id")
	specTaskID := r.URL.Query().Get("spec_task_id")

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
		from, err = time.Parse(time.DateOnly, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.DateOnly, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	}

	metrics, err := s.Store.GetAggregatedUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
		UserID:         user.ID,
		OrganizationID: orgID,
		ProjectID:      projectID,
		SpecTaskID:     specTaskID,
		From:           from,
		To:             to,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return metrics, nil
}
