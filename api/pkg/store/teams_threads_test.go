package store

import (
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestCreateTeamsThread() {
	app := createTestApp(suite.T(), suite.db)

	// Test successful creation
	thread := &types.TeamsThread{
		ConversationID: "test-conversation-123",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		TeamID:         "test-team",
		SessionID:      "test-session-789",
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Require().NoError(err)
	suite.NotNil(createdThread)
	suite.Equal(thread.ConversationID, createdThread.ConversationID)
	suite.Equal(thread.AppID, createdThread.AppID)
	suite.Equal(thread.ChannelID, createdThread.ChannelID)
	suite.Equal(thread.TeamID, createdThread.TeamID)
	suite.Equal(thread.SessionID, createdThread.SessionID)
	suite.False(createdThread.Created.IsZero())
	suite.False(createdThread.Updated.IsZero())

	suite.T().Cleanup(func() {
		// Clean up by deleting threads older than now
		err := suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestCreateTeamsThreadWithEmptyConversationID() {
	app := createTestApp(suite.T(), suite.db)

	thread := &types.TeamsThread{
		ConversationID: "", // Empty conversation ID
		AppID:          app.ID,
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(createdThread)
}

func (suite *PostgresStoreTestSuite) TestCreateTeamsThreadWithEmptyAppID() {
	thread := &types.TeamsThread{
		ConversationID: "test-conversation-123",
		AppID:          "", // Empty app ID
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(createdThread)
}

func (suite *PostgresStoreTestSuite) TestCreateTeamsThreadWithPreSetTimestamps() {
	app := createTestApp(suite.T(), suite.db)

	now := time.Now()
	thread := &types.TeamsThread{
		ConversationID: "test-conversation-123",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
		Created:        now,
		Updated:        now,
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Require().NoError(err)
	suite.NotNil(createdThread)
	suite.Equal(now.Truncate(time.Millisecond), createdThread.Created.Truncate(time.Millisecond))
	suite.Equal(now.Truncate(time.Millisecond), createdThread.Updated.Truncate(time.Millisecond))

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestGetTeamsThread() {
	app := createTestApp(suite.T(), suite.db)

	// First create a thread
	thread := &types.TeamsThread{
		ConversationID: "test-conversation-123",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		TeamID:         "test-team",
		SessionID:      "test-session-789",
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Now retrieve it
	fetchedThread, err := suite.db.GetTeamsThread(suite.ctx, createdThread.AppID, createdThread.ConversationID)
	suite.Require().NoError(err)
	suite.NotNil(fetchedThread)
	suite.Equal(createdThread.ConversationID, fetchedThread.ConversationID)
	suite.Equal(createdThread.AppID, fetchedThread.AppID)
	suite.Equal(createdThread.ChannelID, fetchedThread.ChannelID)
	suite.Equal(createdThread.TeamID, fetchedThread.TeamID)
	suite.Equal(createdThread.SessionID, fetchedThread.SessionID)
	suite.Equal(createdThread.Created.Truncate(time.Millisecond), fetchedThread.Created.Truncate(time.Millisecond))
	suite.Equal(createdThread.Updated.Truncate(time.Millisecond), fetchedThread.Updated.Truncate(time.Millisecond))

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestGetTeamsThreadWithEmptyAppID() {
	fetchedThread, err := suite.db.GetTeamsThread(suite.ctx, "", "test-conversation-123")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestGetTeamsThreadWithEmptyConversationID() {
	app := createTestApp(suite.T(), suite.db)

	fetchedThread, err := suite.db.GetTeamsThread(suite.ctx, app.ID, "")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestGetTeamsThreadNotFound() {
	fetchedThread, err := suite.db.GetTeamsThread(suite.ctx, "non-existent-app", "non-existent-conversation")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestDeleteTeamsThread() {
	app := createTestApp(suite.T(), suite.db)

	// Create multiple threads with different timestamps
	now := time.Now()
	oldThread := &types.TeamsThread{
		ConversationID: "old-conversation-123",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
		Created:        now.Add(-2 * time.Hour),
		Updated:        now.Add(-2 * time.Hour),
	}

	newThread := &types.TeamsThread{
		ConversationID: "new-conversation-456",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
		Created:        now,
		Updated:        now,
	}

	// Create both threads
	createdOldThread, err := suite.db.CreateTeamsThread(suite.ctx, oldThread)
	suite.Require().NoError(err)

	createdNewThread, err := suite.db.CreateTeamsThread(suite.ctx, newThread)
	suite.Require().NoError(err)

	// Delete threads older than 1 hour ago
	err = suite.db.DeleteTeamsThread(suite.ctx, now.Add(-1*time.Hour))
	suite.Require().NoError(err)

	// Verify old thread is deleted
	fetchedOldThread, err := suite.db.GetTeamsThread(suite.ctx, createdOldThread.AppID, createdOldThread.ConversationID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedOldThread)

	// Verify new thread still exists
	fetchedNewThread, err := suite.db.GetTeamsThread(suite.ctx, createdNewThread.AppID, createdNewThread.ConversationID)
	suite.NoError(err)
	suite.NotNil(fetchedNewThread)
	suite.Equal(createdNewThread.ConversationID, fetchedNewThread.ConversationID)

	suite.T().Cleanup(func() {
		// Clean up remaining thread
		err := suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestDeleteTeamsThreadWithZeroTime() {
	app := createTestApp(suite.T(), suite.db)

	// Create a thread
	thread := &types.TeamsThread{
		ConversationID: "test-conversation-123",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Delete with zero time (should not delete anything)
	err = suite.db.DeleteTeamsThread(suite.ctx, time.Time{})
	suite.Require().NoError(err)

	// Verify thread still exists
	fetchedThread, err := suite.db.GetTeamsThread(suite.ctx, createdThread.AppID, createdThread.ConversationID)
	suite.NoError(err)
	suite.NotNil(fetchedThread)
	suite.Equal(createdThread.ConversationID, fetchedThread.ConversationID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestDeleteTeamsThreadWithFutureTime() {
	app := createTestApp(suite.T(), suite.db)

	// Create a thread
	thread := &types.TeamsThread{
		ConversationID: "test-conversation-123",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Delete with future time (should delete all threads)
	err = suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
	suite.Require().NoError(err)

	// Verify thread is deleted
	fetchedThread, err := suite.db.GetTeamsThread(suite.ctx, createdThread.AppID, createdThread.ConversationID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestTeamsThreadUniqueConstraint() {
	app := createTestApp(suite.T(), suite.db)

	// Create a thread
	thread := &types.TeamsThread{
		ConversationID: "test-conversation-123",
		AppID:          app.ID,
		ChannelID:      "test-channel",
		SessionID:      "test-session-789",
	}

	createdThread, err := suite.db.CreateTeamsThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Try to create another thread with the same composite key
	duplicateThread := &types.TeamsThread{
		ConversationID: "test-conversation-123", // Same conversation ID
		AppID:          app.ID,                  // Same app ID
		ChannelID:      "test-channel",
		SessionID:      "different-session",
	}

	duplicateCreatedThread, err := suite.db.CreateTeamsThread(suite.ctx, duplicateThread)
	suite.Error(err) // Should fail due to unique constraint
	suite.Nil(duplicateCreatedThread)

	// Verify original thread still exists
	fetchedThread, err := suite.db.GetTeamsThread(suite.ctx, createdThread.AppID, createdThread.ConversationID)
	suite.NoError(err)
	suite.NotNil(fetchedThread)
	suite.Equal(createdThread.ConversationID, fetchedThread.ConversationID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestTeamsThreadMultipleThreadsSameApp() {
	app := createTestApp(suite.T(), suite.db)

	// Create multiple threads for the same app
	thread1 := &types.TeamsThread{
		ConversationID: "conversation-1",
		AppID:          app.ID,
		ChannelID:      "channel-1",
		SessionID:      "session-1",
	}

	thread2 := &types.TeamsThread{
		ConversationID: "conversation-2",
		AppID:          app.ID,
		ChannelID:      "channel-1",
		SessionID:      "session-2",
	}

	thread3 := &types.TeamsThread{
		ConversationID: "conversation-3",
		AppID:          app.ID,
		ChannelID:      "channel-2",
		SessionID:      "session-3",
	}

	// Create all threads
	createdThread1, err := suite.db.CreateTeamsThread(suite.ctx, thread1)
	suite.Require().NoError(err)

	createdThread2, err := suite.db.CreateTeamsThread(suite.ctx, thread2)
	suite.Require().NoError(err)

	createdThread3, err := suite.db.CreateTeamsThread(suite.ctx, thread3)
	suite.Require().NoError(err)

	// Verify all threads can be retrieved
	fetchedThread1, err := suite.db.GetTeamsThread(suite.ctx, createdThread1.AppID, createdThread1.ConversationID)
	suite.NoError(err)
	suite.NotNil(fetchedThread1)
	suite.Equal(createdThread1.ConversationID, fetchedThread1.ConversationID)

	fetchedThread2, err := suite.db.GetTeamsThread(suite.ctx, createdThread2.AppID, createdThread2.ConversationID)
	suite.NoError(err)
	suite.NotNil(fetchedThread2)
	suite.Equal(createdThread2.ConversationID, fetchedThread2.ConversationID)

	fetchedThread3, err := suite.db.GetTeamsThread(suite.ctx, createdThread3.AppID, createdThread3.ConversationID)
	suite.NoError(err)
	suite.NotNil(fetchedThread3)
	suite.Equal(createdThread3.ConversationID, fetchedThread3.ConversationID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeamsThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}
