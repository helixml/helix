package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationInvitationsTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationInvitationsTestSuite))
}

// OrganizationInvitationsTestSuite exercises the invitation CRUD plus the
// ConsumePendingInvitations path that runs at register time. Each test
// gets its own org + user so the suite can run with -shuffle.
type OrganizationInvitationsTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
	org *types.Organization
}

func (suite *OrganizationInvitationsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()

	orgID := system.GenerateOrganizationID()
	org, err := suite.db.CreateOrganization(suite.ctx, &types.Organization{
		ID:    orgID,
		Name:  "Invitations Test Org " + orgID,
		Owner: "test-owner",
	})
	suite.Require().NoError(err)
	suite.org = org
}

func (suite *OrganizationInvitationsTestSuite) TearDownTest() {
	if suite.org != nil {
		// Deleting the org cascades the invitations.
		_ = suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
	}
}

func (suite *OrganizationInvitationsTestSuite) TestCreateInvitation_HappyPath() {
	inv, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "FreshInvite@Example.com",
		Role:           types.OrganizationRoleMember,
		InvitedBy:      "test-owner",
	})
	suite.Require().NoError(err)
	suite.NotEmpty(inv.ID)
	suite.True(len(inv.ID) > len(system.OrgInvitationPrefix), "id should be prefixed")
	// Email must be normalised to lowercase for case-insensitive lookups.
	suite.Equal("freshinvite@example.com", inv.Email)
	suite.Equal(types.OrganizationRoleMember, inv.Role)
	suite.Equal("test-owner", inv.InvitedBy)
	suite.False(inv.CreatedAt.IsZero())
	suite.False(inv.UpdatedAt.IsZero())
}

func (suite *OrganizationInvitationsTestSuite) TestCreateInvitation_DefaultsRoleToMember() {
	inv, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "norole@example.com",
	})
	suite.Require().NoError(err)
	suite.Equal(types.OrganizationRoleMember, inv.Role)
}

func (suite *OrganizationInvitationsTestSuite) TestCreateInvitation_Duplicate_ReturnsSentinel() {
	first, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "dup@example.com",
		Role:           types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)

	// Same org + same email → ErrInvitationAlreadyExists, AND the existing
	// row is returned so the handler can resend the email idempotently.
	existing, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "DUP@example.com", // different case — should still collide
		Role:           types.OrganizationRoleOwner,
	})
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrInvitationAlreadyExists))
	suite.Require().NotNil(existing)
	suite.Equal(first.ID, existing.ID)
	// Existing role must NOT have been overwritten with the new caller's
	// role — duplicates are a no-op, not a silent role bump.
	suite.Equal(types.OrganizationRoleMember, existing.Role)
}

func (suite *OrganizationInvitationsTestSuite) TestCreateInvitation_Validation() {
	_, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		Email: "missing-org@example.com",
	})
	suite.Error(err)

	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
	})
	suite.Error(err)
}

func (suite *OrganizationInvitationsTestSuite) TestGetInvitation_ByID_AndByOrgEmail() {
	inv, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "byid@example.com",
		Role:           types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)

	byID, err := suite.db.GetOrganizationInvitation(suite.ctx, &GetOrganizationInvitationQuery{ID: inv.ID})
	suite.Require().NoError(err)
	suite.Equal(inv.ID, byID.ID)

	// Lookup by (org_id, email) must be case-insensitive — register-time
	// consumption depends on this.
	byEmail, err := suite.db.GetOrganizationInvitation(suite.ctx, &GetOrganizationInvitationQuery{
		OrganizationID: suite.org.ID,
		Email:          "ByID@Example.com",
	})
	suite.Require().NoError(err)
	suite.Equal(inv.ID, byEmail.ID)

	// Not found behaviour
	_, err = suite.db.GetOrganizationInvitation(suite.ctx, &GetOrganizationInvitationQuery{ID: "oin_nope"})
	suite.ErrorIs(err, ErrNotFound)
}

func (suite *OrganizationInvitationsTestSuite) TestListInvitations() {
	emails := []string{"a@example.com", "b@example.com", "c@example.com"}
	for _, e := range emails {
		_, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
			OrganizationID: suite.org.ID,
			Email:          e,
		})
		suite.Require().NoError(err)
	}

	all, err := suite.db.ListOrganizationInvitations(suite.ctx, &ListOrganizationInvitationsQuery{
		OrganizationID: suite.org.ID,
	})
	suite.Require().NoError(err)
	suite.Len(all, len(emails))

	// Filter by email
	filtered, err := suite.db.ListOrganizationInvitations(suite.ctx, &ListOrganizationInvitationsQuery{
		OrganizationID: suite.org.ID,
		Email:          "a@example.com",
	})
	suite.Require().NoError(err)
	suite.Len(filtered, 1)
}

