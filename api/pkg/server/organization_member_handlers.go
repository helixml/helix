package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listOrganizationMembers godoc
// @Summary List organization members
// @Description List members of an organization, including pending invitations as placeholder rows (user_id starts with "inv_").
// @Tags    organizations
// @Success 200 {array} types.OrganizationMembership
// @Router /api/v1/organizations/{id}/members [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrganizationMembers(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}

	// Check if user has access to view members
	_, err = apiServer.authorizeOrgMember(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	members, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error listing organization members")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Surface pending invitations alongside real members so the same UI
	// (AccessManagement, OrgPeople) can show them without a second endpoint.
	// We synthesise OrganizationMembership rows with user_id set to the
	// invitation ID, the recorded role, and a User stub carrying just the
	// invited email — the frontend keys off the "inv_" prefix to identify
	// these as placeholders and route revoke through DELETE /invitations.
	invitations, err := apiServer.Store.ListOrganizationInvitations(r.Context(), &store.ListOrganizationInvitationsQuery{
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error listing organization invitations")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, inv := range invitations {
		members = append(members, &types.OrganizationMembership{
			OrganizationID: inv.OrganizationID,
			UserID:         inv.ID, // placeholder id (oin_…) so frontend can detect
			CreatedAt:      inv.CreatedAt,
			UpdatedAt:      inv.UpdatedAt,
			Role:           inv.Role,
			User: types.User{
				ID:    inv.ID,
				Email: inv.Email,
			},
		})
	}

	writeResponse(rw, members, http.StatusOK)
}

// addOrganizationMember godoc
// @Summary Add an organization member
// @Description Add a member to an organization. When the user_reference is an email that doesn't match any existing user, a pending invitation is created instead (and an invitation email sent if email is configured).
// @Tags    organizations
// @Success 201 {object} types.AddOrganizationMemberResponse
// @Param request    body types.AddOrganizationMemberRequest true "Request body with user email to add.")
// @Router /api/v1/organizations/{id}/members [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) addOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}

	// Check if user has owner permissions (not just membership)
	_, err = apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Only organization owners can add members: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.AddOrganizationMemberRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserReference == "" {
		http.Error(rw, "user_reference is required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = types.OrganizationRoleMember
	}

	isEmail := strings.Contains(req.UserReference, "@")

	query := &store.GetUserQuery{}
	if isEmail {
		query.Email = req.UserReference
	} else {
		query.ID = req.UserReference
	}

	newMember, err := apiServer.Store.GetUser(r.Context(), query)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Err(err).Msg("error getting user")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// No matching user: if the caller gave us an email, create an
	// invitation row instead. Plain user-IDs that don't resolve are still
	// an error — we don't want to invite by id.
	if newMember == nil {
		if !isEmail {
			http.Error(rw, "User not found", http.StatusNotFound)
			return
		}

		invitation, invErr := apiServer.createOrganizationInvitation(r.Context(), orgID, req.UserReference, req.Role, user)
		if invErr != nil {
			log.Err(invErr).Msg("error creating organization invitation")
			http.Error(rw, invErr.Error(), http.StatusInternalServerError)
			return
		}
		writeResponse(rw, &types.AddOrganizationMemberResponse{
			Invitation: invitation,
			Invited:    true,
		}, http.StatusCreated)
		return
	}

	// Existing user — create the membership directly.
	membership, err := apiServer.Store.CreateOrganizationMembership(r.Context(), &types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         newMember.ID,
		Role:           req.Role,
	})
	if err != nil {
		log.Err(err).Msg("error creating organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, &types.AddOrganizationMemberResponse{
		Membership: membership,
	}, http.StatusCreated)
}

// createOrganizationInvitation persists the invitation row and sends an
// invitation email. If an invitation already exists for this email, the
// existing row is returned and the email is re-sent (idempotent "resend"
// behaviour). Email failures are logged but do not abort the operation —
// owners can still see the pending invitation in the UI.
func (apiServer *HelixAPIServer) createOrganizationInvitation(ctx context.Context, orgID, email string, role types.OrganizationRole, inviter *types.User) (*types.OrganizationInvitation, error) {
	inviterID := ""
	if inviter != nil {
		inviterID = inviter.ID
	}
	invitation, err := apiServer.Store.CreateOrganizationInvitation(ctx, &types.OrganizationInvitation{
		OrganizationID: orgID,
		Email:          email,
		Role:           role,
		InvitedBy:      inviterID,
	})
	if err != nil {
		if errors.Is(err, store.ErrInvitationAlreadyExists) && invitation != nil {
			// Idempotent path: caller is re-inviting; resend the email and
			// return the existing row.
			log.Info().Str("email", email).Str("org_id", orgID).Msg("invitation already exists, resending email")
			apiServer.sendInvitationEmail(ctx, invitation, inviter)
			return invitation, nil
		}
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}

	apiServer.sendInvitationEmail(ctx, invitation, inviter)
	return invitation, nil
}

