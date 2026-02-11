package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateSession() {
	// Create a sample session
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Call the CreateSession method
	createdSession, err := suite.db.CreateSession(context.Background(), session)

	// Assert that no error occurred
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	// Assert that the created session matches the original session
	suite.Equal(session, *createdSession)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetSession() {
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "name" + system.GenerateUUID(),
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Call the CreateSession method to create the session
	_, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	// Call the GetSession method to retrieve the session
	retrievedSession, err := suite.db.GetSession(context.Background(), session.ID)

	// Assert that no error occurred
	suite.NoError(err)

	// Assert that the retrieved session matches the original session
	suite.Equal(session.ID, retrievedSession.ID)
	suite.Equal(session.Name, retrievedSession.Name)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSession() {

	// Create a sample session
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Name:    "name",
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Call the CreateSession method to create the session
	_, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	// Update the session
	session.Name = "new_name"

	// Call the UpdateSession method to update the session
	updatedSession, err := suite.db.UpdateSession(context.Background(), session)

	// Assert that no error occurred
	suite.NoError(err)

	// Assert that the updated session matches the modified session
	suite.Equal("new_name", updatedSession.Name)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteSession() {
	// Create a sample session
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Call the CreateSession method to create the session
	_, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	// Call the DeleteSession method to delete the session
	deletedSession, err := suite.db.DeleteSession(context.Background(), session.ID)

	// Assert that no error occurred
	suite.NoError(err)

	// Assert that the deleted session matches the original session
	suite.Equal(session.ID, deletedSession.ID)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetSessionsPagination() {
	// Create 100 sessions with sequential names
	const totalSessions = 100
	sessionIDs := make([]string, totalSessions)

	for i := 1; i <= totalSessions; i++ {
		session := types.Session{
			ID:        system.GenerateSessionID(),
			Name:      fmt.Sprintf("session-%d", i),
			Owner:     "user_id",
			OwnerType: types.OwnerTypeUser,
			Created:   time.Now(),
			Updated:   time.Now(),
		}

		createdSession, err := suite.db.CreateSession(context.Background(), session)
		suite.NoError(err)
		sessionIDs[i-1] = createdSession.ID
	}

	// Cleanup all created sessions
	suite.T().Cleanup(func() {
		for _, id := range sessionIDs {
			_, _ = suite.db.DeleteSession(context.Background(), id)
		}
	})

	// Test pagination with different page sizes
	testCases := []struct {
		name     string
		page     int
		perPage  int
		expected int
	}{
		{"first page with 10 items", 1, 10, 10},
		{"second page with 10 items", 2, 10, 10},
		{"last page with 10 items", 10, 10, 10},
		{"page with 25 items", 1, 25, 25},
		{"page with 50 items", 1, 50, 50},
		{"page with 100 items", 1, 100, 100},
		{"page beyond available data", 15, 10, 0},
		{"page with 0 per page (should return all)", 1, 0, totalSessions},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			query := ListSessionsQuery{
				Owner:     "user_id",
				OwnerType: types.OwnerTypeUser,
				Page:      tc.page,
				PerPage:   tc.perPage,
			}

			sessions, totalCount, err := suite.db.ListSessions(context.Background(), query)
			suite.NoError(err)

			suite.Equal(totalCount, int64(totalSessions))

			if tc.page == 1 && tc.perPage == 0 {
				// When perPage is 0, it should return all sessions
				suite.Equal(totalSessions, len(sessions))
			} else if tc.page > (totalSessions/tc.perPage)+1 {
				// Page beyond available data should return empty
				suite.Equal(0, len(sessions))
			} else {
				// Regular pagination
				suite.Equal(tc.expected, len(sessions))
			}

			// Verify sessions are ordered by created DESC (newest first)
			if len(sessions) > 1 {
				for i := 0; i < len(sessions)-1; i++ {
					suite.True(sessions[i].Created.After(sessions[i+1].Created) || sessions[i].Created.Equal(sessions[i+1].Created))
				}
			}
		})
	}
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateSession_TruncatesLongName() {
	longName := strings.Repeat("a", 500)
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    longName,
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	created, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	suite.Equal(255, len([]rune(created.Name)))
	suite.Equal(strings.Repeat("a", 255), created.Name)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateSession_TruncatesMultibyteChars() {
	// Use CJK characters (3 bytes each in UTF-8) to verify rune-aware truncation
	longName := strings.Repeat("漢", 300)
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    longName,
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	created, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	suite.Equal(255, len([]rune(created.Name)))
	suite.Equal(strings.Repeat("漢", 255), created.Name)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSession_TruncatesLongName() {
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "short",
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	_, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	session.Name = strings.Repeat("b", 500)
	updated, err := suite.db.UpdateSession(context.Background(), session)
	suite.NoError(err)

	suite.Equal(255, len([]rune(updated.Name)))
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSessionName_TruncatesLongName() {
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "short",
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	_, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	longName := strings.Repeat("c", 500)
	err = suite.db.UpdateSessionName(context.Background(), session.ID, longName)
	suite.NoError(err)

	retrieved, err := suite.db.GetSession(context.Background(), session.ID)
	suite.NoError(err)
	suite.Equal(255, len([]rune(retrieved.Name)))
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSessionMeta_TruncatesLongName() {
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "short",
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	_, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	longName := strings.Repeat("d", 500)
	updated, err := suite.db.UpdateSessionMeta(context.Background(), types.SessionMetaUpdate{
		ID:   session.ID,
		Name: longName,
	})
	suite.NoError(err)
	suite.Equal(255, len([]rune(updated.Name)))
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_SessionName_PreservesExactly255() {
	// Names at exactly 255 chars should not be modified
	exactName := strings.Repeat("e", 255)
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    exactName,
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
	}

	created, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	suite.Equal(exactName, created.Name)
}
