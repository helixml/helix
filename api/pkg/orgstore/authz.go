package orgstore

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// Queryer is the subset of org reads the Authorizer needs. Both *orgstore.Store
// and Helix's store.Store (via its query-struct aliases) satisfy it, so the
// server can delegate to a single implementation without a duplicate copy.
type Queryer interface {
	GetOrganizationMembership(ctx context.Context, q *GetOrganizationMembershipQuery) (*types.OrganizationMembership, error)
	ListTeams(ctx context.Context, q *ListTeamsQuery) ([]*types.Team, error)
	ListAccessGrants(ctx context.Context, q *ListAccessGrantsQuery) ([]*types.AccessGrant, error)
}

// Authorizer implements Helix's org authorization logic over a Queryer.
// Helix's server delegates to this; downstream consumers use it directly.
type Authorizer struct {
	store Queryer
}

// NewAuthorizer returns an Authorizer backed by the given Queryer.
func NewAuthorizer(q Queryer) *Authorizer { return &Authorizer{store: q} }

// Authorizer returns an Authorizer bound to this store.
func (s *Store) Authorizer() *Authorizer { return &Authorizer{store: s} }

// AuthorizeOrgOwner checks the user is an owner of the organization (global
// admins are treated as owners).
func (a *Authorizer) AuthorizeOrgOwner(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	if user.Admin {
		membership, err := a.store.GetOrganizationMembership(ctx, &GetOrganizationMembershipQuery{OrganizationID: orgID, UserID: user.ID})
		if err == nil {
			return membership, nil
		}
		return &types.OrganizationMembership{OrganizationID: orgID, UserID: user.ID, Role: types.OrganizationRoleOwner}, nil
	}
	membership, err := a.store.GetOrganizationMembership(ctx, &GetOrganizationMembershipQuery{OrganizationID: orgID, UserID: user.ID})
	if err != nil {
		return nil, err
	}
	if membership.Role != types.OrganizationRoleOwner {
		return nil, fmt.Errorf("user is not an owner of this organization")
	}
	return membership, nil
}

// AuthorizeOrgMember checks the user is a member of the organization (global
// admins are treated as members/owners).
func (a *Authorizer) AuthorizeOrgMember(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	if user.Admin {
		membership, err := a.store.GetOrganizationMembership(ctx, &GetOrganizationMembershipQuery{OrganizationID: orgID, UserID: user.ID})
		if err == nil {
			return membership, nil
		}
		return &types.OrganizationMembership{OrganizationID: orgID, UserID: user.ID, Role: types.OrganizationRoleOwner}, nil
	}
	membership, err := a.store.GetOrganizationMembership(ctx, &GetOrganizationMembershipQuery{OrganizationID: orgID, UserID: user.ID})
	if err != nil {
		return nil, err
	}
	return membership, nil
}

// AuthorizeUserToResource evaluates the user's team + direct access grants for a
// resource and returns nil if the requested action is allowed.
func (a *Authorizer) AuthorizeUserToResource(ctx context.Context, user *types.User, orgID, resourceID string, resourceType types.Resource, action types.Action) error {
	authzConfigs, err := a.getAuthzConfigs(ctx, user, orgID, resourceID)
	if err != nil {
		return err
	}
	if evaluate(resourceType, action, authzConfigs) {
		return nil
	}
	return fmt.Errorf("user is not authorized to perform this action")
}

func (a *Authorizer) getAuthzConfigs(ctx context.Context, user *types.User, orgID, resourceID string) ([]types.Config, error) {
	var authzConfigs []types.Config

	teams, err := a.store.ListTeams(ctx, &ListTeamsQuery{OrganizationID: orgID, UserID: user.ID})
	if err != nil {
		return nil, err
	}
	var teamIDs []string
	for _, team := range teams {
		teamIDs = append(teamIDs, team.ID)
	}

	grants, err := a.store.ListAccessGrants(ctx, &ListAccessGrantsQuery{
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
