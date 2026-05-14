package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	defaultUsageWindow      = 7 * 24 * time.Hour
	maxUsageWindow          = 366 * 24 * time.Hour
	defaultUsagePageSize    = 25
	maxUsagePageSize        = 200
	maxUsageExportRows      = 100_000
)

// parseUsageQuery turns the request's query string into a GroupedUsageQuery.
// It applies auth: if org_id is set the caller must be a member; if empty
// the caller must be a global admin. by-org callers pass requireAdmin=true
// to reject org-scoped callers regardless of org_id.
func (s *HelixAPIServer) parseUsageQuery(r *http.Request, requireAdmin bool) (*store.GroupedUsageQuery, *system.HTTPError) {
	user := getRequestUser(r)
	q := r.URL.Query()

	// Org scoping.
	orgParam := q.Get("org_id")
	var orgID string
	if orgParam != "" {
		if requireAdmin && !isAdmin(user) {
			return nil, system.NewHTTPError403("by-org listing is admin-only")
		}
		org, err := s.lookupOrg(r.Context(), orgParam)
		if err != nil {
			return nil, system.NewHTTPError404("organization not found")
		}
		if _, err := s.authorizeOrgMember(r.Context(), user, org.ID); err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}
		orgID = org.ID
	} else if !isAdmin(user) {
		return nil, system.NewHTTPError403("org_id is required for non-admin callers")
	}

	// Date range. Default to last 7 days, cap at 366 days.
	now := time.Now()
	to := now
	from := now.Add(-defaultUsageWindow)
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, system.NewHTTPError400("from must be RFC3339")
		}
		from = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, system.NewHTTPError400("to must be RFC3339")
		}
		to = t
	}
	if !to.After(from) {
		return nil, system.NewHTTPError400("to must be after from")
	}
	if to.Sub(from) > maxUsageWindow {
		return nil, system.NewHTTPError400("date range exceeds 366 days")
	}

	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = defaultUsagePageSize
	}
	if pageSize > maxUsagePageSize {
		pageSize = maxUsagePageSize
	}

	return &store.GroupedUsageQuery{
		From:           from,
		To:             to,
		OrganizationID: orgID,
		UserID:         q.Get("user_id"),
		ProjectID:      q.Get("project_id"),
		AppID:          q.Get("app_id"),
		SessionID:      q.Get("session_id"),
		Provider:       q.Get("provider"),
		Model:          q.Get("model"),
		SortBy:         q.Get("sort_by"),
		SortDir:        q.Get("sort_dir"),
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

func pageTotals(totalRows, pageSize int) int {
	if pageSize <= 0 || totalRows <= 0 {
		return 0
	}
	return (totalRows + pageSize - 1) / pageSize
}

// usageSummary godoc
// @Summary Aggregate usage summary
// @Description Returns whole-set totals plus a daily time-series. Org owners must pass org_id matching their org; global admins may omit it to summarize cross-org.
// @Tags    usage
// @Produce json
// @Param   from        query  string  false  "Start of window (RFC3339). Defaults to 7 days ago."
// @Param   to          query  string  false  "End of window (RFC3339). Defaults to now."
// @Param   org_id      query  string  false  "Organization id or slug. Required for non-admins."
// @Param   user_id     query  string  false  "Filter by user id"
// @Param   project_id  query  string  false  "Filter by project id"
// @Param   app_id      query  string  false  "Filter by app id"
// @Param   provider    query  string  false  "Filter by provider"
// @Param   model       query  string  false  "Filter by model"
// @Success 200 {object} types.UsageSummary
// @Router /api/v1/usage/aggregate/summary [get]
// @Security BearerAuth
func (s *HelixAPIServer) usageSummary(_ http.ResponseWriter, r *http.Request) (*types.UsageSummary, *system.HTTPError) {
	q, httpErr := s.parseUsageQuery(r, false)
	if httpErr != nil {
		return nil, httpErr
	}
	res, err := s.Store.GetUsageSummary(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return res, nil
}

// usageByOrg godoc
// @Summary Usage grouped by organization
// @Description Global-admin only. One row per organization with tokens, costs, user/session counts and last activity.
// @Tags    usage
// @Produce json
// @Param   from       query  string  false  "Start of window (RFC3339)"
// @Param   to         query  string  false  "End of window (RFC3339)"
// @Param   sort_by    query  string  false  "Sort column: total_cost, total_tokens, request_count, last_activity"
// @Param   sort_dir   query  string  false  "asc or desc"
// @Param   page       query  int     false  "Page (1-indexed)"
// @Param   page_size  query  int     false  "Page size (max 200, default 25)"
// @Success 200 {object} types.PaginatedUsageByOrg
// @Router /api/v1/usage/aggregate/by-org [get]
// @Security BearerAuth
func (s *HelixAPIServer) usageByOrg(_ http.ResponseWriter, r *http.Request) (*types.PaginatedUsageByOrg, *system.HTTPError) {
	if !isAdmin(getRequestUser(r)) {
		return nil, system.NewHTTPError403("by-org listing is admin-only")
	}
	q, httpErr := s.parseUsageQuery(r, true)
	if httpErr != nil {
		return nil, httpErr
	}
	rows, total, err := s.Store.GetUsageGroupedByOrg(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	totals, err := s.usageTotalsFor(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return &types.PaginatedUsageByOrg{
		Rows:       rows,
		Total:      *totals,
		Page:       max1(q.Page),
		PageSize:   q.PageSize,
		TotalRows:  total,
		TotalPages: pageTotals(total, q.PageSize),
	}, nil
}

// usageByUser godoc
// @Summary Usage grouped by user
// @Description Org members get rows scoped to their org. Global admins may omit org_id to list across all orgs.
// @Tags    usage
// @Produce json
// @Param   from       query  string  false  "Start of window (RFC3339)"
// @Param   to         query  string  false  "End of window (RFC3339)"
// @Param   org_id     query  string  false  "Organization id or slug. Required for non-admins."
// @Param   sort_by    query  string  false  "Sort column"
// @Param   sort_dir   query  string  false  "asc or desc"
// @Param   page       query  int     false  "Page"
// @Param   page_size  query  int     false  "Page size"
// @Success 200 {object} types.PaginatedUsageByUser
// @Router /api/v1/usage/aggregate/by-user [get]
// @Security BearerAuth
func (s *HelixAPIServer) usageByUser(_ http.ResponseWriter, r *http.Request) (*types.PaginatedUsageByUser, *system.HTTPError) {
	q, httpErr := s.parseUsageQuery(r, false)
	if httpErr != nil {
		return nil, httpErr
	}
	rows, total, err := s.Store.GetUsageGroupedByUser(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	totals, err := s.usageTotalsFor(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return &types.PaginatedUsageByUser{
		Rows:       rows,
		Total:      *totals,
		Page:       max1(q.Page),
		PageSize:   q.PageSize,
		TotalRows:  total,
		TotalPages: pageTotals(total, q.PageSize),
	}, nil
}

// usageByProject godoc
// @Summary Usage grouped by project / app / agent
// @Tags    usage
// @Produce json
// @Param   from       query  string  false  "Start of window (RFC3339)"
// @Param   to         query  string  false  "End of window (RFC3339)"
// @Param   org_id     query  string  false  "Organization id or slug"
// @Param   user_id    query  string  false  "Filter by user id"
// @Param   sort_by    query  string  false  "Sort column"
// @Param   sort_dir   query  string  false  "asc or desc"
// @Param   page       query  int     false  "Page"
// @Param   page_size  query  int     false  "Page size"
// @Success 200 {object} types.PaginatedUsageByProject
// @Router /api/v1/usage/aggregate/by-project [get]
// @Security BearerAuth
func (s *HelixAPIServer) usageByProject(_ http.ResponseWriter, r *http.Request) (*types.PaginatedUsageByProject, *system.HTTPError) {
	q, httpErr := s.parseUsageQuery(r, false)
	if httpErr != nil {
		return nil, httpErr
	}
	rows, total, err := s.Store.GetUsageGroupedByProject(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	totals, err := s.usageTotalsFor(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return &types.PaginatedUsageByProject{
		Rows:       rows,
		Total:      *totals,
		Page:       max1(q.Page),
		PageSize:   q.PageSize,
		TotalRows:  total,
		TotalPages: pageTotals(total, q.PageSize),
	}, nil
}

// usageBySession godoc
// @Summary Usage grouped by session
// @Tags    usage
// @Produce json
// @Param   from        query  string  false  "Start of window (RFC3339)"
// @Param   to          query  string  false  "End of window (RFC3339)"
// @Param   org_id      query  string  false  "Organization id or slug"
// @Param   user_id     query  string  false  "Filter by user id"
// @Param   project_id  query  string  false  "Filter by project id"
// @Param   provider    query  string  false  "Filter by provider"
// @Param   model       query  string  false  "Filter by model"
// @Param   sort_by     query  string  false  "Sort column"
// @Param   sort_dir    query  string  false  "asc or desc"
// @Param   page        query  int     false  "Page"
// @Param   page_size   query  int     false  "Page size"
// @Success 200 {object} types.PaginatedUsageBySession
// @Router /api/v1/usage/aggregate/by-session [get]
// @Security BearerAuth
func (s *HelixAPIServer) usageBySession(_ http.ResponseWriter, r *http.Request) (*types.PaginatedUsageBySession, *system.HTTPError) {
	q, httpErr := s.parseUsageQuery(r, false)
	if httpErr != nil {
		return nil, httpErr
	}
	rows, total, err := s.Store.GetUsageGroupedBySession(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	totals, err := s.usageTotalsFor(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return &types.PaginatedUsageBySession{
		Rows:       rows,
		Total:      *totals,
		Page:       max1(q.Page),
		PageSize:   q.PageSize,
		TotalRows:  total,
		TotalPages: pageTotals(total, q.PageSize),
	}, nil
}

// usageByModel godoc
// @Summary Usage grouped by model / provider
// @Tags    usage
// @Produce json
// @Param   from       query  string  false  "Start of window (RFC3339)"
// @Param   to         query  string  false  "End of window (RFC3339)"
// @Param   org_id     query  string  false  "Organization id or slug"
// @Param   sort_by    query  string  false  "Sort column"
// @Param   sort_dir   query  string  false  "asc or desc"
// @Param   page       query  int     false  "Page"
// @Param   page_size  query  int     false  "Page size"
// @Success 200 {object} types.PaginatedUsageByModel
// @Router /api/v1/usage/aggregate/by-model [get]
// @Security BearerAuth
func (s *HelixAPIServer) usageByModel(_ http.ResponseWriter, r *http.Request) (*types.PaginatedUsageByModel, *system.HTTPError) {
	q, httpErr := s.parseUsageQuery(r, false)
	if httpErr != nil {
		return nil, httpErr
	}
	rows, total, err := s.Store.GetUsageGroupedByModel(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	totals, err := s.usageTotalsFor(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return &types.PaginatedUsageByModel{
		Rows:       rows,
		Total:      *totals,
		Page:       max1(q.Page),
		PageSize:   q.PageSize,
		TotalRows:  total,
		TotalPages: pageTotals(total, q.PageSize),
	}, nil
}

// usageTotalsFor reuses GetUsageSummary to get the cross-page totals.
// Cheaper than another round of aggregate SQL since summary already
// produces them.
func (s *HelixAPIServer) usageTotalsFor(ctx context.Context, q *store.GroupedUsageQuery) (*types.UsageTotals, error) {
	sum, err := s.Store.GetUsageSummary(ctx, q)
	if err != nil {
		return nil, err
	}
	return &sum.UsageTotals, nil
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
