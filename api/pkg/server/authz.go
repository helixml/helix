package server

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// authorizeOrgOwner used to check if the user is an owner of the organization to perform certain actions
// such as creating, updating teams, updating or deleting organization
func (apiServer *HelixAPIServer) authorizeOrgOwner(ctx context.Context, user *types.User, orgID string) error {
	memberships, err := apiServer.Store.ListOrganizationMemberships(ctx, &store.ListOrganizationMembershipsQuery{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	if err != nil {
		return err
	}

	if len(memberships) == 0 {
		return fmt.Errorf("user is not a member of this organization")
	}

	if memberships[0].Role != types.OrganizationRoleOwner {
		return fmt.Errorf("user is not an owner of this organization")
	}

	return nil
}

// deleting used to check if the user is a member of the organization to perform certain actions
// such as listing teams, listing members, etc
func (apiServer *HelixAPIServer) authorizeOrgMember(ctx context.Context, user *types.User, orgID string) error {
	memberships, err := apiServer.Store.ListOrganizationMemberships(ctx, &store.ListOrganizationMembershipsQuery{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	if err != nil {
		return err
	}

	if len(memberships) == 0 {
		return fmt.Errorf("user is not a member of this organization")
	}

	// Both roles (owner or member) can list teams and members

	return nil
}

// authorizeUserToResource loads RBAC configuration for the
func (apiServer *HelixAPIServer) authorizeUserToResource(ctx context.Context, user *types.User, orgID, resourceID string, resourceType types.Resource, action types.Action) error {
	// Load all authz configs for the user (teams, direct to user grants)
	authzConfigs, err := getAuthzConfigs(ctx, apiServer.Store, user, orgID, resourceID, resourceType)
	if err != nil {
		return err
	}

	if evaluate(resourceType, action, authzConfigs) {
		return nil
	}

	return fmt.Errorf("user is not authorized to perform this action")
}

func getAuthzConfigs(ctx context.Context, db store.Store, user *types.User, orgID, resourceID string, resourceType types.Resource) ([]types.Config, error) {
	var authzConfigs []types.Config

	// Get all teams
	teams, err := db.ListTeams(ctx, &store.ListTeamsQuery{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	if err != nil {
		return nil, err
	}

	var teamIDs []string
	for _, team := range teams {
		teamIDs = append(teamIDs, team.ID)
	}

	// Check if the user is granted access directly
	grants, err := db.ListAccessGrants(ctx, &store.ListAccessGrantsQuery{
		OrganizationID: orgID,
		UserID:         user.ID,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		TeamIDs:        teamIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list access grants: %w", err)
	}

	for _, grant := range grants {
		for _, role := range grant.Roles {
			authzConfigs = append(authzConfigs, role.Config)
		}
	}

	return authzConfigs, nil
}

func evaluate(requestedResource types.Resource, requestedAction types.Action, configs []types.Config) bool {
	oneAllow := false

	for _, config := range configs {
		for _, rule := range config.Rules {
			for _, resource := range rule.Resources {
				if resource == requestedResource || resource == types.ResourceAny {
					for _, ruleAction := range rule.Actions {
						if ruleAction == requestedAction {
							if rule.Effect == types.EffectDeny {
								return false
							}
							oneAllow = true
						}
					}
				}
			}
		}
	}
	return oneAllow
}
