package server

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	webSearchUsageDateLayout   = "2006-01-02"
	webSearchUsageDefaultDays  = 30
	webSearchUsageMaxDays      = 90
	webSearchRequestsPageSize  = 50
	webSearchRequestsMaxLimit  = 200
	webSearchUsageNotFoundBody = "organization not found"
)

var webSearchEndpoints = []types.WebSearchEndpoint{
	types.WebSearchEndpointSearch,
	types.WebSearchEndpointContents,
	types.WebSearchEndpointFindSimilar,
	types.WebSearchEndpointAnswer,
}

func (s *HelixAPIServer) registerWebSearchUsageRoutes(authRouter *mux.Router) {
	authRouter.HandleFunc("/organizations/{org_id}/websearch/usage", system.Wrapper(s.getOrgWebSearchUsage)).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{org_id}/websearch/requests", system.Wrapper(s.listOrgWebSearchRequests)).Methods(http.MethodGet)
}

// getOrgWebSearchUsage godoc
// @Summary Get organization web search usage
// @Description Web search calls made with the organization's API keys and by its agents, per UTC day and endpoint, per endpoint, and in total. Any member of the organization may read it.
// @Tags    websearch
// @Produce json
// @Param   org_id path string true "Organization ID or name"
// @Param   from query string false "First day, YYYY-MM-DD (UTC). Defaults to 29 days before to."
// @Param   to query string false "Last day, YYYY-MM-DD (UTC), inclusive. Defaults to today."
// @Success 200 {object} types.WebSearchUsage
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/organizations/{org_id}/websearch/usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getOrgWebSearchUsage(_ http.ResponseWriter, r *http.Request) (*types.WebSearchUsage, *system.HTTPError) {
	org, err := s.lookupOrg(r.Context(), mux.Vars(r)["org_id"])
	if err != nil {
		return nil, system.NewHTTPError404(webSearchUsageNotFoundBody)
	}
	if _, err := s.authorizeOrgMember(r.Context(), getRequestUser(r), org.ID); err != nil {
		return nil, system.NewHTTPError404(webSearchUsageNotFoundBody)
	}
	q, httpErr := webSearchRequestsQuery(r, org.ID)
	if httpErr != nil {
		return nil, httpErr
	}
	usage, err := s.Store.GetWebSearchUsage(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return usage, nil
}

// listOrgWebSearchRequests godoc
// @Summary List organization web search requests
// @Description The organization's web search calls, newest first, successful and failed. Any member of the organization may read them.
// @Tags    websearch
// @Produce json
// @Param   org_id path string true "Organization ID or name"
// @Param   endpoint query string false "Only calls to this endpoint" Enums(search, contents, findSimilar, answer)
// @Param   from query string false "First day, YYYY-MM-DD (UTC). Defaults to 29 days before to."
// @Param   to query string false "Last day, YYYY-MM-DD (UTC), inclusive. Defaults to today."
// @Param   limit query int false "Page size, at most 200. Defaults to 50."
// @Param   offset query int false "Rows to skip"
// @Success 200 {object} types.WebSearchRequestList
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/organizations/{org_id}/websearch/requests [get]
// @Security BearerAuth
func (s *HelixAPIServer) listOrgWebSearchRequests(_ http.ResponseWriter, r *http.Request) (*types.WebSearchRequestList, *system.HTTPError) {
	org, err := s.lookupOrg(r.Context(), mux.Vars(r)["org_id"])
	if err != nil {
		return nil, system.NewHTTPError404(webSearchUsageNotFoundBody)
	}
	if _, err := s.authorizeOrgMember(r.Context(), getRequestUser(r), org.ID); err != nil {
		return nil, system.NewHTTPError404(webSearchUsageNotFoundBody)
	}
	q, httpErr := webSearchRequestsQuery(r, org.ID)
	if httpErr != nil {
		return nil, httpErr
	}
	params := r.URL.Query()
	if endpoint := types.WebSearchEndpoint(params.Get("endpoint")); endpoint != "" {
		if !slices.Contains(webSearchEndpoints, endpoint) {
			return nil, system.NewHTTPError400(fmt.Sprintf("endpoint must be one of %v", webSearchEndpoints))
		}
		q.Endpoint = endpoint
	}
	if q.Limit, err = intParam(params.Get("limit"), webSearchRequestsPageSize); err != nil || q.Limit < 1 || q.Limit > webSearchRequestsMaxLimit {
		return nil, system.NewHTTPError400(fmt.Sprintf("limit must be between 1 and %d", webSearchRequestsMaxLimit))
	}
	if q.Offset, err = intParam(params.Get("offset"), 0); err != nil || q.Offset < 0 {
		return nil, system.NewHTTPError400("offset must be a non-negative integer")
	}
	list, err := s.Store.ListWebSearchRequests(r.Context(), q)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return list, nil
}

// webSearchRequestsQuery reads the from and to days, both inclusive and in
// UTC, into the half-open range the store queries.
func webSearchRequestsQuery(r *http.Request, orgID string) (*store.WebSearchRequestsQuery, *system.HTTPError) {
	params := r.URL.Query()
	to := time.Now().UTC().Truncate(24 * time.Hour)
	if raw := params.Get("to"); raw != "" {
		day, err := time.Parse(webSearchUsageDateLayout, raw)
		if err != nil {
			return nil, system.NewHTTPError400("to must be a date in the form YYYY-MM-DD")
		}
		to = day
	}
	from := to.AddDate(0, 0, 1-webSearchUsageDefaultDays)
	if raw := params.Get("from"); raw != "" {
		day, err := time.Parse(webSearchUsageDateLayout, raw)
		if err != nil {
			return nil, system.NewHTTPError400("from must be a date in the form YYYY-MM-DD")
		}
		from = day
	}
	if from.After(to) {
		return nil, system.NewHTTPError400("from must not be after to")
	}
	end := to.AddDate(0, 0, 1)
	if end.After(from.AddDate(0, 0, webSearchUsageMaxDays)) {
		return nil, system.NewHTTPError400(fmt.Sprintf("the range may span at most %d days", webSearchUsageMaxDays))
	}
	return &store.WebSearchRequestsQuery{OrganizationID: orgID, From: from, To: end}, nil
}

func intParam(raw string, defaultValue int) (int, error) {
	if raw == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(raw)
}
