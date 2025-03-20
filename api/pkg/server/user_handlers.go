package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// searchUsers godoc
// @Summary Search users
// @Description Search users by email, name, or username
// @Tags    users
// @Success 200 {object} types.UserSearchResponse
// @Param query query string true "Query"
// @Param organization_id query string false "Organization ID"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Router /api/v1/users/search [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) searchUsers(rw http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := &store.SearchUsersQuery{
		Query:          r.URL.Query().Get("query"),
		OrganizationID: r.URL.Query().Get("organization_id"),
	}

	if query.Query == "" {
		http.Error(rw, "query is required", http.StatusBadRequest)
		return
	}

	if query.OrganizationID != "" {
		_, err := apiServer.authorizeOrgMember(r.Context(), getRequestUser(r), query.OrganizationID)
		if err != nil {
			http.Error(rw, fmt.Sprintf("error authorizing organization member: %v", err), http.StatusForbidden)
			return
		}
		// OK
	}

	// If query is less than 2 characters, return an error
	if len(query.Query) < 2 {
		http.Error(rw, "query must be at least 2 characters", http.StatusBadRequest)
		return
	}

	// Parse pagination parameters
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid limit parameter: %v", err), http.StatusBadRequest)
			return
		}
		query.Limit = limit
	} else {
		// Default limit
		query.Limit = 20
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid offset parameter: %v", err), http.StatusBadRequest)
			return
		}
		query.Offset = offset
	}

	// Execute the search
	users, total, err := apiServer.Store.SearchUsers(r.Context(), query)
	if err != nil {
		http.Error(rw, fmt.Sprintf("error searching users: %v", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, &types.UserSearchResponse{
		Users:      users,
		TotalCount: total,
		Limit:      query.Limit,
		Offset:     query.Offset,
	}, http.StatusOK)
}
