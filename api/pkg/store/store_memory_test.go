package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryFixture creates two apps and three memories: two users on appA and
// one user on appB, so tests can check that queries are scoped by both
// user_id and app_id.
type memoryFixture struct {
	appA, appB         string
	userA, userB       string
	memA, memB, memAOn *types.Memory
}

func (suite *PostgresStoreTestSuite) createMemoryFixture() *memoryFixture {
	t := suite.T()

	newApp := func() string {
		app, err := suite.db.CreateApp(suite.ctx, &types.App{
			Owner:     "test-" + system.GenerateUUID(),
			OwnerType: types.OwnerTypeUser,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = suite.db.DeleteApp(suite.ctx, app.ID) })
		return app.ID
	}
	newMemory := func(userID, appID, contents string) *types.Memory {
		m, err := suite.db.CreateMemory(suite.ctx, &types.Memory{
			UserID: userID, AppID: appID, Contents: contents,
		})
		require.NoError(t, err)
		t.Cleanup(func() { suite.db.gdb.Delete(&types.Memory{}, "id = ?", m.ID) })
		return m
	}

	f := &memoryFixture{
		appA:  newApp(),
		appB:  newApp(),
		userA: "user-a-" + system.GenerateUUID(),
		userB: "user-b-" + system.GenerateUUID(),
	}
	f.memA = newMemory(f.userA, f.appA, "user A on app A")
	f.memB = newMemory(f.userB, f.appA, "user B on app A")
	f.memAOn = newMemory(f.userA, f.appB, "user A on app B")
	return f
}

func (suite *PostgresStoreTestSuite) memoryExists(id string) bool {
	var count int64
	require.NoError(suite.T(), suite.db.gdb.Model(&types.Memory{}).Where("id = ?", id).Count(&count).Error)
	return count == 1
}

func (suite *PostgresStoreTestSuite) TestListMemories_ScopedToUserAndApp() {
	f := suite.createMemoryFixture()

	memories, err := suite.db.ListMemories(suite.ctx, &types.ListMemoryRequest{UserID: f.userA, AppID: f.appA})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), memories, 1)
	assert.Equal(suite.T(), f.memA.ID, memories[0].ID)

	memories, err = suite.db.ListMemories(suite.ctx, &types.ListMemoryRequest{UserID: f.userB, AppID: f.appB})
	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), memories)
}

func (suite *PostgresStoreTestSuite) TestDeleteMemory_OtherUserOrAppIsNotFound() {
	f := suite.createMemoryFixture()

	// Another user's memory on the same app.
	err := suite.db.DeleteMemory(suite.ctx, &types.Memory{ID: f.memB.ID, UserID: f.userA, AppID: f.appA})
	assert.ErrorIs(suite.T(), err, ErrNotFound)
	assert.True(suite.T(), suite.memoryExists(f.memB.ID))

	// The caller's own memory, but on a different app.
	err = suite.db.DeleteMemory(suite.ctx, &types.Memory{ID: f.memAOn.ID, UserID: f.userA, AppID: f.appA})
	assert.ErrorIs(suite.T(), err, ErrNotFound)
	assert.True(suite.T(), suite.memoryExists(f.memAOn.ID))

	// The caller's own memory on the right app.
	err = suite.db.DeleteMemory(suite.ctx, &types.Memory{ID: f.memA.ID, UserID: f.userA, AppID: f.appA})
	require.NoError(suite.T(), err)
	assert.False(suite.T(), suite.memoryExists(f.memA.ID))
}

func (suite *PostgresStoreTestSuite) TestUpdateMemory_OtherUserIsNotFound() {
	f := suite.createMemoryFixture()

	_, err := suite.db.UpdateMemory(suite.ctx, &types.Memory{ID: f.memB.ID, UserID: f.userA, AppID: f.appA, Contents: "changed"})
	assert.ErrorIs(suite.T(), err, ErrNotFound)

	var got types.Memory
	require.NoError(suite.T(), suite.db.gdb.First(&got, "id = ?", f.memB.ID).Error)
	assert.Equal(suite.T(), "user B on app A", got.Contents)

	_, err = suite.db.UpdateMemory(suite.ctx, &types.Memory{ID: f.memA.ID, UserID: f.userA, AppID: f.appA, Contents: "changed"})
	require.NoError(suite.T(), err)
	var own types.Memory
	require.NoError(suite.T(), suite.db.gdb.First(&own, "id = ?", f.memA.ID).Error)
	assert.Equal(suite.T(), "changed", own.Contents)
}
