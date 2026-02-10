package server

import (
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// getQuotasHandler godoc
// @Summary Get quotas
// @Description Get quotas for the user. Returns current usage and limits for desktops, projects, repositories, and spec tasks. Optionally pass org_id query parameter to get organization quotas.
// @Tags quotas
// @Param org_id query string false "Organization ID to get quotas for"
// @Success 200 {object} types.QuotaResponse
// @Router /api/v1/quotas [get]
// @Security BearerAuth
func (s *HelixAPIServer) getQuotasHandler(rw http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)

	orgID := req.URL.Query().Get("org_id") // Optional

	org, err := s.lookupOrg(req.Context(), orgID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to lookup org: %s", err), http.StatusInternalServerError)
		return
	}

	if orgID != "" {
		// Authorize org membe
		_, err := s.authorizeOrgMember(req.Context(), user, org.ID)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusForbidden)
			return
		}
	}

	quotas, err := s.quotaManager.GetQuotas(req.Context(), &types.QuotaRequest{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, quotas, http.StatusOK)
}