func (apiServer *HelixAPIServer) sendInvitationEmail(ctx context.Context, invitation *types.OrganizationInvitation, inviter *types.User) {
	if apiServer.Controller == nil || apiServer.Controller.Options.Notifier == nil {
		return
	}

	org, err := apiServer.Store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: invitation.OrganizationID})
	if err != nil {
		log.Warn().Err(err).Str("org_id", invitation.OrganizationID).Msg("failed to load organization for invitation email")
		return
	}

	displayName := org.DisplayName
	if displayName == "" {
		displayName = org.Name
	}

	inviterName := ""
	if inviter != nil {
		if inviter.FullName != "" {
			inviterName = inviter.FullName
		} else {
			inviterName = inviter.Email
		}
	}

	appURL := apiServer.Cfg.Notifications.AppURL
	if appURL == "" {
		appURL = apiServer.Cfg.WebServer.URL
	}
	acceptURL := fmt.Sprintf("%s/login?invitation=%s", strings.TrimRight(appURL, "/"), invitation.ID)

	notification := &types.Notification{
		Event: types.EventOrgInvitation,
		Email: invitation.Email,
		OrgInvitation: &types.OrgInvitationNotification{
			OrganizationName:        org.Name,
			OrganizationDisplayName: displayName,
			InviterName:             inviterName,
			Role:                    string(invitation.Role),
			AcceptURL:               acceptURL,
		},
	}

	go func() {
		// Detach from request lifecycle — the email send shouldn't block the
		// caller's response, and request-cancellation shouldn't cancel SMTP.
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := apiServer.Controller.Options.Notifier.Notify(bgCtx, notification); err != nil {
			log.Warn().Err(err).Str("email", invitation.Email).Str("org_id", invitation.OrganizationID).Msg("failed to send invitation email")
			return
		}
		log.Info().Str("email", invitation.Email).Str("org_id", invitation.OrganizationID).Str("invitation_id", invitation.ID).Msg("invitation email sent")
	}()
}

// publicInvitationInfo godoc
// @Summary Look up basic info for an invitation by id
// @Description Unauthenticated. Returns the invited email and organization display name so the registration page can pre-fill the form. The invitation ID itself acts as the secret token (same threat model as password-reset tokens).
// @Tags    organizations
// @Success 200 {object} types.PublicInvitationInfo
// @Router /api/v1/invitations/{id}/info [get]
func (apiServer *HelixAPIServer) publicInvitationInfo(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		http.Error(rw, "invitation id required", http.StatusBadRequest)
		return
	}

	invitation, err := apiServer.Store.GetOrganizationInvitation(r.Context(), &store.GetOrganizationInvitationQuery{ID: id})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "Invitation not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error loading invitation for public info")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Resolve org display name. Tolerate org-fetch failures: even if the
	// org row is gone, we can still return the email so the user has
	// somewhere to start registration from.
	displayName := ""
	orgName := ""
	if org, err := apiServer.Store.GetOrganization(r.Context(), &store.GetOrganizationQuery{ID: invitation.OrganizationID}); err == nil && org != nil {
		orgName = org.Name
		displayName = org.DisplayName
		if displayName == "" {
			displayName = org.Name
		}
	}

	writeResponse(rw, &types.PublicInvitationInfo{
		ID:                      invitation.ID,
		Email:                   invitation.Email,
		OrganizationName:        orgName,
		OrganizationDisplayName: displayName,
	}, http.StatusOK)
}