func (suite *OrganizationInvitationsTestSuite) TestDeleteInvitation() {
	inv, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "to-delete@example.com",
	})
	suite.Require().NoError(err)

	err = suite.db.DeleteOrganizationInvitation(suite.ctx, inv.ID)
	suite.Require().NoError(err)

	_, err = suite.db.GetOrganizationInvitation(suite.ctx, &GetOrganizationInvitationQuery{ID: inv.ID})
	suite.ErrorIs(err, ErrNotFound)

	// Deleting a non-existent invitation is a soft error so the handler
	// can return 404 — surface ErrNotFound here.
	err = suite.db.DeleteOrganizationInvitation(suite.ctx, "oin_does-not-exist")
	suite.ErrorIs(err, ErrNotFound)
}

func (suite *OrganizationInvitationsTestSuite) TestConsumePendingInvitations_CreatesMembershipsAndDeletesInvitations() {
	// Two invitations for the same email across two different orgs — both
	// should be consumed in a single transaction when the user registers.
	orgID2 := system.GenerateOrganizationID()
	org2, err := suite.db.CreateOrganization(suite.ctx, &types.Organization{
		ID:    orgID2,
		Name:  "Second Org " + orgID2,
		Owner: "test-owner-2",
	})
	suite.Require().NoError(err)
	defer suite.db.DeleteOrganization(suite.ctx, org2.ID)

	email := "consume@example.com"
	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          email,
		Role:           types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)
	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: org2.ID,
		Email:          email,
		Role:           types.OrganizationRoleOwner,
	})
	suite.Require().NoError(err)

	user := &types.User{
		ID:        system.GenerateUserID(),
		Email:     email,
		CreatedAt: time.Now(),
	}
	_, err = suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)
	defer suite.db.DeleteUser(suite.ctx, user.ID)

	memberships, err := suite.db.ConsumePendingInvitations(suite.ctx, user)
	suite.Require().NoError(err)
	suite.Len(memberships, 2)

	// Memberships actually persisted with the right roles
	m1, err := suite.db.GetOrganizationMembership(suite.ctx, &GetOrganizationMembershipQuery{
		OrganizationID: suite.org.ID,
		UserID:         user.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(types.OrganizationRoleMember, m1.Role)
	m2, err := suite.db.GetOrganizationMembership(suite.ctx, &GetOrganizationMembershipQuery{
		OrganizationID: org2.ID,
		UserID:         user.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(types.OrganizationRoleOwner, m2.Role)

	// Both invitations cleaned up
	remaining, err := suite.db.ListOrganizationInvitations(suite.ctx, &ListOrganizationInvitationsQuery{
		Email: email,
	})
	suite.Require().NoError(err)
	suite.Empty(remaining)
}

func (suite *OrganizationInvitationsTestSuite) TestConsumePendingInvitations_SkipsExistingMembership() {
	// If a membership already exists (e.g. via OIDC domain join), we
	// shouldn't try to create a duplicate — the row would conflict on the
	// composite primary key. The invitation should still be deleted so it
	// doesn't sit around forever.
	email := "skip@example.com"
	user := &types.User{
		ID:        system.GenerateUserID(),
		Email:     email,
		CreatedAt: time.Now(),
	}
	_, err := suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)
	defer suite.db.DeleteUser(suite.ctx, user.ID)

	_, err = suite.db.CreateOrganizationMembership(suite.ctx, &types.OrganizationMembership{
		OrganizationID: suite.org.ID,
		UserID:         user.ID,
		Role:           types.OrganizationRoleOwner,
	})
	suite.Require().NoError(err)

	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          email,
		Role:           types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)

	created, err := suite.db.ConsumePendingInvitations(suite.ctx, user)
	suite.Require().NoError(err)
	suite.Empty(created, "no new memberships should be created")

	// Existing membership keeps its original role
	existing, err := suite.db.GetOrganizationMembership(suite.ctx, &GetOrganizationMembershipQuery{
		OrganizationID: suite.org.ID,
		UserID:         user.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(types.OrganizationRoleOwner, existing.Role, "pre-existing role must not be downgraded")

	// Invitation is gone — it was consumed even though no new membership
	// row was created.
	remaining, err := suite.db.ListOrganizationInvitations(suite.ctx, &ListOrganizationInvitationsQuery{
		Email: email,
	})
	suite.Require().NoError(err)
	suite.Empty(remaining)
}

func (suite *OrganizationInvitationsTestSuite) TestConsumePendingInvitations_NoMatches() {
	user := &types.User{
		ID:        system.GenerateUserID(),
		Email:     "no-invites@example.com",
		CreatedAt: time.Now(),
	}
	_, err := suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)
	defer suite.db.DeleteUser(suite.ctx, user.ID)

	memberships, err := suite.db.ConsumePendingInvitations(suite.ctx, user)
	suite.Require().NoError(err)
	suite.Empty(memberships)
}

func (suite *OrganizationInvitationsTestSuite) TestConsumePendingInvitations_NilUserIsNoOp() {
	memberships, err := suite.db.ConsumePendingInvitations(suite.ctx, nil)
	suite.NoError(err)
	suite.Empty(memberships)

	memberships, err = suite.db.ConsumePendingInvitations(suite.ctx, &types.User{ID: "no-email"})
	suite.NoError(err)
	suite.Empty(memberships)
}

func (suite *OrganizationInvitationsTestSuite) TestConsumePendingInvitations_CreatesAccessGrant() {
	// When an invitation carries an AppID, consuming it should also
	// materialise an AccessGrant on that app so the invitee shows up in
	// the project's access list immediately. Role names are looked up in
	// the org's role table — unknown names are silently skipped.
	memberRole, err := suite.db.CreateRole(suite.ctx, &types.Role{
		ID:             system.GenerateRoleID(),
		OrganizationID: suite.org.ID,
		Name:           "app_user",
	})
	suite.Require().NoError(err)

	appID := "app_invited"
	email := "with-grant@example.com"
	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          email,
		AppID:          appID,
		GrantRoles:     []string{memberRole.Name, "name-that-does-not-exist"},
	})
	suite.Require().NoError(err)

	user := &types.User{
		ID:        system.GenerateUserID(),
		Email:     email,
		CreatedAt: time.Now(),
	}
	_, err = suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)
	defer suite.db.DeleteUser(suite.ctx, user.ID)

	memberships, err := suite.db.ConsumePendingInvitations(suite.ctx, user)
	suite.Require().NoError(err)
	suite.Len(memberships, 1)

	// Access grant should now exist for this resource + user.
	grants, err := suite.db.ListAccessGrants(suite.ctx, &ListAccessGrantsQuery{
		OrganizationID: suite.org.ID,
		ResourceID:     appID,
		UserID:         user.ID,
	})
	suite.Require().NoError(err)
	suite.Require().Len(grants, 1, "exactly one access grant for the resource")
	suite.Equal(user.ID, grants[0].UserID)
	suite.Equal(appID, grants[0].ResourceID)
}

