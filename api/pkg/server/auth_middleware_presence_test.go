package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Presence means "a person has Helix open". Sandboxes and org agents
// authenticate with session-scoped keys owned by a human; their traffic must
// not keep that human online, or anyone with a running agent is green forever.
func TestTouchUserLastSeen_SkipsSessionScopedKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	touched := make(chan string, 1)
	mockStore.EXPECT().TouchUserLastSeen(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, userID string, _ time.Time) error {
			touched <- userID
			return nil
		}).AnyTimes()
	auth := &authMiddleware{store: mockStore}

	auth.touchUserLastSeen(&types.User{ID: "user_agent", TokenType: types.TokenTypeAPIKey, SessionID: "ses_bot"})
	auth.touchUserLastSeen(&types.User{ID: "user_runner", TokenType: types.TokenTypeRunner})
	auth.touchUserLastSeen(&types.User{ID: "user_human", TokenType: types.TokenTypeAPIKey})

	select {
	case userID := <-touched:
		require.Equal(t, "user_human", userID)
	case <-time.After(5 * time.Second):
		t.Fatal("expected the human's plain API key to record presence")
	}
	_, agentSeen := auth.lastSeenCache.Load("user_agent")
	_, runnerSeen := auth.lastSeenCache.Load("user_runner")
	require.False(t, agentSeen)
	require.False(t, runnerSeen)
}