// lookupOrgUser godoc
// @Summary Look up a user by email within the context of an organization
// @Description Returns whether a user account exists for the given email, and whether they are already a member of this organization. Used by the invite UI to choose between "send invitation", "add to org", or "add to project" CTAs without revealing arbitrary user information.
// @Tags    organizations
// @Success 200 {object} types.OrgUserLookupResponse
// @Param email query string true "Email to look up"
// @Router /api/v1/organizations/{id}/users/lookup [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) lookupOrgUser(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}
	// Only org owners (the ones who can actually invite) should be able to
	// poke at this — it's effectively a "does this email belong to a Helix
	// account" oracle, scoped per-org.
	if _, err := apiServer.authorizeOrgOwner(r.Context(), user, orgID); err != nil {
		log.Err(err).Msg("error authorizing org owner for user lookup")
		http.Error(rw, "Only organization owners can look up users: "+err.Error(), http.StatusForbidden)
		return
	}

	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" || !strings.Contains(email, "@") {
		http.Error(rw, "email query parameter required and must look like an email", http.StatusBadRequest)
		return
	}
	emailLower := strings.ToLower(email)

	resp := &types.OrgUserLookupResponse{Email: emailLower}

	existing, err := apiServer.Store.GetUser(r.Context(), &store.GetUserQuery{Email: emailLower})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Err(err).Msg("error looking up user by email")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing != nil {
		resp.Exists = true
		resp.UserID = existing.ID
		resp.FullName = existing.FullName

		_, err := apiServer.Store.GetOrganizationMembership(r.Context(), &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         existing.ID,
		})
		if err == nil {
			resp.IsMember = true
		} else if !errors.Is(err, store.ErrNotFound) {
			log.Err(err).Msg("error checking org membership during lookup")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Always check for a pending invitation regardless of whether the user
	// exists — an admin may have invited someone who then created a Helix
	// account elsewhere, in which case both Exists and IsInvited can be
	// true and we want the UI to know about the dangling invitation.
	pending, err := apiServer.Store.GetOrganizationInvitation(r.Context(), &store.GetOrganizationInvitationQuery{
		OrganizationID: orgID,
		Email:          emailLower,
	})
	if err == nil && pending != nil {
		resp.IsInvited = true
		resp.InvitationID = pending.ID
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Err(err).Msg("error checking pending invitation during lookup")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, resp, http.StatusOK)
}

// listOrganizationInvitations godoc
// @Summary List pending organization invitations
// @Description List pending invitations for users who haven't joined the org yet
// @Tags    organizations
// @Success 200 {array} types.OrganizationInvitation
// @Router /api/v1/organizations/{id}/invitations [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrganizationInvitations(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}

	if _, err := apiServer.authorizeOrgMember(r.Context(), user, orgID); err != nil {
		log.Err(err).Msg("error authorizing org member for invitation list")
		http.Error(rw, "Could not authorize: "+err.Error(), http.StatusForbidden)
		return
	}

	invitations, err := apiServer.Store.ListOrganizationInvitations(r.Context(), &store.ListOrganizationInvitationsQuery{
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error listing organization invitations")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if invitations == nil {
		invitations = []*types.OrganizationInvitation{}
	}
	writeResponse(rw, invitations, http.StatusOK)
}

