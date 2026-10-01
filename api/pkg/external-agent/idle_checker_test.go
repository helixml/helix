package external_agent

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func TestStopIdleDesktopLeavesSessionRunningWhenStopFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	executor := NewMockExecutor(ctrl)
	mockStore := store.NewMockStore(ctrl)
	session := &types.Session{ID: "ses_live"}

	executor.EXPECT().StopDesktop(gomock.Any(), session.ID).Return(errors.New("hydra disconnected"))

	stopIdleDesktop(context.Background(), executor, mockStore, session)

	// The strict store mock has no expectations: a failed stop must not reap a
	// live interaction or mark the still-running desktop terminated_idle.
}
