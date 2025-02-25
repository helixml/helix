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
