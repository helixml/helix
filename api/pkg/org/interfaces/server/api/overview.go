package api

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// ---- Org overview -------------------------------------------------------

// getOverview returns the workers-grouped-by-role payload used by the
// React Overview page (replaces the old position-tree chart).
//
// @Summary Helix-org: get org overview
// @Description Returns roles + workers grouped by role for the helix-org React Overview page.
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
	workers, err := a.deps.Queries.ListWorkers(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list workers: %w", err))
		return
	}
	roles, err := a.deps.Queries.ListRoles(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list roles: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, buildOverview(workers, roles))
}

// buildOverview groups workers by their RoleID.
func buildOverview(workers []orgchart.Worker, roles []orgchart.Role) OrgOverview {
	byRole := make(map[orgchart.BotID][]WorkerBadge)
	for _, wk := range workers {
		rid := wk.RoleID()
		byRole[rid] = append(byRole[rid], WorkerBadge{ID: string(wk.ID()), Kind: string(wk.Kind())})
	}
	sortedRoles := append([]orgchart.Role(nil), roles...)
	sort.SliceStable(sortedRoles, func(i, j int) bool { return sortedRoles[i].ID < sortedRoles[j].ID })
	out := OrgOverview{
		Roles:  make([]RoleBadge, 0, len(sortedRoles)),
		Groups: make([]RoleGroup, 0, len(sortedRoles)),
	}
	for _, ro := range sortedRoles {
		out.Roles = append(out.Roles, RoleBadge{ID: string(ro.ID)})
		group := RoleGroup{RoleID: string(ro.ID), Workers: byRole[ro.ID]}
		sort.SliceStable(group.Workers, func(i, j int) bool { return group.Workers[i].ID < group.Workers[j].ID })
		out.Groups = append(out.Groups, group)
	}
	return out
}
