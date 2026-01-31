package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func createTestApp(t *testing.T, db *PostgresStore) *types.App {
	ownerID := "test-" + system.GenerateUUID()
	app := &types.App{
		ID:        "test-app-456",
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
		Config:    types.AppConfig{},
	}

	createdApp, err := db.CreateApp(context.Background(), app)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := db.DeleteApp(context.Background(), createdApp.ID)
		require.NoError(t, err)
	})

	return createdApp
}

func (suite *PostgresStoreTestSuite) TestCreateSlackThread() {
	app := createTestApp(suite.T(), suite.db)

	// Test successful creation
	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Require().NoError(err)
	suite.NotNil(createdThread)
	suite.Equal(thread.ThreadKey, createdThread.ThreadKey)
	suite.Equal(thread.AppID, createdThread.AppID)
	suite.Equal(thread.Channel, createdThread.Channel)
	suite.Equal(thread.SessionID, createdThread.SessionID)
	suite.False(createdThread.Created.IsZero())
	suite.False(createdThread.Updated.IsZero())

	suite.T().Cleanup(func() {
		// Clean up by deleting threads older than now
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestCreateSlackThreadWithEmptyThreadKey() {
	app := createTestApp(suite.T(), suite.db)

	thread := &types.SlackThread{
		ThreadKey: "", // Empty thread key
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(createdThread)
}

func (suite *PostgresStoreTestSuite) TestCreateSlackThreadWithEmptyAppID() {
	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     "", // Empty app ID
		Channel:   "test-channel",
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(createdThread)
}

func (suite *PostgresStoreTestSuite) TestCreateSlackThreadWithEmptyChannel() {
	app := createTestApp(suite.T(), suite.db)

	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     app.ID,
		Channel:   "", // Empty channel
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(createdThread)
}

func (suite *PostgresStoreTestSuite) TestCreateSlackThreadWithPreSetTimestamps() {
	app := createTestApp(suite.T(), suite.db)

	now := time.Now()
	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
		Created:   now,
		Updated:   now,
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Require().NoError(err)
	suite.NotNil(createdThread)
	suite.Equal(now, createdThread.Created)
	suite.Equal(now, createdThread.Updated)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestGetSlackThread() {
	app := createTestApp(suite.T(), suite.db)

	// First create a thread
	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Now retrieve it
	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, createdThread.AppID, createdThread.Channel, createdThread.ThreadKey)
	suite.Require().NoError(err)
	suite.NotNil(fetchedThread)
	suite.Equal(createdThread.ThreadKey, fetchedThread.ThreadKey)
	suite.Equal(createdThread.AppID, fetchedThread.AppID)
	suite.Equal(createdThread.Channel, fetchedThread.Channel)
	suite.Equal(createdThread.SessionID, fetchedThread.SessionID)
	suite.Equal(createdThread.Created.Truncate(time.Millisecond), fetchedThread.Created.Truncate(time.Millisecond))
	suite.Equal(createdThread.Updated.Truncate(time.Millisecond), fetchedThread.Updated.Truncate(time.Millisecond))

	suite.T().Cleanup(func() {
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestGetSlackThreadWithEmptyAppID() {
	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, "", "test-channel", "test-thread-123")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestGetSlackThreadWithEmptyChannel() {
	app := createTestApp(suite.T(), suite.db)

	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, app.ID, "", "test-thread-123")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestGetSlackThreadWithEmptyThreadKey() {
	app := createTestApp(suite.T(), suite.db)

	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, app.ID, "test-channel", "")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestGetSlackThreadNotFound() {
	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, "non-existent-app", "non-existent-channel", "non-existent-thread")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestDeleteSlackThread() {
	app := createTestApp(suite.T(), suite.db)

	// Create multiple threads with different timestamps
	now := time.Now()
	oldThread := &types.SlackThread{
		ThreadKey: "old-thread-123",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
		Created:   now.Add(-2 * time.Hour),
		Updated:   now.Add(-2 * time.Hour),
	}

	newThread := &types.SlackThread{
		ThreadKey: "new-thread-456",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
		Created:   now,
		Updated:   now,
	}

	// Create both threads
	createdOldThread, err := suite.db.CreateSlackThread(suite.ctx, oldThread)
	suite.Require().NoError(err)

	createdNewThread, err := suite.db.CreateSlackThread(suite.ctx, newThread)
	suite.Require().NoError(err)

	// Delete threads older than 1 hour ago
	err = suite.db.DeleteSlackThread(suite.ctx, now.Add(-1*time.Hour))
	suite.Require().NoError(err)

	// Verify old thread is deleted
	fetchedOldThread, err := suite.db.GetSlackThread(suite.ctx, createdOldThread.AppID, createdOldThread.Channel, createdOldThread.ThreadKey)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedOldThread)

	// Verify new thread still exists
	fetchedNewThread, err := suite.db.GetSlackThread(suite.ctx, createdNewThread.AppID, createdNewThread.Channel, createdNewThread.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedNewThread)
	suite.Equal(createdNewThread.ThreadKey, fetchedNewThread.ThreadKey)

	suite.T().Cleanup(func() {
		// Clean up remaining thread
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestDeleteSlackThreadWithZeroTime() {
	app := createTestApp(suite.T(), suite.db)

	// Create a thread
	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Delete with zero time (should not delete anything)
	err = suite.db.DeleteSlackThread(suite.ctx, time.Time{})
	suite.Require().NoError(err)

	// Verify thread still exists
	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, createdThread.AppID, createdThread.Channel, createdThread.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedThread)
	suite.Equal(createdThread.ThreadKey, fetchedThread.ThreadKey)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestDeleteSlackThreadWithFutureTime() {
	app := createTestApp(suite.T(), suite.db)

	// Create a thread
	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Delete with future time (should delete all threads)
	err = suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
	suite.Require().NoError(err)

	// Verify thread is deleted
	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, createdThread.AppID, createdThread.Channel, createdThread.ThreadKey)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
	suite.Nil(fetchedThread)
}

func (suite *PostgresStoreTestSuite) TestSlackThreadUniqueConstraint() {
	app := createTestApp(suite.T(), suite.db)

	// Create a thread
	thread := &types.SlackThread{
		ThreadKey: "test-thread-123",
		AppID:     app.ID,
		Channel:   "test-channel",
		SessionID: "test-session-789",
	}

	createdThread, err := suite.db.CreateSlackThread(suite.ctx, thread)
	suite.Require().NoError(err)

	// Try to create another thread with the same composite key
	duplicateThread := &types.SlackThread{
		ThreadKey: "test-thread-123", // Same thread key
		AppID:     app.ID,            // Same app ID
		Channel:   "test-channel",    // Same channel
		SessionID: "different-session",
	}

	duplicateCreatedThread, err := suite.db.CreateSlackThread(suite.ctx, duplicateThread)
	suite.Error(err) // Should fail due to unique constraint
	suite.Nil(duplicateCreatedThread)

	// Verify original thread still exists
	fetchedThread, err := suite.db.GetSlackThread(suite.ctx, createdThread.AppID, createdThread.Channel, createdThread.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedThread)
	suite.Equal(createdThread.ThreadKey, fetchedThread.ThreadKey)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestSlackThreadMultipleThreadsSameApp() {
	app := createTestApp(suite.T(), suite.db)
	channel := "test-channel"

	// Create multiple threads for the same app and channel
	thread1 := &types.SlackThread{
		ThreadKey: "thread-1",
		AppID:     app.ID,
		Channel:   channel,
		SessionID: "session-1",
	}

	thread2 := &types.SlackThread{
		ThreadKey: "thread-2",
		AppID:     app.ID,
		Channel:   channel,
		SessionID: "session-2",
	}

	thread3 := &types.SlackThread{
		ThreadKey: "thread-3",
		AppID:     app.ID,
		Channel:   channel,
		SessionID: "session-3",
	}

	// Create all threads
	createdThread1, err := suite.db.CreateSlackThread(suite.ctx, thread1)
	suite.Require().NoError(err)

	createdThread2, err := suite.db.CreateSlackThread(suite.ctx, thread2)
	suite.Require().NoError(err)

	createdThread3, err := suite.db.CreateSlackThread(suite.ctx, thread3)
	suite.Require().NoError(err)

	// Verify all threads can be retrieved
	fetchedThread1, err := suite.db.GetSlackThread(suite.ctx, createdThread1.AppID, createdThread1.Channel, createdThread1.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedThread1)
	suite.Equal(createdThread1.ThreadKey, fetchedThread1.ThreadKey)

	fetchedThread2, err := suite.db.GetSlackThread(suite.ctx, createdThread2.AppID, createdThread2.Channel, createdThread2.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedThread2)
	suite.Equal(createdThread2.ThreadKey, fetchedThread2.ThreadKey)

	fetchedThread3, err := suite.db.GetSlackThread(suite.ctx, createdThread3.AppID, createdThread3.Channel, createdThread3.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedThread3)
	suite.Equal(createdThread3.ThreadKey, fetchedThread3.ThreadKey)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestSlackThreadDifferentChannels() {
	app := createTestApp(suite.T(), suite.db)
	threadKey := "test-thread-123"

	// Create threads with same app and thread key but different channels
	thread1 := &types.SlackThread{
		ThreadKey: threadKey,
		AppID:     app.ID,
		Channel:   "channel-1",
		SessionID: "session-1",
	}

	thread2 := &types.SlackThread{
		ThreadKey: threadKey,
		AppID:     app.ID,
		Channel:   "channel-2",
		SessionID: "session-2",
	}

	// Create both threads
	createdThread1, err := suite.db.CreateSlackThread(suite.ctx, thread1)
	suite.Require().NoError(err)

	createdThread2, err := suite.db.CreateSlackThread(suite.ctx, thread2)
	suite.Require().NoError(err)

	// Verify both threads can be retrieved
	fetchedThread1, err := suite.db.GetSlackThread(suite.ctx, createdThread1.AppID, createdThread1.Channel, createdThread1.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedThread1)
	suite.Equal(createdThread1.ThreadKey, fetchedThread1.ThreadKey)
	suite.Equal(createdThread1.Channel, fetchedThread1.Channel)

	fetchedThread2, err := suite.db.GetSlackThread(suite.ctx, createdThread2.AppID, createdThread2.Channel, createdThread2.ThreadKey)
	suite.NoError(err)
	suite.NotNil(fetchedThread2)
	suite.Equal(createdThread2.ThreadKey, fetchedThread2.ThreadKey)
	suite.Equal(createdThread2.Channel, fetchedThread2.Channel)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteSlackThread(suite.ctx, time.Now().Add(time.Hour))
		suite.NoError(err)
	})
}
