package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GrantCreditsRequest is the body for POST /admin/users/{id}/credits.
//
// org_id is required when the target user owns one or more organisations;
// admins must pick explicitly which wallet receives the grant. It must be
// omitted when the user owns no organisations (the grant is stashed on the
// user and applied to their first owned org via consumeUserAdminCredits).
type GrantCreditsRequest struct {
	Credits float64 `json:"credits"`
	OrgID   string  `json:"org_id,omitempty"`
}

// OwnedOrgSummary is the per-org payload returned by
// GET /admin/users/{id}/owned-orgs. Kept narrow on purpose: the admin
// dialog only needs id + name to populate its org picker.
type OwnedOrgSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

// GrantCreditsResponse describes the outcome of an admin credit grant.
//
// Status values:
//   - "stashed": user has no orgs yet; credits parked on the user and applied
//     when they create their first org via consumeUserAdminCredits.
//   - "applied": wallet on the user's oldest owned org was topped up right now,
//     independent of any active or absent Stripe subscription.
type GrantCreditsResponse struct {
	User   *types.User `json:"user"`
	OrgID  string      `json:"org_id,omitempty"`
	Status string      `json:"status"`
}

// consumeUserAdminCredits applies any admin-stashed credit grant on the user
// to the given org's wallet. Called after an org (and its owning membership)
// has been created. Best-effort: errors are logged but do not fail the caller,
// so a Stripe outage doesn't block org creation.
//
// On success: wallet balance is topped up by the stashed credits, the user's
// PendingAdminCreditsOnFirstOrg field is cleared so subsequent orgs don't
// trigger again.
func (s *HelixAPIServer) consumeUserAdminCredits(ctx context.Context, user *types.User, orgID string) {
	if user == nil || user.PendingAdminCreditsOnFirstOrg == nil || *user.PendingAdminCreditsOnFirstOrg <= 0 {
		return
	}
	if !s.Cfg.Stripe.BillingEnabled {
		log.Warn().
			Str("user_id", user.ID).
			Str("org_id", orgID).
			Msg("user has stashed admin credit grant but billing is disabled; skipping")
		return
	}

	credits := *user.PendingAdminCreditsOnFirstOrg

	wallet, err := s.getOrCreateWallet(ctx, user, orgID)
	if err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Str("org_id", orgID).
			Msg("failed to get/create wallet for admin credit consumption")
		return
	}

	if _, err := s.Store.UpdateWalletBalance(ctx, wallet.ID, credits, types.TransactionMetadata{
		TransactionType: types.TransactionTypeAdminGrant,
	}); err != nil {
		log.Warn().Err(err).
			Str("wallet_id", wallet.ID).
			Float64("credits", credits).
			Msg("failed to apply stashed admin credit grant to wallet")
		return
	}

	user.PendingAdminCreditsOnFirstOrg = nil
	if _, err := s.Store.UpdateUser(ctx, user); err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Msg("failed to clear PendingAdminCreditsOnFirstOrg after consumption")
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("org_id", orgID).
		Str("wallet_id", wallet.ID).
		Float64("credits", credits).
		Msg("admin-stashed credit grant applied to wallet")
}

