package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// TestPostgresStore_DismissAttentionEventsForTask covers the auto-clear path
// used when a spec task transitions to a terminal state (e.g. merged to main).
func (suite *PostgresStoreTestSuite) TestPostgresStore_DismissAttentionEventsForTask() {
	ctx := context.Background()

	taskA := "task_" + system.GenerateUUID()
	taskB := "task_" + system.GenerateUUID()
	user := "user_" + system.GenerateUUID()
	org := "org_" + system.GenerateUUID()
	project := "prj_" + system.GenerateUUID()

	mkEvent := func(taskID string, qualifier string) *types.AttentionEvent {
		return &types.AttentionEvent{
			ID:             system.GenerateAttentionEventID(),
			UserID:         user,
			OrganizationID: org,
			ProjectID:      project,
			SpecTaskID:     taskID,
			EventType:      types.AttentionEventAgentInteractionCompleted,
			Title:          "Agent finished working",
			CreatedAt:      time.Now(),
			IdempotencyKey: types.BuildAttentionEventIdempotencyKey(taskID, types.AttentionEventAgentInteractionCompleted, qualifier),
		}
	}

	e1 := mkEvent(taskA, "1")
	e2 := mkEvent(taskA, "2")
	other := mkEvent(taskB, "1")

	for _, e := range []*types.AttentionEvent{e1, e2, other} {
		_, err := suite.db.CreateAttentionEvent(ctx, e)
		suite.Require().NoError(err)
		suite.T().Cleanup(func() {
			_ = suite.db.gdb.WithContext(ctx).Where("id = ?", e.ID).Delete(&types.AttentionEvent{}).Error
		})
	}

	// First call dismisses both events for taskA.
	n, err := suite.db.DismissAttentionEventsForTask(ctx, taskA)
	suite.Require().NoError(err)
	suite.Equal(int64(2), n)

	// taskA events are dismissed.
	got1, err := suite.db.GetAttentionEvent(ctx, e1.ID)
	suite.Require().NoError(err)
	suite.NotNil(got1.DismissedAt)
	got2, err := suite.db.GetAttentionEvent(ctx, e2.ID)
	suite.Require().NoError(err)
	suite.NotNil(got2.DismissedAt)

	// taskB event is untouched.
	gotOther, err := suite.db.GetAttentionEvent(ctx, other.ID)
	suite.Require().NoError(err)
	suite.Nil(gotOther.DismissedAt)

	// Idempotent: a second call returns 0 with no error.
	n, err = suite.db.DismissAttentionEventsForTask(ctx, taskA)
	suite.Require().NoError(err)
	suite.Equal(int64(0), n)

	// Unknown task ID is a no-op (0 rows, no error).
	n, err = suite.db.DismissAttentionEventsForTask(ctx, "task_does_not_exist")
	suite.Require().NoError(err)
	suite.Equal(int64(0), n)

	// Empty task ID is a validation error.
	_, err = suite.db.DismissAttentionEventsForTask(ctx, "")
	suite.Error(err)
}