func (suite *OrganizationInvitationsTestSuite) TestConsumePendingInvitations_AccessGrant_IdempotentOnExistingGrant() {
	// If an access grant already exists for (resource, user), the
	// consumer must not error out — the invitation should just be
	// cleaned up. Re-running consumption (e.g. retry after partial
	// failure) shouldn't double-grant.
	role, err := suite.db.CreateRole(suite.ctx, &types.Role{
		ID:             system.GenerateRoleID(),
		OrganizationID: suite.org.ID,
		Name:           "app_user",
	})
	suite.Require().NoError(err)

	user := &types.User{
		ID:        system.GenerateUserID(),
		Email:     "dup-grant@example.com",
		CreatedAt: time.Now(),
	}
	_, err = suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)
	defer suite.db.DeleteUser(suite.ctx, user.ID)

	appID := "app_existing"
	_, err = suite.db.CreateAccessGrant(suite.ctx, &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceID:     appID,
		UserID:         user.ID,
	}, []*types.Role{role})
	suite.Require().NoError(err)

	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          user.Email,
		AppID:          appID,
		GrantRoles:     []string{role.Name},
	})
	suite.Require().NoError(err)

	_, err = suite.db.ConsumePendingInvitations(suite.ctx, user)
	suite.Require().NoError(err)

	grants, err := suite.db.ListAccessGrants(suite.ctx, &ListAccessGrantsQuery{
		OrganizationID: suite.org.ID,
		ResourceID:     appID,
		UserID:         user.ID,
	})
	suite.Require().NoError(err)
	suite.Len(grants, 1, "no duplicate grants should be created")
}

func (suite *OrganizationInvitationsTestSuite) TestListInvitations_FilterByAppID() {
	// Project-scoped invitations should be retrievable independently
	// from org-wide ones via the AppID filter.
	_, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "org-wide@example.com",
	})
	suite.Require().NoError(err)
	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "app1@example.com",
		AppID:          "app_one",
	})
	suite.Require().NoError(err)
	_, err = suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "app2@example.com",
		AppID:          "app_two",
	})
	suite.Require().NoError(err)

	scoped, err := suite.db.ListOrganizationInvitations(suite.ctx, &ListOrganizationInvitationsQuery{
		OrganizationID: suite.org.ID,
		AppID:          "app_one",
	})
	suite.Require().NoError(err)
	suite.Require().Len(scoped, 1)
	suite.Equal("app1@example.com", scoped[0].Email)
}

func (suite *OrganizationInvitationsTestSuite) TestConsumePendingInvitations_CaseInsensitive() {
	// Invitations are stored lowercase; user.Email might be mixed-case.
	_, err := suite.db.CreateOrganizationInvitation(suite.ctx, &types.OrganizationInvitation{
		OrganizationID: suite.org.ID,
		Email:          "case@example.com",
	})
	suite.Require().NoError(err)

	user := &types.User{
		ID:        system.GenerateUserID(),
		Email:     "CASE@Example.COM",
		CreatedAt: time.Now(),
	}
	_, err = suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)
	defer suite.db.DeleteUser(suite.ctx, user.ID)

	created, err := suite.db.ConsumePendingInvitations(suite.ctx, user)
	suite.Require().NoError(err)
	suite.Len(created, 1)
}
