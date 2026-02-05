package server

import (
	"encoding/json"
	"net/http"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// resourceSearch godoc
// @Summary Search across resources
// @Description Search across projects, tasks, sessions, prompts, knowledge, repositories, and apps concurrently
// @Tags search
// @Accept json
// @Produce json
// @Param request body types.ResourceSearchRequest true "Search request"
// @Success 200 {object} types.ResourceSearchResponse
// @Router /api/v1/resource-search [post]
// @Security BearerAuth
func (s *HelixAPIServer) resourceSearch(_ http.ResponseWriter, r *http.Request) (*types.ResourceSearchResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	var req types.ResourceSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}

	if req.Query == "" {
		return nil, system.NewHTTPError400("query is required")
	}

	if req.OrganizationID != "" {
		_, err := s.authorizeOrgMember(ctx, user, req.OrganizationID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}
	}

	// Set owner from authenticated user
	req.UserID = user.ID

	resp, err := s.Store.ResourceSearch(ctx, &req)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return resp, nil
}
