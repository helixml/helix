package server

import (
	"context"
	"fmt"
	"sort"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// resolveOrgOwner returns the user that org-wide bot work is attributed to
// when no human is on the request: the organization's recorded owner while
// they are still a member, otherwise the earliest owner-role membership. It
// fails when the organization has no owner left, so callers never fall back
// to the requester.
func resolveOrgOwner(ctx context.Context, st helixstore.Store, orgID string) (*types.User, error) {
	org, err := st.GetOrganization(ctx, &helixstore.GetOrganizationQuery{ID: orgID})
	if err != nil {
		return nil, fmt.Errorf("get organization %s: %w", orgID, err)
	}
	if org == nil {
		return nil, fmt.Errorf("organization %s not found", orgID)
	}
	memberships, err := st.ListOrganizationMemberships(ctx, &helixstore.ListOrganizationMembershipsQuery{OrganizationID: orgID})
	if err != nil {
		return nil, fmt.Errorf("list organization %s memberships: %w", orgID, err)
	}
	ownerID := ""
	for _, m := range memberships {
		if m != nil && m.UserID == org.Owner && org.Owner != "" {
			ownerID = org.Owner
			break
		}
	}
	if ownerID == "" {
		owners := make([]*types.OrganizationMembership, 0, len(memberships))
		for _, m := range memberships {
			if m != nil && m.Role == types.OrganizationRoleOwner {
				owners = append(owners, m)
			}
		}
		sort.SliceStable(owners, func(i, j int) bool { return owners[i].CreatedAt.Before(owners[j].CreatedAt) })
		if len(owners) > 0 {
			ownerID = owners[0].UserID
		}
	}
	if ownerID == "" {
		return nil, fmt.Errorf("organization %s has no owner", orgID)
	}
	user, err := st.GetUser(ctx, &helixstore.GetUserQuery{ID: ownerID})
	if err != nil {
		return nil, fmt.Errorf("get organization owner %s: %w", ownerID, err)
	}
	if user == nil {
		return nil, fmt.Errorf("organization owner %s not found", ownerID)
	}
	return user, nil
}

// botProjectAuthorizer answers whether a Bot may be given a project: the
// Bot's hiring user (or, for a Bot hired before hiring users were recorded,
// the organization owner) must be able to read it. It is the write-time twin
// of the spec-task runtime's per-call check.
type botProjectAuthorizer struct {
	orgStore  *helixorgstore.Store
	authorize func(ctx context.Context, orgID, userID, projectID string, action types.Action) error
}

func (b botProjectAuthorizer) AuthorizeBotProject(ctx context.Context, orgID string, botID orgchart.NodeID, projectID string) error {
	state, err := runtimehelix.LoadState(ctx, b.orgStore, orgID, botID)
	if err != nil {
		return fmt.Errorf("load bot %s state: %w", botID, err)
	}
	return b.authorize(ctx, orgID, state.HiringUserID, projectID, types.ActionGet)
}
