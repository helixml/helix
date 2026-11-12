package store

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestUsersTestSuite(t *testing.T) {
	suite.Run(t, new(UsersTestSuite))
}

type UsersTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *UsersTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()
}

func (suite *UsersTestSuite) TearDownTestSuite() {
	// No need to close the database connection here as it's managed by TestMain
}

func (suite *UsersTestSuite) TestListUsers_EmailFilter_ExactMatch() {
	// Create test users with unique emails
	prefix := "test-email-filter-" + system.GenerateAppID()

	user1 := &types.User{
		ID:       prefix + "-user1",
		Email:    prefix + "-user1@example.com",
		Username: prefix + "-user1",
	}
	user2 := &types.User{
		ID:       prefix + "-user2",
		Email:    prefix + "-user2@example.com",
		Username: prefix + "-user2",
	}
	user3 := &types.User{
		ID:       prefix + "-user3",
		Email:    prefix + "-user3@otherdomain.com",
		Username: prefix + "-user3",
	}

	// Create all users
	_, err := suite.db.CreateUser(suite.ctx, user1)
	suite.Require().NoError(err)
	_, err = suite.db.CreateUser(suite.ctx, user2)
	suite.Require().NoError(err)
	_, err = suite.db.CreateUser(suite.ctx, user3)
	suite.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = suite.db.DeleteUser(suite.ctx, user1.ID)
		_ = suite.db.DeleteUser(suite.ctx, user2.ID)
		_ = suite.db.DeleteUser(suite.ctx, user3.ID)
	}()

	// Test exact email match - should find exactly one user
	users, total, err := suite.db.ListUsers(suite.ctx, &ListUsersQuery{
		Email: user1.Email,
	})
	suite.NoError(err)
	suite.Equal(int64(1), total, "Should find exactly one user with exact email match")
	suite.Len(users, 1)
	suite.Equal(user1.ID, users[0].ID)
	suite.Equal(user1.Email, users[0].Email)

	// Test with different case - should still match (case-insensitive)
	users, total, err = suite.db.ListUsers(suite.ctx, &ListUsersQuery{
		Email: prefix + "-USER1@EXAMPLE.COM",
	})
	suite.NoError(err)
	suite.Equal(int64(1), total, "Should find user with case-insensitive email match")
	suite.Len(users, 1)
	suite.Equal(user1.ID, users[0].ID)
}

func (suite *UsersTestSuite) TestListUsers_EmailFilter_DomainMatch() {
	// Create test users with unique emails
	prefix := "test-domain-filter-" + system.GenerateAppID()

	user1 := &types.User{
		ID:       prefix + "-user1",
		Email:    prefix + "-user1@testdomain.com",
		Username: prefix + "-user1",
	}
	user2 := &types.User{
		ID:       prefix + "-user2",
		Email:    prefix + "-user2@testdomain.com",
		Username: prefix + "-user2",
	}
	user3 := &types.User{
		ID:       prefix + "-user3",
		Email:    prefix + "-user3@otherdomain.com",
		Username: prefix + "-user3",
	}

	// Create all users
	_, err := suite.db.CreateUser(suite.ctx, user1)
	suite.Require().NoError(err)
	_, err = suite.db.CreateUser(suite.ctx, user2)
	suite.Require().NoError(err)
	_, err = suite.db.CreateUser(suite.ctx, user3)
	suite.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = suite.db.DeleteUser(suite.ctx, user1.ID)
		_ = suite.db.DeleteUser(suite.ctx, user2.ID)
		_ = suite.db.DeleteUser(suite.ctx, user3.ID)
	}()

	// Test domain filter (no @ in query) - should find users at that domain
	users, total, err := suite.db.ListUsers(suite.ctx, &ListUsersQuery{
		Email: "testdomain.com",
	})
	suite.NoError(err)
	suite.GreaterOrEqual(total, int64(2), "Should find at least 2 users with domain filter")

	// Filter to only our test users
	var ourUsers []*types.User
	for _, u := range users {
		if u.ID == user1.ID || u.ID == user2.ID {
			ourUsers = append(ourUsers, u)
		}
	}
	suite.Len(ourUsers, 2, "Should find our 2 test users with testdomain.com")

	// Test that otherdomain.com filter doesn't include testdomain.com users
	users, _, err = suite.db.ListUsers(suite.ctx, &ListUsersQuery{
		Email: "otherdomain.com",
	})
	suite.NoError(err)

	// Filter to our test users
	var otherDomainUsers []*types.User
	for _, u := range users {
		if u.ID == user3.ID {
			otherDomainUsers = append(otherDomainUsers, u)
		}
	}
	suite.Len(otherDomainUsers, 1, "Should find user3 with otherdomain.com filter")

	// Ensure user1 and user2 are not in otherdomain.com results
	for _, u := range users {
		suite.NotEqual(user1.ID, u.ID, "user1 should not appear in otherdomain.com results")
		suite.NotEqual(user2.ID, u.ID, "user2 should not appear in otherdomain.com results")
	}
}

