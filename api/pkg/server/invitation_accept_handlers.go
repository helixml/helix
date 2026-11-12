package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// invitationAcceptURL is the link sent to an invitee: the login page, which
// registers or signs the user in and then accepts with the token.
func (apiServer *HelixAPIServer) invitationAcceptURL(invitation *types.OrganizationInvitation) string {
	appURL := apiServer.Cfg.Notifications.AppURL
	if appURL == "" {
		appURL = apiServer.Cfg.WebServer.URL
	}
	return fmt.Sprintf("%s/login?invitation=%s&token=%s", strings.TrimRight(appURL, "/"),
		url.QueryEscape(invitation.ID), url.QueryEscape(invitation.Token))
}

// acceptInvitation accepts invitation id for user when token is its secret
// token and it was sent to the user's email. An accepted invitation means the
// user has an org and is vouched for by an org owner, so onboarding is marked
// complete and the waitlist lifted. Returns whether it was accepted.
func (apiServer *HelixAPIServer) acceptInvitation(ctx context.Context, user *types.User, id, token string) (bool, error) {
	consumed, err := apiServer.Store.ConsumeInvitation(ctx, user, id, token)
	if err != nil {
		return false, err
	}
	if len(consumed) == 0 {
		return false, nil
	}
	log.Info().Str("user_id", user.ID).Str("invitation_id", id).Msg("accepted org invitation")
	user.OnboardingCompleted = true
	user.OnboardingCompletedAt = time.Now()
	user.Waitlisted = false
	if _, err := apiServer.Store.UpdateUser(ctx, user); err != nil {
		log.Warn().Err(err).Str("user_id", user.ID).Msg("failed to mark invited user as onboarded")
	}
	return true, nil
}

// acceptInvitationHandler godoc
// @Summary Accept an organization invitation
// @Description Accept the invitation from an invite link as the signed-in user. Requires the invitation's secret token from the link, and the account's email must be the invited address. Works for waitlisted accounts (an invitation lifts the waitlist).
// @Tags    organizations
// @Param id path string true "Invitation ID"
// @Param request body types.AcceptInvitationRequest true "Token from the invite link"
// @Success 200 {object} types.UserResponse
// @Router /api/v1/invitations/{id}/accept [post]
func (apiServer *HelixAPIServer) acceptInvitationHandler(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// This route is outside extractMiddleware so that waitlisted accounts
	// (refused there) can accept the invitation that lifts the waitlist; the
	// session and CSRF checks are done here instead.
	if apiServer.sessionManager == nil {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}
	session, err := apiServer.sessionManager.GetSessionFromRequest(ctx, r)
	if err != nil || session == nil {
		http.Error(rw, "Sign in to accept the invitation", http.StatusUnauthorized)
		return
	}
	if !authpkg.ValidateCSRF(r) {
		http.Error(rw, "CSRF validation failed", http.StatusForbidden)
		return
	}
	user, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: session.UserID})
	if err != nil {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := user.AccountAccessError(); err != nil && !errors.Is(err, types.ErrAccountWaitlisted) {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	var req types.AcceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		http.Error(rw, "token is required", http.StatusBadRequest)
		return
	}

	accepted, err := apiServer.acceptInvitation(ctx, user, mux.Vars(r)["id"], req.Token)
	if err != nil {
		log.Err(err).Msg("error accepting invitation")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !accepted {
		// Unknown ID, wrong token, already used, or sent to another address.
		http.Error(rw, "This invitation link is invalid, has been used, or was sent to a different email address", http.StatusNotFound)
		return
	}
	writeResponse(rw, userResponse(user, ""), http.StatusOK)
}

// createInvitationLink godoc
// @Summary Issue a new invite link for a pending invitation
// @Description Owners only. Generates a new secret token for the invitation and returns its invite link, for sending outside email. Links issued earlier (including the emailed one) stop working.
// @Tags    organizations
// @Success 200 {object} types.InvitationLinkResponse
// @Router /api/v1/organizations/{id}/invitations/{invitation_id}/link [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createInvitationLink(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}
	if _, err := apiServer.authorizeOrgOwner(r.Context(), user, orgID); err != nil {
		http.Error(rw, "Only organization owners can issue invite links", http.StatusForbidden)
		return
	}
	invitationID := mux.Vars(r)["invitation_id"]
	existing, err := apiServer.Store.GetOrganizationInvitation(r.Context(), &store.GetOrganizationInvitationQuery{ID: invitationID})
	if err != nil || existing.OrganizationID != orgID {
		http.Error(rw, "Invitation not found", http.StatusNotFound)
		return
	}
	invitation, err := apiServer.Store.RotateOrganizationInvitationToken(r.Context(), invitationID)
	if err != nil {
		log.Err(err).Msg("error rotating invitation token")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
	writeResponse(rw, &types.InvitationLinkResponse{URL: apiServer.invitationAcceptURL(invitation)}, http.StatusOK)
}

// canManageOrgInvitations mirrors authorizeOrgOwner for a membership the
// caller already holds: org owners and global admins manage invitations.
func canManageOrgInvitations(user *types.User, membership *types.OrganizationMembership) bool {
	return (user != nil && user.Admin) || (membership != nil && membership.Role == types.OrganizationRoleOwner)
}

// redactInvitations hides invitation IDs from callers who may not manage
// invitations: the ID identifies the invitation in links, revocation and
// lookups, which are owner business.
func redactInvitations(invitations []*types.OrganizationInvitation) []*types.OrganizationInvitation {
	out := make([]*types.OrganizationInvitation, 0, len(invitations))
	for _, inv := range invitations {
		c := *inv
		c.ID = ""
		c.InvitedBy = ""
		out = append(out, &c)
	}
	return out
}
