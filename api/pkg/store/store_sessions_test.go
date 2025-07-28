package store

import (
	"context"
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