func (suite *UsersTestSuite) TestListUsers_EmailFilter_NoMatch() {
	// Test with email that doesn't exist
	users, total, err := suite.db.ListUsers(suite.ctx, &ListUsersQuery{
		Email: "nonexistent-user-" + system.GenerateAppID() + "@nowhere.com",
	})
	suite.NoError(err)
	suite.Equal(int64(0), total, "Should find no users for non-existent email")
	suite.Len(users, 0)
}

func (suite *UsersTestSuite) TestGetUser_DesktopIdentity() {
	prefix := "desktop-identity-" + system.GenerateAppID()
	plain := &types.User{ID: prefix + "-plain", Email: prefix + "-plain@example.com", Username: prefix + "-plain", Admin: true}
	desktop := &types.User{ID: prefix + "-desktop", Email: prefix + "-desktop@example.com", Username: prefix + "-desktop", Admin: true, DesktopIdentity: true}
	for _, u := range []*types.User{plain, desktop} {
		_, err := suite.db.CreateUser(suite.ctx, u)
		suite.Require().NoError(err)
		defer suite.db.DeleteUser(suite.ctx, u.ID)
	}

	got, err := suite.db.GetUser(suite.ctx, &GetUserQuery{DesktopIdentity: true})
	suite.Require().NoError(err)
	suite.True(got.DesktopIdentity)
	suite.NotEqual(plain.ID, got.ID)
}

// Emails are matched case-insensitively, so "Taken@x" is taken by "taken@x".
func (suite *UsersTestSuite) TestGetUser_EmailIsCaseInsensitive() {
	prefix := "case-" + system.GenerateAppID()
	u := &types.User{ID: prefix, Email: prefix + "@example.com", Username: prefix}
	_, err := suite.db.CreateUser(suite.ctx, u)
	suite.Require().NoError(err)
	defer suite.db.DeleteUser(suite.ctx, u.ID)

	got, err := suite.db.GetUser(suite.ctx, &GetUserQuery{Email: "  " + strings.ToUpper(u.Email) + " "})
	suite.Require().NoError(err)
	suite.Equal(u.ID, got.ID)
}

// Rows created before case-insensitive matching may differ only in case.
// Each must keep resolving to itself when looked up by its exact address.
func (suite *UsersTestSuite) TestGetUser_PrefersExactCaseAmongLegacyDuplicates() {
	prefix := "dupcase-" + system.GenerateAppID()
	older := &types.User{ID: prefix + "-a", Email: prefix + "@example.com", Username: prefix + "-a"}
	newer := &types.User{ID: prefix + "-b", Email: strings.ToUpper(prefix[:1]) + prefix[1:] + "@example.com", Username: prefix + "-b"}
	for _, u := range []*types.User{older, newer} {
		_, err := suite.db.CreateUser(suite.ctx, u)
		suite.Require().NoError(err)
		defer suite.db.DeleteUser(suite.ctx, u.ID)
	}

	got, err := suite.db.GetUser(suite.ctx, &GetUserQuery{Email: newer.Email})
	suite.Require().NoError(err)
	suite.Equal(newer.ID, got.ID, "exact-case match wins")

	got, err = suite.db.GetUser(suite.ctx, &GetUserQuery{Email: older.Email})
	suite.Require().NoError(err)
	suite.Equal(older.ID, got.ID)

	got, err = suite.db.GetUser(suite.ctx, &GetUserQuery{Email: strings.ToUpper(older.Email)})
	suite.Require().NoError(err)
	suite.Equal(older.ID, got.ID, "no exact match: oldest row")
}
