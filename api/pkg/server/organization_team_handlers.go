package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listTeams godoc
// @Summary List teams in an organization
// @Description List all teams in an organization. Organization members can list teams.
// @Tags    organizations
// @Success 200 {array} types.Team
// @Router /api/v1/organizations/{id}/teams [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listTeams(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]

	// Check if user has access to view teams
	err := apiServer.authorizeOrgMember(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org member")
		http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
		return
	}

	teams, err := apiServer.Store.ListTeams(r.Context(), &store.ListTeamsQuery{
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error listing teams")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, teams, http.StatusOK)
}

// createTeam godoc
// @Summary Create a new team
// @Description Create a new team in an organization. Only organization owners can create teams.
// @Tags    organizations
// @Accept  json
// @Produce json
// @Param request body types.CreateTeamRequest true "Request body"
// @Success 201 {object} types.Team
// @Router /api/v1/organizations/{id}/teams [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createTeam(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]

	// Check if user has access to create teams (needs to be an owner)
	err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.CreateTeamRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create team
	team := &types.Team{
		ID:             system.GenerateTeamID(),
		Name:           req.Name,
		OrganizationID: orgID,
	}

	createdTeam, err := apiServer.Store.CreateTeam(r.Context(), team)
	if err != nil {
		log.Err(err).Msg("error creating team")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, createdTeam, http.StatusCreated)
}

// updateTeam godoc
// @Summary Update a team
// @Description Update a team's details. Only organization owners can update teams.
// @Tags    organizations
// @Accept  json
// @Produce json
// @Param request body types.UpdateTeamRequest true "Request body"
// @Success 200 {object} types.Team
// @Router /api/v1/organizations/{id}/teams/{team_id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateTeam(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	teamID := mux.Vars(r)["team_id"]

	// Check if user has access to update teams (needs to be an owner)
	err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.UpdateTeamRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing team
	team, err := apiServer.Store.GetTeam(r.Context(), &store.GetTeamQuery{
		ID:             teamID,
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error getting team")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update team name
	team.Name = req.Name

	// Save updated team
	updatedTeam, err := apiServer.Store.UpdateTeam(r.Context(), team)
	if err != nil {
		log.Err(err).Msg("error updating team")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, updatedTeam, http.StatusOK)
}

// deleteTeam godoc
// @Summary Delete a team
// @Description Delete a team from an organization. Only organization owners can delete teams.
// @Tags    organizations
// @Success 200
// @Router /api/v1/organizations/{id}/teams/{team_id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteTeam(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	teamID := mux.Vars(r)["team_id"]

	// Check if user has access to delete teams (needs to be an owner)
	err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	// Get the team
	team, err := apiServer.Store.GetTeam(r.Context(), &store.GetTeamQuery{
		ID:             teamID,
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error getting team")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete team
	err = apiServer.Store.DeleteTeam(r.Context(), team.ID)
	if err != nil {
		log.Err(err).Msg("error deleting team")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, nil, http.StatusOK)
}

// listTeamMembers godoc
// @Summary List members of a team
// @Description List all members of a team.
// @Tags    organizations
// @Success 200 {array} types.TeamMembership
// @Router /api/v1/organizations/{id}/teams/{team_id}/members [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listTeamMembers(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	teamID := mux.Vars(r)["team_id"]

	// Check if user has access to view team members
	err := apiServer.authorizeOrgMember(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org member")
		http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
		return
	}

	members, err := apiServer.Store.ListTeamMemberships(r.Context(), &store.ListTeamMembershipsQuery{
		TeamID: teamID,
	})
	if err != nil {
		log.Err(err).Msg("error listing team members")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, members, http.StatusOK)
}

// addTeamMember godoc
// @Summary Add a new member to a team
// @Description Add a new member to a team. Only organization owners can add members to teams.
// @Tags    organizations
// @Accept  json
// @Produce json
// @Param request body types.AddTeamMemberRequest true "Request body"
// @Success 201 {object} types.TeamMembership
// @Router /api/v1/organizations/{id}/teams/{team_id}/members [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) addTeamMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	teamID := mux.Vars(r)["team_id"]

	// Check if user has access to add members to the team (needs to be an owner)
	err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
	}

	// Get team
	_, err = apiServer.Store.GetTeam(r.Context(), &store.GetTeamQuery{
		ID:             teamID,
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error getting team")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var req types.AddOrganizationMemberRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := &store.GetUserQuery{}

	if strings.Contains(req.UserReference, "@") {
		query.Email = req.UserReference
	} else {
		query.ID = req.UserReference
	}

	// Get user
	newMember, err := apiServer.Store.GetUser(r.Context(), query)
	if err != nil {
		log.Err(err).Msg("error getting user")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check for existing membership
	existingMembership, err := apiServer.Store.GetTeamMembership(r.Context(), &store.GetTeamMembershipQuery{
		TeamID: teamID,
		UserID: newMember.ID,
	})
	if err != nil {
		log.Err(err).Msg("error getting team membership")
		if !errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// OK
	}

	if existingMembership != nil {
		http.Error(rw, "User already a member of the team", http.StatusBadRequest)
		return
	}

	// Create membership
	membership := &types.TeamMembership{
		TeamID: teamID,
		UserID: newMember.ID,
	}

	createdMembership, err := apiServer.Store.CreateTeamMembership(r.Context(), membership)
	if err != nil {
		log.Err(err).Msg("error creating team membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, createdMembership, http.StatusCreated)
}

func (apiServer *HelixAPIServer) removeTeamMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	teamID := mux.Vars(r)["team_id"]
	memberUserID := mux.Vars(r)["user_id"]

	// Check if user has access to add members to the team (needs to be an owner)
	err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
	}

	// Check whether we have this team in the organization
	team, err := apiServer.Store.GetTeam(r.Context(), &store.GetTeamQuery{
		ID:             teamID,
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error getting team")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get membership
	membership, err := apiServer.Store.GetTeamMembership(r.Context(), &store.GetTeamMembershipQuery{
		TeamID: teamID,
		UserID: memberUserID,
	})
	if err != nil {
		log.Err(err).Msg("error getting team membership")
		if errors.Is(err, store.ErrNotFound) {
			// Noop
			writeResponse(rw, nil, http.StatusOK)
			return
		}
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete membership
	err = apiServer.Store.DeleteTeamMembership(r.Context(), team.ID, membership.UserID)
	if err != nil {
		log.Err(err).Msg("error deleting team membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, nil, http.StatusOK)
}
