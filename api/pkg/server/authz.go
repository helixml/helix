package server

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// authorizeOrgOwner used to check if the user is an owner of the organization to perform certain actions
// such as creating, updating teams, updating or deleting organization
func (apiServer *HelixAPIServer) authorizeOrgOwner(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	// Global admins can perform any operation, regardless of their organization membership
	if user.Admin {
		// For global admins, we'll still try to get their membership if it exists
		membership, err := apiServer.Store.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         user.ID,
		})

		// If the global admin has membership, return it
		if err == nil {
			return membership, nil
		}

		// If the global admin doesn't have a membership, create a temporary one with owner role
		// This won't be stored in the database, just returned for the current operation
		return &types.OrganizationMembership{
			OrganizationID: orgID,
			UserID:         user.ID,
			Role:           types.OrganizationRoleOwner,
		}, nil
	}

	// For non-admin users, proceed with the existing logic
	membership, err := apiServer.Store.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	if err != nil {
		return nil, err
	}

	if membership.Role != types.OrganizationRoleOwner {
		return nil, fmt.Errorf("user is not an owner of this organization")
	}

	return membership, nil
}

// deleting used to check if the user is a member of the organization to perform certain actions
// such as listing teams, listing members, etc
func (apiServer *HelixAPIServer) authorizeOrgMember(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	// Global admins can perform any operation, regardless of their organization membership
	if user.Admin {
		// For global admins, we'll still try to get their membership if it exists
		membership, err := apiServer.Store.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         user.ID,
		})

		// If the global admin has membership, return it
		if err == nil {
			return membership, nil
		}

		// If the global admin doesn't have a membership, create a temporary one with owner role
		// This won't be stored in the database, just returned for the current operation
		return &types.OrganizationMembership{
			OrganizationID: orgID,
			UserID:         user.ID,
			Role:           types.OrganizationRoleOwner,
		}, nil
	}

	// For non-admin users, proceed with the existing logic
	membership, err := apiServer.Store.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	if err != nil {
		return nil, err
	}

	// Both roles (owner or member) can list teams and members
	return membership, nil
}

// authorizeUserToAppAccessGrants checks if the user is a member of the organization or the app owner
// and has the necessary permissions to perform the action on the access grant
func (apiServer *HelixAPIServer) authorizeUserToAppAccessGrants(ctx context.Context, user *types.User, app *types.App, action types.Action) error {
	// Check if user is a member of the org
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, app.OrganizationID)
	if err != nil {
		return err
	}

	// App owner can always access the app
	if user.ID == app.Owner {
		return nil
	}

	// Org owner can always access the app
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	return apiServer.authorizeUserToResource(ctx, user, app.OrganizationID, app.ID, types.ResourceAccessGrants, action)
}

// authorizeUserToApp checks if a user has access to an app
// This is a server-level method that centralizes the authorization logic
func (apiServer *HelixAPIServer) authorizeUserToApp(ctx context.Context, user *types.User, app *types.App, action types.Action) error {
	// If the organization ID is not set and the user is not the app owner, then error
	if app.OrganizationID == "" {
		// This is the old style app logic, where the app is owned by a user and optionally made global

		// If the user is the owner of the app, they can access it
		if user.ID == app.Owner {
			return nil
		}

		// If the app is global, the user can access it
		if app.Global {
			// But only admins can update or delete global apps
			if action == types.ActionUpdate || action == types.ActionDelete {
				if !isAdmin(user) {
					return fmt.Errorf("only admin users can update or delete global apps")
				}
			}

			// If the app is global, the user can access it
			return nil
		}

		// Otherwise the user is not allowed to access the app
		return fmt.Errorf("user is not the owner of the app")
	}

	// If organization ID is set, authorize the user against the organization
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, app.OrganizationID)
	if err != nil {
		return err
	}

	// App owner can always access the app
	if user.ID == app.Owner {
		return nil
	}

	// Org owner can always access the app
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	return apiServer.authorizeUserToResource(ctx, user, app.OrganizationID, app.ID, types.ResourceApplication, action)
}

func (apiServer *HelixAPIServer) authorizeUserToProject(ctx context.Context, user *types.User, project *types.Project, action types.Action) error {
	// If the organization ID is not set and the user is not the project owner, then error
	if project.OrganizationID == "" {
		// This is the old style project logic, where the project is owned by a user and optionally made global

		// If the user is the owner of the project, they can access it
		if user.ID == project.UserID {
			return nil
		}

		// Otherwise the user is not allowed to access the app
		return fmt.Errorf("user is not the owner of the app")
	}

	// If organization ID is set, authorize the user against the organization
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, project.OrganizationID)
	if err != nil {
		return err
	}

	// Project owner can always access the project (they still have to have
	// org membership)
	if user.ID == project.UserID {
		return nil
	}

	// Org owner can always access the app
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	return apiServer.authorizeUserToResource(ctx, user, project.OrganizationID, project.ID, types.ResourceProject, action)
}

// authorizeUserToResource loads RBAC configuration for the
func (apiServer *HelixAPIServer) authorizeUserToResource(ctx context.Context, user *types.User, orgID, resourceID string, resourceType types.Resource, action types.Action) error {
	// Load all authz configs for the user (teams, direct to user grants)
	authzConfigs, err := getAuthzConfigs(ctx, apiServer.Store, user, orgID, resourceID)
	if err != nil {
		return err
	}

	if evaluate(resourceType, action, authzConfigs) {
		return nil
	}

	return fmt.Errorf("user is not authorized to perform this action")
}

func getAuthzConfigs(ctx context.Context, db store.Store, user *types.User, orgID, resourceID string) ([]types.Config, error) {
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