// adminGrantCredits godoc
// @Summary Grant credits to a user (Admin, cloud only)
// @Description Adds credits to the wallet of an explicitly chosen organisation the user owns, or stashes the grant on the user when they own no organisations yet (the grant is applied to their first owned org on creation). Works regardless of subscription state.
// @Tags    users
// @Accept  json
// @Produce json
// @Param id path string true "User ID"
// @Param request body GrantCreditsRequest true "Credits to grant (must be > 0) and the target org_id (required iff user owns ≥1 orgs)"
// @Success 200 {object} GrantCreditsResponse
// @Router /api/v1/admin/users/{id}/credits [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminGrantCredits(_ http.ResponseWriter, req *http.Request) (*GrantCreditsResponse, error) {
	ctx := req.Context()
	adminUser := getRequestUser(req)
	if !adminUser.Admin {
		return nil, system.NewHTTPError403("only admins can grant credits")
	}
	if apiServer.Cfg.Edition != "cloud" {
		return nil, system.NewHTTPError400("admin credit grants are only available on the cloud edition")
	}
	if !apiServer.Cfg.Stripe.BillingEnabled {
		return nil, system.NewHTTPError400("Stripe billing must be enabled")
	}

	targetUserID := mux.Vars(req)["id"]
	if targetUserID == "" {
		return nil, system.NewHTTPError400("user ID is required")
	}

	body := GrantCreditsRequest{}
	if req.Body == nil || req.ContentLength == 0 {
		return nil, system.NewHTTPError400("request body is required")
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}
	if body.Credits <= 0 {
		return nil, system.NewHTTPError400("credits must be greater than 0")
	}

	targetUser, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError404("user not found")
	}

	ownedOrgs, err := apiServer.Store.ListOrganizations(ctx, &store.ListOrganizationsQuery{Owner: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError500("failed to list user organizations: " + err.Error())
	}

	// Path A: no owned org yet — stash on user, consumeUserAdminCredits applies
	// it when the user creates their first org. Reject any org_id here since
	// there's no org to validate it against.
	if len(ownedOrgs) == 0 {
		if body.OrgID != "" {
			return nil, system.NewHTTPError400("user owns no organisations; remove org_id to stash the grant for their first owned org")
		}
		credits := body.Credits
		targetUser.PendingAdminCreditsOnFirstOrg = &credits
		updated, err := apiServer.Store.UpdateUser(ctx, targetUser)
		if err != nil {
			return nil, system.NewHTTPError500("failed to stash admin credit grant: " + err.Error())
		}
		log.Info().
			Str("admin_id", adminUser.ID).
			Str("target_user_id", targetUserID).
			Float64("credits", credits).
			Msg("admin stashed credit grant on user (no org yet)")
		return &GrantCreditsResponse{User: updated, Status: "stashed"}, nil
	}

	// Path B: user owns one or more orgs. org_id MUST be supplied so the
	// admin's intent is explicit; silent "oldest owned" picks land credits
	// on the wrong wallet when the target user owns several orgs.
	if body.OrgID == "" {
		return nil, system.NewHTTPError400(fmt.Sprintf("org_id is required: user owns %d organisation(s), pick which one receives the grant", len(ownedOrgs)))
	}

	var selectedOrg *types.Organization
	for _, org := range ownedOrgs {
		if org.ID == body.OrgID {
			selectedOrg = org
			break
		}
	}
	if selectedOrg == nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("user does not own organisation %s", body.OrgID))
	}

	wallet, err := apiServer.getOrCreateWallet(ctx, targetUser, selectedOrg.ID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to get wallet for selected org: " + err.Error())
	}

	if _, err := apiServer.Store.UpdateWalletBalance(ctx, wallet.ID, body.Credits, types.TransactionMetadata{
		TransactionType: types.TransactionTypeAdminGrant,
	}); err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update wallet balance: %s", err))
	}

	log.Info().
		Str("admin_id", adminUser.ID).
		Str("target_user_id", targetUserID).
		Str("org_id", selectedOrg.ID).
		Str("wallet_id", wallet.ID).
		Float64("credits", body.Credits).
		Msg("admin granted credits to org wallet")

	return &GrantCreditsResponse{User: targetUser, OrgID: selectedOrg.ID, Status: "applied"}, nil
}

// listUserOwnedOrgs godoc
// @Summary List a user's owned organisations (Admin, cloud only)
// @Description Returns the organisations the target user is the owner of, sorted by creation time ascending. Used by the admin "Grant credits" dialog to populate its org picker.
// @Tags    users
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {array} OwnedOrgSummary
// @Router /api/v1/admin/users/{id}/owned-orgs [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listUserOwnedOrgs(_ http.ResponseWriter, req *http.Request) ([]OwnedOrgSummary, error) {
	ctx := req.Context()
	adminUser := getRequestUser(req)
	if !adminUser.Admin {
		return nil, system.NewHTTPError403("only admins can list a user's owned organisations")
	}
	if apiServer.Cfg.Edition != "cloud" {
		return nil, system.NewHTTPError400("only available on the cloud edition")
	}

	targetUserID := mux.Vars(req)["id"]
	if targetUserID == "" {
		return nil, system.NewHTTPError400("user ID is required")
	}

	orgs, err := apiServer.Store.ListOrganizations(ctx, &store.ListOrganizationsQuery{Owner: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError500("failed to list user organizations: " + err.Error())
	}

	sort.Slice(orgs, func(i, j int) bool { return orgs[i].CreatedAt.Before(orgs[j].CreatedAt) })

	out := make([]OwnedOrgSummary, 0, len(orgs))
	for _, org := range orgs {
		out = append(out, OwnedOrgSummary{
			ID:          org.ID,
			Name:        org.Name,
			DisplayName: org.DisplayName,
		})
	}
	return out, nil
}
