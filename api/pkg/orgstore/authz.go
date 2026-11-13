package orgstore

import (
	"context"
	"errors"
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

// ErrKeyOrgScope is returned when an org-scoped credential is used against a
// different organization.
var ErrKeyOrgScope = errors.New("credentials are scoped to another organization")

// ErrNotOrgOwner is returned when an org member without the owner role asks
// for an owner-only action.
var ErrNotOrgOwner = errors.New("user is not an owner of this organization")

// enforceKeyOrgScope confines an org-scoped credential to its own
// organization. Organization API keys carry the org they were created for on
// the request user; without this check a key created by a user belonging to
// several organizations inherits their memberships everywhere.
func enforceKeyOrgScope(user *types.User, orgID string) error {
	if user == nil || user.OrganizationID == "" || user.OrganizationID == orgID {
		return nil
	}
	return ErrKeyOrgScope
}

// enforceKeyNotAgentScoped refuses org-owner authority to a credential minted
// for a project or a session (sandbox agents, chat sessions, helix-org
// workers). Such a key belongs to a human who may own the organization, but
// it is handed to an agent for one piece of work: it must not change who is
// in the organization, its settings, keys or billing, or delete it. Org API
// keys carry neither field and keep their owner's role.
func enforceKeyNotAgentScoped(user *types.User) error {
	if user == nil || (user.ProjectID == "" && user.SessionID == "") {
		return nil
	}
	return fmt.Errorf("%w: project- and session-scoped credentials cannot act as organization owner", ErrNotOrgOwner)
}

// Authorizer returns an Authorizer bound to this store.
func (s *Store) Authorizer() *Authorizer { return &Authorizer{store: s} }

// AuthorizeOrgOwner checks the user is an owner of the organization (global
// admins are treated as owners).
func (a *Authorizer) AuthorizeOrgOwner(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	// Org-scoped credentials (organization API keys and ephemeral session
	// keys) authorize only within their own organization, no matter which
	// other organizations or privileges the owning user has.
	if err := enforceKeyOrgScope(user, orgID); err != nil {
		return nil, err
	}
	if err := enforceKeyNotAgentScoped(user); err != nil {
		return nil, err
	}
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
		return nil, ErrNotOrgOwner
	}
	return membership, nil
}

// AuthorizeOrgMember checks the user is a member of the organization (global
// admins are treated as members/owners).
func (a *Authorizer) AuthorizeOrgMember(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	if err := enforceKeyOrgScope(user, orgID); err != nil {
		return nil, err
	}
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
