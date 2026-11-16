package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// taskIsBusy is what protects a live implementation run now that the
// staleness predicate no longer exempts implementation by status alone.
func TestTaskIsBusy(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (*SpecTaskOrchestrator, *store.MockStore) {
		ctrl := gomock.NewController(t)
		st := store.NewMockStore(ctrl)
		return &SpecTaskOrchestrator{store: st}, st
	}

	t.Run("running external agent is busy", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_1"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_1").
			Return(&types.SpecTaskExternalAgent{Status: "running"}, nil)

		assert.True(t, o.taskIsBusy(ctx, task))
	})

	t.Run("stopped external agent with no session is idle", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_2"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_2").
			Return(&types.SpecTaskExternalAgent{Status: "stopped"}, nil)

		assert.False(t, o.taskIsBusy(ctx, task))
	})

	t.Run("no external agent row falls through to the session", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_3", PlanningSessionID: "ses_3"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_3").
			Return(nil, fmt.Errorf("wrapped: %w", store.ErrNotFound))
		st.EXPECT().GetSession(ctx, "ses_3").
			Return(&types.Session{Metadata: types.SessionMetadata{ExternalAgentStatus: "stopped"}}, nil)

		assert.False(t, o.taskIsBusy(ctx, task))
	})

	t.Run("running planning session is busy", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_4", PlanningSessionID: "ses_4"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_4").
			Return(nil, fmt.Errorf("wrapped: %w", store.ErrNotFound))
		st.EXPECT().GetSession(ctx, "ses_4").
			Return(&types.Session{Metadata: types.SessionMetadata{ExternalAgentStatus: "running"}}, nil)

		assert.True(t, o.taskIsBusy(ctx, task))
	})

	for _, status := range []string{"starting", "restarting"} {
		t.Run(status+" planning session is busy", func(t *testing.T) {
			o, st := setup(t)
			task := &types.SpecTask{ID: "spt_boot", PlanningSessionID: "ses_boot"}
			st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_boot").
				Return(nil, fmt.Errorf("wrapped: %w", store.ErrNotFound))
			st.EXPECT().GetSession(ctx, "ses_boot").
				Return(&types.Session{Metadata: types.SessionMetadata{ExternalAgentStatus: status}}, nil)

			assert.True(t, o.taskIsBusy(ctx, task),
				"a desktop that is booting must not have its task archived")
		})
	}

	t.Run("creating external agent is busy", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_7"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_7").
			Return(&types.SpecTaskExternalAgent{Status: "creating"}, nil)

		assert.True(t, o.taskIsBusy(ctx, task))
	})

	t.Run("terminated_idle planning session is idle", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_8", PlanningSessionID: "ses_8"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_8").
			Return(nil, fmt.Errorf("wrapped: %w", store.ErrNotFound))
		st.EXPECT().GetSession(ctx, "ses_8").
			Return(&types.Session{Metadata: types.SessionMetadata{ExternalAgentStatus: "terminated_idle"}}, nil)

		assert.False(t, o.taskIsBusy(ctx, task))
	})

	t.Run("agent lookup error is treated as busy", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_5"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_5").
			Return(nil, fmt.Errorf("database is on fire"))

		assert.True(t, o.taskIsBusy(ctx, task),
			"an unknown agent state must never lead to an archive")
	})

	t.Run("session lookup error is treated as busy", func(t *testing.T) {
		o, st := setup(t)
		task := &types.SpecTask{ID: "spt_6", PlanningSessionID: "ses_6"}
		st.EXPECT().GetSpecTaskExternalAgent(ctx, "spt_6").
			Return(nil, fmt.Errorf("wrapped: %w", store.ErrNotFound))
		st.EXPECT().GetSession(ctx, "ses_6").
			Return(nil, fmt.Errorf("database is on fire"))

		assert.True(t, o.taskIsBusy(ctx, task))
	})
}