// createOrganizationInvitationHandler godoc
// @Summary Invite a user to the organization by email
// @Description Create a pending invitation for a non-Helix user. When they register with this email they will automatically join the organization.
// @Tags    organizations
// @Success 201 {object} types.OrganizationInvitation
// @Param request    body types.AddOrganizationMemberRequest true "Request body with user_reference (email) and role.")
// @Router /api/v1/organizations/{id}/invitations [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createOrganizationInvitationHandler(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}
	if _, err := apiServer.authorizeOrgOwner(r.Context(), user, orgID); err != nil {
		log.Err(err).Msg("error authorizing org owner for invite")
		http.Error(rw, "Only organization owners can invite users: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.AddOrganizationMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}
	if !strings.Contains(req.UserReference, "@") {
		http.Error(rw, "user_reference must be an email address", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = types.OrganizationRoleMember
	}

	invitation, err := apiServer.createOrganizationInvitation(r.Context(), orgID, req.UserReference, req.Role, user)
	if err != nil {
		log.Err(err).Msg("error creating invitation")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	writeResponse(rw, invitation, http.StatusCreated)
}

// deleteOrganizationInvitation godoc
// @Summary Revoke a pending organization invitation
// @Description Revoke a pending invitation by ID
// @Tags    organizations
// @Success 200
// @Router /api/v1/organizations/{id}/invitations/{invitation_id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteOrganizationInvitation(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}
	invitationID := mux.Vars(r)["invitation_id"]
	if invitationID == "" {
		http.Error(rw, "invitation_id required", http.StatusBadRequest)
		return
	}

	if _, err := apiServer.authorizeOrgOwner(r.Context(), user, orgID); err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Only organization owners can revoke invitations: "+err.Error(), http.StatusForbidden)
		return
	}

	invitation, err := apiServer.Store.GetOrganizationInvitation(r.Context(), &store.GetOrganizationInvitationQuery{ID: invitationID})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "Invitation not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error fetching invitation")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if invitation.OrganizationID != orgID {
		// Don't leak existence of invitations belonging to other orgs.
		http.Error(rw, "Invitation not found", http.StatusNotFound)
		return
	}

	if err := apiServer.Store.DeleteOrganizationInvitation(r.Context(), invitationID); err != nil {
		log.Err(err).Msg("error deleting invitation")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeResponse(rw, nil, http.StatusOK)
}


// removeOrganizationMember godoc
// @Summary Remove an organization member
// @Description Remove a member from an organization
// @Tags    organizations
// @Success 200
// @Router /api/v1/organizations/{id}/members/{user_id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) removeOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}
	userIDToRemove := mux.Vars(r)["user_id"]

	// Check if user has owner permissions (not just membership)
	_, err = apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Only organization owners can remove members: "+err.Error(), http.StatusForbidden)
		return
	}

	// Get the membership we're trying to remove
	memberToRemove, err := apiServer.Store.GetOrganizationMembership(r.Context(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userIDToRemove,
	})
	if err != nil {
		log.Err(err).Msg("error getting organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If the member is an owner, check if they're the last owner
	if memberToRemove.Role == types.OrganizationRoleOwner {
		// Get all owners in the organization
		allMembers, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
			OrganizationID: orgID,
		})
		if err != nil {
			log.Err(err).Msg("error listing organization members")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Count owners
		ownerCount := 0
		for _, member := range allMembers {
			if member.Role == types.OrganizationRoleOwner {
				ownerCount++
			}
		}

		// If this is the last owner, prevent deletion
		if ownerCount <= 1 {
			log.Warn().Msg("attempted to remove the last owner of an organization")
			http.Error(rw, "Cannot remove the last owner of an organization", http.StatusBadRequest)
			return
		}
	}

	// Delete membership (this will cascade delete team memberships in the store layer)
	err = apiServer.Store.DeleteOrganizationMembership(r.Context(), orgID, userIDToRemove)
	if err != nil {
		log.Err(err).Msg("error removing organization member")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, nil, http.StatusOK)
}

// updateOrganizationMember godoc
// @Summary Update an organization member
// @Description Update a member's role in an organization
// @Tags    organizations
// @Success 200 {object} types.OrganizationMembership
// @Param request    body types.UpdateOrganizationMemberRequest true "Request body with role to update to.")
// @Router /api/v1/organizations/{id}/members/{user_id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID, err := apiServer.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}
	userIDToUpdate := mux.Vars(r)["user_id"]

	// Check if user has access to modify members (needs to be an owner)
	_, err = apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.UpdateOrganizationMemberRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing membership
	membership, err := apiServer.Store.GetOrganizationMembership(r.Context(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userIDToUpdate,
	})
	if err != nil {
		log.Err(err).Msg("error getting organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If changing from owner to member, check if they're the last owner
	if membership.Role == types.OrganizationRoleOwner && req.Role != types.OrganizationRoleOwner {
		// Get all owners in the organization
		allMembers, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
			OrganizationID: orgID,
		})
		if err != nil {
			log.Err(err).Msg("error listing organization members")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Count owners
		ownerCount := 0
		for _, member := range allMembers {
			if member.Role == types.OrganizationRoleOwner {
				ownerCount++
			}
		}

		// If this is the last owner, prevent role change
		if ownerCount <= 1 {
			log.Warn().Msg("attempted to change the role of the last owner of an organization")
			http.Error(rw, "Cannot change the role of the last owner of an organization", http.StatusBadRequest)
			return
		}
	}

	// Update role
	membership.Role = req.Role

	// Save updated membership
	updatedMembership, err := apiServer.Store.UpdateOrganizationMembership(r.Context(), membership)
	if err != nil {
		log.Err(err).Msg("error updating organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, updatedMembership, http.StatusOK)
}
