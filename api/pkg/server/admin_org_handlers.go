package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

const (
	defaultAdminOrgsPerPage = 25
	maxAdminOrgsPerPage     = 100
)

// AdminOrganizationsResponse is the paginated response returned by the admin
// organizations endpoint.
type AdminOrganizationsResponse struct {
	Organizations []types.OrgDetails `json:"organizations"`
	Page          int                `json:"page"`
	PageSize      int                `json:"pageSize"`
	TotalCount    int                `json:"totalCount"`
	TotalPages    int                `json:"totalPages"`
}

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
// @Description List organizations with server-side pagination and name search
// @Tags    organizations
// @Param page query int false "Page number (default: 1)"
// @Param per_page query int false "Organizations per page (default: 25, max: 100)"
// @Param query query string false "Search organization display name or name"
// @Success 200 {object} AdminOrganizationsResponse
// @Router /api/v1/admin/orgs [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminListOrganizations(rw http.ResponseWriter, r *http.Request) {
	organizations, err := apiServer.Store.ListOrganizations(r.Context(), &store.ListOrganizationsQuery{})
	if err != nil {
		log.Err(err).Msg("error listing organizations")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Sort first so pagination is stable across requests. Preserve the table's
	// existing search behavior: prefer display_name and fall back to name.
	sort.Slice(organizations, func(i, j int) bool {
		left, right := organizationSearchName(organizations[i]), organizationSearchName(organizations[j])
		if left == right {
			return organizations[i].ID < organizations[j].ID
		}
		return left < right
	})
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("query")))
	if search != "" {
		filtered := organizations[:0]
		for _, org := range organizations {
			if strings.Contains(organizationSearchName(org), search) {
				filtered = append(filtered, org)
			}
		}
		organizations = filtered
	}

	page := positiveQueryInt(r, "page", 1)
	perPage := positiveQueryInt(r, "per_page", defaultAdminOrgsPerPage)
	if perPage > maxAdminOrgsPerPage {
		perPage = maxAdminOrgsPerPage
	}
	totalCount := len(organizations)
	totalPages := (totalCount + perPage - 1) / perPage
	start := (page - 1) * perPage
	if start > totalCount {
		start = totalCount
	}
	end := min(start+perPage, totalCount)
	organizations = organizations[start:end]

	response := AdminOrganizationsResponse{
		Organizations: []types.OrgDetails{},
		Page:          page, PageSize: perPage, TotalCount: totalCount, TotalPages: totalPages,
	}
	if len(organizations) == 0 {
		writeResponse(rw, response, http.StatusOK)
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

	result := make([]types.OrgDetails, len(organizations))
	group, groupContext := errgroup.WithContext(r.Context())
	group.SetLimit(8)
	for i, org := range organizations {
		i, org := i, org
		group.Go(func() error {
			memberships, err := apiServer.Store.ListOrganizationMemberships(groupContext, &store.ListOrganizationMembershipsQuery{
				OrganizationID: org.ID,
			})
			if err != nil {
				return err
			}

			members := make([]types.User, 0, len(memberships))
			for _, membership := range memberships {
				members = append(members, membership.User)
			}

			projects, err := apiServer.Store.ListProjects(groupContext, &store.ListProjectsQuery{
				OrganizationID: org.ID,
			})
			if err != nil {
				return err
			}

			orgProjects := make([]types.Project, 0, len(projects))
			for _, project := range projects {
				orgProjects = append(orgProjects, *project)
			}

			result[i] = types.OrgDetails{
				Organization: *org,
				Wallet:       walletsByOrgID[org.ID],
				Members:      members,
				Projects:     orgProjects,
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		log.Err(err).Msg("error loading organization details")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response.Organizations = result
	writeResponse(rw, response, http.StatusOK)
}

func organizationSearchName(org *types.Organization) string {
	name := org.DisplayName
	if name == "" {
		name = org.Name
	}
	return strings.ToLower(name)
}

func positiveQueryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
