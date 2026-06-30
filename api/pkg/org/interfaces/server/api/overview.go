package api

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// ---- Org overview -------------------------------------------------------

// getOverview returns a flat list of every Bot in the org for the React
// Overview page. The page renders the reporting graph from the bots +
// their parent_ids (fetched via GET /bots).
//
// @Summary Helix-org: get org overview
// @Description Returns the flat set of Bots in the org for the helix-org React Overview page.
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.OrgOverview
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/overview [get]
func (a *apiHandler) getOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	bs, err := a.deps.Queries.ListBots(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list bots: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, buildOverview(bs))
}

// buildOverview collapses the Bot set into the flat overview payload.
func buildOverview(bots []orgchart.Bot) OrgOverview {
	out := OrgOverview{Bots: make([]BotBadge, 0, len(bots))}
	for _, b := range bots {
		out.Bots = append(out.Bots, BotBadge{ID: string(b.ID)})
	}
	sort.SliceStable(out.Bots, func(i, j int) bool { return out.Bots[i].ID < out.Bots[j].ID })
	return out
}
