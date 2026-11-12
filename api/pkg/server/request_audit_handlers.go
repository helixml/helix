package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listRequestAuditEvents godoc
// @Summary  List request audit events
// @Description Admin-only forensic read of the request audit trail, newest first.
// @Tags     admin
// @Produce  json
// @Param    user_id     query    string false "Filter by user ID"
// @Param    remote_addr query    string false "Filter by TCP remote address"
// @Param    path        query    string false "Filter by path prefix"
// @Param    method      query    string false "Filter by HTTP method"
// @Param    status      query    int    false "Filter by response status"
// @Param    since       query    string false "Only events created at or after this RFC3339 time"
// @Param    until       query    string false "Only events created at or before this RFC3339 time"
// @Param    limit       query    int    false "Page size (default 50, max 500)"
// @Param    offset      query    int    false "Page offset"
// @Success  200 {object} types.RequestAuditEventResponse
// @Router   /api/v1/admin/audit-events [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listRequestAuditEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filters := &types.RequestAuditEventFilters{
		UserID:     query.Get("user_id"),
		RemoteAddr: query.Get("remote_addr"),
		PathPrefix: query.Get("path"),
		Method:     query.Get("method"),
	}

	if raw := query.Get("status"); raw != "" {
		status, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, "invalid status filter", http.StatusBadRequest)
			return
		}
		filters.Status = &status
	}
	if raw := query.Get("since"); raw != "" {
		since, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid since filter: expected RFC3339", http.StatusBadRequest)
			return
		}
		filters.Since = &since
	}
	if raw := query.Get("until"); raw != "" {
		until, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid until filter: expected RFC3339", http.StatusBadRequest)
			return
		}
		filters.Until = &until
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 0 {
			http.Error(w, "invalid limit filter", http.StatusBadRequest)
			return
		}
		filters.Limit = limit
	}
	if raw := query.Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			http.Error(w, "invalid offset filter", http.StatusBadRequest)
			return
		}
		filters.Offset = offset
	}

	response, err := apiServer.Store.ListRequestAuditEvents(r.Context(), filters)
	if err != nil {
		log.Err(err).Msg("failed to list request audit events")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeResponse(w, response, http.StatusOK)
}
