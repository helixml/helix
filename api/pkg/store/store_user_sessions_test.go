package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateUserSession() {
	session := &types.UserSession{
		UserID:       system.GenerateUserID(),
		AuthProvider: types.AuthProviderOIDC,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		UserAgent:    "Test-Agent/1.0",
		IPAddress:    "192.168.1.1",
	}

	createdSession, err := suite.db.CreateUserSession(suite.ctx, session)

	suite.NoError(err)
	suite.NotNil(createdSession)
	suite.NotEmpty(createdSession.ID)
	suite.True(createdSession.ID[:4] == "uss_") // ULID with prefix
	suite.Equal(session.UserID, createdSession.UserID)
	suite.Equal(session.AuthProvider, createdSession.AuthProvider)
	suite.False(createdSession.CreatedAt.IsZero())
	suite.False(createdSession.UpdatedAt.IsZero())
	suite.False(createdSession.LastUsedAt.IsZero())

	suite.T().Cleanup(func() {
		_ = suite.db.DeleteUserSession(suite.ctx, createdSession.ID)
	})
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateUserSession_RequiresUserID() {
	session := &types.UserSession{
		AuthProvider: types.AuthProviderOIDC,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	_, err := suite.db.CreateUserSession(suite.ctx, session)

	suite.Error(err)
	suite.Contains(err.Error(), "user ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetUserSession() {
	session := &types.UserSession{
		UserID:       system.GenerateUserID(),
		AuthProvider: types.AuthProviderRegular,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	created, err := suite.db.CreateUserSession(suite.ctx, session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_ = suite.db.DeleteUserSession(suite.ctx, created.ID)
	})

	retrieved, err := suite.db.GetUserSession(suite.ctx, created.ID)

	suite.NoError(err)
	suite.NotNil(retrieved)
	suite.Equal(created.ID, retrieved.ID)
	suite.Equal(created.UserID, retrieved.UserID)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetUserSession_RequiresID() {
	_, err := suite.db.GetUserSession(suite.ctx, "")

	suite.Error(err)
	suite.Contains(err.Error(), "session ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetUserSessionsByUser() {
	userID := system.GenerateUserID()

	// Create multiple sessions for the same user
	for i := 0; i < 3; i++ {
		session := &types.UserSession{
			UserID:       userID,
			AuthProvider: types.AuthProviderOIDC,
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		}
		created, err := suite.db.CreateUserSession(suite.ctx, session)
		suite.NoError(err)

		suite.T().Cleanup(func() {
			_ = suite.db.DeleteUserSession(suite.ctx, created.ID)
		})
	}

	sessions, err := suite.db.GetUserSessionsByUser(suite.ctx, userID)

	suite.NoError(err)
	suite.Len(sessions, 3)
	for _, s := range sessions {
		suite.Equal(userID, s.UserID)
	}
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetUserSessionsByUser_RequiresUserID() {
	_, err := suite.db.GetUserSessionsByUser(suite.ctx, "")

	suite.Error(err)
	suite.Contains(err.Error(), "user ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateUserSession() {
	session := &types.UserSession{
		UserID:       system.GenerateUserID(),
		AuthProvider: types.AuthProviderOIDC,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	created, err := suite.db.CreateUserSession(suite.ctx, session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_ = suite.db.DeleteUserSession(suite.ctx, created.ID)
	})

	// Update the session
	created.UserAgent = "Updated-Agent/2.0"
	created.IPAddress = "10.0.0.1"

	updated, err := suite.db.UpdateUserSession(suite.ctx, created)

	suite.NoError(err)
	suite.NotNil(updated)
	suite.Equal("Updated-Agent/2.0", updated.UserAgent)
	suite.Equal("10.0.0.1", updated.IPAddress)
	suite.True(updated.UpdatedAt.After(created.CreatedAt))
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateUserSession_RequiresID() {
	session := &types.UserSession{
		UserID: system.GenerateUserID(),
	}

	_, err := suite.db.UpdateUserSession(suite.ctx, session)

	suite.Error(err)
	suite.Contains(err.Error(), "session ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateUserSession_RequiresUserID() {
	session := &types.UserSession{
		ID: system.GenerateUserSessionID(),
	}

	_, err := suite.db.UpdateUserSession(suite.ctx, session)

	suite.Error(err)
	suite.Contains(err.Error(), "user ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteUserSession() {
	session := &types.UserSession{
		UserID:       system.GenerateUserID(),
		AuthProvider: types.AuthProviderOIDC,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	created, err := suite.db.CreateUserSession(suite.ctx, session)
	suite.NoError(err)

	err = suite.db.DeleteUserSession(suite.ctx, created.ID)
	suite.NoError(err)

	// Verify it was deleted
	_, err = suite.db.GetUserSession(suite.ctx, created.ID)
	suite.Error(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteUserSession_RequiresID() {
	err := suite.db.DeleteUserSession(suite.ctx, "")

	suite.Error(err)
	suite.Contains(err.Error(), "session ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteUserSessionsByUser() {
	userID := system.GenerateUserID()

	// Create multiple sessions
	var sessionIDs []string
	for i := 0; i < 3; i++ {
		session := &types.UserSession{
			UserID:       userID,
			AuthProvider: types.AuthProviderOIDC,
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		}
		created, err := suite.db.CreateUserSession(suite.ctx, session)
		suite.NoError(err)
		sessionIDs = append(sessionIDs, created.ID)
	}

	err := suite.db.DeleteUserSessionsByUser(suite.ctx, userID)
	suite.NoError(err)

	// Verify all were deleted
	sessions, err := suite.db.GetUserSessionsByUser(suite.ctx, userID)
	suite.NoError(err)
	suite.Len(sessions, 0)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteUserSessionsByUser_RequiresUserID() {
	err := suite.db.DeleteUserSessionsByUser(suite.ctx, "")

	suite.Error(err)
	suite.Contains(err.Error(), "user ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetUserSessionsNearOIDCExpiry() {
	userID := system.GenerateUserID()

	// Create a session with OIDC token expiring soon
	expiringSession := &types.UserSession{
		UserID:           userID,
		AuthProvider:     types.AuthProviderOIDC,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		OIDCRefreshToken: "refresh_token",
		OIDCAccessToken:  "access_token",
		OIDCTokenExpiry:  time.Now().Add(2 * time.Minute), // Expires in 2 minutes
	}

	created, err := suite.db.CreateUserSession(suite.ctx, expiringSession)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_ = suite.db.DeleteUserSession(suite.ctx, created.ID)
	})

	// Query for sessions expiring before 5 minutes from now
	sessions, err := suite.db.GetUserSessionsNearOIDCExpiry(context.Background(), time.Now().Add(5*time.Minute))

	suite.NoError(err)
	suite.NotEmpty(sessions)

	found := false
	for _, s := range sessions {
		if s.ID == created.ID {
			found = true
			break
		}
	}
	suite.True(found, "Expected to find the expiring session")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteExpiredUserSessions() {
	userID := system.GenerateUserID()

	// Create an expired session
	expiredSession := &types.UserSession{
		UserID:       userID,
		AuthProvider: types.AuthProviderOIDC,
		ExpiresAt:    time.Now().Add(-time.Hour), // Already expired
	}

	created, err := suite.db.CreateUserSession(suite.ctx, expiredSession)
	suite.NoError(err)

	err = suite.db.DeleteExpiredUserSessions(suite.ctx)
	suite.NoError(err)

	// Verify the expired session was deleted
	_, err = suite.db.GetUserSession(suite.ctx, created.ID)
	suite.Error(err)
}
