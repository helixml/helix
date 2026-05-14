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
	defaultUsageWindow   = 7 * 24 * time.Hour
	maxUsageWindow       = 366 * 24 * time.Hour
	defaultUsagePageSize = 25
	maxUsagePageSize     = 200
)

// parseUsageQuery turns the request's query string into a GroupedUsageQuery.
// It applies auth: if org_id is set the caller must be a member; if empty
// the caller must be a global admin. The `requireAdmin` flag is for
// admin-only groupings (group_by=org) and rejects org-scoped callers
// regardless of whether they pass org_id.
func (s *HelixAPIServer) parseUsageQuery(r *http.Request, requireAdmin bool) (*store.GroupedUsageQuery, *system.HTTPError) {
	user := getRequestUser(r)
	q := r.URL.Query()

	if requireAdmin && !isAdmin(user) {
		return nil, system.NewHTTPError403("this grouping is admin-only")
	}

	// Org scoping.
	orgParam := q.Get("org_id")
	var orgID string
	if orgParam != "" {
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

	// Date range. Default last 7 days, cap 366 days.
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

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// usageSummary godoc
// @Summary Aggregate usage summary
// @Description Returns whole-set totals plus a daily time-series. Org members must pass org_id; global admins may omit it to summarize cross-org.
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

// usageGrouped godoc
// @Summary Aggregate usage grouped by a dimension
// @Description One endpoint, five groupings. Pass `group_by` to choose what each row represents. `group_by=org` is global-admin only.
//
// @Description Row shape per `group_by` (see types/types.go):
// @Description   - org: UsageByOrg (organization_id, organization_name, user_count, session_count, top_model, last_activity)
// @Description   - user: UsageByUser (user_id, email, organization_id, session_count, top_model, last_activity)
// @Description   - project: UsageByProject (project_id, app_id, name, kind, owner_user_id, organization_id, session_count)
// @Description   - session: UsageBySession (session_id, name, user_id, project_id, organization_id, provider, model, call_count, started_at, ended_at)
// @Description   - model: UsageByModel (provider, model, unique_users, unique_sessions, unique_projects)
//
// @Description Every row also carries the shared UsageTotals fields (prompt/completion/cache tokens and costs, total_cost, request_count).
//
// @Tags    usage
// @Produce json
// @Param   group_by    query  string  true   "Grouping: org|user|project|session|model"
// @Param   from        query  string  false  "Start of window (RFC3339)"
// @Param   to          query  string  false  "End of window (RFC3339)"
// @Param   org_id      query  string  false  "Organization id or slug"
// @Param   user_id     query  string  false  "Filter by user id"
// @Param   project_id  query  string  false  "Filter by project id"
// @Param   app_id      query  string  false  "Filter by app id"
// @Param   session_id  query  string  false  "Filter by session id"
// @Param   provider    query  string  false  "Filter by provider"
// @Param   model       query  string  false  "Filter by model"
// @Param   sort_by     query  string  false  "Sort column (per-grouping whitelist)"
// @Param   sort_dir    query  string  false  "asc or desc"
// @Param   page        query  int     false  "Page (1-indexed)"
// @Param   page_size   query  int     false  "Page size (max 200, default 25)"
// @Success 200 {object} types.UsageGroupedResponse
// @Router /api/v1/usage/aggregate/grouped [get]
// @Security BearerAuth
func (s *HelixAPIServer) usageGrouped(_ http.ResponseWriter, r *http.Request) (*types.UsageGroupedResponse, *system.HTTPError) {
	groupBy := r.URL.Query().Get("group_by")
	if groupBy == "" {
		return nil, system.NewHTTPError400("group_by is required")
	}
	q, httpErr := s.parseUsageQuery(r, groupBy == "org")
	if httpErr != nil {
		return nil, httpErr
	}

	var rows any
	var total int
	var err error
	switch groupBy {
	case "org":
		rows, total, err = unpackGroup[*types.UsageByOrg](s.Store.GetUsageGroupedByOrg(r.Context(), q))
	case "user":
		rows, total, err = unpackGroup[*types.UsageByUser](s.Store.GetUsageGroupedByUser(r.Context(), q))
	case "project":
		rows, total, err = unpackGroup[*types.UsageByProject](s.Store.GetUsageGroupedByProject(r.Context(), q))
	case "session":
		rows, total, err = unpackGroup[*types.UsageBySession](s.Store.GetUsageGroupedBySession(r.Context(), q))
	case "model":
		rows, total, err = unpackGroup[*types.UsageByModel](s.Store.GetUsageGroupedByModel(r.Context(), q))
	default:
		return nil, system.NewHTTPError400("group_by must be one of: org, user, project, session, model")
	}
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	totals, err := s.usageTotalsFor(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return &types.UsageGroupedResponse{
		GroupBy:    groupBy,
		Rows:       rows,
		Total:      *totals,
		Page:       max1(q.Page),
		PageSize:   q.PageSize,
		TotalRows:  total,
		TotalPages: pageTotals(total, q.PageSize),
	}, nil
}

// unpackGroup is a tiny generic helper that lets the switch above call
// the typed store methods uniformly without losing the row type.
func unpackGroup[T any](rows []T, total int, err error) (any, int, error) {
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
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
