package server

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

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
