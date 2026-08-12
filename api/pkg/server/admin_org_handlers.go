package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SetOrgPlanRequest is the body for POST /admin/orgs/{id}/plan.
type SetOrgPlanRequest struct {
	// Plan: "pro" | "free" forces the org's quota tier independent of Stripe
	// (for customers who paid out-of-band). "" clears the override and reverts
	// to the Stripe-derived tier.
	Plan string `json:"plan"`
}

// adminSetOrgPlan godoc
// @Summary Set an organization's plan override (admin only)
// @Description Force an org's quota tier independent of Stripe — for customers who paid out-of-band. plan: "pro" | "free" | "" (clear). Never reverted by a Stripe webhook.
// @Tags    organizations
// @Param id path string true "Organization ID"
// @Param request body SetOrgPlanRequest true "Plan override"
// @Success 200 {object} types.Wallet
// @Router /api/v1/admin/orgs/{id}/plan [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminSetOrgPlan(rw http.ResponseWriter, r *http.Request) {
	orgID := mux.Vars(r)["id"]
	if orgID == "" {
		http.Error(rw, "organization id is required", http.StatusBadRequest)
		return
	}
	var body SetOrgPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	switch body.Plan {
	case "", types.PlanOverridePro, types.PlanOverrideFree:
	default:
		http.Error(rw, "plan must be one of: pro, free, or empty", http.StatusBadRequest)
		return
	}

	adminUser := getRequestUser(r)
	wallet, err := apiServer.getOrCreateWallet(r.Context(), adminUser, orgID)
	if err != nil {
		log.Err(err).Str("org_id", orgID).Msg("failed to get/create org wallet")
		http.Error(rw, "failed to get org wallet: "+err.Error(), http.StatusInternalServerError)
		return
	}
	wallet.PlanOverride = body.Plan
	updated, err := apiServer.Store.UpdateWallet(r.Context(), wallet)
	if err != nil {
		log.Err(err).Str("org_id", orgID).Msg("failed to update org wallet plan override")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info().Str("admin_id", adminUser.ID).Str("org_id", orgID).Str("plan", body.Plan).
		Msg("admin set org plan override")
	writeResponse(rw, updated, http.StatusOK)
}

// adminListOrganizations godoc
// @Summary List organizations with wallets (admin only)
// @Description List all organizations
// @Tags    organizations
// @Success 200 {array} types.OrgDetails
// @Router /api/v1/admin/orgs [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminListOrganizations(rw http.ResponseWriter, r *http.Request) {
	organizations, err := apiServer.Store.ListOrganizations(r.Context(), &store.ListOrganizationsQuery{})
	if err != nil {
		log.Err(err).Msg("error listing organizations")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(organizations) == 0 {
		writeResponse(rw, []types.OrgDetails{}, http.StatusOK)
		return
	}

	orgIDs := make([]string, 0, len(organizations))
	for _, org := range organizations {
		orgIDs = append(orgIDs, org.ID)
	}

	wallets, err := apiServer.Store.ListWallets(r.Context(), &store.ListWalletsQuery{
		OwnerIDs:  orgIDs,
		OwnerType: types.OwnerTypeOrg,
	})
	if err != nil {
		log.Err(err).Msg("error listing organization wallets")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	walletsByOrgID := make(map[string]types.Wallet, len(wallets))
	for _, wallet := range wallets {
		walletsByOrgID[wallet.OrgID] = *wallet
	}

	result := make([]types.OrgDetails, 0, len(organizations))
	for _, org := range organizations {
		memberships, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
			OrganizationID: org.ID,
		})
		if err != nil {
			log.Err(err).Msg("error listing organization memberships")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		members := make([]types.User, 0, len(memberships))
		for _, membership := range memberships {
			members = append(members, membership.User)
		}

		projects, err := apiServer.Store.ListProjects(r.Context(), &store.ListProjectsQuery{
			OrganizationID: org.ID,
		})
		if err != nil {
			log.Err(err).Msg("error listing organization projects")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		orgProjects := make([]types.Project, 0, len(projects))
		for _, project := range projects {
			orgProjects = append(orgProjects, *project)
		}

		result = append(result, types.OrgDetails{
			Organization: *org,
			Wallet:       walletsByOrgID[org.ID],
			Members:      members,
			Projects:     orgProjects,
		})
	}

	writeResponse(rw, result, http.StatusOK)
}
